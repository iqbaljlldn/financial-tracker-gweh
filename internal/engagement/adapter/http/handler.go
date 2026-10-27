package http

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/engagement/application/command"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/engagement/application/query"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/security"
)

type contextKey string

const (
	userIDContextKey contextKey = "engagement.userID"
)

// ErrorDetails models standard API error payload per 09_api_specification.md §7.2.
type ErrorDetails struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Field     string `json:"field,omitempty"`
	RequestID string `json:"requestId,omitempty"`
}

// ErrorEnvelope wraps ErrorDetails.
type ErrorEnvelope struct {
	Error ErrorDetails `json:"error"`
}

// HandlerConfig encapsulates HTTP handler dependencies.
type HandlerConfig struct {
	GetPreferencesQuery    *query.GetPreferencesHandler
	UpdatePreferencesCmd   *command.UpdatePreferencesHandler
	ProcessInboundReplyCmd *command.ProcessInboundReplyHandler
	ProcessDeliveryStatus  *command.ProcessDeliveryStatusHandler
	SessionManager         *security.SessionTokenManager
	MetaVerifyToken        string
	MetaAppSecret          string
	Logger                 *slog.Logger
}

// Handler serves HTTP endpoints for engagement and WhatsApp webhooks.
type Handler struct {
	getPreferencesQuery    *query.GetPreferencesHandler
	updatePreferencesCmd   *command.UpdatePreferencesHandler
	processInboundReplyCmd *command.ProcessInboundReplyHandler
	processDeliveryStatus  *command.ProcessDeliveryStatusHandler
	sessionManager         *security.SessionTokenManager
	metaVerifyToken        string
	metaAppSecret          string
	logger                 *slog.Logger
}

// NewHandler constructs an engagement HTTP handler.
func NewHandler(cfg HandlerConfig) *Handler {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Handler{
		getPreferencesQuery:    cfg.GetPreferencesQuery,
		updatePreferencesCmd:   cfg.UpdatePreferencesCmd,
		processInboundReplyCmd: cfg.ProcessInboundReplyCmd,
		processDeliveryStatus:  cfg.ProcessDeliveryStatus,
		sessionManager:         cfg.SessionManager,
		metaVerifyToken:        cfg.MetaVerifyToken,
		metaAppSecret:          cfg.MetaAppSecret,
		logger:                 cfg.Logger,
	}
}

// RegisterRoutes mounts engagement routes.
func (h *Handler) RegisterRoutes(r chi.Router) {
	mountRoutes := func(prefix string) {
		r.Route(prefix+"/notifications/preferences", func(cr chi.Router) {
			cr.Use(h.SessionAuthMiddleware)
			cr.Get("/", h.GetPreferences)
			cr.Patch("/", h.UpdatePreferences)
		})

		r.Route(prefix+"/webhooks/whatsapp", func(cr chi.Router) {
			cr.Post("/fonnte", h.HandleFonnteWebhook)
			cr.Post("/delivery-status", h.HandleDeliveryStatusWebhook)
			cr.Get("/meta", h.VerifyMetaWebhook)
			cr.Post("/meta", h.HandleMetaWebhook)
		})
	}

	mountRoutes("/api/v1")
	mountRoutes("/v1")
	mountRoutes("")
}

// SessionAuthMiddleware validates Authorization: Bearer <token>.
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

// --- Preferences Endpoints ---

// GetPreferences handles GET /notifications/preferences (FR-NOTIF-05).
func (h *Handler) GetPreferences(w http.ResponseWriter, r *http.Request) {
	userIDStr, ok := r.Context().Value(userIDContextKey).(string)
	if !ok || userIDStr == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Missing user context", "", requestID(r))
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_USER_ID", "Malformed user UUID", "", requestID(r))
		return
	}

	pref, err := h.getPreferencesQuery.Handle(r.Context(), userID)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "failed to get notification preferences", "error", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve preferences", "", requestID(r))
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"userId":                   pref.UserID.String(),
		"phoneNumber":              pref.PhoneNumber,
		"dailySummaryEnabled":      pref.DailySummaryEnabled,
		"weeklySummaryEnabled":     pref.WeeklySummaryEnabled,
		"monthlyReportEnabled":     pref.MonthlyReportEnabled,
		"budgetAlertsEnabled":      pref.BudgetAlertsEnabled,
		"aiRecommendationsEnabled": pref.AiRecommendationsEnabled,
	})
}

// UpdatePreferences handles PATCH /notifications/preferences (FR-NOTIF-05).
func (h *Handler) UpdatePreferences(w http.ResponseWriter, r *http.Request) {
	userIDStr, ok := r.Context().Value(userIDContextKey).(string)
	if !ok || userIDStr == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Missing user context", "", requestID(r))
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_USER_ID", "Malformed user UUID", "", requestID(r))
		return
	}

	var req struct {
		PhoneNumber              *string `json:"phoneNumber"`
		DailySummaryEnabled      *bool   `json:"dailySummaryEnabled"`
		WeeklySummaryEnabled     *bool   `json:"weeklySummaryEnabled"`
		MonthlyReportEnabled     *bool   `json:"monthlyReportEnabled"`
		BudgetAlertsEnabled      *bool   `json:"budgetAlertsEnabled"`
		AiRecommendationsEnabled *bool   `json:"aiRecommendationsEnabled"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PAYLOAD", "Malformed request body", "", requestID(r))
		return
	}

	cmd := command.UpdatePreferencesCommand{
		UserID:                   userID,
		PhoneNumber:              req.PhoneNumber,
		DailySummaryEnabled:      req.DailySummaryEnabled,
		WeeklySummaryEnabled:     req.WeeklySummaryEnabled,
		MonthlyReportEnabled:     req.MonthlyReportEnabled,
		BudgetAlertsEnabled:      req.BudgetAlertsEnabled,
		AiRecommendationsEnabled: req.AiRecommendationsEnabled,
	}

	updated, err := h.updatePreferencesCmd.Handle(r.Context(), cmd)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "failed to update notification preferences", "error", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to save preferences", "", requestID(r))
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"userId":                   updated.UserID.String(),
		"phoneNumber":              updated.PhoneNumber,
		"dailySummaryEnabled":      updated.DailySummaryEnabled,
		"weeklySummaryEnabled":     updated.WeeklySummaryEnabled,
		"monthlyReportEnabled":     updated.MonthlyReportEnabled,
		"budgetAlertsEnabled":      updated.BudgetAlertsEnabled,
		"aiRecommendationsEnabled": updated.AiRecommendationsEnabled,
	})
}

// --- Webhooks ---

// HandleFonnteWebhook processes incoming messages and delivery statuses from Fonnte.
func (h *Handler) HandleFonnteWebhook(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		ID      string `json:"id"`
		Sender  string `json:"sender"`
		Message string `json:"message"`
		Button  string `json:"button"`
		Status  string `json:"status"`
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PAYLOAD", "Cannot read request body", "", requestID(r))
		return
	}

	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		// Also accept form data if Fonnte sends application/x-www-form-urlencoded
		payload.Sender = r.FormValue("sender")
		payload.Message = r.FormValue("message")
		payload.Button = r.FormValue("button")
		payload.Status = r.FormValue("status")
		payload.ID = r.FormValue("id")
	}

	h.logger.InfoContext(r.Context(), "received Fonnte webhook",
		"sender", payload.Sender,
		"status", payload.Status,
		"id", payload.ID,
	)

	// If it's a delivery status update:
	if payload.Status != "" && payload.ID != "" && h.processDeliveryStatus != nil {
		_ = h.processDeliveryStatus.Handle(r.Context(), command.DeliveryStatusCommand{
			ProviderMessageID: payload.ID,
			Status:            payload.Status,
			Timestamp:         time.Now().UTC(),
		})
	}

	// If it's an inbound reply message:
	if (payload.Message != "" || payload.Button != "") && payload.Sender != "" && h.processInboundReplyCmd != nil {
		_, _ = h.processInboundReplyCmd.Handle(r.Context(), command.InboundReplyCommand{
			SenderPhone: payload.Sender,
			MessageText: payload.Message,
			ButtonID:    payload.Button,
		})
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// HandleDeliveryStatusWebhook accepts delivery status callbacks per Diagram 7.
func (h *Handler) HandleDeliveryStatusWebhook(w http.ResponseWriter, r *http.Request) {
	var payload command.DeliveryStatusCommand
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PAYLOAD", "Malformed request body", "", requestID(r))
		return
	}

	if h.processDeliveryStatus != nil {
		if err := h.processDeliveryStatus.Handle(r.Context(), payload); err != nil {
			h.logger.ErrorContext(r.Context(), "failed to process delivery status", "error", err)
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update status", "", requestID(r))
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "recorded"})
}

// VerifyMetaWebhook handles GET /webhooks/whatsapp/meta (hub.challenge verification).
func (h *Handler) VerifyMetaWebhook(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("hub.mode")
	token := r.URL.Query().Get("hub.verify_token")
	challenge := r.URL.Query().Get("hub.challenge")

	if mode == "subscribe" && token == h.metaVerifyToken && h.metaVerifyToken != "" {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(challenge))
		return
	}

	http.Error(w, "Forbidden", http.StatusForbidden)
}

// HandleMetaWebhook handles POST /webhooks/whatsapp/meta.
func (h *Handler) HandleMetaWebhook(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	// Verify signature if app secret configured
	if h.metaAppSecret != "" {
		sig := r.Header.Get("X-Hub-Signature-256")
		if !verifyMetaSignature(bodyBytes, sig, h.metaAppSecret) {
			http.Error(w, "Unauthorized signature", http.StatusUnauthorized)
			return
		}
	}

	var metaPayload struct {
		Entry []struct {
			Changes []struct {
				Value struct {
					Messages []struct {
						From string `json:"from"`
						Text struct {
							Body string `json:"body"`
						} `json:"text"`
						Interactive struct {
							ButtonReply struct {
								ID    string `json:"id"`
								Title string `json:"title"`
							} `json:"button_reply"`
						} `json:"interactive"`
					} `json:"messages"`
					Statuses []struct {
						ID     string `json:"id"`
						Status string `json:"status"`
					} `json:"statuses"`
				} `json:"value"`
			} `json:"changes"`
		} `json:"entry"`
	}

	if err := json.Unmarshal(bodyBytes, &metaPayload); err == nil {
		for _, entry := range metaPayload.Entry {
			for _, ch := range entry.Changes {
				// Process delivery statuses
				for _, st := range ch.Value.Statuses {
					if h.processDeliveryStatus != nil {
						_ = h.processDeliveryStatus.Handle(r.Context(), command.DeliveryStatusCommand{
							ProviderMessageID: st.ID,
							Status:            st.Status,
							Timestamp:         time.Now().UTC(),
						})
					}
				}

				// Process inbound messages
				for _, msg := range ch.Value.Messages {
					if h.processInboundReplyCmd != nil {
						btnID := msg.Interactive.ButtonReply.ID
						body := msg.Text.Body
						_, _ = h.processInboundReplyCmd.Handle(r.Context(), command.InboundReplyCommand{
							SenderPhone: msg.From,
							MessageText: body,
							ButtonID:    btnID,
						})
					}
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func verifyMetaSignature(payload []byte, headerSig, secret string) bool {
	if headerSig == "" {
		return false
	}
	parts := strings.SplitN(headerSig, "=", 2)
	if len(parts) != 2 || parts[0] != "sha256" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(parts[1]), []byte(expected))
}

// Helpers
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
	reqID := r.Header.Get("X-Request-ID")
	if reqID == "" {
		reqID = uuid.New().String()
	}
	return reqID
}
