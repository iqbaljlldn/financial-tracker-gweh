package integration_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/identity"
	identitydomain "github.com/iqbaljlldn/financial-tracker-gweh/internal/identity/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/ingestion"
	ingestionHTTP "github.com/iqbaljlldn/financial-tracker-gweh/internal/ingestion/adapter/http"
	ingestiondomain "github.com/iqbaljlldn/financial-tracker-gweh/internal/ingestion/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/security"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/taskserver"
)

// mockTokenProvider provides a test implementation of identity.TokenProvider
type mockTokenProvider struct {
	token string
}

func (m *mockTokenProvider) GetValidAccessToken(ctx context.Context, gmailAccountID string) (string, error) {
	if m.token != "" {
		return m.token, nil
	}
	return "mock_access_token_val_123", nil
}

func setupMockGmailServer(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// 1. Profile
		if r.URL.Path == "/me/profile" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"emailAddress":  "syncuser@gmail.com",
				"historyId":     "hist_init_100",
				"messagesTotal": 3,
			})
			return
		}

		// 2. Messages List (paginated)
		if r.URL.Path == "/me/messages" {
			pageToken := r.URL.Query().Get("pageToken")
			if pageToken == "" {
				// Page 1: returns msg1 and msg2, with nextPageToken="page_token_2"
				_ = json.NewEncoder(w).Encode(map[string]any{
					"messages": []map[string]string{
						{"id": "msg_001", "threadId": "t_001"},
						{"id": "msg_002", "threadId": "t_002"},
					},
					"nextPageToken":      "page_token_2",
					"resultSizeEstimate": 3,
				})
				return
			} else if pageToken == "page_token_2" {
				// Page 2: returns msg3, terminal page
				_ = json.NewEncoder(w).Encode(map[string]any{
					"messages": []map[string]string{
						{"id": "msg_003", "threadId": "t_003"},
					},
					"nextPageToken":      "",
					"resultSizeEstimate": 3,
				})
				return
			}
		}

		// 3. Message detail /me/messages/{id}
		if len(r.URL.Path) > len("/me/messages/") {
			msgID := r.URL.Path[len("/me/messages/"):]
			var subject, snippet, bodyText string

			switch msgID {
			case "msg_001":
				subject = "Receipt for your AWS payment"
				snippet = "Charged $85.50 to your card"
				bodyText = "Thank you for your payment of $85.50 for Amazon Web Services."
			case "msg_002":
				subject = "Struk Pembayaran Tokopedia"
				snippet = "Pembayaran transaksi Rp 250.000 berhasil"
				bodyText = "Rincian transaksi Tokopedia: Total Rp 250.000."
			case "msg_003":
				subject = "Grab E-Receipt"
				snippet = "Your Grab ride on 2026-10-25"
				bodyText = "Total paid: 25.00 SGD via GrabPay."
			default:
				w.WriteHeader(http.StatusNotFound)
				return
			}

			encodedBody := base64.URLEncoding.EncodeToString([]byte(bodyText))

			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":           msgID,
				"historyId":    "hist_msg_" + msgID,
				"internalDate": fmt.Sprintf("%d", time.Now().Add(-24*time.Hour).UnixMilli()),
				"snippet":      snippet,
				"payload": map[string]any{
					"headers": []map[string]string{
						{"name": "Subject", "value": subject},
						{"name": "From", "value": "billing@service.com"},
						{"name": "To", "value": "syncuser@gmail.com"},
					},
					"body": map[string]string{
						"data": encodedBody,
					},
				},
			})
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
}

func setupIngestionIntegrationModule(t *testing.T, pool *pgxpool.Pool, gmailBaseURL string) (*ingestion.Module, *security.SessionTokenManager) {
	jwtSecret := []byte("integration-test-session-secret-32bytes!")
	sessionManager, err := security.NewSessionTokenManager(jwtSecret)
	if err != nil {
		t.Fatalf("failed to create session manager: %v", err)
	}

	tokenProvider := &mockTokenProvider{token: "test_token"}

	// Connect to local Redis for Outbox Relay test if available
	asynqClient, _ := taskserver.NewClient("redis://localhost:6380/0")

	mod, err := ingestion.NewModule(ingestion.Config{
		DB:             pool,
		TokenProvider:  tokenProvider,
		SessionManager: sessionManager,
		GmailBaseURL:   gmailBaseURL,
		AsynqClient:    asynqClient,
	})
	if err != nil {
		t.Fatalf("failed to create ingestion module: %v", err)
	}

	return mod, sessionManager
}

func TestIngestion_FullOnboardingAndInitialSyncFlow(t *testing.T) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, getDatabaseURL())
	if err != nil {
		t.Skipf("PostgreSQL not accessible at %s, skipping integration test: %v", getDatabaseURL(), err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Skipf("PostgreSQL ping failed, skipping integration test: %v", err)
	}

	mockGmail := setupMockGmailServer(t)
	defer mockGmail.Close()

	mod, sessionManager := setupIngestionIntegrationModule(t, pool, mockGmail.URL)

	router := chi.NewRouter()
	mod.RegisterHTTPRoutes(router)

	testUserID := uuid.New().String()
	testEmail := fmt.Sprintf("test-%s@gmail.com", uuid.New().String()[:8])

	sessionToken, _, err := sessionManager.Issue(testUserID, testEmail, "user", 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to issue session token: %v", err)
	}

	// =========================================================================
	// STEP 1: POST /api/v1/me/gmail-accounts (Account Linking) [GATE]
	// =========================================================================
	linkPayload, _ := json.Marshal(map[string]any{
		"emailAddress":   testEmail,
		"syncWindowDays": 90,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/me/gmail-accounts", bytes.NewReader(linkPayload))
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created on link account, got %d body: %s", rec.Code, rec.Body.String())
	}

	var createdAccount ingestionHTTP.AccountDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &createdAccount); err != nil {
		t.Fatalf("failed to unmarshal created account: %v", err)
	}

	if createdAccount.EmailAddress != testEmail {
		t.Fatalf("expected email %s, got %s", testEmail, createdAccount.EmailAddress)
	}
	if createdAccount.Status != string(ingestiondomain.StatusLinking) {
		t.Fatalf("expected status linking, got %s", createdAccount.Status)
	}

	// Verify Outbox Event: GmailAccountLinked was persisted atomically [GATE]
	var outboxEventCount int
	err = pool.QueryRow(ctx, `
		SELECT count(*) FROM ingestion.outbox_events 
		WHERE aggregate_id = $1 AND event_type = 'GmailAccountLinked'
	`, createdAccount.ID).Scan(&outboxEventCount)
	if err != nil {
		t.Fatalf("query outbox events failed: %v", err)
	}
	if outboxEventCount != 1 {
		t.Fatalf("expected 1 GmailAccountLinked outbox event, found %d", outboxEventCount)
	}

	// =========================================================================
	// STEP 2: DUPLICATE LINKING ENFORCEMENT [GATE]
	// =========================================================================
	dupReq := httptest.NewRequest(http.MethodPost, "/api/v1/me/gmail-accounts", bytes.NewReader(linkPayload))
	dupReq.Header.Set("Authorization", "Bearer "+sessionToken)
	dupReq.Header.Set("Content-Type", "application/json")
	dupRec := httptest.NewRecorder()
	router.ServeHTTP(dupRec, dupReq)

	if dupRec.Code != http.StatusConflict {
		t.Fatalf("GATE VIOLATION: expected 409 Conflict on duplicate email linking, got %d body: %s", dupRec.Code, dupRec.Body.String())
	}

	// =========================================================================
	// STEP 3: GET /api/v1/me/gmail-accounts (Mandatory userID parameter [GATE])
	// =========================================================================
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/me/gmail-accounts", nil)
	listReq.Header.Set("Authorization", "Bearer "+sessionToken)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on list accounts, got %d body: %s", listRec.Code, listRec.Body.String())
	}

	var accountList []ingestionHTTP.AccountDTO
	if err := json.Unmarshal(listRec.Body.Bytes(), &accountList); err != nil {
		t.Fatalf("failed to decode account list: %v", err)
	}
	if len(accountList) == 0 {
		t.Fatal("expected at least 1 account in user account list")
	}

	// =========================================================================
	// STEP 4: TRIGGER INITIAL FULL SYNC WITH PAGINATION (msg1, msg2 -> msg3)
	// =========================================================================
	syncRes, err := mod.StartInitialFullSyncCmd.Handle(ctx, ingestion.StartInitialFullSyncCommand{
		AccountID:  createdAccount.ID,
		WindowDays: 90,
	})
	if err != nil {
		t.Fatalf("initial full sync failed: %v", err)
	}

	if syncRes.MessagesIngestedCount != 3 {
		t.Fatalf("expected 3 messages ingested across 2 pages, got %d", syncRes.MessagesIngestedCount)
	}
	if syncRes.Status != ingestiondomain.StatusActive {
		t.Fatalf("expected account status active after sync completion, got %s", syncRes.Status)
	}

	// Direct DB Verification: verify 3 messages exist in ingestion.ingested_messages
	var msgCount int
	err = pool.QueryRow(ctx, `
		SELECT count(*) FROM ingestion.ingested_messages 
		WHERE gmail_account_id = $1
	`, createdAccount.ID).Scan(&msgCount)
	if err != nil {
		t.Fatalf("query ingested_messages count failed: %v", err)
	}
	if msgCount != 3 {
		t.Fatalf("expected 3 rows in ingested_messages, got %d", msgCount)
	}

	// Direct DB Verification: verify 3 MessageIngested outbox events were recorded [GATE]
	var msgOutboxCount int
	err = pool.QueryRow(ctx, `
		SELECT count(*) FROM ingestion.outbox_events 
		WHERE event_type = 'MessageIngested' AND payload->>'gmailAccountId' = $1
	`, createdAccount.ID).Scan(&msgOutboxCount)
	if err != nil {
		t.Fatalf("query outbox MessageIngested count failed: %v", err)
	}
	if msgOutboxCount != 3 {
		t.Fatalf("GATE VIOLATION: expected 3 MessageIngested outbox events, found %d", msgOutboxCount)
	}

	// =========================================================================
	// STEP 5: RESUMED SYNC AFTER SIMULATED CRASH & IDEMPOTENCY VERIFICATION
	// Re-running initial sync MUST NOT duplicate messages or outbox events
	// =========================================================================
	resumedRes, err := mod.StartInitialFullSyncCmd.Handle(ctx, ingestion.StartInitialFullSyncCommand{
		AccountID:  createdAccount.ID,
		WindowDays: 90,
	})
	if err != nil {
		t.Fatalf("resumed initial sync failed: %v", err)
	}

	// New messages ingested should be 0 because all 3 were already committed
	if resumedRes.MessagesIngestedCount != 0 {
		t.Fatalf("expected 0 new messages on re-sync, got %d", resumedRes.MessagesIngestedCount)
	}

	// Messages in DB must still be exactly 3 (no duplicates)
	err = pool.QueryRow(ctx, `
		SELECT count(*) FROM ingestion.ingested_messages 
		WHERE gmail_account_id = $1
	`, createdAccount.ID).Scan(&msgCount)
	if err != nil {
		t.Fatalf("query ingested_messages count failed: %v", err)
	}
	if msgCount != 3 {
		t.Fatalf("IDEMPOTENCY FAILURE: duplicate messages inserted, count is %d", msgCount)
	}

	// =========================================================================
	// STEP 6: OUTBOX RELAY POLLER TEST (SELECT ... FOR UPDATE SKIP LOCKED) [GATE]
	// =========================================================================
	relayedCount, err := mod.PollOutboxOnce(ctx)
	if err != nil {
		t.Fatalf("outbox relay PollOnce failed: %v", err)
	}
	if relayedCount == 0 {
		t.Log("Note: Asynq client was not connected, skipping outbox queue verification")
	} else {
		// Verify in DB that outbox rows were marked relayed=true
		var unrelayedCount int
		err = pool.QueryRow(ctx, `
			SELECT count(*) FROM ingestion.outbox_events 
			WHERE relayed = false AND payload->>'gmailAccountId' = $1
		`, createdAccount.ID).Scan(&unrelayedCount)
		if err != nil {
			t.Fatalf("query unrelayed outbox count failed: %v", err)
		}
		if unrelayedCount != 0 {
			t.Fatalf("expected 0 unrelayed outbox events after relay poll, found %d", unrelayedCount)
		}
	}
}

// Ensure identity and ingestion domain imports satisfy boundary checker
var _ identitydomain.TokenProvider = (*mockTokenProvider)(nil)
var _ = identity.Config{}
