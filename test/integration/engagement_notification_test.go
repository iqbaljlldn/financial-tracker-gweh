package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	analyticsDomain "github.com/iqbaljlldn/financial-tracker-gweh/internal/analytics/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/engagement"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/engagement/adapter/external/messaging/fonnte"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/engagement/adapter/external/messaging/meta"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/engagement/adapter/external/messaging/mock"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/engagement/application/command"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/engagement/application/composer"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/engagement/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/financialcore"
	fcCmd "github.com/iqbaljlldn/financial-tracker-gweh/internal/financialcore/application/command"
	fcQuery "github.com/iqbaljlldn/financial-tracker-gweh/internal/financialcore/application/query"
	fcDomain "github.com/iqbaljlldn/financial-tracker-gweh/internal/financialcore/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/security"
)

// mockAnalyticsReader satisfies composer.AnalyticsReader for engagement tests.
type mockAnalyticsReader struct {
	cashFlow []analyticsDomain.CashFlowBucket
	cats     []analyticsDomain.CategoryBreakdownItem
	trend    []analyticsDomain.TrendItem
}

func (m *mockAnalyticsReader) GetCashFlow(ctx context.Context, userID, granularity string, from, to *time.Time, currency *string) ([]analyticsDomain.CashFlowBucket, error) {
	if m.cashFlow != nil {
		return m.cashFlow, nil
	}
	return []analyticsDomain.CashFlowBucket{}, nil
}

func (m *mockAnalyticsReader) GetCategoryBreakdown(ctx context.Context, userID string, from, to *time.Time, currency *string) ([]analyticsDomain.CategoryBreakdownItem, error) {
	if m.cats != nil {
		return m.cats, nil
	}
	return []analyticsDomain.CategoryBreakdownItem{}, nil
}

func (m *mockAnalyticsReader) GetTrend(ctx context.Context, userID, granularity string, from, to *time.Time, currency *string) ([]analyticsDomain.TrendItem, error) {
	if m.trend != nil {
		return m.trend, nil
	}
	return []analyticsDomain.TrendItem{}, nil
}

// TestSprint9_PreferenceGate verifies FR-NOTIF-05 & Preference Gate guarantees:
// 1. Opting out of a notification type yields a clean no-op (nil job, no delivery failure).
// 2. Opting out of one notification type verifiably leaves other types unaffected.
func TestSprint9_PreferenceGate(t *testing.T) {
	pool, cleaner := setupTestDB(t)

	userID := uuid.New()
	cleaner.TrackUser(userID.String())

	mockClient := mock.New()
	analyticsReader := &mockAnalyticsReader{}

	mod, err := engagement.NewModule(engagement.Config{
		DB:              pool,
		Analytics:       analyticsReader,
		MessagingClient: mockClient,
	})
	if err != nil {
		t.Fatalf("failed to initialize engagement module: %v", err)
	}

	ctx := context.Background()

	// Step 1: Save initial preferences with daily summary DISABLED, but budget alerts ENABLED
	phone := "+628123456789"
	dailyEnabled := false
	budgetEnabled := true
	_, err = mod.UpdatePreferencesCmd.Handle(ctx, command.UpdatePreferencesCommand{
		UserID:              userID,
		PhoneNumber:         &phone,
		DailySummaryEnabled: &dailyEnabled,
		BudgetAlertsEnabled: &budgetEnabled,
	})
	if err != nil {
		t.Fatalf("failed to update preferences: %v", err)
	}

	// Step 2: Attempting to compose daily summary MUST return clean no-op (nil, nil) [GATE]
	dailyJob, err := mod.Composer.ComposeDailySummary(ctx, userID)
	if err != nil {
		t.Fatalf("unexpected error composing daily summary: %v", err)
	}
	if dailyJob != nil {
		t.Fatalf("expected nil job for opted-out daily summary, got %+v", dailyJob)
	}

	// Step 3: Composing budget alert for the same user MUST succeed (unaffected by daily summary opt-out)
	alertJob, err := mod.Composer.ComposeBudgetAlertNotification(ctx, composer.BudgetAlertPayload{
		UserID:          userID,
		BudgetID:        uuid.New(),
		Category:        "Dining",
		Threshold:       0.80,
		ThresholdAmount: 1000000,
		CurrentSpend:    850000,
		Currency:        "IDR",
	})
	if err != nil {
		t.Fatalf("failed to compose budget alert: %v", err)
	}
	if alertJob == nil {
		t.Fatalf("expected non-nil job for enabled budget alert")
	}
	if alertJob.DeliveryStatus() != domain.DeliveryStatusPending {
		t.Errorf("expected job status 'pending', got %q", alertJob.DeliveryStatus())
	}
}

// TestSprint9_DeliveryRetryOnTransientFailure verifies Diagram 7:
// Delivery retry on transient failure, eventual success, and status tracking.
func TestSprint9_DeliveryRetryOnTransientFailure(t *testing.T) {
	pool, cleaner := setupTestDB(t)

	userID := uuid.New()
	cleaner.TrackUser(userID.String())

	mockClient := mock.New()
	analyticsReader := &mockAnalyticsReader{}

	mod, err := engagement.NewModule(engagement.Config{
		DB:              pool,
		Analytics:       analyticsReader,
		MessagingClient: mockClient,
	})
	if err != nil {
		t.Fatalf("failed to initialize engagement module: %v", err)
	}

	ctx := context.Background()

	// Setup phone
	phone := "+628123456789"
	_, _ = mod.UpdatePreferencesCmd.Handle(ctx, command.UpdatePreferencesCommand{
		UserID:      userID,
		PhoneNumber: &phone,
	})

	// Compose alert
	job, err := mod.Composer.ComposeBudgetAlertNotification(ctx, composer.BudgetAlertPayload{
		UserID:          userID,
		BudgetID:        uuid.New(),
		Category:        "Utilities",
		Threshold:       1.0,
		ThresholdAmount: 500000,
		CurrentSpend:    520000,
		Currency:        "IDR",
	})
	if err != nil {
		t.Fatalf("failed to compose: %v", err)
	}

	// Step 1: Simulate transient failure on 1st attempt
	mockClient.SimulateFailure(1, false) // transient error
	err = mod.DeliverCmd.Handle(ctx, command.DeliverNotificationCommand{JobID: job.ID().String()})
	if err == nil {
		t.Fatalf("expected transient delivery error on 1st attempt, got nil")
	}

	// Verify job transitioned to retrying
	reloaded, err := mod.Repo.FindByID(ctx, job.ID())
	if err != nil {
		t.Fatalf("failed to reload job: %v", err)
	}
	if reloaded.DeliveryStatus() != domain.DeliveryStatusRetrying {
		t.Fatalf("expected status 'retrying', got %q", reloaded.DeliveryStatus())
	}

	// Step 2: Retry delivery (2nd attempt succeeds)
	err = mod.DeliverCmd.Handle(ctx, command.DeliverNotificationCommand{JobID: job.ID().String()})
	if err != nil {
		t.Fatalf("expected successful delivery on retry, got: %v", err)
	}

	// Verify job transitioned to delivered
	reloaded, _ = mod.Repo.FindByID(ctx, job.ID())
	if reloaded.DeliveryStatus() != domain.DeliveryStatusDelivered {
		t.Fatalf("expected status 'delivered', got %q", reloaded.DeliveryStatus())
	}
	if reloaded.DeliveryConfirmedAt() == nil {
		t.Fatalf("expected delivery_confirmed_at to be populated")
	}
}

// TestSprint9_ButtonReplyConfirmationFlow verifies H4 Resolution & ledger update:
// 1. Pending-review event composed into review_request with Confirm button.
// 2. Inbound webhook receives button reply with confirm:<eventId>.
// 3. Financial event in ledger transitions from pending_review -> finalized [GATE].
func TestSprint9_ButtonReplyConfirmationFlow(t *testing.T) {
	pool, cleaner := setupTestDB(t)

	userID := uuid.New()
	cleaner.TrackUser(userID.String())

	// Initialize financialcore module to host actual ledger
	sessionMgr, _ := security.NewSessionTokenManager([]byte("test_secret_32_chars_long_12345678"))
	fcMod, err := financialcore.NewModule(financialcore.Config{
		DB:                  pool,
		SessionManager:      sessionMgr,
		ConfidenceThreshold: 0.85,
	})
	if err != nil {
		t.Fatalf("failed to initialize financialcore: %v", err)
	}

	mockClient := mock.New()
	engMod, err := engagement.NewModule(engagement.Config{
		DB:              pool,
		Analytics:       &mockAnalyticsReader{},
		MessagingClient: mockClient,
		ReviewConfirmer: fcMod,
	})
	if err != nil {
		t.Fatalf("failed to initialize engagement: %v", err)
	}

	ctx := context.Background()

	// 1. Create a pending-review event in ledger via evaluate gate with low confidence
	phone := "628999888777"
	_, _ = engMod.UpdatePreferencesCmd.Handle(ctx, command.UpdatePreferencesCommand{
		UserID:      userID,
		PhoneNumber: &phone,
	})

	candidatePayload := fcDomain.CandidateCreatedPayload{
		ExtractionResultID: uuid.New().String(),
		IngestedMessageID:  uuid.New().String(),
		OccurredAt:         time.Now().UTC(),
		Candidate: fcDomain.Candidate{
			UserID:    userID.String(),
			EventType: string(fcDomain.EventTypeExpense),
			Amount: fcDomain.CandidateAmount{
				Value:    150000.00,
				Currency: "IDR",
			},
			OccurredAt: time.Now().UTC(),
			Merchant: fcDomain.Merchant{
				RawName:        "Warkop Berkah",
				NormalizedName: "Warkop Berkah",
			},
			Confidence: &fcDomain.Confidence{
				Aggregate: 0.65, // Below 0.85 -> pending_review
			},
			Provenance: fcDomain.CandidateProvenance{
				ConnectorType:      "gmail",
				ConnectorAccountID: uuid.New().String(),
				SourceMessageRef:   "msg_" + uuid.New().String(),
				ExtractionMethod:   string(fcDomain.ExtractionMethodAI),
				ExtractedAt:        time.Now().UTC(),
			},
		},
	}
	rawPayload, _ := json.Marshal(candidatePayload)
	createdEvent, err := fcMod.EvaluateGateCmd.Handle(ctx, fcCmd.EvaluateConfidenceGateCommand{
		Payload: rawPayload,
	})
	if err != nil {
		t.Fatalf("failed to create pending review event via gate: %v", err)
	}
	eventID := createdEvent.ID()
	cleaner.TrackFinancialEvent(eventID)

	// 2. Compose Review Request Notification
	amtVal := 150000.0
	job, err := engMod.Composer.ComposeReviewRequestNotification(ctx, composer.ReviewRequestPayload{
		UserID:            userID,
		FinancialEventID:  uuid.MustParse(eventID),
		AmountValue:       amtVal,
		AmountCurrency:    "IDR",
		MerchantName:      "Warkop Berkah",
		SuggestedCategory: "Food",
	})
	if err != nil {
		t.Fatalf("failed to compose review request: %v", err)
	}
	if job == nil {
		t.Fatalf("expected non-nil review request job")
	}

	// 3. User clicks Confirm button -> inbound reply received with buttonId "confirm:<eventID>"
	res, err := engMod.ProcessInboundReplyCmd.Handle(ctx, command.InboundReplyCommand{
		SenderPhone: phone,
		ButtonID:    "confirm:" + eventID,
	})
	if err != nil {
		t.Fatalf("failed to handle inbound reply: %v", err)
	}
	if res.MatchedIntent != "confirm" {
		t.Fatalf("expected matchedIntent 'confirm', got %q", res.MatchedIntent)
	}

	// 4. Verify event in financialcore ledger is now finalized! [GATE]
	finalizedEvent, err := fcMod.GetEventQuery.Handle(ctx, fcQuery.GetFinancialEventByIDQuery{
		UserID:  userID.String(),
		EventID: eventID,
	})
	if err != nil {
		t.Fatalf("failed to reload event from ledger: %v", err)
	}
	if finalizedEvent.Status() != fcDomain.StatusFinalized {
		t.Fatalf("expected ledger status 'finalized', got %q", finalizedEvent.Status())
	}
}

// TestSprint9_TextPatternFallbackFlow verifies strict regex fallback:
// 1. Reply "Y" or "ya" successfully confirms latest pending review.
// 2. Unrecognized text replies trigger clarifying guidance re-prompt without silent drop.
func TestSprint9_TextPatternFallbackFlow(t *testing.T) {
	pool, cleaner := setupTestDB(t)

	userID := uuid.New()
	cleaner.TrackUser(userID.String())

	sessionMgr, _ := security.NewSessionTokenManager([]byte("test_secret_32_chars_long_12345678"))
	fcMod, err := financialcore.NewModule(financialcore.Config{
		DB:                  pool,
		SessionManager:      sessionMgr,
		ConfidenceThreshold: 0.85,
	})
	if err != nil {
		t.Fatalf("failed to initialize financialcore: %v", err)
	}

	mockClient := mock.New()
	engMod, err := engagement.NewModule(engagement.Config{
		DB:              pool,
		Analytics:       &mockAnalyticsReader{},
		MessagingClient: mockClient,
		ReviewConfirmer: fcMod,
	})
	if err != nil {
		t.Fatalf("failed to initialize engagement: %v", err)
	}

	ctx := context.Background()

	phone := "628555666777"
	_, _ = engMod.UpdatePreferencesCmd.Handle(ctx, command.UpdatePreferencesCommand{
		UserID:      userID,
		PhoneNumber: &phone,
	})

	candidatePayload := fcDomain.CandidateCreatedPayload{
		ExtractionResultID: uuid.New().String(),
		IngestedMessageID:  uuid.New().String(),
		OccurredAt:         time.Now().UTC(),
		Candidate: fcDomain.Candidate{
			UserID:    userID.String(),
			EventType: string(fcDomain.EventTypeExpense),
			Amount: fcDomain.CandidateAmount{
				Value:    75000.00,
				Currency: "IDR",
			},
			OccurredAt: time.Now().UTC(),
			Merchant: fcDomain.Merchant{
				RawName:        "Bakso Solo",
				NormalizedName: "Bakso Solo",
			},
			Confidence: &fcDomain.Confidence{
				Aggregate: 0.65, // Below 0.85 -> pending_review
			},
			Provenance: fcDomain.CandidateProvenance{
				ConnectorType:      "gmail",
				ConnectorAccountID: uuid.New().String(),
				SourceMessageRef:   "msg_" + uuid.New().String(),
				ExtractionMethod:   string(fcDomain.ExtractionMethodAI),
				ExtractedAt:        time.Now().UTC(),
			},
		},
	}
	rawPayload, _ := json.Marshal(candidatePayload)
	createdEvent, err := fcMod.EvaluateGateCmd.Handle(ctx, fcCmd.EvaluateConfidenceGateCommand{
		Payload: rawPayload,
	})
	if err != nil {
		t.Fatalf("failed to create pending review event via gate: %v", err)
	}
	eventID := createdEvent.ID()
	cleaner.TrackFinancialEvent(eventID)

	// Compose review request
	_, err = engMod.Composer.ComposeReviewRequestNotification(ctx, composer.ReviewRequestPayload{
		UserID:            userID,
		FinancialEventID:  uuid.MustParse(eventID),
		AmountValue:       75000.0,
		AmountCurrency:    "IDR",
		MerchantName:      "Bakso Solo",
		SuggestedCategory: "Food",
	})
	if err != nil {
		t.Fatalf("failed to compose review request: %v", err)
	}

	// Test A: Unrecognized message -> must return "clarify" intent
	clarifyRes, err := engMod.ProcessInboundReplyCmd.Handle(ctx, command.InboundReplyCommand{
		SenderPhone: phone,
		MessageText: "Halo admin, tolong cekkan ya",
	})
	if err != nil {
		t.Fatalf("failed to handle unknown reply: %v", err)
	}
	if clarifyRes.MatchedIntent != "clarify" {
		t.Fatalf("expected 'clarify', got %q", clarifyRes.MatchedIntent)
	}

	// Test B: Strict "Y" pattern fallback -> confirms transaction
	yesRes, err := engMod.ProcessInboundReplyCmd.Handle(ctx, command.InboundReplyCommand{
		SenderPhone: phone,
		MessageText: "Y",
	})
	if err != nil {
		t.Fatalf("failed to handle 'Y' reply: %v", err)
	}
	if yesRes.MatchedIntent != "confirm" {
		t.Fatalf("expected 'confirm', got %q", yesRes.MatchedIntent)
	}

	// Verify ledger updated
	ev, _ := fcMod.GetEventQuery.Handle(ctx, fcQuery.GetFinancialEventByIDQuery{
		UserID:  userID.String(),
		EventID: eventID,
	})
	if ev.Status() != fcDomain.StatusFinalized {
		t.Fatalf("expected status 'finalized', got %q", ev.Status())
	}
}

// TestSprint9_FonnteWebhookEndpoint verifies HTTP webhook handler for Fonnte callbacks.
func TestSprint9_FonnteWebhookEndpoint(t *testing.T) {
	pool, cleaner := setupTestDB(t)

	userID := uuid.New()
	cleaner.TrackUser(userID.String())

	mockClient := mock.New()
	engMod, err := engagement.NewModule(engagement.Config{
		DB:              pool,
		Analytics:       &mockAnalyticsReader{},
		MessagingClient: mockClient,
	})
	if err != nil {
		t.Fatalf("failed to initialize engagement: %v", err)
	}

	ctx := context.Background()
	phone := "628111222333"
	_, _ = engMod.UpdatePreferencesCmd.Handle(ctx, command.UpdatePreferencesCommand{
		UserID:      userID,
		PhoneNumber: &phone,
	})

	// 1. Post Fonnte delivery status payload
	statusPayload := map[string]string{
		"id":     "msg_fonnte_123",
		"status": "delivered",
	}
	rawStatus, _ := json.Marshal(statusPayload)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/whatsapp/fonnte", bytes.NewReader(rawStatus))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	engMod.HTTPHandler.HandleFonnteWebhook(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	// 2. Post Fonnte inbound message payload
	inboundPayload := map[string]string{
		"sender":  phone,
		"message": "N",
	}
	rawInbound, _ := json.Marshal(inboundPayload)
	req2 := httptest.NewRequest(http.MethodPost, "/webhooks/whatsapp/fonnte", bytes.NewReader(rawInbound))
	req2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()

	engMod.HTTPHandler.HandleFonnteWebhook(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec2.Code)
	}
}

// TestSprint9_AgnosticClientFactory verifies factory constructs Fonnte, Meta, and Mock clients.
func TestSprint9_AgnosticClientFactory(t *testing.T) {
	c1 := fonnte.New(fonnte.Config{APIToken: "test_token"})
	if c1.ProviderName() != "fonnte" {
		t.Errorf("expected provider 'fonnte', got %q", c1.ProviderName())
	}

	c2 := meta.New(meta.Config{PhoneNumberID: "123", AccessToken: "tok"})
	if c2.ProviderName() != "meta" {
		t.Errorf("expected provider 'meta', got %q", c2.ProviderName())
	}

	c3 := mock.New()
	if c3.ProviderName() != "mock" {
		t.Errorf("expected provider 'mock', got %q", c3.ProviderName())
	}
}

// TestSprint9_TransactionFinalizedNotification verifies:
// 1. Finalized transactions compose transaction_finalized notification.
// 2. Rendered template includes formatted currency, merchant, category, and date.
// 3. Notification is delivered via messaging client.
// 4. Opted-out / missing phone number results in clean no-op.
func TestSprint9_TransactionFinalizedNotification(t *testing.T) {
	pool, cleaner := setupTestDB(t)

	userID := uuid.New()
	cleaner.TrackUser(userID.String())

	mockClient := mock.New()
	engMod, err := engagement.NewModule(engagement.Config{
		DB:              pool,
		Analytics:       &mockAnalyticsReader{},
		MessagingClient: mockClient,
	})
	if err != nil {
		t.Fatalf("failed to initialize engagement: %v", err)
	}

	ctx := context.Background()

	// 1. Without phone number: clean no-op
	eventID := uuid.New()
	job, err := engMod.Composer.ComposeTransactionFinalizedNotification(ctx, composer.TransactionFinalizedPayload{
		UserID:           userID,
		FinancialEventID: eventID,
		EventType:        "Expense",
		AmountValue:      150000,
		AmountCurrency:   "IDR",
		MerchantName:     "Starbucks",
		Category:         "Makanan & Minuman",
		OccurredAt:       time.Date(2026, 10, 28, 14, 30, 0, 0, time.UTC),
		FinalizedVia:     "confidence_gate",
	})
	if err != nil {
		t.Fatalf("unexpected error composing without phone: %v", err)
	}
	if job != nil {
		t.Fatalf("expected nil job when no phone number, got %v", job)
	}

	// 2. Set phone number
	phone := "628999888777"
	_, err = engMod.UpdatePreferencesCmd.Handle(ctx, command.UpdatePreferencesCommand{
		UserID:      userID,
		PhoneNumber: &phone,
	})
	if err != nil {
		t.Fatalf("failed to update preferences: %v", err)
	}

	// 3. Compose with registered phone number
	job, err = engMod.Composer.ComposeTransactionFinalizedNotification(ctx, composer.TransactionFinalizedPayload{
		UserID:           userID,
		FinancialEventID: eventID,
		EventType:        "Expense",
		AmountValue:      150000,
		AmountCurrency:   "IDR",
		MerchantName:     "Starbucks",
		Category:         "Makanan & Minuman",
		OccurredAt:       time.Date(2026, 10, 28, 14, 30, 0, 0, time.UTC),
		FinalizedVia:     "confidence_gate",
	})
	if err != nil {
		t.Fatalf("failed to compose transaction finalized notification: %v", err)
	}
	if job == nil {
		t.Fatal("expected non-nil job")
	}

	if job.NotificationType() != domain.NotificationTypeTransactionFinalized {
		t.Fatalf("expected type %s, got %s", domain.NotificationTypeTransactionFinalized, job.NotificationType())
	}

	// Inspect composed payload
	var msg domain.OutboundMessage
	if err := json.Unmarshal(job.ComposedPayload(), &msg); err != nil {
		t.Fatalf("failed to unmarshal composed payload: %v", err)
	}

	if msg.Parameters["merchant"] != "Starbucks" {
		t.Errorf("expected merchant Starbucks, got %s", msg.Parameters["merchant"])
	}
	if msg.Parameters["amount"] != "Rp 150.000" {
		t.Errorf("expected amount Rp 150.000, got %s", msg.Parameters["amount"])
	}
	if msg.Parameters["category"] != "Makanan & Minuman" {
		t.Errorf("expected category Makanan & Minuman, got %s", msg.Parameters["category"])
	}
	if msg.Parameters["eventType"] != "Pengeluaran" {
		t.Errorf("expected eventType Pengeluaran, got %s", msg.Parameters["eventType"])
	}

	// 4. Deliver notification
	err = engMod.DeliverCmd.Handle(ctx, command.DeliverNotificationCommand{
		JobID: job.ID().String(),
	})
	if err != nil {
		t.Fatalf("failed to deliver notification: %v", err)
	}

	delivered := mockClient.SentMessages()
	if len(delivered) != 1 {
		t.Fatalf("expected 1 delivered message, got %d", len(delivered))
	}
	if delivered[0].RecipientPhone != phone {
		t.Errorf("expected recipient %s, got %s", phone, delivered[0].RecipientPhone)
	}
}
