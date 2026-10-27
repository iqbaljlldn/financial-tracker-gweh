package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	adminHTTP "github.com/iqbaljlldn/financial-tracker-gweh/internal/administration/adapter/http"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/administration/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/security"
)

type mockAuditRepo struct {
	entries []*domain.AuditLogEntry
	logged  []map[string]any
}

func (m *mockAuditRepo) Insert(ctx context.Context, entry *domain.AuditLogEntry) error {
	m.entries = append(m.entries, entry)
	return nil
}

func (m *mockAuditRepo) List(ctx context.Context, filter domain.AuditLogFilter) ([]*domain.AuditLogEntry, int64, error) {
	return m.entries, int64(len(m.entries)), nil
}

func (m *mockAuditRepo) Log(ctx context.Context, userID *string, action, actor string, metadata map[string]any) error {
	m.logged = append(m.logged, map[string]any{
		"action": action,
		"actor":  actor,
	})
	entry := domain.NewAuditLogEntry("audit-1", userID, action, actor, metadata, time.Now().UTC())
	return m.Insert(ctx, entry)
}

type mockConnectorRepo struct{}

func (m *mockConnectorRepo) Insert(ctx context.Context, h *domain.ConnectorHealth) error {
	return nil
}

func (m *mockConnectorRepo) ListLatest(ctx context.Context) ([]*domain.ConnectorHealth, error) {
	return []*domain.ConnectorHealth{
		{
			ID:                 "ch-1",
			ConnectorType:      "gmail",
			ConnectorAccountID: "acc-1",
			Status:             "active",
			ErrorCount:         0,
			LatencyMs:          120,
			RecordedAt:         time.Now().UTC(),
		},
	}, nil
}

type mockParserRepo struct{}

func (m *mockParserRepo) Insert(ctx context.Context, metric *domain.ParserMetric) error {
	return nil
}

func (m *mockParserRepo) GetAggregated(ctx context.Context) ([]*domain.ParserMetric, error) {
	return []*domain.ParserMetric{
		{
			SenderPattern:   "orders@tokopedia.com",
			TemplateVersion: 1,
			TotalParsed:     100,
			FailedParsed:    2,
			FailureRate:     0.02,
			SpikeAlert:      false,
			RecordedAt:      time.Now().UTC(),
		},
	}, nil
}

type mockPromptRepo struct{}

func (m *mockPromptRepo) Insert(ctx context.Context, metric *domain.PromptMetric) error {
	return nil
}

func (m *mockPromptRepo) GetSummary(ctx context.Context) ([]*domain.PromptMetricsSummary, error) {
	return []*domain.PromptMetricsSummary{
		{
			PromptVersion:   "v1",
			ModelIdentifier: "qwen/qwen3.8-27b",
			TotalCalls:      50,
			AvgConfidence:   0.94,
			ValidCount:      48,
			InvalidCount:    2,
			OverriddenCount: 1,
			AvgLatencyMs:    450.5,
			TotalTokens:     25000,
		},
	}, nil
}

func (m *mockPromptRepo) ListRecent(ctx context.Context, limit int32) ([]*domain.PromptMetric, error) {
	return nil, nil
}

func setupTestRouter(t *testing.T) (*chi.Mux, *security.SessionTokenManager, *mockAuditRepo) {
	signingKey := []byte("session-secret-key-must-be-at-least-32-bytes!")
	sessionMgr, err := security.NewSessionTokenManager(signingKey)
	if err != nil {
		t.Fatalf("failed to create session manager: %v", err)
	}

	auditRepo := &mockAuditRepo{}
	connectorRepo := &mockConnectorRepo{}
	parserRepo := &mockParserRepo{}
	promptRepo := &mockPromptRepo{}

	handler := adminHTTP.NewHandler(adminHTTP.HandlerConfig{
		AuditRepo:     auditRepo,
		AuditLogger:   auditRepo,
		ConnectorRepo: connectorRepo,
		ParserRepo:    parserRepo,
		PromptRepo:    promptRepo,
		SessionMgr:    sessionMgr,
	})

	r := chi.NewRouter()
	handler.RegisterRoutes(r)
	return r, sessionMgr, auditRepo
}

func TestAdminRoutes_RBACEnforcement(t *testing.T) {
	router, sessionMgr, auditRepo := setupTestRouter(t)

	// Issue user-role token (regular user)
	userToken, _, err := sessionMgr.Issue("user-123", "user@example.com", "user", 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to issue user token: %v", err)
	}

	// Issue ops-role token
	opsToken, _, err := sessionMgr.Issue("ops-456", "ops@fingweh.internal", "ops", 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to issue ops token: %v", err)
	}

	testEndpoints := []string{
		"/api/v1/admin/connectors/health",
		"/api/v1/admin/parsers/metrics",
		"/api/v1/admin/prompts/metrics",
		"/api/v1/admin/audit-log",
	}

	for _, ep := range testEndpoints {
		t.Run("Unauthorized without token: "+ep, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, ep, nil)
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			if rr.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401 Unauthorized, got %d", rr.Code)
			}
		})

		t.Run("Forbidden with regular user role: "+ep, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, ep, nil)
			req.Header.Set("Authorization", "Bearer "+userToken)
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			if rr.Code != http.StatusForbidden {
				t.Fatalf("expected 403 Forbidden for user role, got %d", rr.Code)
			}
		})

		t.Run("Allowed with ops role: "+ep, func(t *testing.T) {
			initialAuditCount := len(auditRepo.entries)

			req := httptest.NewRequest(http.MethodGet, ep, nil)
			req.Header.Set("Authorization", "Bearer "+opsToken)
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("expected 200 OK for ops role, got %d. Body: %s", rr.Code, rr.Body.String())
			}

			// Verify audit-log-on-every-access per 16_security.md §6.3
			if len(auditRepo.entries) <= initialAuditCount {
				t.Fatalf("expected audit log entry to be created upon ops access to %s", ep)
			}
			lastEntry := auditRepo.entries[len(auditRepo.entries)-1]
			if lastEntry.Action() != "admin_access" {
				t.Fatalf("expected action admin_access, got %s", lastEntry.Action())
			}
		})
	}
}

func TestAdminAuditLog_FiltersAndPagination(t *testing.T) {
	router, sessionMgr, _ := setupTestRouter(t)

	opsToken, _, err := sessionMgr.Issue("ops-456", "ops@fingweh.internal", "ops", 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to issue ops token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/audit-log?limit=10&offset=0", nil)
	req.Header.Set("Authorization", "Bearer "+opsToken)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rr.Code)
	}

	var res adminHTTP.AuditLogListResponse
	if err := json.NewDecoder(rr.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if res.Pagination.Limit != 10 {
		t.Fatalf("expected limit 10, got %d", res.Pagination.Limit)
	}
}
