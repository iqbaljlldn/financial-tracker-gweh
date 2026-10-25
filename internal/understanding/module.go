package understanding

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/security"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/adapter/external"
	understandinghttp "github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/adapter/http"
	understandingpg "github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/adapter/postgres"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/application/categorization"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/application/command"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/application/executor"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/application/governance"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/application/prompt"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/fixtures"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// AIConfig configures external LLM provider connection settings (11_ai_blueprint.md §8).
type AIConfig struct {
	BaseURL      string
	APIKey       string
	DefaultModel string
	Timeout      time.Duration
}

// Config provides configuration dependencies for the understanding module.
type Config struct {
	DB                *pgxpool.Pool
	RedisClient       *redis.Client
	AsynqClient       *asynq.Client
	SessionManager    *security.SessionTokenManager
	LLMExtractor      domain.LLMExtractor // Optional override for testing
	AIConfig          AIConfig
	BudgetConfig      governance.BudgetConfig
	ConfidenceWeights domain.ConfidenceWeights
	Logger            *slog.Logger
}

// Module encapsulates the understanding module components.
type Module struct {
	Repo                *understandingpg.Repository
	Matcher             *executor.TemplateMatcher
	Executor            *executor.ExtractionExecutor
	PromptRegistry      *prompt.Registry
	Categorizer         *categorization.Engine
	Governor            *governance.DailyBudgetGovernor
	LLMExtractor        domain.LLMExtractor
	ApplyTemplateCmd    *command.ApplySenderTemplateHandler
	ClassifyAndRouteCmd *command.ClassifyAndRouteMessageHandler
	InvokeAICmd         *command.InvokeAIExtractorHandler
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

	promptReg := prompt.NewRegistry()
	catEngine := categorization.NewEngine(categorization.Config{
		UserHistory:   categorization.NewInMemoryUserHistoryStore(),
		GlobalMapping: categorization.NewInMemoryGlobalMappingStore(),
	})
	governor := governance.NewDailyBudgetGovernor(cfg.BudgetConfig)

	llmExtractor := cfg.LLMExtractor
	if llmExtractor == nil {
		llmExtractor = external.NewOpenAICompatibleLLMClient(external.LLMClientConfig{
			BaseURL:      cfg.AIConfig.BaseURL,
			APIKey:       cfg.AIConfig.APIKey,
			DefaultModel: cfg.AIConfig.DefaultModel,
			Timeout:      cfg.AIConfig.Timeout,
			Logger:       cfg.Logger,
		})
	}

	weights := cfg.ConfidenceWeights
	if weights.Amount == 0 && weights.Merchant == 0 {
		weights = domain.DefaultConfidenceWeights()
	}

	invokeAICmd := command.NewInvokeAIExtractorHandler(
		llmExtractor,
		promptReg,
		catEngine,
		governor,
		repo,
		repo,
		repo,
		weights,
		cfg.Logger,
	)

	applyCmd := command.NewApplySenderTemplateHandler(matcher, repo, repo, cfg.Logger)
	applyCmd.SetAIExtractor(invokeAICmd, cfg.AsynqClient)

	classifyCmd := command.NewClassifyAndRouteMessageHandler(matcher, applyCmd, cfg.AsynqClient, cfg.Logger)
	classifyCmd.SetAIExtractor(invokeAICmd)

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
		PromptRegistry:      promptReg,
		Categorizer:         catEngine,
		Governor:            governor,
		LLMExtractor:        llmExtractor,
		ApplyTemplateCmd:    applyCmd,
		ClassifyAndRouteCmd: classifyCmd,
		InvokeAICmd:         invokeAICmd,
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

	mux.HandleFunc(command.TaskInvokeAIExtractor, func(ctx context.Context, t *asynq.Task) error {
		var cmd command.InvokeAIExtractorCommand
		if err := json.Unmarshal(t.Payload(), &cmd); err != nil {
			return fmt.Errorf("unmarshaling InvokeAIExtractorCommand: %w", err)
		}
		_, err := m.InvokeAICmd.Handle(ctx, cmd)
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
