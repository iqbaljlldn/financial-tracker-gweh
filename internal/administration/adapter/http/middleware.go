package http

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/administration/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/observability"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/security"
)

type contextKey string

const (
	UserIDContextKey contextKey = "admin_user_id"
	RoleContextKey   contextKey = "admin_role"
)

// RequireOpsRoleMiddleware enforces individual, non-shared ops role access to /admin/*
// endpoints and audit-logs every access attempt per 16_security.md §6.3 and 09_api_specification.md §5.3.
func RequireOpsRoleMiddleware(sessionManager *security.SessionTokenManager, auditRepo domain.AuditLogger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if !strings.HasPrefix(authHeader, "Bearer ") {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Missing or invalid authorization token", "")
				return
			}

			tokenString := strings.TrimPrefix(authHeader, "Bearer ")
			claims, err := sessionManager.Validate(tokenString)
			if err != nil {
				if errors.Is(err, security.ErrJWTExpired) {
					writeError(w, http.StatusUnauthorized, "SESSION_EXPIRED", "Platform session token has expired", "")
					return
				}
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid session token signature or claims", "")
				return
			}

			// RBAC enforcement: must have role "ops" or "admin" (16_security.md §6.1, §6.3)
			if claims.Role != "ops" && claims.Role != "admin" {
				writeError(w, http.StatusForbidden, "FORBIDDEN", "Forbidden: role 'ops' is required for administrative surface", "")
				return
			}

			// Audit-log every ops-role access per 16_security.md §6.3
			actor := claims.UserID
			if claims.Email != "" {
				actor = claims.UserID + " (" + claims.Email + ")"
			}
			meta := map[string]any{
				"path":      r.URL.Path,
				"method":    r.Method,
				"remoteIp":  r.RemoteAddr,
				"userAgent": r.UserAgent(),
			}
			_ = auditRepo.Log(r.Context(), &claims.UserID, "admin_access", actor, meta)

			// Record Prometheus metric per 17_observability.md §4.7
			metrics := observability.GetMetrics()
			if metrics != nil && metrics.AdminOpsAccessTotal != nil {
				metrics.AdminOpsAccessTotal.WithLabelValues(r.URL.Path).Inc()
			}

			ctx := context.WithValue(r.Context(), UserIDContextKey, claims.UserID)
			ctx = context.WithValue(ctx, RoleContextKey, claims.Role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
