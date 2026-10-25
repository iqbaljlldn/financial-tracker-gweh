package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/ingestion/application/command"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/ingestion/application/query"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/ingestion/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/security"
)

type contextKey string

const (
	UserIDContextKey contextKey = "ingestion.userID"
)

// Handler exposes HTTP endpoints for Ingestion module per 09_api_specification.md §6.1.
type Handler struct {
	linkCmd        *command.LinkGmailAccountHandler
	syncCmd        *command.StartInitialFullSyncHandler
	listQuery      *query.ListGmailAccountsHandler
	sessionManager *security.SessionTokenManager
	oauthAuthURL   string
}

// HandlerConfig configures Ingestion HTTP Handler dependencies.
type HandlerConfig struct {
	LinkCmd        *command.LinkGmailAccountHandler
	SyncCmd        *command.StartInitialFullSyncHandler
	ListQuery      *query.ListGmailAccountsHandler
	SessionManager *security.SessionTokenManager
	OAuthAuthURL   string
}

// NewHandler constructs an Ingestion HTTP Handler.
func NewHandler(cfg HandlerConfig) *Handler {
	return &Handler{
		linkCmd:        cfg.LinkCmd,
		syncCmd:        cfg.SyncCmd,
		listQuery:      cfg.ListQuery,
		sessionManager: cfg.SessionManager,
		oauthAuthURL:   cfg.OAuthAuthURL,
	}
}

// RegisterRoutes mounts Ingestion endpoints on Chi router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	mount := func(prefix string) {
		r.Route(prefix+"/me/gmail-accounts", func(acc chi.Router) {
			acc.Use(h.SessionAuthMiddleware)
			acc.Get("/", h.ListAccounts)
			acc.Post("/", h.LinkAccount)
			acc.Post("/{id}/sync", h.TriggerSync)
		})
	}

	mount("/api/v1")
	mount("/v1")
	mount("")
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
// Mandatory row-level userID ownership enforced [GATE].
func (h *Handler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDContextKey).(string)
	if !ok || userID == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "User identity not found in context", "")
		return
	}

	accounts, err := h.listQuery.Handle(r.Context(), query.ListGmailAccountsQuery{
		UserID: userID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve linked accounts", "")
		return
	}

	dtos := make([]AccountDTO, len(accounts))
	for i, a := range accounts {
		dtos[i] = toAccountDTO(a)
	}

	writeJSON(w, http.StatusOK, dtos)
}

// LinkAccountRequest models the request body for POST /me/gmail-accounts.
type LinkAccountRequest struct {
	EmailAddress   string `json:"emailAddress"`
	SyncWindowDays int    `json:"syncWindowDays"`
}

// LinkAccount handles POST /me/gmail-accounts per 09_api_specification.md §6.1.
// UNIQUE (email_address) constraint enforced at DB level — duplicate returns 409 [GATE].
func (h *Handler) LinkAccount(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDContextKey).(string)
	if !ok || userID == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "User identity not found in context", "")
		return
	}

	var req LinkAccountRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	// If email address is provided, link directly
	if strings.TrimSpace(req.EmailAddress) != "" {
		res, err := h.linkCmd.Handle(r.Context(), command.LinkGmailAccountCommand{
			UserID:                userID,
			EmailAddress:          req.EmailAddress,
			InitialSyncWindowDays: req.SyncWindowDays,
		})
		if err != nil {
			if errors.Is(err, domain.ErrEmailAlreadyLinked) {
				writeError(w, http.StatusConflict, "EMAIL_ALREADY_LINKED", "Email address is already linked to an account", "emailAddress")
				return
			}
			writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error(), "")
			return
		}

		writeJSON(w, http.StatusCreated, toAccountDTO(res.Account))
		return
	}

	// If no email provided, return 202 Accepted with OAuth consent URL per 09_api_specification.md §6.1
	authURL := h.oauthAuthURL
	if authURL == "" {
		authURL = "/api/v1/auth/google/initiate"
	}

	writeJSON(w, http.StatusAccepted, map[string]string{
		"authorizationUrl": authURL,
		"status":           "awaiting_consent",
	})
}

// TriggerSync handles POST /me/gmail-accounts/{id}/sync for on-demand sync execution.
func (h *Handler) TriggerSync(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "id")
	if accountID == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "Account ID is required", "id")
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
