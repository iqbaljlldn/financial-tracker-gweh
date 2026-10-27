package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/analytics/application/query"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/observability"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/security"
)

type contextKey string

const (
	userIDContextKey contextKey = "analytics.userID"
)

// ErrorDetails represents the standard error response payload per 09_api_specification.md §7.2.
type ErrorDetails struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Field     string `json:"field,omitempty"`
	RequestID string `json:"requestId,omitempty"`
}

// ErrorEnvelope wraps ErrorDetails in the standard error response envelope.
type ErrorEnvelope struct {
	Error ErrorDetails `json:"error"`
}

// HandlerConfig encapsulates dependencies for the analytics HTTP handler.
type HandlerConfig struct {
	CashFlowQuery          *query.GetCashFlowHandler
	CategoryBreakdownQuery *query.GetCategoryBreakdownHandler
	MerchantRankingQuery   *query.GetMerchantRankingHandler
	TrendQuery             *query.GetTrendHandler
	SessionManager         *security.SessionTokenManager
}

// Handler serves HTTP endpoints for the analytics module (09_api_specification.md §6.3).
type Handler struct {
	cashFlowQuery          *query.GetCashFlowHandler
	categoryBreakdownQuery *query.GetCategoryBreakdownHandler
	merchantRankingQuery   *query.GetMerchantRankingHandler
	trendQuery             *query.GetTrendHandler
	sessionManager         *security.SessionTokenManager
}

// NewHandler constructs an analytics HTTP handler.
func NewHandler(cfg HandlerConfig) *Handler {
	return &Handler{
		cashFlowQuery:          cfg.CashFlowQuery,
		categoryBreakdownQuery: cfg.CategoryBreakdownQuery,
		merchantRankingQuery:   cfg.MerchantRankingQuery,
		trendQuery:             cfg.TrendQuery,
		sessionManager:         cfg.SessionManager,
	}
}

// RegisterRoutes mounts analytics endpoints on the router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	mountRoutes := func(prefix string) {
		r.Route(prefix+"/analytics", func(cr chi.Router) {
			cr.Use(h.SessionAuthMiddleware)
			cr.Get("/cash-flow", h.GetCashFlow)
			cr.Get("/category-breakdown", h.GetCategoryBreakdown)
			cr.Get("/merchant-ranking", h.GetMerchantRanking)
			cr.Get("/trend", h.GetTrend)
		})
	}

	mountRoutes("/api/v1")
	mountRoutes("/v1")
	mountRoutes("")
}

// SessionAuthMiddleware validates Authorization: Bearer <token> and places userID into context.
func (h *Handler) SessionAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Missing or invalid authorization token", "", requestID(r))
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		claims, err := h.sessionManager.Validate(tokenString)
		if err != nil {
			if errors.Is(err, security.ErrJWTExpired) {
				writeError(w, http.StatusUnauthorized, "SESSION_EXPIRED", "Platform session token has expired", "", requestID(r))
				return
			}
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid session token", "", requestID(r))
			return
		}

		ctx := context.WithValue(r.Context(), userIDContextKey, claims.UserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetCashFlow serves GET /analytics/cash-flow.
func (h *Handler) GetCashFlow(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		if m := observability.GetMetrics(); m != nil {
			m.AnalyticsQueryDurationSeconds.WithLabelValues("cash_flow").Observe(time.Since(start).Seconds())
		}
	}()

	userID := getUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "User context not found", "", requestID(r))
		return
	}

	q := r.URL.Query()
	granularity := q.Get("granularity")
	from := parseDateParam(q.Get("from"))
	to := parseDateParam(q.Get("to"))

	var currency *string
	if c := strings.TrimSpace(q.Get("currency")); c != "" {
		currency = &c
	}

	res, err := h.cashFlowQuery.Handle(r.Context(), query.GetCashFlowParams{
		UserID:      userID,
		Granularity: granularity,
		From:        from,
		To:          to,
		Currency:    currency,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), "", requestID(r))
		return
	}

	writeJSON(w, http.StatusOK, res)
}

// GetCategoryBreakdown serves GET /analytics/category-breakdown.
func (h *Handler) GetCategoryBreakdown(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		if m := observability.GetMetrics(); m != nil {
			m.AnalyticsQueryDurationSeconds.WithLabelValues("category_breakdown").Observe(time.Since(start).Seconds())
		}
	}()

	userID := getUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "User context not found", "", requestID(r))
		return
	}

	q := r.URL.Query()
	from := parseDateParam(q.Get("from"))
	to := parseDateParam(q.Get("to"))

	var currency *string
	if c := strings.TrimSpace(q.Get("currency")); c != "" {
		currency = &c
	}

	res, err := h.categoryBreakdownQuery.Handle(r.Context(), query.GetCategoryBreakdownParams{
		UserID:   userID,
		From:     from,
		To:       to,
		Currency: currency,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), "", requestID(r))
		return
	}

	writeJSON(w, http.StatusOK, res)
}

// GetMerchantRanking serves GET /analytics/merchant-ranking.
func (h *Handler) GetMerchantRanking(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		if m := observability.GetMetrics(); m != nil {
			m.AnalyticsQueryDurationSeconds.WithLabelValues("merchant_ranking").Observe(time.Since(start).Seconds())
		}
	}()

	userID := getUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "User context not found", "", requestID(r))
		return
	}

	q := r.URL.Query()
	from := parseDateParam(q.Get("from"))
	to := parseDateParam(q.Get("to"))

	var currency *string
	if c := strings.TrimSpace(q.Get("currency")); c != "" {
		currency = &c
	}

	var limit int32 = 10
	if lStr := q.Get("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			limit = int32(l)
		}
	}

	res, err := h.merchantRankingQuery.Handle(r.Context(), query.GetMerchantRankingParams{
		UserID:   userID,
		From:     from,
		To:       to,
		Currency: currency,
		Limit:    limit,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), "", requestID(r))
		return
	}

	writeJSON(w, http.StatusOK, res)
}

// GetTrend serves GET /analytics/trend.
func (h *Handler) GetTrend(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		if m := observability.GetMetrics(); m != nil {
			m.AnalyticsQueryDurationSeconds.WithLabelValues("trend").Observe(time.Since(start).Seconds())
		}
	}()

	userID := getUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "User context not found", "", requestID(r))
		return
	}

	q := r.URL.Query()
	granularity := q.Get("granularity")
	from := parseDateParam(q.Get("from"))
	to := parseDateParam(q.Get("to"))

	var currency *string
	if c := strings.TrimSpace(q.Get("currency")); c != "" {
		currency = &c
	}

	res, err := h.trendQuery.Handle(r.Context(), query.GetTrendParams{
		UserID:      userID,
		Granularity: granularity,
		From:        from,
		To:          to,
		Currency:    currency,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), "", requestID(r))
		return
	}

	writeJSON(w, http.StatusOK, res)
}

// Helpers

func getUserID(ctx context.Context) string {
	if val, ok := ctx.Value(userIDContextKey).(string); ok {
		return val
	}
	return ""
}

func parseDateParam(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return &t
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return &t
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, code, msg, field, reqID string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorEnvelope{
		Error: ErrorDetails{
			Code:      code,
			Message:   msg,
			Field:     field,
			RequestID: reqID,
		},
	})
}

func requestID(r *http.Request) string {
	if id := r.Header.Get("X-Request-ID"); id != "" {
		return id
	}
	return ""
}
