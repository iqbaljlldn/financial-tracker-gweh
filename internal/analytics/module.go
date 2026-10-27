package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"

	httpadapter "github.com/iqbaljlldn/financial-tracker-gweh/internal/analytics/adapter/http"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/analytics/adapter/postgres"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/analytics/application/projection"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/analytics/application/query"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/analytics/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/security"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/taskserver"
)

// Module encapsulates the Analytics bounded context (06_architecture.md §7).
// It owns the platform's rebuildable CQRS read-side projections.
type Module struct {
	pool      *pgxpool.Pool
	repo      domain.Repository
	projector *projection.Projector
	rebuilder *projection.Rebuilder

	cashFlowQuery          *query.GetCashFlowHandler
	categoryBreakdownQuery *query.GetCategoryBreakdownHandler
	merchantRankingQuery   *query.GetMerchantRankingHandler
	trendQuery             *query.GetTrendHandler

	httpHandler *httpadapter.Handler
}

// ModuleConfig configures the Analytics module.
type ModuleConfig struct {
	DBPool         *pgxpool.Pool
	SessionManager *security.SessionTokenManager
}

// NewModule initializes the Analytics module.
func NewModule(cfg ModuleConfig) *Module {
	repo := postgres.NewRepository(cfg.DBPool)
	proj := projection.NewProjector(repo)
	rebuilder := projection.NewRebuilder(repo, proj)

	cfQuery := query.NewGetCashFlowHandler(repo)
	catQuery := query.NewGetCategoryBreakdownHandler(repo)
	merchQuery := query.NewGetMerchantRankingHandler(repo)
	trendQuery := query.NewGetTrendHandler(repo)

	httpHandler := httpadapter.NewHandler(httpadapter.HandlerConfig{
		CashFlowQuery:          cfQuery,
		CategoryBreakdownQuery: catQuery,
		MerchantRankingQuery:   merchQuery,
		TrendQuery:             trendQuery,
		Rebuilder:              rebuilder,
		Pool:                   cfg.DBPool,
		SessionManager:         cfg.SessionManager,
	})

	return &Module{
		pool:                   cfg.DBPool,
		repo:                   repo,
		projector:              proj,
		rebuilder:              rebuilder,
		cashFlowQuery:          cfQuery,
		categoryBreakdownQuery: catQuery,
		merchantRankingQuery:   merchQuery,
		trendQuery:             trendQuery,
		httpHandler:            httpHandler,
	}
}

// RegisterHTTPRoutes mounts all analytics endpoints.
func (m *Module) RegisterHTTPRoutes(r chi.Router) {
	m.httpHandler.RegisterRoutes(r)
}

// ProjectAnalyticsTaskPayload represents the task payload sent via asynq for projection.
type ProjectAnalyticsTaskPayload struct {
	OutboxEventID string          `json:"outboxEventId"`
	EventType     string          `json:"eventType"`
	Payload       json.RawMessage `json:"payload"`
	CreatedAt     time.Time       `json:"createdAt"`
}

// RegisterWorkerTasks mounts asynq task handlers for asynchronous projection processing.
func (m *Module) RegisterWorkerTasks(mux *asynq.ServeMux) {
	mux.HandleFunc(taskserver.TaskProjectAnalytics, m.handleProjectAnalyticsTask)
}

func (m *Module) handleProjectAnalyticsTask(ctx context.Context, t *asynq.Task) error {
	var p ProjectAnalyticsTaskPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("unmarshal project analytics task payload: %w", err)
	}

	if err := m.projector.ProjectAnalyticsReadModels(ctx, p.OutboxEventID, p.EventType, p.Payload, p.CreatedAt); err != nil {
		slog.ErrorContext(ctx, "failed to project analytics read models",
			"outboxEventId", p.OutboxEventID,
			"eventType", p.EventType,
			"error", err,
		)
		return err
	}

	return nil
}

// Rebuild wipes and reconstructs all read models from canonical outbox events.
func (m *Module) Rebuild(ctx context.Context) error {
	return m.rebuilder.RebuildFromDB(ctx, m.pool)
}

// Projector returns the underlying projector.
func (m *Module) Projector() *projection.Projector {
	return m.projector
}

// Rebuilder returns the underlying rebuilder.
func (m *Module) Rebuilder() *projection.Rebuilder {
	return m.rebuilder
}

// Repo returns the underlying repository.
func (m *Module) Repo() domain.Repository {
	return m.repo
}
