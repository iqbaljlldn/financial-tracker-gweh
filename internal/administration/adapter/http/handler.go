package http

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/administration/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/security"
)

// HandlerConfig configures the Administration HTTP handler.
type HandlerConfig struct {
	AuditRepo     domain.AuditLogRepository
	AuditLogger   domain.AuditLogger
	ConnectorRepo domain.ConnectorHealthRepository
	ParserRepo    domain.ParserMetricsRepository
	PromptRepo    domain.PromptMetricsRepository
	SessionMgr    *security.SessionTokenManager
}

// Handler serves administrative REST endpoints under /admin/* per 09_api_specification.md §6.6.
type Handler struct {
	auditRepo     domain.AuditLogRepository
	auditLogger   domain.AuditLogger
	connectorRepo domain.ConnectorHealthRepository
	parserRepo    domain.ParserMetricsRepository
	promptRepo    domain.PromptMetricsRepository
	sessionMgr    *security.SessionTokenManager
}

// NewHandler constructs a Handler instance.
func NewHandler(cfg HandlerConfig) *Handler {
	return &Handler{
		auditRepo:     cfg.AuditRepo,
		auditLogger:   cfg.AuditLogger,
		connectorRepo: cfg.ConnectorRepo,
		parserRepo:    cfg.ParserRepo,
		promptRepo:    cfg.PromptRepo,
		sessionMgr:    cfg.SessionMgr,
	}
}

// RegisterRoutes mounts all administrative routes onto the Chi router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	mountAdmin := func(prefix string) {
		r.Route(prefix+"/admin", func(admin chi.Router) {
			admin.Use(RequireOpsRoleMiddleware(h.sessionMgr, h.auditLogger))

			admin.Get("/connectors/health", h.GetConnectorHealth)
			admin.Get("/parsers/metrics", h.GetParserMetrics)
			admin.Get("/prompts/metrics", h.GetPromptMetrics)
			admin.Get("/audit-log", h.GetAuditLog)
		})
	}

	mountAdmin("/api/v1")
	mountAdmin("/v1")
	mountAdmin("")
}

// GetConnectorHealth serves GET /admin/connectors/health (FR-ADMIN-01).
func (h *Handler) GetConnectorHealth(w http.ResponseWriter, r *http.Request) {
	metrics, err := h.connectorRepo.ListLatest(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve connector health", err.Error())
		return
	}

	items := make([]ConnectorHealthDTO, len(metrics))
	for i, m := range metrics {
		items[i] = ConnectorHealthDTO{
			ID:                 m.ID,
			ConnectorType:      m.ConnectorType,
			ConnectorAccountID: m.ConnectorAccountID,
			Status:             m.Status,
			ErrorCount:         m.ErrorCount,
			LatencyMs:          m.LatencyMs,
			RecordedAt:         m.RecordedAt,
		}
	}

	writeJSON(w, http.StatusOK, items)
}

// GetParserMetrics serves GET /admin/parsers/metrics (FR-ADMIN-02).
func (h *Handler) GetParserMetrics(w http.ResponseWriter, r *http.Request) {
	metrics, err := h.parserRepo.GetAggregated(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve parser metrics", err.Error())
		return
	}

	items := make([]ParserMetricDTO, len(metrics))
	for i, m := range metrics {
		items[i] = ParserMetricDTO{
			SenderPattern:   m.SenderPattern,
			TemplateVersion: m.TemplateVersion,
			TotalParsed:     m.TotalParsed,
			FailedParsed:    m.FailedParsed,
			FailureRate:     m.FailureRate,
			SpikeAlert:      m.SpikeAlert,
			LastRecordedAt:  m.RecordedAt,
		}
	}

	writeJSON(w, http.StatusOK, items)
}

// GetPromptMetrics serves GET /admin/prompts/metrics (FR-ADMIN-03).
func (h *Handler) GetPromptMetrics(w http.ResponseWriter, r *http.Request) {
	summaries, err := h.promptRepo.GetSummary(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve prompt metrics", err.Error())
		return
	}

	items := make([]PromptMetricSummaryDTO, len(summaries))
	for i, s := range summaries {
		items[i] = PromptMetricSummaryDTO{
			PromptVersion:   s.PromptVersion,
			ModelIdentifier: s.ModelIdentifier,
			TotalCalls:      s.TotalCalls,
			AvgConfidence:   s.AvgConfidence,
			ValidCount:      s.ValidCount,
			InvalidCount:    s.InvalidCount,
			OverriddenCount: s.OverriddenCount,
			AvgLatencyMs:    s.AvgLatencyMs,
			TotalTokens:     s.TotalTokens,
		}
	}

	writeJSON(w, http.StatusOK, items)
}

// GetAuditLog serves GET /admin/audit-log (FR-ADMIN-04).
func (h *Handler) GetAuditLog(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	limit := 50
	if l := q.Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 200 {
			limit = parsed
		}
	}

	offset := 0
	if o := q.Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	filter := domain.AuditLogFilter{
		Limit:  limit,
		Offset: offset,
	}

	if uid := q.Get("userId"); uid != "" {
		filter.UserID = &uid
	}
	if act := q.Get("action"); act != "" {
		filter.Action = &act
	}
	if fromStr := q.Get("from"); fromStr != "" {
		if t, err := time.Parse(time.RFC3339, fromStr); err == nil {
			filter.FromTime = &t
		}
	}
	if toStr := q.Get("to"); toStr != "" {
		if t, err := time.Parse(time.RFC3339, toStr); err == nil {
			filter.ToTime = &t
		}
	}

	entries, total, err := h.auditRepo.List(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve audit log", err.Error())
		return
	}

	items := make([]AuditLogItemDTO, len(entries))
	for i, e := range entries {
		items[i] = AuditLogItemDTO{
			ID:         e.ID(),
			UserID:     e.UserID(),
			Action:     e.Action(),
			Actor:      e.Actor(),
			Metadata:   e.Metadata(),
			OccurredAt: e.OccurredAt().Format(time.RFC3339),
		}
	}

	res := AuditLogListResponse{
		Items: items,
		Pagination: PaginationResponse{
			Total:  total,
			Limit:  limit,
			Offset: offset,
		},
	}

	writeJSON(w, http.StatusOK, res)
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, code, message, field string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{
		Error: ErrorDetail{
			Code:    code,
			Message: message,
			Field:   field,
		},
	})
}
