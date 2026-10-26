package financialcore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"

	fcHttp "github.com/iqbaljlldn/financial-tracker-gweh/internal/financialcore/adapter/http"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/financialcore/adapter/postgres"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/financialcore/application/command"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/financialcore/application/query"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/financialcore/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/security"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/taskserver"
)

// Config configures the financialcore module.
type Config struct {
	DB                  *pgxpool.Pool
	SessionManager      *security.SessionTokenManager
	ConfidenceThreshold float64
	PendingReviewSLA    time.Duration
	CategorySuggester   domain.CategorySuggester
	CategoryFeedback    domain.CategoryFeedback
	Logger              *slog.Logger
}

// Module encapsulates the financialcore bounded context (the ledger aggregate root).
type Module struct {
	EvaluateGateCmd    *command.EvaluateConfidenceGateHandler
	ConfirmReviewCmd   *command.ConfirmPendingReviewHandler
	OverrideCmd        *command.SubmitEventOverrideHandler
	ArchiveCmd         *command.ArchiveFinancialEventsHandler
	ManualEventCmd     *command.CreateManualEventHandler
	CheckSLACmd        *command.CheckPendingReviewSLAHandler
	ListEventsQuery    *query.ListFinancialEventsHandler
	GetEventQuery      *query.GetFinancialEventByIDHandler
	ListOverridesQuery *query.ListEventOverridesHandler
	HTTPHandler        *fcHttp.Handler
	logger             *slog.Logger
}

// NewModule constructs the financialcore module with its postgres adapters and application services.
func NewModule(cfg Config) (*Module, error) {
	if cfg.DB == nil {
		return nil, errors.New("financialcore: DB pool is required")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.ConfidenceThreshold <= 0 {
		cfg.ConfidenceThreshold = domain.DefaultConfidenceThreshold
	}

	repo := postgres.NewRepository(cfg.DB)
	uow := postgres.NewUnitOfWork(cfg.DB)

	evalCmd := command.NewEvaluateConfidenceGateHandler(uow, cfg.ConfidenceThreshold, cfg.CategorySuggester, cfg.Logger)
	confirmCmd := command.NewConfirmPendingReviewHandler(uow, cfg.ConfidenceThreshold, cfg.CategoryFeedback, cfg.Logger)
	overrideCmd := command.NewSubmitEventOverrideHandler(uow, cfg.CategoryFeedback, cfg.Logger)
	archiveCmd := command.NewArchiveFinancialEventsHandler(uow, cfg.Logger)
	manualCmd := command.NewCreateManualEventHandler(uow, cfg.ConfidenceThreshold, cfg.Logger)
	checkSLACmd := command.NewCheckPendingReviewSLAHandler(repo, cfg.PendingReviewSLA, cfg.Logger)

	listQuery := query.NewListFinancialEventsHandler(repo)
	getQuery := query.NewGetFinancialEventByIDHandler(repo)
	listOverridesQuery := query.NewListEventOverridesHandler(repo)

	httpHandler := fcHttp.NewHandler(fcHttp.HandlerConfig{
		ListEventsQuery:    listQuery,
		GetEventQuery:      getQuery,
		ListOverridesQuery: listOverridesQuery,
		ManualEventCmd:     manualCmd,
		OverrideCmd:        overrideCmd,
		ConfirmCmd:         confirmCmd,
		SessionManager:     cfg.SessionManager,
	})

	return &Module{
		EvaluateGateCmd:    evalCmd,
		ConfirmReviewCmd:   confirmCmd,
		OverrideCmd:        overrideCmd,
		ArchiveCmd:         archiveCmd,
		ManualEventCmd:     manualCmd,
		CheckSLACmd:        checkSLACmd,
		ListEventsQuery:    listQuery,
		GetEventQuery:      getQuery,
		ListOverridesQuery: listOverridesQuery,
		HTTPHandler:        httpHandler,
		logger:             cfg.Logger,
	}, nil
}

// RegisterHTTPRoutes mounts financialcore HTTP endpoints.
func (m *Module) RegisterHTTPRoutes(r chi.Router) {
	m.HTTPHandler.RegisterRoutes(r)
}

// RegisterWorkerTasks binds financialcore asynchronous task consumers onto the Asynq worker router.
func (m *Module) RegisterWorkerTasks(mux *asynq.ServeMux) {
	mux.HandleFunc(taskserver.TaskEvaluateConfidenceGate, m.handleEvaluateConfidenceGateTask)
	mux.HandleFunc(taskserver.TaskArchiveFinancialEvents, m.handleArchiveEventsTask)
}

func (m *Module) handleEvaluateConfidenceGateTask(ctx context.Context, task *asynq.Task) error {
	_, err := m.EvaluateGateCmd.Handle(ctx, command.EvaluateConfidenceGateCommand{
		Payload: task.Payload(),
	})
	if err != nil {
		if errors.Is(err, domain.ErrInvalidCandidate) ||
			errors.Is(err, domain.ErrInvalidPayload) ||
			errors.Is(err, domain.ErrInvalidEventType) ||
			errors.Is(err, domain.ErrConfidenceNotEvaluated) {
			m.logger.ErrorContext(ctx, "non-retryable candidate validation failure at confidence gate",
				"error", err,
			)
			return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
		}
		return err
	}
	return nil
}

func (m *Module) handleArchiveEventsTask(ctx context.Context, task *asynq.Task) error {
	var cmd command.ArchiveFinancialEventsCommand
	if err := json.Unmarshal(task.Payload(), &cmd); err != nil {
		return fmt.Errorf("%w: malformed archive task payload: %v", asynq.SkipRetry, err)
	}
	_, err := m.ArchiveCmd.Handle(ctx, cmd)
	return err
}
