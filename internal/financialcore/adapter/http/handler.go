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

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/financialcore/application/command"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/financialcore/application/query"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/financialcore/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/security"
)

type contextKey string

const (
	userIDContextKey contextKey = "financialcore.userID"
)

// HandlerConfig encapsulates handler dependencies.
type HandlerConfig struct {
	ListEventsQuery    *query.ListFinancialEventsHandler
	GetEventQuery      *query.GetFinancialEventByIDHandler
	ListOverridesQuery *query.ListEventOverridesHandler
	ManualEventCmd     *command.CreateManualEventHandler
	OverrideCmd        *command.SubmitEventOverrideHandler
	ConfirmCmd         *command.ConfirmPendingReviewHandler
	SessionManager     *security.SessionTokenManager
}

// Handler serves HTTP endpoints for the financialcore ledger module (09_api_specification.md §6.2).
type Handler struct {
	listEventsQuery    *query.ListFinancialEventsHandler
	getEventQuery      *query.GetFinancialEventByIDHandler
	listOverridesQuery *query.ListEventOverridesHandler
	manualEventCmd     *command.CreateManualEventHandler
	overrideCmd        *command.SubmitEventOverrideHandler
	confirmCmd         *command.ConfirmPendingReviewHandler
	sessionManager     *security.SessionTokenManager
}

// NewHandler constructs a financialcore HTTP handler.
func NewHandler(cfg HandlerConfig) *Handler {
	return &Handler{
		listEventsQuery:    cfg.ListEventsQuery,
		getEventQuery:      cfg.GetEventQuery,
		listOverridesQuery: cfg.ListOverridesQuery,
		manualEventCmd:     cfg.ManualEventCmd,
		overrideCmd:        cfg.OverrideCmd,
		confirmCmd:         cfg.ConfirmCmd,
		sessionManager:     cfg.SessionManager,
	}
}

// RegisterRoutes mounts financialcore routes on the provided Chi router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	mountLedgerRoutes := func(prefix string) {
		r.Route(prefix+"/financial-events", func(cr chi.Router) {
			cr.Use(h.SessionAuthMiddleware)
			cr.Get("/", h.ListFinancialEvents)
			cr.Post("/", h.CreateManualEvent)
			cr.Get("/{id}", h.GetFinancialEvent)
			cr.Patch("/{id}", h.PatchFinancialEvent)
			cr.Post("/{id}/confirm", h.ConfirmFinancialEvent)
			cr.Get("/{id}/overrides", h.ListEventOverrides)
		})
	}

	mountLedgerRoutes("/api/v1")
	mountLedgerRoutes("/v1")
	mountLedgerRoutes("")
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

// ListFinancialEvents handles GET /financial-events.
func (h *Handler) ListFinancialEvents(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(userIDContextKey).(string)
	if !ok || userID == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Missing user identity", "", requestID(r))
		return
	}

	q := r.URL.Query()
	filter := domain.ListFilter{
		UserID:    userID,
		Status:    strings.TrimSpace(q.Get("status")),
		EventType: strings.TrimSpace(q.Get("eventType")),
		Merchant:  strings.TrimSpace(q.Get("merchant")),
		Category:  strings.TrimSpace(q.Get("category")),
	}

	if fromStr := strings.TrimSpace(q.Get("from")); fromStr != "" {
		if t, err := time.Parse(time.RFC3339, fromStr); err == nil {
			filter.From = &t
		} else {
			writeError(w, http.StatusBadRequest, "INVALID_QUERY_PARAM", "Invalid 'from' timestamp (RFC3339 required)", "from", requestID(r))
			return
		}
	}

	if toStr := strings.TrimSpace(q.Get("to")); toStr != "" {
		if t, err := time.Parse(time.RFC3339, toStr); err == nil {
			filter.To = &t
		} else {
			writeError(w, http.StatusBadRequest, "INVALID_QUERY_PARAM", "Invalid 'to' timestamp (RFC3339 required)", "to", requestID(r))
			return
		}
	}

	if cursorStr := strings.TrimSpace(q.Get("cursor")); cursorStr != "" {
		cur, err := domain.DecodeCursor(cursorStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_CURSOR", "Malformed pagination cursor", "cursor", requestID(r))
			return
		}
		filter.Cursor = cur
	}

	if limitStr := strings.TrimSpace(q.Get("limit")); limitStr != "" {
		if lim, err := strconv.Atoi(limitStr); err == nil && lim > 0 {
			filter.Limit = lim
		}
	}

	res, err := h.listEventsQuery.Handle(r.Context(), query.ListFinancialEventsQuery{Filter: filter})
	if err != nil {
		h.handleDomainError(w, r, err)
		return
	}

	items := make([]FinancialEventResponse, len(res.Items))
	for i, it := range res.Items {
		items[i] = ToEventResponse(it, false)
	}

	writeJSON(w, http.StatusOK, ListFinancialEventsResponse{
		Items: items,
		Pagination: PaginationDTO{
			NextCursor: res.NextCursor,
			HasMore:    res.HasMore,
		},
	})
}

// GetFinancialEvent handles GET /financial-events/{id}.
func (h *Handler) GetFinancialEvent(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(userIDContextKey).(string)
	if !ok || userID == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Missing user identity", "", requestID(r))
		return
	}
	eventID := chi.URLParam(r, "id")

	fe, err := h.getEventQuery.Handle(r.Context(), query.GetFinancialEventByIDQuery{
		UserID:  userID,
		EventID: eventID,
	})
	if err != nil {
		h.handleDomainError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, ToEventResponse(fe, true))
}

// CreateManualEvent handles POST /financial-events (FR-LEDGER-03).
func (h *Handler) CreateManualEvent(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(userIDContextKey).(string)
	if !ok || userID == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Missing user identity", "", requestID(r))
		return
	}

	var req CreateManualEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "MALFORMED_JSON", "Invalid request body", "", requestID(r))
		return
	}

	occurredAt := time.Now()
	if req.OccurredAt != "" {
		t, err := time.Parse(time.RFC3339, req.OccurredAt)
		if err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_DATE", "occurredAt must be RFC3339", "occurredAt", requestID(r))
			return
		}
		occurredAt = t
	}

	merchantName := ""
	if req.Merchant != nil {
		merchantName = req.Merchant.NormalizedName
		if merchantName == "" {
			merchantName = req.Merchant.RawName
		}
	}

	pmType := ""
	pmHint := ""
	if req.PaymentMethod != nil {
		pmType = req.PaymentMethod.Type
		pmHint = req.PaymentMethod.Hint
	}

	fe, err := h.manualEventCmd.Handle(r.Context(), command.CreateManualEventCommand{
		UserID:            userID,
		EventType:         req.EventType,
		AmountValue:       req.Amount.Value,
		AmountCurrency:    req.Amount.Currency,
		OccurredAt:        occurredAt,
		MerchantName:      merchantName,
		PaymentMethodType: pmType,
		PaymentMethodHint: pmHint,
		Category:          req.Category,
		Payload:           req.TypeSpecificPayload,
	})
	if err != nil {
		h.handleDomainError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, ToEventResponse(fe, true))
}

// PatchFinancialEvent handles PATCH /financial-events/{id}.
func (h *Handler) PatchFinancialEvent(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(userIDContextKey).(string)
	if !ok || userID == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Missing user identity", "", requestID(r))
		return
	}
	eventID := chi.URLParam(r, "id")

	var req PatchFinancialEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "MALFORMED_JSON", "Invalid request body", "", requestID(r))
		return
	}

	var amt *command.AmountField
	if req.Fields.Amount != nil {
		amt = &command.AmountField{
			Value:    req.Fields.Amount.Value,
			Currency: req.Fields.Amount.Currency,
		}
	}

	fe, _, err := h.overrideCmd.Handle(r.Context(), command.SubmitEventOverrideCommand{
		UserID:      userID,
		EventID:     eventID,
		ActorUserID: userID,
		Fields: command.OverrideFields{
			Amount:    amt,
			Merchant:  req.Fields.Merchant,
			EventType: req.Fields.EventType,
			Category:  req.Fields.Category,
		},
	})
	if err != nil {
		h.handleDomainError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, ToEventResponse(fe, true))
}

// ConfirmFinancialEvent handles POST /financial-events/{id}/confirm.
func (h *Handler) ConfirmFinancialEvent(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(userIDContextKey).(string)
	if !ok || userID == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Missing user identity", "", requestID(r))
		return
	}
	eventID := chi.URLParam(r, "id")
	idempotencyKey := r.Header.Get("Idempotency-Key")

	var req ConfirmFinancialEventRequest
	if r.Body != nil && r.ContentLength > 0 {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	var corrections *command.OverrideFields
	if req.Corrections != nil {
		var amt *command.AmountField
		if req.Corrections.Amount != nil {
			amt = &command.AmountField{
				Value:    req.Corrections.Amount.Value,
				Currency: req.Corrections.Amount.Currency,
			}
		}
		corrections = &command.OverrideFields{
			Amount:    amt,
			Merchant:  req.Corrections.Merchant,
			EventType: req.Corrections.EventType,
			Category:  req.Corrections.Category,
		}
	}

	fe, err := h.confirmCmd.Handle(r.Context(), command.ConfirmPendingReviewCommand{
		UserID:         userID,
		EventID:        eventID,
		IdempotencyKey: idempotencyKey,
		ActorUserID:    userID,
		Corrections:    corrections,
	})
	if err != nil {
		h.handleDomainError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": string(fe.Status()),
		"event":  ToEventResponse(fe, true),
	})
}

// ListEventOverrides handles GET /financial-events/{id}/overrides.
func (h *Handler) ListEventOverrides(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(userIDContextKey).(string)
	if !ok || userID == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Missing user identity", "", requestID(r))
		return
	}
	eventID := chi.URLParam(r, "id")

	overrides, err := h.listOverridesQuery.Handle(r.Context(), query.ListEventOverridesQuery{
		UserID:  userID,
		EventID: eventID,
	})
	if err != nil {
		h.handleDomainError(w, r, err)
		return
	}

	resp := make([]EventOverrideResponse, len(overrides))
	for i, o := range overrides {
		resp[i] = ToOverrideResponse(o)
	}

	writeJSON(w, http.StatusOK, resp)
}

var domainErrorMappings = []struct {
	target error
	status int
	code   string
	field  string
}{
	{domain.ErrEventNotFound, http.StatusNotFound, "EVENT_NOT_FOUND", ""},
	{domain.ErrCurrencyImmutable, http.StatusUnprocessableEntity, "CURRENCY_IMMUTABLE", "amount.currency"},
	{domain.ErrBelowThreshold, http.StatusUnprocessableEntity, "BELOW_CONFIDENCE_THRESHOLD", "confidence"},
	{domain.ErrConfidenceNotEvaluated, http.StatusUnprocessableEntity, "CONFIDENCE_NOT_EVALUATED", "confidence"},
	{domain.ErrEventArchived, http.StatusConflict, "EVENT_ARCHIVED", "status"},
	{domain.ErrNotPendingReview, http.StatusConflict, "NOT_PENDING_REVIEW", "status"},
	{domain.ErrInvalidAmount, http.StatusBadRequest, "INVALID_AMOUNT", "amount.value"},
	{domain.ErrInvalidCurrency, http.StatusBadRequest, "INVALID_CURRENCY", "amount.currency"},
	{domain.ErrInvalidEventType, http.StatusBadRequest, "INVALID_EVENT_TYPE", "eventType"},
	{domain.ErrInvalidPayload, http.StatusBadRequest, "INVALID_PAYLOAD", "typeSpecificPayload"},
	{domain.ErrInvalidCandidate, http.StatusBadRequest, "INVALID_CANDIDATE", ""},
	{domain.ErrNoOverrideChange, http.StatusBadRequest, "NO_OVERRIDE_CHANGE", ""},
	{domain.ErrInvalidCategory, http.StatusBadRequest, "INVALID_CATEGORY", "category"},
	{domain.ErrUserIDRequired, http.StatusBadRequest, "USER_ID_REQUIRED", "userId"},
	{domain.ErrInvalidCursor, http.StatusBadRequest, "INVALID_CURSOR", "cursor"},
}

func (h *Handler) handleDomainError(w http.ResponseWriter, r *http.Request, err error) {
	reqID := requestID(r)
	for _, m := range domainErrorMappings {
		if errors.Is(err, m.target) {
			writeError(w, m.status, m.code, err.Error(), m.field, reqID)
			return
		}
	}
	writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred", "", reqID)
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
