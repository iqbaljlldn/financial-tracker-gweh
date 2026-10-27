package financialcore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	fcHttp "github.com/iqbaljlldn/financial-tracker-gweh/internal/financialcore/adapter/http"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/financialcore/adapter/postgres"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/financialcore/adapter/postgres/gen"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/financialcore/application/command"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/financialcore/application/query"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/financialcore/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/security"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/taskserver"
)

// Config configures the financialcore module.
type Config struct {
	DB                  *pgxpool.Pool
	AsynqClient         *asynq.Client
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
	dbPool             *pgxpool.Pool
	asynqClient        *asynq.Client

	outboxRelayCancel context.CancelFunc
	outboxRelayWg     sync.WaitGroup
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
		dbPool:             cfg.DB,
		asynqClient:        cfg.AsynqClient,
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

// StartOutboxRelay starts background poller for unrelayed financialcore outbox events.
func (m *Module) StartOutboxRelay(ctx context.Context) error {
	relayCtx, cancel := context.WithCancel(ctx)
	m.outboxRelayCancel = cancel
	m.outboxRelayWg.Add(1)

	go func() {
		defer m.outboxRelayWg.Done()
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-relayCtx.Done():
				return
			case <-ticker.C:
				m.pollOutbox(relayCtx)
			}
		}
	}()

	return nil
}

// StopOutboxRelay stops the outbox relay background poller.
func (m *Module) StopOutboxRelay() {
	if m.outboxRelayCancel != nil {
		m.outboxRelayCancel()
		m.outboxRelayWg.Wait()
	}
}

func (m *Module) pollOutbox(ctx context.Context) {
	queries := gen.New(m.dbPool)
	rows, err := queries.ListUnrelayedFinancialCoreOutbox(ctx, 50)
	if err != nil || len(rows) == 0 {
		return
	}

	ids := make([]pgtype.UUID, 0, len(rows))
	for _, row := range rows {
		// Relay to Budget evaluation if Finalized Expense
		if row.EventType == domain.EventFinancialEventFinalized {
			var payload domain.FinancialEventFinalizedPayload
			if err := json.Unmarshal(row.Payload, &payload); err == nil {
				if strings.EqualFold(payload.EventType, "Expense") && m.asynqClient != nil {
					evalPayload, _ := json.Marshal(map[string]any{
						"userId":     payload.UserID,
						"eventId":    payload.EventID,
						"eventType":  payload.EventType,
						"amount":     payload.Amount,
						"category":   payload.Category,
						"occurredAt": payload.OccurredAt,
					})
					task := asynq.NewTask(taskserver.TaskEvaluateBudgetImpact, evalPayload)
					_, _ = m.asynqClient.Enqueue(task, asynq.Queue(taskserver.QueueBudgetEvaluate))
				}
			}
		}

		// Relay to Analytics projection read models
		if row.EventType == domain.EventFinancialEventFinalized ||
			row.EventType == domain.EventFinancialEventOverridden ||
			row.EventType == domain.EventFinancialEventArchived ||
			row.EventType == domain.EventCategoryAssignmentChanged {
			if m.asynqClient != nil {
				analyticsPayload, _ := json.Marshal(map[string]any{
					"outboxEventId": uuid.UUID(row.ID.Bytes).String(),
					"eventType":     row.EventType,
					"payload":       json.RawMessage(row.Payload),
					"createdAt":     row.CreatedAt.Time,
				})
				task := asynq.NewTask(taskserver.TaskProjectAnalytics, analyticsPayload)
				_, _ = m.asynqClient.Enqueue(task, asynq.Queue(taskserver.QueueAnalyticsProject))
			}
		}

		ids = append(ids, row.ID)
	}

	_ = queries.MarkFinancialCoreOutboxRelayed(ctx, ids)
}
