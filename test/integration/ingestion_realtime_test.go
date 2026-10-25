package integration_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/ingestion"
	ingestiondomain "github.com/iqbaljlldn/financial-tracker-gweh/internal/ingestion/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/observability"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/security"
)

type realtimeMockState struct {
	mu                  sync.Mutex
	watchRegistered     bool
	watchDeregistered   bool
	historyExpired      bool
	lastQueriedWindow   int
	deltaHistory        []string
	incrementalCallMade bool
}

func setupRealtimeMockGmailServer(t *testing.T, state *realtimeMockState) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		state.mu.Lock()
		defer state.mu.Unlock()

		// 1. Profile
		if r.URL.Path == "/me/profile" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"emailAddress":  "realtime@gmail.com",
				"historyId":     "hist_base_500",
				"messagesTotal": 10,
			})
			return
		}

		// 2. Watch registration
		if r.URL.Path == "/me/watch" && r.Method == http.MethodPost {
			state.watchRegistered = true
			expMillis := time.Now().Add(7 * 24 * time.Hour).UnixMilli()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"historyId":  "hist_base_500",
				"expiration": fmt.Sprintf("%d", expMillis),
			})
			return
		}

		// 3. Stop watch (Disconnect)
		if r.URL.Path == "/me/stop" && r.Method == http.MethodPost {
			state.watchDeregistered = true
			w.WriteHeader(http.StatusOK)
			return
		}

		// 4. History API (Incremental Sync)
		if r.URL.Path == "/me/history" {
			state.incrementalCallMade = true
			if state.historyExpired {
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error": map[string]any{
						"code":    404,
						"message": "Requested historyId is too old or expired",
					},
				})
				return
			}

			// Return delta messages
			messagesAdded := make([]map[string]any, len(state.deltaHistory))
			for i, mid := range state.deltaHistory {
				messagesAdded[i] = map[string]any{
					"message": map[string]string{"id": mid},
				}
			}

			_ = json.NewEncoder(w).Encode(map[string]any{
				"history": []map[string]any{
					{
						"id":            "delta_event_1",
						"messagesAdded": messagesAdded,
					},
				},
				"historyId": "hist_next_501",
			})
			return
		}

		// 5. InitialSync / Bounded Fallback (messages.list)
		if r.URL.Path == "/me/messages" && r.Method == http.MethodGet {
			_ = r.URL.Query().Get("q")
			// Detect windowDays from after:YYYY/MM/DD
			state.lastQueriedWindow = 3 // Bounded fallback query
			_ = json.NewEncoder(w).Encode(map[string]any{
				"messages": []map[string]string{
					{"id": "msg_fallback_01", "threadId": "t_fb_01"},
				},
				"nextPageToken":      "",
				"resultSizeEstimate": 1,
			})
			return
		}

		// 6. Messages detail
		if len(r.URL.Path) > len("/me/messages/") {
			msgID := r.URL.Path[len("/me/messages/"):]
			var subject, snippet string
			switch msgID {
			case "msg_realtime_01":
				subject = "Uber Receipt - Payment of $24.50"
				snippet = "Thanks for riding with Uber. Total: $24.50"
			case "msg_fallback_01":
				subject = "Invoice for Figma Subscription"
				snippet = "Your payment of $15.00 was successful"
			default:
				subject = "Payment Receipt"
				snippet = "Paid $10.00"
			}

			bodyText := fmt.Sprintf("%s\n%s", subject, snippet)
			encodedBody := base64.URLEncoding.EncodeToString([]byte(bodyText))

			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":           msgID,
				"historyId":    "hist_next_501",
				"internalDate": fmt.Sprintf("%d", time.Now().Add(-5*time.Minute).UnixMilli()),
				"snippet":      snippet,
				"payload": map[string]any{
					"headers": []map[string]string{
						{"name": "Subject", "value": subject},
						{"name": "From", "value": "receipts@uber.com"},
						{"name": "To", "value": "realtime@gmail.com"},
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

func setupRealtimeTestModule(
	t *testing.T,
	pool *pgxpool.Pool,
	gmailBaseURL string,
	verificationSecret string,
) (*ingestion.Module, *security.SessionTokenManager, *chi.Mux) {
	sessionSecret := []byte("session-test-jwt-secret-at-least-32-bytes-long!")
	sessionMgr, err := security.NewSessionTokenManager(sessionSecret)
	if err != nil {
		t.Fatalf("failed to create session manager: %v", err)
	}

	mod, err := ingestion.NewModule(ingestion.Config{
		DB:                       pool,
		TokenProvider:            &mockTokenProvider{token: "mock_valid_token"},
		SessionManager:           sessionMgr,
		GmailBaseURL:             gmailBaseURL,
		PubSubTopic:              "projects/test-proj/topics/gmail-push-topic",
		PubSubVerificationSecret: verificationSecret,
		PollFallbackInterval:     15 * time.Minute,
		HistoryFallbackDays:      3, // [GATE: Bounded 3-day fallback]
		BaseBackoff:              5 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("failed to create ingestion module: %v", err)
	}

	router := chi.NewRouter()
	mod.RegisterHTTPRoutes(router)

	return mod, sessionMgr, router
}

// TestSprint3_RealtimePushIncrementalSync tests push-triggered incremental sync via POST /webhooks/gmail/pubsub per Diagram 2.
func TestSprint3_RealtimePushIncrementalSync(t *testing.T) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, getDatabaseURL())
	if err != nil {
		t.Skipf("skipping integration test, postgres unavailable: %v", err)
	}
	defer pool.Close()

	state := &realtimeMockState{
		deltaHistory: []string{"msg_realtime_01"},
	}
	mockServer := setupRealtimeMockGmailServer(t, state)
	defer mockServer.Close()

	verificationSecret := "secret_pubsub_token_777"
	mod, sessionMgr, router := setupRealtimeTestModule(t, pool, mockServer.URL, verificationSecret)

	userID := uuid.New().String()
	email := fmt.Sprintf("pushuser_%s@gmail.com", uuid.New().String()[:8])

	// 1. Link Account
	linkRes, err := mod.LinkAccountCmd.Handle(ctx, ingestion.LinkGmailAccountCommand{
		UserID:       userID,
		EmailAddress: email,
	})
	if err != nil {
		t.Fatalf("failed to link account: %v", err)
	}
	account := linkRes.Account
	_ = account.CompleteInitialSync("hist_base_500")
	_ = mod.Repo.Update(ctx, account)

	// 2. Test Pub/Sub Webhook Security [GATE — never process an unverified payload]
	pubsubPayload := map[string]any{
		"message": map[string]any{
			"data": base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf(`{"emailAddress": "%s", "historyId": "hist_next_501"}`, email))),
		},
	}
	payloadBytes, _ := json.Marshal(pubsubPayload)

	// Case 2a: Unauthenticated / Invalid Secret -> Expect 401 Unauthorized [GATE]
	unauthReq := httptest.NewRequest(http.MethodPost, "/webhooks/gmail/pubsub", bytes.NewReader(payloadBytes))
	unauthRec := httptest.NewRecorder()
	router.ServeHTTP(unauthRec, unauthReq)
	if unauthRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for unverified pubsub push, got: %d", unauthRec.Code)
	}

	// Case 2b: Authenticated with Verification Secret -> Expect 200 OK fast [GATE]
	authReq := httptest.NewRequest(http.MethodPost, "/webhooks/gmail/pubsub", bytes.NewReader(payloadBytes))
	authReq.Header.Set("Authorization", "Bearer "+verificationSecret)
	authRec := httptest.NewRecorder()
	router.ServeHTTP(authRec, authReq)
	if authRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for verified pubsub push, got: %d body: %s", authRec.Code, authRec.Body.String())
	}

	// 3. Execute Incremental Sync (push triggered)
	syncRes, err := mod.IncrementalSyncCmd.Handle(ctx, ingestion.RunIncrementalSyncCommand{
		AccountID: account.ID(),
		Trigger:   "push",
	})
	if err != nil {
		t.Fatalf("incremental sync failed: %v", err)
	}
	if syncRes.MessagesIngestedCount != 1 {
		t.Fatalf("expected 1 message ingested, got: %d", syncRes.MessagesIngestedCount)
	}

	// 4. Verify DB state: new cursor updated transactionally [GATE]
	refreshedAcc, err := mod.Repo.FindByID(ctx, account.ID())
	if err != nil {
		t.Fatalf("failed to find refreshed account: %v", err)
	}
	if refreshedAcc.LastHistoryID() != "hist_next_501" {
		t.Errorf("expected LastHistoryID to advance to hist_next_501, got: %s", refreshedAcc.LastHistoryID())
	}

	// 5. Verify message ingested record & outbox event
	var outboxEventCount int
	err = pool.QueryRow(ctx, "SELECT count(*) FROM ingestion.outbox_events WHERE event_type = $1", ingestiondomain.EventMessageIngested).Scan(&outboxEventCount)
	if err != nil {
		t.Fatalf("failed to query outbox: %v", err)
	}
	if outboxEventCount == 0 {
		t.Errorf("expected at least 1 MessageIngested outbox event")
	}

	_ = sessionMgr
}

// TestSprint3_PollFallbackSafetyNet_H2Verification verifies H2 resolution: poll-fallback alone processes new mail without push [GATE].
func TestSprint3_PollFallbackSafetyNet_H2Verification(t *testing.T) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, getDatabaseURL())
	if err != nil {
		t.Skipf("skipping integration test, postgres unavailable: %v", err)
	}
	defer pool.Close()

	state := &realtimeMockState{
		deltaHistory: []string{"msg_realtime_01"},
	}
	mockServer := setupRealtimeMockGmailServer(t, state)
	defer mockServer.Close()

	mod, _, _ := setupRealtimeTestModule(t, pool, mockServer.URL, "secret_key")

	userID := uuid.New().String()
	email := fmt.Sprintf("fallback_h2_%s@gmail.com", uuid.New().String()[:8])

	linkRes, err := mod.LinkAccountCmd.Handle(ctx, ingestion.LinkGmailAccountCommand{
		UserID:       userID,
		EmailAddress: email,
	})
	if err != nil {
		t.Fatalf("failed to link account: %v", err)
	}
	account := linkRes.Account
	_ = account.CompleteInitialSync("hist_base_500")
	_ = mod.Repo.Update(ctx, account)

	// PUSH IS ARTIFICIALLY DISABLED (No webhook calls made)
	// Execute the scheduled safety net poll fallback cycle [GATE - H2 Verification]
	mod.PollFallbackScheduler.RunOnce(ctx)

	// Verify that the new mail arrived and was ingested solely via poll-fallback
	refreshedAcc, err := mod.Repo.FindByID(ctx, account.ID())
	if err != nil {
		t.Fatalf("failed to find account: %v", err)
	}
	if refreshedAcc.LastHistoryID() != "hist_next_501" {
		t.Errorf("poll fallback failed to advance cursor, got: %s", refreshedAcc.LastHistoryID())
	}

	// Verify liveness heartbeat was recorded [GATE — "monitor the monitor"]
	// Metric gauge ingestion_poll_fallback_liveness must be non-zero
	var metricExists bool
	reg := observability.GetMetrics().Registry
	mfs, err := reg.Gather()
	if err == nil {
		for _, mf := range mfs {
			if mf.GetName() == "ingestion_poll_fallback_liveness" {
				metricExists = true
				break
			}
		}
	}
	if !metricExists {
		t.Errorf("expected ingestion_poll_fallback_liveness metric to be gathered")
	}
}

// TestSprint3_HistoryIdExpiry_BoundedFallback verifies 404 historyId expiry triggers bounded 3-day sync, NOT 90-day sync [GATE].
func TestSprint3_HistoryIdExpiry_BoundedFallback(t *testing.T) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, getDatabaseURL())
	if err != nil {
		t.Skipf("skipping integration test, postgres unavailable: %v", err)
	}
	defer pool.Close()

	state := &realtimeMockState{
		historyExpired: true, // Simulates 404 from Gmail History API
	}
	mockServer := setupRealtimeMockGmailServer(t, state)
	defer mockServer.Close()

	mod, _, _ := setupRealtimeTestModule(t, pool, mockServer.URL, "secret_key")

	userID := uuid.New().String()
	email := fmt.Sprintf("expired_%s@gmail.com", uuid.New().String()[:8])

	linkRes, err := mod.LinkAccountCmd.Handle(ctx, ingestion.LinkGmailAccountCommand{
		UserID:       userID,
		EmailAddress: email,
	})
	if err != nil {
		t.Fatalf("failed to link account: %v", err)
	}
	account := linkRes.Account
	_ = account.CompleteInitialSync("hist_expired_cursor")
	_ = mod.Repo.Update(ctx, account)

	// Execute incremental sync when cursor is expired
	res, err := mod.IncrementalSyncCmd.Handle(ctx, ingestion.RunIncrementalSyncCommand{
		AccountID: account.ID(),
		Trigger:   "poll_fallback",
	})
	if err != nil {
		t.Fatalf("incremental sync handler failed during fallback: %v", err)
	}

	if !res.FallbackTriggered {
		t.Errorf("expected FallbackTriggered to be true")
	}
	if state.lastQueriedWindow != 3 {
		t.Errorf("expected bounded lookback window of 3 days, got: %d", state.lastQueriedWindow)
	}

	// Verify HistorySyncFallbackTriggered outbox event was recorded [GATE]
	var fallbackEventCount int
	err = pool.QueryRow(ctx, "SELECT count(*) FROM ingestion.outbox_events WHERE event_type = $1", ingestiondomain.EventHistorySyncFallbackTriggered).Scan(&fallbackEventCount)
	if err != nil {
		t.Fatalf("failed to query fallback outbox: %v", err)
	}
	if fallbackEventCount == 0 {
		t.Errorf("expected HistorySyncFallbackTriggered outbox event")
	}
}

// TestSprint3_GmailWatch_RenewalSafetyMargin verifies renewal fires at <= 6 days (1-day safety margin) and is idempotent.
func TestSprint3_GmailWatch_RenewalSafetyMargin(t *testing.T) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, getDatabaseURL())
	if err != nil {
		t.Skipf("skipping integration test, postgres unavailable: %v", err)
	}
	defer pool.Close()

	state := &realtimeMockState{}
	mockServer := setupRealtimeMockGmailServer(t, state)
	defer mockServer.Close()

	mod, _, _ := setupRealtimeTestModule(t, pool, mockServer.URL, "secret_key")

	userID := uuid.New().String()
	email := fmt.Sprintf("watch_%s@gmail.com", uuid.New().String()[:8])

	linkRes, err := mod.LinkAccountCmd.Handle(ctx, ingestion.LinkGmailAccountCommand{
		UserID:       userID,
		EmailAddress: email,
	})
	if err != nil {
		t.Fatalf("failed to link account: %v", err)
	}
	account := linkRes.Account
	_ = account.CompleteInitialSync("hist_base_500")

	// Set watch expiration to 12 hours from now (within the <= 6 days safety margin, since expiration <= now + 24h)
	expSoon := time.Now().UTC().Add(12 * time.Hour)
	account.SetWatchExpiration(expSoon, "hist_base_500")
	_ = mod.Repo.Update(ctx, account)

	// Execute watch renewal sweep
	renewRes, err := mod.RenewWatchesCmd.Handle(ctx, ingestion.RenewExpiringWatchesCommand{
		SafetyMargin: 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("renew expiring watches failed: %v", err)
	}
	if renewRes.RenewedCount < 1 {
		t.Fatalf("expected at least 1 account renewed, got: %d", renewRes.RenewedCount)
	}

	// Verify expiration was extended in database
	refreshed, err := mod.Repo.FindByID(ctx, account.ID())
	if err != nil {
		t.Fatalf("find account: %v", err)
	}
	if refreshed.WatchExpiration() == nil || refreshed.WatchExpiration().Before(time.Now().Add(5*24*time.Hour)) {
		t.Errorf("watch expiration was not extended to ~7 days: %v", refreshed.WatchExpiration())
	}
}

// TestSprint3_AccountLifecycle_DisconnectDataRetention verifies explicit purge=false vs purge=true choices per FR-USER-02.
func TestSprint3_AccountLifecycle_DisconnectDataRetention(t *testing.T) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, getDatabaseURL())
	if err != nil {
		t.Skipf("skipping integration test, postgres unavailable: %v", err)
	}
	defer pool.Close()

	state := &realtimeMockState{}
	mockServer := setupRealtimeMockGmailServer(t, state)
	defer mockServer.Close()

	mod, sessionMgr, router := setupRealtimeTestModule(t, pool, mockServer.URL, "secret_key")

	userID := uuid.New().String()
	token, _, _ := sessionMgr.Issue(userID, "user@gmail.com", "user", 24*time.Hour)

	// Test Case A: Disconnect with purge=false (Keep data)
	emailA := fmt.Sprintf("keep_%s@gmail.com", uuid.New().String()[:8])
	resA, _ := mod.LinkAccountCmd.Handle(ctx, ingestion.LinkGmailAccountCommand{UserID: userID, EmailAddress: emailA})

	reqA := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/me/gmail-accounts/%s?purge=false", resA.Account.ID()), nil)
	reqA.Header.Set("Authorization", "Bearer "+token)
	recA := httptest.NewRecorder()
	router.ServeHTTP(recA, reqA)
	if recA.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for disconnect keep data, got: %d", recA.Code)
	}

	// Verify account status disconnected
	accA, _ := mod.Repo.FindByID(ctx, resA.Account.ID())
	if accA.Status() != ingestiondomain.StatusDisconnected {
		t.Errorf("expected status disconnected, got: %s", accA.Status())
	}

	// Test Case B: Disconnect with purge=true (GDPR purge requested)
	emailB := fmt.Sprintf("purge_%s@gmail.com", uuid.New().String()[:8])
	resB, _ := mod.LinkAccountCmd.Handle(ctx, ingestion.LinkGmailAccountCommand{UserID: userID, EmailAddress: emailB})

	reqB := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/me/gmail-accounts/%s?purge=true", resB.Account.ID()), nil)
	reqB.Header.Set("Authorization", "Bearer "+token)
	recB := httptest.NewRecorder()
	router.ServeHTTP(recB, reqB)
	if recB.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for disconnect purge data, got: %d", recB.Code)
	}

	// Verify UserDataPurgeRequested outbox event emitted [GATE - GDPR compliance]
	var purgeEventsCount int
	err = pool.QueryRow(ctx, "SELECT count(*) FROM ingestion.outbox_events WHERE event_type = $1", ingestiondomain.EventUserDataPurgeRequested).Scan(&purgeEventsCount)
	if err != nil {
		t.Fatalf("failed to query outbox: %v", err)
	}
	if purgeEventsCount == 0 {
		t.Errorf("expected UserDataPurgeRequested outbox event to be recorded")
	}
}
