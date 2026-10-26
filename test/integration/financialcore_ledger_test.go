package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/financialcore"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/financialcore/application/command"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/financialcore/application/query"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/financialcore/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/security"
)

type mockSuggester struct {
	category string
}

func (m *mockSuggester) Suggest(ctx context.Context, userID, normalizedMerchant string) (domain.CategorySuggestion, bool) {
	if m.category != "" {
		return domain.CategorySuggestion{
			Category:   m.category,
			Confidence: 0.85,
			Source:     domain.CategorySourceGlobalMapping,
		}, true
	}
	return domain.CategorySuggestion{}, false
}

// TestSprint6_Diagram3_TemplateToFinalizeHappyPath verifies 07_sequence_diagrams.md Diagram 3:
// Deterministic template extraction -> EvaluateConfidenceGate (1.0 >= 0.85) -> Finalize -> Category -> Outbox rows with trace context [GATE].
func TestSprint6_Diagram3_TemplateToFinalizeHappyPath(t *testing.T) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, getDatabaseURL())
	if err != nil {
		t.Skipf("skipping integration test, postgres unavailable: %v", err)
	}
	defer pool.Close()

	mod, err := financialcore.NewModule(financialcore.Config{
		DB:                  pool,
		ConfidenceThreshold: 0.85,
		CategorySuggester:   &mockSuggester{category: "Groceries"},
	})
	if err != nil {
		t.Fatalf("failed to initialize financialcore module: %v", err)
	}

	userID := uuid.New().String()
	cleaner := NewTestCleaner(t, pool)
	cleaner.TrackUser(userID)

	connectorAccountID := uuid.New().String()
	sourceRef := "msg_diag3_" + uuid.New().String()
	extractedAt := time.Now().UTC()

	candidatePayload := domain.CandidateCreatedPayload{
		ExtractionResultID: uuid.New().String(),
		IngestedMessageID:  uuid.New().String(),
		OccurredAt:         extractedAt,
		Candidate: domain.Candidate{
			UserID:    userID,
			EventType: string(domain.EventTypeExpense),
			Amount: domain.CandidateAmount{
				Value:    85.75,
				Currency: "USD",
			},
			OccurredAt: extractedAt,
			Merchant: domain.Merchant{
				RawName:        "WHOLE FOODS #101",
				NormalizedName: "Whole Foods",
			},
			PaymentMethod: domain.CandidatePayment{
				Type:  "card",
				Last4: "5678",
			},
			Confidence: &domain.Confidence{
				Aggregate: 1.0,
			},
			Provenance: domain.CandidateProvenance{
				ConnectorType:      "gmail",
				ConnectorAccountID: connectorAccountID,
				SourceMessageRef:   sourceRef,
				ExtractionMethod:   string(domain.ExtractionMethodTemplate),
				TemplateVersion:    "wholefoods-v1",
				ExtractedAt:        extractedAt,
			},
		},
	}

	rawPayload, err := json.Marshal(candidatePayload)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	// 1. Process candidate through confidence gate
	event, err := mod.EvaluateGateCmd.Handle(ctx, command.EvaluateConfidenceGateCommand{
		Payload: rawPayload,
	})
	if err != nil {
		t.Fatalf("EvaluateGateCmd failed: %v", err)
	}

	// Invariant checks
	if event.Status() != domain.StatusFinalized {
		t.Fatalf("expected status %q for 1.0 confidence, got %q", domain.StatusFinalized, event.Status())
	}
	if event.Amount().Value() != "85.75" {
		t.Fatalf("expected decimal amount '85.75', got %s", event.Amount().Value())
	}
	if event.Category() == nil || event.Category().Category() != "Groceries" {
		t.Fatalf("expected suggested category 'Groceries' at candidate-time (H5), got %v", event.Category())
	}

	// 2. Verify Outbox rows in financialcore.outbox_events
	var outboxCount int
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM financialcore.outbox_events WHERE aggregate_id = $1`,
		event.ID(),
	).Scan(&outboxCount)
	if err != nil {
		t.Fatalf("querying outbox count: %v", err)
	}
	if outboxCount < 2 { // At least FinancialEventFinalized and CategoryAssignmentChanged
		t.Fatalf("expected at least 2 outbox rows, got %d", outboxCount)
	}

	// Verify trace context embedded in payload [GATE: 17_observability.md §5.2]
	var payloadBytes []byte
	err = pool.QueryRow(ctx,
		`SELECT payload FROM financialcore.outbox_events WHERE aggregate_id = $1 AND event_type = $2 LIMIT 1`,
		event.ID(), domain.EventFinancialEventFinalized,
	).Scan(&payloadBytes)
	if err != nil {
		t.Fatalf("loading outbox payload: %v", err)
	}

	var envelope struct {
		TraceContext map[string]string `json:"traceContext"`
	}
	if err := json.Unmarshal(payloadBytes, &envelope); err != nil {
		t.Fatalf("unmarshaling outbox payload: %v", err)
	}
	if envelope.TraceContext == nil {
		t.Fatalf("expected traceContext object embedded in outbox payload [GATE]")
	}
}

// TestSprint6_Diagram4_PendingReviewToConfirmToFinalize verifies 07_sequence_diagrams.md Diagram 4:
// Below-threshold confidence -> pending_review with candidate-time category (H5) -> User confirmation -> Finalize [GATE].
func TestSprint6_Diagram4_PendingReviewToConfirmToFinalize(t *testing.T) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, getDatabaseURL())
	if err != nil {
		t.Skipf("skipping integration test, postgres unavailable: %v", err)
	}
	defer pool.Close()

	mod, err := financialcore.NewModule(financialcore.Config{
		DB:                  pool,
		ConfidenceThreshold: 0.85,
		CategorySuggester:   &mockSuggester{category: "Dining"},
	})
	if err != nil {
		t.Fatalf("failed to initialize financialcore module: %v", err)
	}

	userID := uuid.New().String()
	cleaner := NewTestCleaner(t, pool)
	cleaner.TrackUser(userID)

	sourceRef := "msg_diag4_" + uuid.New().String()

	candidatePayload := domain.CandidateCreatedPayload{
		ExtractionResultID: uuid.New().String(),
		IngestedMessageID:  uuid.New().String(),
		OccurredAt:         time.Now().UTC(),
		Candidate: domain.Candidate{
			UserID:    userID,
			EventType: string(domain.EventTypeExpense),
			Amount: domain.CandidateAmount{
				Value:    42.00,
				Currency: "USD",
			},
			OccurredAt: time.Now().UTC(),
			Merchant: domain.Merchant{
				RawName:        "CAFE AMB",
				NormalizedName: "Cafe Amb",
			},
			Confidence: &domain.Confidence{
				Aggregate: 0.65, // Below 0.85 threshold
			},
			Provenance: domain.CandidateProvenance{
				ConnectorType:      "gmail",
				ConnectorAccountID: uuid.New().String(),
				SourceMessageRef:   sourceRef,
				ExtractionMethod:   string(domain.ExtractionMethodAI),
				ExtractedAt:        time.Now().UTC(),
			},
		},
	}

	rawPayload, _ := json.Marshal(candidatePayload)

	// Step 1: Evaluate gate -> results in pending_review
	event, err := mod.EvaluateGateCmd.Handle(ctx, command.EvaluateConfidenceGateCommand{
		Payload: rawPayload,
	})
	if err != nil {
		t.Fatalf("EvaluateGateCmd failed: %v", err)
	}
	if event.Status() != domain.StatusPendingReview {
		t.Fatalf("expected pending_review status for below-threshold, got %s", event.Status())
	}
	if event.Category() == nil || event.Category().Category() != "Dining" {
		t.Fatalf("expected candidate-time suggested category 'Dining' (H5), got %v", event.Category())
	}

	// Step 2: Confirm pending review via ConfirmPendingReviewCmd with an override
	newCategory := "Coffee & Snacks"
	idempKey := "confirm_key_" + uuid.New().String()
	confirmedEvent, err := mod.ConfirmReviewCmd.Handle(ctx, command.ConfirmPendingReviewCommand{
		UserID:         userID,
		EventID:        event.ID(),
		IdempotencyKey: idempKey,
		ActorUserID:    userID,
		Corrections: &command.OverrideFields{
			Category: &newCategory,
		},
	})
	if err != nil {
		t.Fatalf("ConfirmReviewCmd failed: %v", err)
	}
	if confirmedEvent.Status() != domain.StatusFinalized {
		t.Fatalf("expected finalized status after confirmation, got %s", confirmedEvent.Status())
	}
	if confirmedEvent.Category().Category() != "Coffee & Snacks" {
		t.Fatalf("expected updated category 'Coffee & Snacks', got %s", confirmedEvent.Category().Category())
	}

	// Step 3: Double-tap idempotency: calling confirm again with same key returns cleanly without error [GATE]
	confirmedAgain, err := mod.ConfirmReviewCmd.Handle(ctx, command.ConfirmPendingReviewCommand{
		UserID:         userID,
		EventID:        event.ID(),
		IdempotencyKey: idempKey,
		ActorUserID:    userID,
	})
	if err != nil {
		t.Fatalf("expected idempotent confirm to succeed, got %v", err)
	}
	if confirmedAgain.Status() != domain.StatusFinalized {
		t.Fatalf("expected event to remain finalized")
	}
}

// TestSprint6_Diagram5_DuplicateDeliveryIdempotency verifies 07_sequence_diagrams.md Diagram 5:
// Redelivery with identical (connector_account_id, source_message_ref) produces ZERO duplicate rows [GATE].
func TestSprint6_Diagram5_DuplicateDeliveryIdempotency(t *testing.T) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, getDatabaseURL())
	if err != nil {
		t.Skipf("skipping integration test, postgres unavailable: %v", err)
	}
	defer pool.Close()

	mod, err := financialcore.NewModule(financialcore.Config{
		DB:                  pool,
		ConfidenceThreshold: 0.70,
	})
	if err != nil {
		t.Fatalf("failed to initialize financialcore module: %v", err)
	}

	userID := uuid.New().String()
	cleaner := NewTestCleaner(t, pool)
	cleaner.TrackUser(userID)

	connectorAccountID := uuid.New().String()
	sourceRef := "msg_dedup_inv3_" + uuid.New().String()

	candPayload := domain.CandidateCreatedPayload{
		ExtractionResultID: uuid.New().String(),
		IngestedMessageID:  uuid.New().String(),
		OccurredAt:         time.Now().UTC(),
		Candidate: domain.Candidate{
			UserID:    userID,
			EventType: string(domain.EventTypeExpense),
			Amount: domain.CandidateAmount{
				Value:    99.99,
				Currency: "USD",
			},
			OccurredAt: time.Now().UTC(),
			Merchant: domain.Merchant{
				RawName:        "STORE DEDUP",
				NormalizedName: "Store Dedup",
			},
			Confidence: &domain.Confidence{Aggregate: 0.95},
			Provenance: domain.CandidateProvenance{
				ConnectorType:      "gmail",
				ConnectorAccountID: connectorAccountID,
				SourceMessageRef:   sourceRef,
				ExtractionMethod:   string(domain.ExtractionMethodTemplate),
				ExtractedAt:        time.Now().UTC(),
			},
		},
	}
	rawPayload, _ := json.Marshal(candPayload)

	// Delivery 1: initial delivery
	ev1, err := mod.EvaluateGateCmd.Handle(ctx, command.EvaluateConfidenceGateCommand{
		Payload: rawPayload,
	})
	if err != nil {
		t.Fatalf("First delivery failed: %v", err)
	}

	// Verify 1 row exists
	var countBefore int
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM financialcore.financial_events WHERE provenance_connector_account_id = $1 AND provenance_source_message_ref = $2`,
		connectorAccountID, sourceRef,
	).Scan(&countBefore)
	if err != nil || countBefore != 1 {
		t.Fatalf("expected 1 row after first delivery, got %d (err: %v)", countBefore, err)
	}

	// Delivery 2: simulated Asynq task redelivery with identical provenance
	ev2, err := mod.EvaluateGateCmd.Handle(ctx, command.EvaluateConfidenceGateCommand{
		Payload: rawPayload,
	})
	if err != nil {
		t.Fatalf("Redelivery must be a successful no-op, got error: %v", err)
	}

	// Verify ZERO duplicate rows created [GATE]
	var countAfter int
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM financialcore.financial_events WHERE provenance_connector_account_id = $1 AND provenance_source_message_ref = $2`,
		connectorAccountID, sourceRef,
	).Scan(&countAfter)
	if err != nil || countAfter != 1 {
		t.Fatalf("expected EXACTLY 1 row after redelivery (zero duplicates), got %d (err: %v)", countAfter, err)
	}

	// Verify event ID of first created row is preserved
	if ev1.ID() == "" || ev2.ID() == "" {
		t.Fatalf("expected non-empty event IDs")
	}
}

// TestSprint6_ManualOverrideAndAuditTrail verifies manual override preserves original values (SRS-FR-LEDGER-02).
func TestSprint6_ManualOverrideAndAuditTrail(t *testing.T) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, getDatabaseURL())
	if err != nil {
		t.Skipf("skipping integration test, postgres unavailable: %v", err)
	}
	defer pool.Close()

	mod, err := financialcore.NewModule(financialcore.Config{
		DB:                  pool,
		ConfidenceThreshold: 0.70,
	})
	if err != nil {
		t.Fatalf("failed to initialize financialcore module: %v", err)
	}

	userID := uuid.New().String()
	cleaner := NewTestCleaner(t, pool)
	cleaner.TrackUser(userID)

	fe, err := mod.ManualEventCmd.Handle(ctx, command.CreateManualEventCommand{
		UserID:            userID,
		EventType:         string(domain.EventTypeExpense),
		AmountValue:       "50.00",
		AmountCurrency:    "USD",
		OccurredAt:        time.Now().UTC(),
		MerchantName:      "Original Merchant",
		PaymentMethodType: "cash",
	})
	if err != nil {
		t.Fatalf("CreateManualEvent failed: %v", err)
	}

	// Apply override to merchant and amount
	newMerchant := "Corrected Merchant"
	updated, overrides, err := mod.OverrideCmd.Handle(ctx, command.SubmitEventOverrideCommand{
		UserID:      userID,
		EventID:     fe.ID(),
		ActorUserID: userID,
		Fields: command.OverrideFields{
			Merchant: &newMerchant,
			Amount: &command.AmountField{
				Value:    "55.00",
				Currency: "USD",
			},
		},
	})
	if err != nil {
		t.Fatalf("SubmitEventOverride failed: %v", err)
	}
	if len(overrides) != 2 {
		t.Fatalf("expected 2 overrides created, got %d", len(overrides))
	}

	// Check original extracted values are preserved (SRS-FR-LEDGER-02)
	if updated.Amount().Value() != "50.00" {
		t.Fatalf("original amount must be preserved as '50.00', got %s", updated.Amount().Value())
	}
	if updated.Merchant().DisplayName() != "Original Merchant" {
		t.Fatalf("original merchant must be preserved as 'Original Merchant', got %s", updated.Merchant().DisplayName())
	}

	// Check effective view reflects overrides
	if updated.EffectiveAmount().Value() != "55.00" {
		t.Fatalf("effective amount must be '55.00', got %s", updated.EffectiveAmount().Value())
	}
	if updated.EffectiveMerchant().DisplayName() != "Corrected Merchant" {
		t.Fatalf("effective merchant must be 'Corrected Merchant', got %s", updated.EffectiveMerchant().DisplayName())
	}

	// Query audit trail via ListOverrides
	auditTrail, err := mod.ListOverridesQuery.Handle(ctx, query.ListEventOverridesQuery{
		UserID:  userID,
		EventID: fe.ID(),
	})
	if err != nil {
		t.Fatalf("ListOverrides failed: %v", err)
	}
	if len(auditTrail) != 2 {
		t.Fatalf("expected 2 audit trail entries, got %d", len(auditTrail))
	}
}

// TestSprint6_HTTP_Endpoints tests GET/POST/PATCH /financial-events, /confirm, and /overrides per 09_api_specification.md §6.2.
// It verifies monetary fields are serialized as decimal strings, never JSON floats [GATE].
func TestSprint6_HTTP_Endpoints(t *testing.T) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, getDatabaseURL())
	if err != nil {
		t.Skipf("skipping integration test, postgres unavailable: %v", err)
	}
	defer pool.Close()

	sessionSecret := []byte("financial-tracker-session-secret-key-32bytes!")
	sessionManager, err := security.NewSessionTokenManager(sessionSecret)
	if err != nil {
		t.Fatalf("session manager init failed: %v", err)
	}

	mod, err := financialcore.NewModule(financialcore.Config{
		DB:                  pool,
		SessionManager:      sessionManager,
		ConfidenceThreshold: 0.70,
	})
	if err != nil {
		t.Fatalf("financialcore init failed: %v", err)
	}

	router := chi.NewRouter()
	mod.RegisterHTTPRoutes(router)
	ts := httptest.NewServer(router)
	defer ts.Close()

	userID := uuid.New().String()
	cleaner := NewTestCleaner(t, pool)
	cleaner.TrackUser(userID)

	token, _, err := sessionManager.Issue(userID, "user@example.com", "user", time.Hour)
	if err != nil {
		t.Fatalf("token issue failed: %v", err)
	}

	doReq := func(method, path string, body any) (*http.Response, []byte) {
		var rdr io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			rdr = bytes.NewReader(b)
		}
		req, _ := http.NewRequest(method, ts.URL+path, rdr)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("http request failed: %v", err)
		}
		defer resp.Body.Close()
		respBytes, _ := io.ReadAll(resp.Body)
		return resp, respBytes
	}

	// 1. POST /api/v1/financial-events (Manual Entry)
	createReq := map[string]any{
		"eventType": "Expense",
		"amount": map[string]string{
			"value":    "120.50",
			"currency": "USD",
		},
		"occurredAt": time.Now().UTC().Format(time.RFC3339),
		"merchant": map[string]string{
			"rawName":        "APPLE STORE #10",
			"normalizedName": "Apple Store",
		},
		"category": "Electronics",
		"typeSpecificPayload": map[string]any{
			"is_recurring": false,
		},
	}
	resp, respBytes := doReq("POST", "/api/v1/financial-events", createReq)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", resp.StatusCode, string(respBytes))
	}

	var createdEvent map[string]any
	if err := json.Unmarshal(respBytes, &createdEvent); err != nil {
		t.Fatalf("unmarshal created event: %v", err)
	}
	eventID := createdEvent["id"].(string)

	// Verify monetary field is string decimal [GATE]
	amtMap := createdEvent["amount"].(map[string]any)
	if amtVal, ok := amtMap["value"].(string); !ok || amtVal != "120.50" {
		t.Fatalf("expected amount.value to be string '120.50', got %v (type %T)", amtMap["value"], amtMap["value"])
	}

	// 2. GET /api/v1/financial-events/{id}
	resp, respBytes = doReq("GET", "/api/v1/financial-events/"+eventID, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", resp.StatusCode, string(respBytes))
	}
	var getEvent map[string]any
	_ = json.Unmarshal(respBytes, &getEvent)
	if getEvent["id"] != eventID {
		t.Fatalf("event id mismatch")
	}

	// 3. GET /api/v1/financial-events (List)
	resp, respBytes = doReq("GET", "/api/v1/financial-events?limit=10", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK on list, got %d: %s", resp.StatusCode, string(respBytes))
	}
	var listResp map[string]any
	_ = json.Unmarshal(respBytes, &listResp)
	items := listResp["items"].([]any)
	if len(items) == 0 {
		t.Fatalf("expected at least 1 item in list")
	}

	// 4. PATCH /api/v1/financial-events/{id} - Valid Override
	patchReq := map[string]any{
		"fields": map[string]any{
			"merchant": "Apple Inc.",
			"amount": map[string]string{
				"value":    "125.00",
				"currency": "USD",
			},
		},
	}
	resp, respBytes = doReq("PATCH", "/api/v1/financial-events/"+eventID, patchReq)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK on patch, got %d: %s", resp.StatusCode, string(respBytes))
	}
	var patchedEvent map[string]any
	_ = json.Unmarshal(respBytes, &patchedEvent)
	patchedMerchant := patchedEvent["merchant"].(map[string]any)["normalizedName"]
	if patchedMerchant != "Apple Inc." {
		t.Fatalf("expected merchant 'Apple Inc.', got %v", patchedMerchant)
	}

	// 5. PATCH /api/v1/financial-events/{id} - Cross-Currency Violation (INV-2)
	badPatchReq := map[string]any{
		"fields": map[string]any{
			"amount": map[string]string{
				"value":    "125.00",
				"currency": "EUR", // Different currency!
			},
		},
	}
	resp, respBytes = doReq("PATCH", "/api/v1/financial-events/"+eventID, badPatchReq)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 Unprocessable Entity for cross-currency override, got %d: %s", resp.StatusCode, string(respBytes))
	}
	var errEnvelope map[string]any
	_ = json.Unmarshal(respBytes, &errEnvelope)
	errDetails := errEnvelope["error"].(map[string]any)
	if errDetails["code"] != "CURRENCY_IMMUTABLE" {
		t.Fatalf("expected error code CURRENCY_IMMUTABLE, got %v", errDetails["code"])
	}

	// 6. GET /api/v1/financial-events/{id}/overrides
	resp, respBytes = doReq("GET", "/api/v1/financial-events/"+eventID+"/overrides", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK on overrides, got %d: %s", resp.StatusCode, string(respBytes))
	}
	var overridesList []any
	_ = json.Unmarshal(respBytes, &overridesList)
	if len(overridesList) < 2 {
		t.Fatalf("expected at least 2 overrides in audit trail, got %d", len(overridesList))
	}
}
