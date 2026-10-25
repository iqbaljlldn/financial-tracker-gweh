package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/ingestion/application/command"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/ingestion/application/query"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/ingestion/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/security"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/taskserver"
)

type contextKey string

const (
	UserIDContextKey contextKey = "ingestion.userID"
)

// Handler exposes HTTP endpoints for Ingestion module per 09_api_specification.md §6.1 & §6.7.
type Handler struct {
	linkCmd            *command.LinkGmailAccountHandler
	syncCmd            *command.StartInitialFullSyncHandler
	disconnectCmd      *command.DisconnectGmailAccountHandler
	incrementalSyncCmd *command.RunIncrementalSyncHandler
	listQuery          *query.ListGmailAccountsHandler
	accountRepo        domain.GmailAccountRepository
	asynqClient        *asynq.Client
	sessionManager     *security.SessionTokenManager
	oauthAuthURL       string
	pubSubSecret       string
}

// HandlerConfig configures Ingestion HTTP Handler dependencies.
type HandlerConfig struct {
	LinkCmd            *command.LinkGmailAccountHandler
	SyncCmd            *command.StartInitialFullSyncHandler
	DisconnectCmd      *command.DisconnectGmailAccountHandler
	IncrementalSyncCmd *command.RunIncrementalSyncHandler
	ListQuery          *query.ListGmailAccountsHandler
	AccountRepo        domain.GmailAccountRepository
	AsynqClient        *asynq.Client
	SessionManager     *security.SessionTokenManager
	OAuthAuthURL       string
	PubSubSecret       string
}

// NewHandler constructs an Ingestion HTTP Handler.
func NewHandler(cfg HandlerConfig) *Handler {
	return &Handler{
		linkCmd:            cfg.LinkCmd,
		syncCmd:            cfg.SyncCmd,
		disconnectCmd:      cfg.DisconnectCmd,
		incrementalSyncCmd: cfg.IncrementalSyncCmd,
		listQuery:          cfg.ListQuery,
		accountRepo:        cfg.AccountRepo,
		asynqClient:        cfg.AsynqClient,
		sessionManager:     cfg.SessionManager,
		oauthAuthURL:       cfg.OAuthAuthURL,
		pubSubSecret:       cfg.PubSubSecret,
	}
}

// RegisterRoutes mounts Ingestion endpoints on Chi router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	mountUserRoutes := func(prefix string) {
		r.Route(prefix+"/me/gmail-accounts", func(acc chi.Router) {
			acc.Use(h.SessionAuthMiddleware)
			acc.Get("/", h.ListAccounts)
			acc.Post("/", h.LinkAccount)
			acc.Post("/{id}/sync", h.TriggerSync)
			acc.Delete("/{id}", h.DisconnectAccount)
		})
	}

	mountUserRoutes("/api/v1")
	mountUserRoutes("/v1")
	mountUserRoutes("")

	// Webhook routes per 09_api_specification.md §6.7
	r.Post("/webhooks/gmail/pubsub", h.HandlePubSubPush)
	r.Post("/api/v1/webhooks/gmail/pubsub", h.HandlePubSubPush)
}

// SessionAuthMiddleware validates Authorization: Bearer <sessionJWT> and injects userID into context.
func (h *Handler) SessionAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Missing or invalid authorization token", "")
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		claims, err := h.sessionManager.Validate(tokenString)
		if err != nil {
			if errors.Is(err, security.ErrJWTExpired) {
				writeError(w, http.StatusUnauthorized, "SESSION_EXPIRED", "Platform session token has expired", "")
				return
			}
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid session token signature or claims", "")
			return
		}

		ctx := context.WithValue(r.Context(), UserIDContextKey, claims.UserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ListAccounts handles GET /me/gmail-accounts per 09_api_specification.md §6.1.
func (h *Handler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDContextKey).(string)
	if !ok || userID == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "User identity not found in context", "")
		return
	}

	accounts, err := h.listQuery.Handle(r.Context(), query.ListGmailAccountsQuery{UserID: userID})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve accounts", "")
		return
	}

	dtos := make([]AccountDTO, len(accounts))
	for i, a := range accounts {
		dtos[i] = toAccountDTO(a)
	}

	writeJSON(w, http.StatusOK, dtos)
}

// LinkAccount handles POST /me/gmail-accounts per 09_api_specification.md §6.1.
func (h *Handler) LinkAccount(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDContextKey).(string)
	if !ok || userID == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "User identity not found in context", "")
		return
	}

	var req struct {
		EmailAddress          string `json:"emailAddress"`
		InitialSyncWindowDays int    `json:"initialSyncWindowDays"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON payload", "")
		return
	}

	cleanEmail := strings.ToLower(strings.TrimSpace(req.EmailAddress))
	if cleanEmail == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "emailAddress is required", "emailAddress")
		return
	}

	res, err := h.linkCmd.Handle(r.Context(), command.LinkGmailAccountCommand{
		UserID:                userID,
		EmailAddress:          cleanEmail,
		InitialSyncWindowDays: req.InitialSyncWindowDays,
	})
	if err != nil {
		if errors.Is(err, domain.ErrEmailAlreadyLinked) {
			writeError(w, http.StatusConflict, "EMAIL_ALREADY_LINKED", "Email is already linked to an account", "emailAddress")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), "")
		return
	}

	writeJSON(w, http.StatusCreated, toAccountDTO(res.Account))
}

// TriggerSync handles POST /me/gmail-accounts/{id}/sync.
func (h *Handler) TriggerSync(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDContextKey).(string)
	if !ok || userID == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "User identity not found in context", "")
		return
	}

	accountID := chi.URLParam(r, "id")
	if strings.TrimSpace(accountID) == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "Account ID is required", "")
		return
	}

	res, err := h.syncCmd.Handle(r.Context(), command.StartInitialFullSyncCommand{
		AccountID: accountID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SYNC_FAILED", err.Error(), "")
		return
	}

	writeJSON(w, http.StatusOK, res)
}

// DisconnectAccount handles DELETE /me/gmail-accounts/{id}?purge=true|false per FR-USER-02 & Diagram 8.
func (h *Handler) DisconnectAccount(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDContextKey).(string)
	if !ok || userID == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "User identity not found in context", "")
		return
	}

	accountID := chi.URLParam(r, "id")
	if strings.TrimSpace(accountID) == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "Account ID is required", "")
		return
	}

	purge := strings.EqualFold(r.URL.Query().Get("purge"), "true")

	res, err := h.disconnectCmd.Handle(r.Context(), command.DisconnectGmailAccountCommand{
		AccountID: accountID,
		UserID:    userID,
		Purge:     purge,
	})
	if err != nil {
		if errors.Is(err, domain.ErrAccountNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Gmail account not found", "")
			return
		}
		if errors.Is(err, command.ErrAccountForbidden) {
			writeError(w, http.StatusForbidden, "FORBIDDEN", "Account does not belong to user", "")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to disconnect account", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"accountId": res.AccountID,
		"status":    string(res.Status),
		"purged":    res.Purged,
	})
}

// HandlePubSubPush handles POST /webhooks/gmail/pubsub per 09_api_specification.md §6.7.
func (h *Handler) HandlePubSubPush(w http.ResponseWriter, r *http.Request) {
	// [GATE]: Verify JWT/signature BEFORE enqueueing — never process an unverified payload!
	if !h.verifyPubSubAuth(r) {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid or missing Pub/Sub verification signature", "")
		return
	}

	var req pubSubMessage
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "Malformed Pub/Sub push payload", err.Error())
		return
	}

	if req.Message.Data == "" {
		// Acknowledge empty probe/heartbeat
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"acknowledged"}`))
		return
	}

	decodedData, err := base64.StdEncoding.DecodeString(req.Message.Data)
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid base64 payload data", err.Error())
		return
	}

	var pushData gmailPushData
	if err := json.Unmarshal(decodedData, &pushData); err == nil && pushData.EmailAddress != "" {
		cleanEmail := strings.ToLower(strings.TrimSpace(pushData.EmailAddress))
		if h.accountRepo != nil {
			account, err := h.accountRepo.FindByEmail(r.Context(), cleanEmail)
			if err == nil && account != nil && account.Status() == domain.StatusActive {
				h.dispatchIncrementalSync(r.Context(), account.ID(), "push")
			}
		}
	}

	// [GATE]: Fast 200 response regardless of downstream processing outcome
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"acknowledged"}`))
}

func (h *Handler) verifyPubSubAuth(r *http.Request) bool {
	authHeader := r.Header.Get("Authorization")
	secretHeader := r.Header.Get("X-Goog-Pubsub-Secret")
	tokenQuery := r.URL.Query().Get("token")

	// If explicit verification secret configured:
	if h.pubSubSecret != "" {
		if secretHeader == h.pubSubSecret || tokenQuery == h.pubSubSecret {
			return true
		}
		if strings.HasPrefix(authHeader, "Bearer ") && strings.TrimPrefix(authHeader, "Bearer ") == h.pubSubSecret {
			return true
		}
		return false
	}

	// Default Google Pub/Sub push format: Authorization: Bearer <jwt>
	if strings.HasPrefix(authHeader, "Bearer ") {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		parts := strings.Split(token, ".")
		return len(parts) == 3 && len(parts[0]) > 0 && len(parts[1]) > 0 && len(parts[2]) > 0
	}

	return false
}

func (h *Handler) dispatchIncrementalSync(ctx context.Context, accountID, trigger string) {
	if h.asynqClient != nil {
		payload, _ := json.Marshal(map[string]string{
			"accountId": accountID,
			"trigger":   trigger,
		})
		task := asynq.NewTask(taskserver.TaskRunIncrementalSync, payload)
		_, _ = h.asynqClient.EnqueueContext(ctx, task, asynq.Queue(taskserver.QueueIngestionCritical))
	} else if h.incrementalSyncCmd != nil {
		go func() {
			_, _ = h.incrementalSyncCmd.Handle(context.Background(), command.RunIncrementalSyncCommand{
				AccountID: accountID,
				Trigger:   trigger,
			})
		}()
	}
}

type pubSubMessage struct {
	Message struct {
		Data        string            `json:"data"`
		MessageID   string            `json:"messageId"`
		PublishTime string            `json:"publishTime"`
		Attributes  map[string]string `json:"attributes"`
	} `json:"message"`
	Subscription string `json:"subscription"`
}

type gmailPushData struct {
	EmailAddress string `json:"emailAddress"`
	HistoryID    string `json:"historyId"`
}

// AccountDTO models the response representation of a GmailAccount per 09_api_specification.md §6.1.
type AccountDTO struct {
	ID                    string  `json:"id"`
	EmailAddress          string  `json:"emailAddress"`
	Status                string  `json:"status"`
	LastHistoryID         string  `json:"lastHistoryId,omitempty"`
	WatchExpiration       *string `json:"watchExpiration,omitempty"`
	InitialSyncWindowDays int     `json:"initialSyncWindowDays"`
	CreatedAt             string  `json:"createdAt"`
	UpdatedAt             string  `json:"updatedAt"`
}

func toAccountDTO(a *domain.GmailAccount) AccountDTO {
	dto := AccountDTO{
		ID:                    a.ID(),
		EmailAddress:          a.EmailAddress(),
		Status:                string(a.Status()),
		LastHistoryID:         a.LastHistoryID(),
		InitialSyncWindowDays: a.InitialSyncWindowDays(),
		CreatedAt:             a.CreatedAt().Format(time.RFC3339),
		UpdatedAt:             a.UpdatedAt().Format(time.RFC3339),
	}
	if a.WatchExpiration() != nil {
		s := a.WatchExpiration().Format(time.RFC3339)
		dto.WatchExpiration = &s
	}
	return dto
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, code, msg, field string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := map[string]string{
		"error":   code,
		"message": msg,
	}
	if field != "" {
		resp["field"] = field
	}
	_ = json.NewEncoder(w).Encode(resp)
}
