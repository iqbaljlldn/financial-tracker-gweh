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

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/budget/application/command"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/budget/application/query"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/budget/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/security"
)

type contextKey string

const (
	userIDContextKey contextKey = "budget.userID"
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

// HandlerConfig encapsulates handler dependencies.
type HandlerConfig struct {
	CreateBudgetCmd      *command.CreateBudgetHandler
	UpdateBudgetCmd      *command.UpdateBudgetHandler
	DeleteBudgetCmd      *command.DeleteBudgetHandler
	CreateSavingsGoalCmd *command.CreateSavingsGoalHandler
	UpdateSavingsGoalCmd *command.UpdateSavingsGoalHandler
	DeleteSavingsGoalCmd *command.DeleteSavingsGoalHandler
	ListBudgetsQuery     *query.ListBudgetsHandler
	ListSavingsGoals     *query.ListSavingsGoalsHandler
	SessionManager       *security.SessionTokenManager
}

// Handler serves HTTP endpoints for the budget module (09_api_specification.md §6.4).
type Handler struct {
	createBudgetCmd      *command.CreateBudgetHandler
	updateBudgetCmd      *command.UpdateBudgetHandler
	deleteBudgetCmd      *command.DeleteBudgetHandler
	createSavingsGoalCmd *command.CreateSavingsGoalHandler
	updateSavingsGoalCmd *command.UpdateSavingsGoalHandler
	deleteSavingsGoalCmd *command.DeleteSavingsGoalHandler
	listBudgetsQuery     *query.ListBudgetsHandler
	listSavingsGoals     *query.ListSavingsGoalsHandler
	sessionManager       *security.SessionTokenManager
}

// NewHandler constructs a budget HTTP handler.
func NewHandler(cfg HandlerConfig) *Handler {
	return &Handler{
		createBudgetCmd:      cfg.CreateBudgetCmd,
		updateBudgetCmd:      cfg.UpdateBudgetCmd,
		deleteBudgetCmd:      cfg.DeleteBudgetCmd,
		createSavingsGoalCmd: cfg.CreateSavingsGoalCmd,
		updateSavingsGoalCmd: cfg.UpdateSavingsGoalCmd,
		deleteSavingsGoalCmd: cfg.DeleteSavingsGoalCmd,
		listBudgetsQuery:     cfg.ListBudgetsQuery,
		listSavingsGoals:     cfg.ListSavingsGoals,
		sessionManager:       cfg.SessionManager,
	}
}

// RegisterRoutes mounts budget and savings goals endpoints on the router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	mountRoutes := func(prefix string) {
		r.Route(prefix+"/budgets", func(cr chi.Router) {
			cr.Use(h.SessionAuthMiddleware)
			cr.Get("/", h.ListBudgets)
			cr.Post("/", h.CreateBudget)
			cr.Patch("/{id}", h.UpdateBudget)
			cr.Delete("/{id}", h.DeleteBudget)
		})

		r.Route(prefix+"/savings-goals", func(cr chi.Router) {
			cr.Use(h.SessionAuthMiddleware)
			cr.Get("/", h.ListSavingsGoals)
			cr.Post("/", h.CreateSavingsGoal)
			cr.Patch("/{id}", h.UpdateSavingsGoal)
			cr.Delete("/{id}", h.DeleteSavingsGoal)
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
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid session token signature or claims", "", requestID(r))
			return
		}

		ctx := context.WithValue(r.Context(), userIDContextKey, claims.UserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// --- Budgets ---

type CreateBudgetRequest struct {
	Scope           string       `json:"scope"`
	PeriodType      string       `json:"periodType"`
	ThresholdAmount domain.Money `json:"thresholdAmount"`
	AlertThresholds []float64    `json:"alertThresholds"`
}

type UpdateBudgetRequest struct {
	Scope           string        `json:"scope"`
	PeriodType      string        `json:"periodType"`
	ThresholdAmount *domain.Money `json:"thresholdAmount,omitempty"`
	AlertThresholds []float64     `json:"alertThresholds"`
	Active          *bool         `json:"active,omitempty"`
}

func (h *Handler) ListBudgets(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(userIDContextKey).(string)
	if !ok || userID == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Missing user identity", "", requestID(r))
		return
	}

	includeInactive := r.URL.Query().Get("includeInactive") == "true"
	items, err := h.listBudgetsQuery.Handle(r.Context(), query.ListBudgetsQuery{
		UserID:          userID,
		IncludeInactive: includeInactive,
	})
	if err != nil {
		h.handleDomainError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) CreateBudget(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(userIDContextKey).(string)
	if !ok || userID == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Missing user identity", "", requestID(r))
		return
	}

	var req CreateBudgetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "MALFORMED_JSON", "Invalid request body JSON", "", requestID(r))
		return
	}

	periodType := req.PeriodType
	if periodType == "" {
		periodType = "monthly"
	}

	b, err := h.createBudgetCmd.Handle(r.Context(), command.CreateBudgetCommand{
		UserID:          userID,
		Scope:           req.Scope,
		PeriodType:      periodType,
		ThresholdAmount: req.ThresholdAmount,
		AlertThresholds: req.AlertThresholds,
	})
	if err != nil {
		h.handleDomainError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, b)
}

func (h *Handler) UpdateBudget(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(userIDContextKey).(string)
	if !ok || userID == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Missing user identity", "", requestID(r))
		return
	}
	id := chi.URLParam(r, "id")

	var req UpdateBudgetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "MALFORMED_JSON", "Invalid request body JSON", "", requestID(r))
		return
	}

	var amount domain.Money
	if req.ThresholdAmount != nil {
		amount = *req.ThresholdAmount
	}
	active := true
	if req.Active != nil {
		active = *req.Active
	}

	b, err := h.updateBudgetCmd.Handle(r.Context(), command.UpdateBudgetCommand{
		ID:              id,
		UserID:          userID,
		Scope:           req.Scope,
		PeriodType:      req.PeriodType,
		ThresholdAmount: amount,
		AlertThresholds: req.AlertThresholds,
		Active:          active,
	})
	if err != nil {
		h.handleDomainError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, b)
}

func (h *Handler) DeleteBudget(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(userIDContextKey).(string)
	if !ok || userID == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Missing user identity", "", requestID(r))
		return
	}
	id := chi.URLParam(r, "id")

	if err := h.deleteBudgetCmd.Handle(r.Context(), command.DeleteBudgetCommand{
		ID:     id,
		UserID: userID,
	}); err != nil {
		h.handleDomainError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- Savings Goals ---

type CreateSavingsGoalRequest struct {
	TargetAmount domain.Money `json:"targetAmount"`
	TargetDate   string       `json:"targetDate"` // "YYYY-MM-DD"
}

type UpdateSavingsGoalRequest struct {
	TargetAmount *domain.Money `json:"targetAmount,omitempty"`
	TargetDate   string        `json:"targetDate,omitempty"`
}

func (h *Handler) ListSavingsGoals(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(userIDContextKey).(string)
	if !ok || userID == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Missing user identity", "", requestID(r))
		return
	}

	items, err := h.listSavingsGoals.Handle(r.Context(), query.ListSavingsGoalsQuery{
		UserID: userID,
	})
	if err != nil {
		h.handleDomainError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) CreateSavingsGoal(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(userIDContextKey).(string)
	if !ok || userID == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Missing user identity", "", requestID(r))
		return
	}

	var req CreateSavingsGoalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "MALFORMED_JSON", "Invalid request body JSON", "", requestID(r))
		return
	}

	targetDate, err := time.Parse("2006-01-02", strings.TrimSpace(req.TargetDate))
	if err != nil {
		targetDate, err = time.Parse(time.RFC3339, strings.TrimSpace(req.TargetDate))
		if err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_TARGET_DATE", "targetDate must be YYYY-MM-DD", "targetDate", requestID(r))
			return
		}
	}

	g, err := h.createSavingsGoalCmd.Handle(r.Context(), command.CreateSavingsGoalCommand{
		UserID:       userID,
		TargetAmount: req.TargetAmount,
		TargetDate:   targetDate,
	})
	if err != nil {
		h.handleDomainError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, g)
}

func (h *Handler) UpdateSavingsGoal(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(userIDContextKey).(string)
	if !ok || userID == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Missing user identity", "", requestID(r))
		return
	}
	id := chi.URLParam(r, "id")

	var req UpdateSavingsGoalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "MALFORMED_JSON", "Invalid request body JSON", "", requestID(r))
		return
	}

	var targetDate time.Time
	if req.TargetDate != "" {
		var err error
		targetDate, err = time.Parse("2006-01-02", strings.TrimSpace(req.TargetDate))
		if err != nil {
			targetDate, err = time.Parse(time.RFC3339, strings.TrimSpace(req.TargetDate))
			if err != nil {
				writeError(w, http.StatusBadRequest, "INVALID_TARGET_DATE", "targetDate must be YYYY-MM-DD", "targetDate", requestID(r))
				return
			}
		}
	}

	var amount domain.Money
	if req.TargetAmount != nil {
		amount = *req.TargetAmount
	}

	g, err := h.updateSavingsGoalCmd.Handle(r.Context(), command.UpdateSavingsGoalCommand{
		ID:           id,
		UserID:       userID,
		TargetAmount: amount,
		TargetDate:   targetDate,
	})
	if err != nil {
		h.handleDomainError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, g)
}

func (h *Handler) DeleteSavingsGoal(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(userIDContextKey).(string)
	if !ok || userID == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Missing user identity", "", requestID(r))
		return
	}
	id := chi.URLParam(r, "id")

	if err := h.deleteSavingsGoalCmd.Handle(r.Context(), command.DeleteSavingsGoalCommand{
		ID:     id,
		UserID: userID,
	}); err != nil {
		h.handleDomainError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- Error handling ---

func (h *Handler) handleDomainError(w http.ResponseWriter, r *http.Request, err error) {
	reqID := requestID(r)
	switch {
	case errors.Is(err, domain.ErrBudgetNotFound):
		writeError(w, http.StatusNotFound, "BUDGET_NOT_FOUND", err.Error(), "id", reqID)
	case errors.Is(err, domain.ErrGoalNotFound):
		writeError(w, http.StatusNotFound, "GOAL_NOT_FOUND", err.Error(), "id", reqID)
	case errors.Is(err, domain.ErrInvalidScope):
		writeError(w, http.StatusBadRequest, "INVALID_SCOPE", err.Error(), "scope", reqID)
	case errors.Is(err, domain.ErrInvalidPeriodType):
		writeError(w, http.StatusBadRequest, "INVALID_PERIOD_TYPE", err.Error(), "periodType", reqID)
	case errors.Is(err, domain.ErrInvalidThresholdAmount), errors.Is(err, domain.ErrInvalidTargetAmount):
		writeError(w, http.StatusBadRequest, "INVALID_AMOUNT", err.Error(), "amount.value", reqID)
	case errors.Is(err, domain.ErrInvalidCurrency), errors.Is(err, domain.ErrInvalidGoalCurrency):
		writeError(w, http.StatusBadRequest, "INVALID_CURRENCY", err.Error(), "amount.currency", reqID)
	case errors.Is(err, domain.ErrInvalidAlertThresholds):
		writeError(w, http.StatusBadRequest, "INVALID_THRESHOLDS", err.Error(), "alertThresholds", reqID)
	default:
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred", "", reqID)
	}
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, code, message, field, reqID string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorEnvelope{
		Error: ErrorDetails{
			Code:      code,
			Message:   message,
			Field:     field,
			RequestID: reqID,
		},
	})
}

func requestID(r *http.Request) string {
	if rid := r.Header.Get("X-Request-ID"); rid != "" {
		return rid
	}
	return "req_" + strconv.FormatInt(time.Now().UnixNano(), 36)
}
