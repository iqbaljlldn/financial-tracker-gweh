package understanding

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/security"
	understandinghttp "github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/adapter/http"
	understandingpg "github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/adapter/postgres"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/application/command"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/application/executor"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/fixtures"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Config provides configuration dependencies for the understanding module.
type Config struct {
	DB             *pgxpool.Pool
	RedisClient    *redis.Client
	AsynqClient    *asynq.Client
	SessionManager *security.SessionTokenManager
	Logger         *slog.Logger
}

// Module encapsulates the understanding module components.
type Module struct {
	Repo                *understandingpg.Repository
	Matcher             *executor.TemplateMatcher
	Executor            *executor.ExtractionExecutor
	ApplyTemplateCmd    *command.ApplySenderTemplateHandler
	ClassifyAndRouteCmd *command.ClassifyAndRouteMessageHandler
	PublishTemplateCmd  *command.PublishTemplateHandler
	RollupMetricsCmd    *command.RollupParserMetricsHandler
	httpHandler         *understandinghttp.Handler
	logger              *slog.Logger
}

// NewModule initializes and wires the understanding module.
func NewModule(cfg Config) (*Module, error) {
	if cfg.DB == nil {
		return nil, fmt.Errorf("database connection pool is required")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	repo := understandingpg.NewRepository(cfg.DB)
	matcher := executor.NewTemplateMatcher(repo, cfg.RedisClient, cfg.Logger)
	exec := executor.NewExtractionExecutor()

	applyCmd := command.NewApplySenderTemplateHandler(matcher, repo, repo, cfg.Logger)
	classifyCmd := command.NewClassifyAndRouteMessageHandler(matcher, applyCmd, cfg.AsynqClient, cfg.Logger)
	publishCmd := command.NewPublishTemplateHandler(repo, matcher, cfg.Logger)
	metricsCmd := command.NewRollupParserMetricsHandler(repo, repo, cfg.Logger)

	httpHandler := understandinghttp.NewHandler(
		repo,
		repo,
		publishCmd,
		metricsCmd,
		cfg.SessionManager,
	)

	m := &Module{
		Repo:                repo,
		Matcher:             matcher,
		Executor:            exec,
		ApplyTemplateCmd:    applyCmd,
		ClassifyAndRouteCmd: classifyCmd,
		PublishTemplateCmd:  publishCmd,
		RollupMetricsCmd:    metricsCmd,
		httpHandler:         httpHandler,
		logger:              cfg.Logger,
	}

	return m, nil
}

// RegisterHTTPRoutes mounts understanding admin endpoints.
func (m *Module) RegisterHTTPRoutes(r chi.Router) {
	m.httpHandler.RegisterRoutes(r)
}

// RegisterWorkerTasks registers understanding worker handlers on Asynq ServeMux.
func (m *Module) RegisterWorkerTasks(mux *asynq.ServeMux) {
	mux.HandleFunc(command.TaskApplySenderTemplate, func(ctx context.Context, t *asynq.Task) error {
		var cmd command.ApplySenderTemplateCommand
		if err := json.Unmarshal(t.Payload(), &cmd); err != nil {
			return fmt.Errorf("unmarshaling ApplySenderTemplateCommand: %w", err)
		}
		_, err := m.ApplyTemplateCmd.Handle(ctx, cmd)
		return err
	})

	mux.HandleFunc(command.TaskClassifyAndRouteMessage, func(ctx context.Context, t *asynq.Task) error {
		var cmd command.ClassifyAndRouteMessageCommand
		if err := json.Unmarshal(t.Payload(), &cmd); err != nil {
			return fmt.Errorf("unmarshaling ClassifyAndRouteMessageCommand: %w", err)
		}
		_, err := m.ClassifyAndRouteCmd.Handle(ctx, cmd)
		return err
	})

	mux.HandleFunc(command.TaskRollupParserMetrics, func(ctx context.Context, t *asynq.Task) error {
		_, err := m.RollupMetricsCmd.Handle(ctx, command.RollupParserMetricsCommand{Trigger: "scheduled"})
		return err
	})
}

// SeedCuratedTemplates publishes initial MVP templates if they do not already exist in database [GATE].
func (m *Module) SeedCuratedTemplates(ctx context.Context) error {
	sampleSets := fixtures.GetSampleSets()
	for _, tmpl := range fixtures.GetCuratedTemplates() {
		// Check if active template exists
		existing, err := m.Repo.FindActiveByPattern(ctx, tmpl.SenderPattern())
		if err == nil && existing != nil {
			continue // Already seeded
		}

		sampleSet := sampleSets[tmpl.SenderPattern()]
		_, err = m.PublishTemplateCmd.Handle(ctx, command.PublishTemplateCommand{
			SenderPattern:   tmpl.SenderPattern(),
			Version:         tmpl.Version(),
			ExtractionRules: tmpl.Rules(),
			SampleSet:       sampleSet,
		})
		if err != nil {
			m.logger.Warn("failed to seed curated template",
				slog.String("senderPattern", tmpl.SenderPattern()),
				slog.String("error", err.Error()),
			)
			return err
		}
	}
	return nil
}
