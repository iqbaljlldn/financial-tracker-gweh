package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/identity/adapter/external"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/identity/application/command"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/identity/application/query"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/identity/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/security"
)

type contextKey string

const (
	UserIDContextKey contextKey = "identity.userID"
	RoleContextKey   contextKey = "identity.role"
	EmailContextKey  contextKey = "identity.email"
)

// UserIDFromContext extracts the authenticated user ID from the request context.
func UserIDFromContext(ctx context.Context) (string, bool) {
	val, ok := ctx.Value(UserIDContextKey).(string)
	return val, ok && val != ""
}

// RoleFromContext extracts the authenticated user role from the request context.
func RoleFromContext(ctx context.Context) (string, bool) {
	val, ok := ctx.Value(RoleContextKey).(string)
	return val, ok && val != ""
}

// Handler orchestrates Identity HTTP routes and translates DTOs per 09_api_specification.md §5 & §6.1.
type Handler struct {
	linkAccountCmd    *command.LinkGoogleAccountHandler
	updateSettingsCmd *command.UpdateUserSettingsHandler
	getUserQuery      *query.GetUserByIDHandler
	refreshRepo       domain.RefreshTokenRepository
	userRepo          domain.UserRepository
	oauthClient       *external.GoogleOAuthClient
	csrfManager       *security.OAuthStateManager
	sessionManager    *security.SessionTokenManager
	pkceStoreMu       sync.Mutex
	pkceStore         map[string]string // state -> code_verifier (with expiration cleanup)
}

// HandlerConfig provides initialization dependencies for the Identity HTTP Handler.
type HandlerConfig struct {
	LinkAccountCmd    *command.LinkGoogleAccountHandler
	UpdateSettingsCmd *command.UpdateUserSettingsHandler
	GetUserQuery      *query.GetUserByIDHandler
	RefreshRepo       domain.RefreshTokenRepository
	UserRepo          domain.UserRepository
	OAuthClient       *external.GoogleOAuthClient
	CSRFManager       *security.OAuthStateManager
	SessionManager    *security.SessionTokenManager
}

// NewHandler creates a new Identity HTTP Handler instance.
func NewHandler(cfg HandlerConfig) *Handler {
	return &Handler{
		linkAccountCmd:    cfg.LinkAccountCmd,
		updateSettingsCmd: cfg.UpdateSettingsCmd,
		getUserQuery:      cfg.GetUserQuery,
		refreshRepo:       cfg.RefreshRepo,
		userRepo:          cfg.UserRepo,
		oauthClient:       cfg.OAuthClient,
		csrfManager:       cfg.CSRFManager,
		sessionManager:    cfg.SessionManager,
		pkceStore:         make(map[string]string),
	}
}

// RegisterRoutes mounts all identity endpoints onto the provided chi.Router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	mount := func(prefix string) {
		r.Route(prefix+"/auth", func(auth chi.Router) {
			auth.Post("/google/initiate", h.InitiateGoogleAuth)
			auth.Get("/google/callback", h.GoogleAuthCallback)
			auth.Post("/refresh", h.RefreshSession)
		})

		r.Route(prefix+"/oauth", func(oauth chi.Router) {
			oauth.Post("/google/initiate", h.InitiateGoogleAuth)
			oauth.Get("/google/callback", h.GoogleAuthCallback)
		})

		r.Route(prefix+"/me", func(me chi.Router) {
			me.Use(h.SessionAuthMiddleware)
			me.Get("/", h.GetMe)
			me.Patch("/settings", h.UpdateSettings)
		})
	}

	mount("/api/v1")
	mount("/v1")
	mount("")
}

// SessionAuthMiddleware validates Authorization: Bearer <token> header and injects identity into context.
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
		ctx = context.WithValue(ctx, RoleContextKey, claims.Role)
		ctx = context.WithValue(ctx, EmailContextKey, claims.Email)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// InitiateGoogleAuth handles POST /api/v1/auth/google/initiate per 09_api_specification.md §5.1.
func (h *Handler) InitiateGoogleAuth(w http.ResponseWriter, r *http.Request) {
	redirectURL := r.URL.Query().Get("redirectUrl")

	// 1. Generate CSRF state token
	state, err := h.csrfManager.GenerateState(redirectURL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to generate CSRF state token", "")
		return
	}

	// 2. Generate PKCE code verifier and challenge
	verifier, challenge, err := external.GeneratePKCE()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to generate PKCE challenge", "")
		return
	}

	// Cache PKCE verifier keyed by state
	h.pkceStoreMu.Lock()
	h.pkceStore[state] = verifier
	h.pkceStoreMu.Unlock()

	// 3. Build Google consent screen URL with PKCE challenge and mandated scopes [GATE]
	authURL := h.oauthClient.BuildAuthorizationURL(state, challenge)

	writeJSON(w, http.StatusOK, InitiateAuthResponse{
		AuthorizationURL: authURL,
		State:            state,
	})
}

// GoogleAuthCallback handles GET /api/v1/auth/google/callback per 09_api_specification.md §5.1.
func (h *Handler) GoogleAuthCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		writeError(w, http.StatusBadRequest, "OAUTH_CALLBACK_INVALID", "Missing required code or state parameter", "")
		return
	}

	// 1. Validate CSRF state token
	_, err := h.csrfManager.ValidateState(state)
	if err != nil {
		writeError(w, http.StatusBadRequest, "OAUTH_CALLBACK_INVALID", "Invalid or expired OAuth CSRF state token", "state")
		return
	}

	// 2. Retrieve cached PKCE verifier
	h.pkceStoreMu.Lock()
	verifier, ok := h.pkceStore[state]
	if ok {
		delete(h.pkceStore, state)
	}
	h.pkceStoreMu.Unlock()

	if !ok || verifier == "" {
		writeError(w, http.StatusBadRequest, "OAUTH_CALLBACK_INVALID", "Missing PKCE code verifier for state", "state")
		return
	}

	// 3. Execute LinkGoogleAccount command
	res, err := h.linkAccountCmd.Handle(r.Context(), command.LinkGoogleAccountCommand{
		Code:         code,
		CodeVerifier: verifier,
	})
	if err != nil {
		if strings.Contains(r.Header.Get("Accept"), "text/html") {
			http.Redirect(w, r, "http://localhost:3000/?error="+url.QueryEscape(err.Error()), http.StatusFound)
			return
		}
		if errors.Is(err, domain.ErrEmailAlreadyLinked) {
			writeError(w, http.StatusConflict, "EMAIL_ALREADY_LINKED", "Email is already linked to another account", "")
			return
		}
		writeError(w, http.StatusBadRequest, "OAUTH_CALLBACK_INVALID", err.Error(), "")
		return
	}

	if strings.Contains(r.Header.Get("Accept"), "text/html") {
		targetURL := "http://localhost:3000/?sessionToken=" + url.QueryEscape(res.SessionToken) +
			"&refreshToken=" + url.QueryEscape(res.RefreshToken) +
			"&expiresAt=" + url.QueryEscape(FormatRFC3339(res.ExpiresAt))
		http.Redirect(w, r, targetURL, http.StatusFound)
		return
	}

	writeJSON(w, http.StatusOK, CallbackAuthResponse{
		SessionToken: res.SessionToken,
		RefreshToken: res.RefreshToken,
		ExpiresAt:    FormatRFC3339(res.ExpiresAt),
		User:         ToUserDTO(res.User),
	})
}

// RefreshSession handles POST /api/v1/auth/refresh per 16_security.md §5.1.
func (h *Handler) RefreshSession(w http.ResponseWriter, r *http.Request) {
	var req RefreshSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "Valid refreshToken is required", "refreshToken")
		return
	}

	// 1. Hash opaque token to look up valid DB record
	tokenHash := security.HashRefreshToken(req.RefreshToken)
	userID, err := h.refreshRepo.FindValidByHash(r.Context(), tokenHash)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "INVALID_REFRESH_TOKEN", "Refresh token is invalid or has been revoked", "refreshToken")
		return
	}

	// 2. Retrieve user profile
	user, err := h.userRepo.FindByID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Associated user account was not found", "")
		return
	}

	// 3. Issue fresh platform session JWT
	newSessionToken, expiresAt, err := h.sessionManager.Issue(user.ID(), user.Email(), "user", 1*time.Hour)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to issue new session token", "")
		return
	}

	writeJSON(w, http.StatusOK, RefreshSessionResponse{
		SessionToken: newSessionToken,
		ExpiresAt:    FormatRFC3339(expiresAt),
	})
}

// GetMe handles GET /api/v1/me per 09_api_specification.md §6.1.
func (h *Handler) GetMe(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized access", "")
		return
	}

	// Execute query with mandatory userID ownership check [GATE]
	user, err := h.getUserQuery.Handle(r.Context(), query.GetUserByIDQuery{UserID: userID})
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "User profile not found", "")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), "")
		return
	}

	writeJSON(w, http.StatusOK, ToUserDTO(user))
}

// UpdateSettings handles PATCH /api/v1/me/settings per 09_api_specification.md §6.1.
func (h *Handler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized access", "")
		return
	}

	var req UpdateSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "Malformed request body", "")
		return
	}

	displayName := ""
	if req.DisplayName != nil {
		displayName = *req.DisplayName
	}
	timezone := ""
	if req.Timezone != nil {
		timezone = *req.Timezone
	}

	// Execute command with mandatory userID ownership check [GATE]
	user, err := h.updateSettingsCmd.Handle(r.Context(), command.UpdateUserSettingsCommand{
		UserID:                userID,
		DisplayName:           displayName,
		Timezone:              timezone,
		NotificationDailyTime: req.NotificationDailyTime,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), "")
		return
	}

	writeJSON(w, http.StatusOK, ToUserDTO(user))
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, code, message, field string) {
	reqID := w.Header().Get("X-Request-ID")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorEnvelope{
		Error: ErrorDetail{
			Code:      code,
			Message:   message,
			Field:     field,
			RequestID: reqID,
		},
	})
}
