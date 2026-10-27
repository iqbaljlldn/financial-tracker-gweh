package budget

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	budgetHttp "github.com/iqbaljlldn/financial-tracker-gweh/internal/budget/adapter/http"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/budget/adapter/postgres"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/budget/adapter/postgres/gen"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/budget/application/command"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/budget/application/query"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/security"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/taskserver"
)

// Config encapsulates dependencies to initialize the budget module.
type Config struct {
	DB             *pgxpool.Pool
	SessionManager *security.SessionTokenManager
	AsynqClient    *asynq.Client
	Logger         *slog.Logger
}

// Module encapsulates the budget bounded context per 04_domain_model.md §9.
type Module struct {
	Repo                    *postgres.Repository
	CreateBudgetCmd         *command.CreateBudgetHandler
	UpdateBudgetCmd         *command.UpdateBudgetHandler
	DeleteBudgetCmd         *command.DeleteBudgetHandler
	CreateSavingsGoalCmd    *command.CreateSavingsGoalHandler
	UpdateSavingsGoalCmd    *command.UpdateSavingsGoalHandler
	DeleteSavingsGoalCmd    *command.DeleteSavingsGoalHandler
	EvaluateBudgetImpactCmd *command.EvaluateBudgetImpactHandler
	ListBudgetsQuery        *query.ListBudgetsHandler
	ListSavingsGoalsQuery   *query.ListSavingsGoalsHandler
	HTTPHandler             *budgetHttp.Handler
	logger                  *slog.Logger
	dbPool                  *pgxpool.Pool
	asynqClient             *asynq.Client

	outboxRelayCancel context.CancelFunc
	outboxRelayWg     sync.WaitGroup
}

// NewModule constructs the budget module.
func NewModule(cfg Config) (*Module, error) {
	if cfg.DB == nil {
		return nil, errors.New("budget: DB pool is required")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	repo := postgres.NewRepository(cfg.DB)

	createBudgetCmd := command.NewCreateBudgetHandler(repo)
	updateBudgetCmd := command.NewUpdateBudgetHandler(repo)
	deleteBudgetCmd := command.NewDeleteBudgetHandler(repo)

	createSavingsGoalCmd := command.NewCreateSavingsGoalHandler(repo.Goals())
	updateSavingsGoalCmd := command.NewUpdateSavingsGoalHandler(repo.Goals())
	deleteSavingsGoalCmd := command.NewDeleteSavingsGoalHandler(repo.Goals())

	evalBudgetCmd := command.NewEvaluateBudgetImpactHandler(repo, repo, repo, repo, cfg.Logger)

	listBudgetsQuery := query.NewListBudgetsHandler(repo, repo)
	listSavingsGoalsQuery := query.NewListSavingsGoalsHandler(repo.Goals(), repo)

	httpHandler := budgetHttp.NewHandler(budgetHttp.HandlerConfig{
		CreateBudgetCmd:      createBudgetCmd,
		UpdateBudgetCmd:      updateBudgetCmd,
		DeleteBudgetCmd:      deleteBudgetCmd,
		CreateSavingsGoalCmd: createSavingsGoalCmd,
		UpdateSavingsGoalCmd: updateSavingsGoalCmd,
		DeleteSavingsGoalCmd: deleteSavingsGoalCmd,
		ListBudgetsQuery:     listBudgetsQuery,
		ListSavingsGoals:     listSavingsGoalsQuery,
		SessionManager:       cfg.SessionManager,
	})

	return &Module{
		Repo:                    repo,
		CreateBudgetCmd:         createBudgetCmd,
		UpdateBudgetCmd:         updateBudgetCmd,
		DeleteBudgetCmd:         deleteBudgetCmd,
		CreateSavingsGoalCmd:    createSavingsGoalCmd,
		UpdateSavingsGoalCmd:    updateSavingsGoalCmd,
		DeleteSavingsGoalCmd:    deleteSavingsGoalCmd,
		EvaluateBudgetImpactCmd: evalBudgetCmd,
		ListBudgetsQuery:        listBudgetsQuery,
		ListSavingsGoalsQuery:   listSavingsGoalsQuery,
		HTTPHandler:             httpHandler,
		logger:                  cfg.Logger,
		dbPool:                  cfg.DB,
		asynqClient:             cfg.AsynqClient,
	}, nil
}

// RegisterHTTPRoutes mounts budget endpoints on the provided Chi router.
func (m *Module) RegisterHTTPRoutes(r chi.Router) {
	m.HTTPHandler.RegisterRoutes(r)
}

// RegisterWorkerTasks binds budget task handlers to the Asynq worker router.
func (m *Module) RegisterWorkerTasks(mux *asynq.ServeMux) {
	mux.HandleFunc(taskserver.TaskEvaluateBudgetImpact, m.handleEvaluateBudgetImpactTask)
}

func (m *Module) handleEvaluateBudgetImpactTask(ctx context.Context, task *asynq.Task) error {
	var cmd command.EvaluateBudgetImpactCommand
	if err := json.Unmarshal(task.Payload(), &cmd); err != nil {
		m.logger.ErrorContext(ctx, "failed to unmarshal evaluate budget impact task payload",
			"error", err,
		)
		return fmt.Errorf("%w: malformed payload: %v", asynq.SkipRetry, err)
	}

	_, err := m.EvaluateBudgetImpactCmd.Handle(ctx, cmd)
	if err != nil {
		m.logger.ErrorContext(ctx, "failed to execute EvaluateBudgetImpact",
			"error", err,
			"userId", cmd.UserID,
			"eventId", cmd.EventID,
		)
		return err
	}
	return nil
}

// StartOutboxRelay starts background poller for unrelayed budget outbox events.
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
	rows, err := queries.ListUnrelayedOutboxEvents(ctx, 50)
	if err != nil {
		return
	}
	if len(rows) == 0 {
		return
	}

	ids := make([]pgtype.UUID, 0, len(rows))
	for _, row := range rows {
		if row.EventType == "BudgetThresholdCrossed" && m.asynqClient != nil {
			var p struct {
				BudgetID        string  `json:"budgetId"`
				UserID          string  `json:"userId"`
				Category        string  `json:"category"`
				Threshold       float64 `json:"threshold"`
				ThresholdAmount float64 `json:"thresholdAmount"`
				CurrentSpend    float64 `json:"currentSpend"`
			}
			if err := json.Unmarshal(row.Payload, &p); err == nil {
				composePayload, _ := json.Marshal(map[string]any{
					"type":            "budget_alert",
					"userId":          p.UserID,
					"budgetId":        p.BudgetID,
					"category":        p.Category,
					"threshold":       p.Threshold,
					"thresholdAmount": p.ThresholdAmount,
					"currentSpend":    p.CurrentSpend,
				})
				task := asynq.NewTask(taskserver.TaskComposeNotification, composePayload)
				_, _ = m.asynqClient.Enqueue(task, asynq.Queue(taskserver.QueueEngagementCompose))
			}
		}

		m.logger.InfoContext(ctx, "relayed budget outbox event",
			"event_type", row.EventType,
			"aggregate_id", row.AggregateID,
		)
		ids = append(ids, row.ID)
	}

	_ = queries.MarkOutboxEventsRelayed(ctx, ids)
}
