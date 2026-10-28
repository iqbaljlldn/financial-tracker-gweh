package engagement

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	httpadapter "github.com/iqbaljlldn/financial-tracker-gweh/internal/engagement/adapter/http"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/engagement/adapter/postgres"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/engagement/adapter/postgres/gen"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/engagement/application/command"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/engagement/application/composer"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/engagement/application/query"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/engagement/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/security"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/taskserver"
)

// Config configures the Engagement module.
type Config struct {
	DB              *pgxpool.Pool
	SessionManager  *security.SessionTokenManager
	Analytics       composer.AnalyticsReader
	MessagingClient domain.MessagingClient
	ReviewConfirmer command.ReviewConfirmer
	AsynqClient     *asynq.Client
	MetaVerifyToken string
	MetaAppSecret   string
	Logger          *slog.Logger
}

// Module encapsulates the Engagement bounded context (15_notification_system.md).
type Module struct {
	Repo                   *postgres.Repository
	Composer               *composer.Service
	DeliverCmd             *command.DeliverNotificationHandler
	ProcessInboundReplyCmd *command.ProcessInboundReplyHandler
	ProcessDeliveryStatus  *command.ProcessDeliveryStatusHandler
	GetPreferencesQuery    *query.GetPreferencesHandler
	UpdatePreferencesCmd   *command.UpdatePreferencesHandler
	HTTPHandler            *httpadapter.Handler

	dbPool      *pgxpool.Pool
	asynqClient *asynq.Client
	logger      *slog.Logger

	outboxRelayCancel context.CancelFunc
	outboxRelayWg     sync.WaitGroup
}

// NewModule initializes the Engagement module.
func NewModule(cfg Config) (*Module, error) {
	if cfg.DB == nil {
		return nil, errors.New("engagement: DB pool is required")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	repo := postgres.NewRepository(cfg.DB)

	providerName := "fonnte"
	if cfg.MessagingClient != nil {
		providerName = cfg.MessagingClient.ProviderName()
	}

	composerSvc, err := composer.NewService(composer.Config{
		JobRepo:         repo,
		PrefRepo:        repo,
		AnalyticsReader: cfg.Analytics,
		AsynqClient:     cfg.AsynqClient,
		ProviderName:    providerName,
		Logger:          cfg.Logger,
	})
	if err != nil {
		return nil, fmt.Errorf("engagement: initializing composer service: %w", err)
	}

	deliverCmd := command.NewDeliverNotificationHandler(repo, repo, cfg.MessagingClient, cfg.Logger)
	replyCmd := command.NewProcessInboundReplyHandler(repo, repo, cfg.MessagingClient, cfg.ReviewConfirmer, cfg.Logger)
	statusCmd := command.NewProcessDeliveryStatusHandler(repo, cfg.Logger)

	getPrefQuery := query.NewGetPreferencesHandler(repo)
	updatePrefCmd := command.NewUpdatePreferencesHandler(repo)

	httpHandler := httpadapter.NewHandler(httpadapter.HandlerConfig{
		GetPreferencesQuery:    getPrefQuery,
		UpdatePreferencesCmd:   updatePrefCmd,
		ProcessInboundReplyCmd: replyCmd,
		ProcessDeliveryStatus:  statusCmd,
		SessionManager:         cfg.SessionManager,
		MetaVerifyToken:        cfg.MetaVerifyToken,
		MetaAppSecret:          cfg.MetaAppSecret,
		Logger:                 cfg.Logger,
	})

	return &Module{
		Repo:                   repo,
		Composer:               composerSvc,
		DeliverCmd:             deliverCmd,
		ProcessInboundReplyCmd: replyCmd,
		ProcessDeliveryStatus:  statusCmd,
		GetPreferencesQuery:    getPrefQuery,
		UpdatePreferencesCmd:   updatePrefCmd,
		HTTPHandler:            httpHandler,
		dbPool:                 cfg.DB,
		asynqClient:            cfg.AsynqClient,
		logger:                 cfg.Logger,
	}, nil
}

// RegisterHTTPRoutes mounts engagement routes onto the Chi router.
func (m *Module) RegisterHTTPRoutes(r chi.Router) {
	m.HTTPHandler.RegisterRoutes(r)
}

// RegisterWorkerTasks registers background task handlers for composition and delivery.
func (m *Module) RegisterWorkerTasks(mux *asynq.ServeMux) {
	mux.HandleFunc(taskserver.TaskComposeNotification, m.handleComposeTask)
	mux.HandleFunc(taskserver.TaskDeliverNotification, m.handleDeliverTask)
	mux.HandleFunc(taskserver.TaskRunScheduledBroadcast, m.handleScheduledBroadcastTask)
}

func (m *Module) handleComposeTask(ctx context.Context, task *asynq.Task) error {
	var payload struct {
		Type              string    `json:"type"` // "daily_summary", "weekly_summary", "monthly_report", "budget_alert", "review_request", "transaction_finalized"
		UserID            string    `json:"userId"`
		BudgetID          string    `json:"budgetId,omitempty"`
		Category          string    `json:"category,omitempty"`
		Threshold         float64   `json:"threshold,omitempty"`
		ThresholdAmount   float64   `json:"thresholdAmount,omitempty"`
		CurrentSpend      float64   `json:"currentSpend,omitempty"`
		Currency          string    `json:"currency,omitempty"`
		FinancialEventID  string    `json:"financialEventId,omitempty"`
		AmountValue       float64   `json:"amountValue,omitempty"`
		AmountCurrency    string    `json:"amountCurrency,omitempty"`
		MerchantName      string    `json:"merchantName,omitempty"`
		SuggestedCategory string    `json:"suggestedCategory,omitempty"`
		EventType         string    `json:"eventType,omitempty"`
		OccurredAt        time.Time `json:"occurredAt,omitempty"`
		FinalizedVia      string    `json:"finalizedVia,omitempty"`
	}

	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		m.logger.ErrorContext(ctx, "failed to unmarshal compose task payload", "error", err)
		return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
	}

	userUUID, err := uuid.Parse(payload.UserID)
	if err != nil {
		return fmt.Errorf("%w: invalid user id %s: %v", asynq.SkipRetry, payload.UserID, err)
	}

	switch payload.Type {
	case string(domain.NotificationTypeDailySummary):
		_, err = m.Composer.ComposeDailySummary(ctx, userUUID)
	case string(domain.NotificationTypeWeeklySummary):
		_, err = m.Composer.ComposeWeeklySummary(ctx, userUUID)
	case string(domain.NotificationTypeMonthlyReport):
		_, err = m.Composer.ComposeMonthlyReport(ctx, userUUID)
	case string(domain.NotificationTypeBudgetAlert):
		bID, _ := uuid.Parse(payload.BudgetID)
		_, err = m.Composer.ComposeBudgetAlertNotification(ctx, composer.BudgetAlertPayload{
			UserID:          userUUID,
			BudgetID:        bID,
			Category:        payload.Category,
			Threshold:       payload.Threshold,
			ThresholdAmount: payload.ThresholdAmount,
			CurrentSpend:    payload.CurrentSpend,
			Currency:        payload.Currency,
		})
	case string(domain.NotificationTypeReviewRequest):
		feID, _ := uuid.Parse(payload.FinancialEventID)
		_, err = m.Composer.ComposeReviewRequestNotification(ctx, composer.ReviewRequestPayload{
			UserID:            userUUID,
			FinancialEventID:  feID,
			AmountValue:       payload.AmountValue,
			AmountCurrency:    payload.AmountCurrency,
			MerchantName:      payload.MerchantName,
			SuggestedCategory: payload.SuggestedCategory,
		})
	case string(domain.NotificationTypeTransactionFinalized):
		feID, _ := uuid.Parse(payload.FinancialEventID)
		cat := payload.Category
		if cat == "" {
			cat = payload.SuggestedCategory
		}
		_, err = m.Composer.ComposeTransactionFinalizedNotification(ctx, composer.TransactionFinalizedPayload{
			UserID:           userUUID,
			FinancialEventID: feID,
			EventType:        payload.EventType,
			AmountValue:      payload.AmountValue,
			AmountCurrency:   payload.AmountCurrency,
			MerchantName:     payload.MerchantName,
			Category:         cat,
			OccurredAt:       payload.OccurredAt,
			FinalizedVia:     payload.FinalizedVia,
		})
	default:
		m.logger.WarnContext(ctx, "unrecognized notification compose type", "type", payload.Type)
		return nil
	}

	return err
}

func (m *Module) handleDeliverTask(ctx context.Context, task *asynq.Task) error {
	var payload struct {
		JobID string `json:"jobId"`
	}
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("%w: invalid deliver task payload: %v", asynq.SkipRetry, err)
	}

	return m.DeliverCmd.Handle(ctx, command.DeliverNotificationCommand{
		JobID: payload.JobID,
	})
}

func (m *Module) handleScheduledBroadcastTask(ctx context.Context, task *asynq.Task) error {
	var payload struct {
		NotificationType string `json:"notificationType"` // "daily_summary", "weekly_summary", "monthly_report"
	}
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("%w: invalid scheduled task payload: %v", asynq.SkipRetry, err)
	}

	targets, err := m.Repo.ListUsersForSchedule(ctx, domain.NotificationType(payload.NotificationType))
	if err != nil {
		return fmt.Errorf("listing users for scheduled %s: %w", payload.NotificationType, err)
	}

	m.logger.InfoContext(ctx, "triggering scheduled summary broadcast",
		"type", payload.NotificationType,
		"eligibleUsers", len(targets),
	)

	for _, target := range targets {
		composePayload, _ := json.Marshal(map[string]any{
			"type":   payload.NotificationType,
			"userId": target.UserID.String(),
		})
		cTask := asynq.NewTask(taskserver.TaskComposeNotification, composePayload)
		if m.asynqClient != nil {
			_, _ = m.asynqClient.EnqueueContext(ctx, cTask, asynq.Queue(taskserver.QueueEngagementCompose))
		}
	}

	return nil
}

// StartOutboxRelay starts background poller for unrelayed engagement outbox events.
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
	rows, err := queries.ListUnrelayedEngagementOutbox(ctx, 50)
	if err != nil || len(rows) == 0 {
		return
	}

	ids := make([]pgtype.UUID, 0, len(rows))
	for _, row := range rows {
		m.logger.InfoContext(ctx, "relayed engagement outbox event",
			"event_type", row.EventType,
			"aggregate_id", row.AggregateID,
		)
		ids = append(ids, row.ID)
	}

	_ = queries.MarkEngagementOutboxRelayed(ctx, ids)
}
