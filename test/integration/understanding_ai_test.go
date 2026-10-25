package integration_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding"
	understandingCmd "github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/application/command"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/application/validator"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

type mockIntegrationLLM struct {
	responseJSON string
}

func (m *mockIntegrationLLM) Extract(_ context.Context, _ domain.ExtractionPromptRequest) (*domain.StructuredLLMResponse, error) {
	return &domain.StructuredLLMResponse{
		RawJSON:         m.responseJSON,
		ModelIdentifier: "gpt-4o-mini",
		LatencyMs:       180,
		TokenUsage: domain.LLMTokenUsage{
			PromptTokens:     250,
			CompletionTokens: 80,
			TotalTokens:      330,
		},
	}, nil
}

// TestSprint5_Diagram4_UnknownSenderAIExtraction_LowConfidenceReviewPath verifies Diagram 4:
// 1. Unknown-sender message triggers AI extractor fallback
// 2. Structured JSON response is validated, weighted confidence is computed (e.g. 0.58 < 0.70 threshold)
// 3. Extraction result stored in understanding.extraction_results (method=ai, prompt_version=v1)
// 4. Record written to administration.prompt_metrics table
// 5. FinancialEventCandidateCreated emitted to outbox for Financial Core confidence gating [GATE].
func TestSprint5_Diagram4_UnknownSenderAIExtraction_LowConfidenceReviewPath(t *testing.T) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, getDatabaseURL())
	if err != nil {
		t.Skipf("skipping integration test, postgres unavailable: %v", err)
	}
	defer pool.Close()

	// Simulated LLM response for Diagram 4's Amazon purchase with ambiguous confidence (agg ~0.58)
	// Amount: 45.00 (conf: 0.70)
	// OccurredAt: 2026-08-03 (conf: 0.60)
	// Merchant: AMZN Mktp (conf: 0.60)
	// EventType: Expense (conf: 0.70)
	// PaymentMethod: card 4821 (conf: 0.50)
	// CategoryHint: Shopping (conf: 0.60)
	// Weighted: (0.70*0.35)+(0.60*0.20)+(0.70*0.20)+(0.60*0.15)+(0.50*0.05)+(0.60*0.05) = 0.245+0.12+0.14+0.09+0.025+0.03 = 0.65
	simulatedLLMJSON := `{
  "eventType": "Expense",
  "amount": { "value": "45.00", "currency": "USD", "confidence": 0.60 },
  "occurredAt": { "value": "2026-08-03T14:22:00Z", "confidence": 0.50 },
  "merchant": { "rawName": "AMZN Mktp US", "normalizedName": "Amazon", "confidence": 0.60 },
  "paymentMethod": { "type": "card", "hint": "4821", "confidence": 0.50 },
  "categoryHint": { "value": "Shopping", "confidence": 0.60 },
  "extractionNotes": "Amount was somewhat ambiguous on order slip; selected total."
}`

	mockLLM := &mockIntegrationLLM{responseJSON: simulatedLLMJSON}

	mod, err := understanding.NewModule(understanding.Config{
		DB:           pool,
		LLMExtractor: mockLLM,
	})
	if err != nil {
		t.Fatalf("failed to initialize understanding module: %v", err)
	}

	userID := uuid.New().String()
	msgID := uuid.New().String()
	accID := uuid.New().String()

	// Ingested email from unknown sender (receipts@unregistered-store.com)
	rawEmail := domain.RawEmailMessage{
		MessageID:      msgID,
		GmailAccountID: accID,
		Sender:         "orders@unregistered-store.com",
		Subject:        "Your Purchase Confirmation",
		BodyText:       "Thank you. Your card 4111 2222 3333 4821 was charged $45.00 on Aug 3, 2026.",
		ReceivedAt:     time.Now().UTC(),
	}

	// 1. Execute routing decision (Diagram 4)
	routeRes, err := mod.ClassifyAndRouteCmd.Handle(ctx, understandingCmd.ClassifyAndRouteMessageCommand{
		MessageID:      msgID,
		GmailAccountID: accID,
		UserID:         userID,
		RawEmail:       rawEmail,
	})
	if err != nil {
		t.Fatalf("ClassifyAndRoute failed: %v", err)
	}

	// Unknown sender must route to ai_extractor [GATE]
	if routeRes.Route != "ai_extractor" {
		t.Fatalf("expected route 'ai_extractor', got '%s'", routeRes.Route)
	}

	// 2. Execute AI extractor command
	aiRes, err := mod.InvokeAICmd.Handle(ctx, understandingCmd.InvokeAIExtractorCommand{
		MessageID:      msgID,
		GmailAccountID: accID,
		UserID:         userID,
		RawEmail:       rawEmail,
		TriggerReason:  "unknown_sender",
	})
	if err != nil {
		t.Fatalf("InvokeAICmd failed: %v", err)
	}

	if aiRes.Outcome != understandingCmd.OutcomeSuccess {
		t.Errorf("expected outcome 'success', got %s", aiRes.Outcome)
	}
	if aiRes.Candidate == nil {
		t.Fatal("expected candidate to be non-nil")
	}

	// [GATE]: Low confidence (< 0.70 threshold) routing to pending review
	if aiRes.ConfidenceAggregate >= 0.70 {
		t.Errorf("expected low confidence < 0.70 for pending review path, got %f", aiRes.ConfidenceAggregate)
	}
	if aiRes.Candidate.Amount.Value != 45.00 || aiRes.Candidate.Amount.Currency != "USD" {
		t.Errorf("expected 45.00 USD, got %v", aiRes.Candidate.Amount)
	}
	if aiRes.Candidate.Provenance.ExtractionMethod != "ai" {
		t.Errorf("expected extraction method 'ai', got %s", aiRes.Candidate.Provenance.ExtractionMethod)
	}

	// 3. Verify extraction_results persisted in PostgreSQL
	var dbMethod, dbPromptVer string
	var dbConf float64
	err = pool.QueryRow(ctx, `
		SELECT method, prompt_version, confidence_aggregate
		FROM understanding.extraction_results
		WHERE id = $1
	`, aiRes.ExtractionResultID).Scan(&dbMethod, &dbPromptVer, &dbConf)
	if err != nil {
		t.Fatalf("querying extraction_results failed: %v", err)
	}
	if dbMethod != "ai" || dbPromptVer != "v1" {
		t.Errorf("expected db record (method=ai, prompt_version=v1), got (%s, %s)", dbMethod, dbPromptVer)
	}

	// 4. Verify administration.prompt_metrics recorded
	var metricOutcome string
	var metricTokens int
	err = pool.QueryRow(ctx, `
		SELECT schema_validation_outcome, token_usage
		FROM administration.prompt_metrics
		WHERE prompt_version = 'v1'
		ORDER BY recorded_at DESC
		LIMIT 1
	`).Scan(&metricOutcome, &metricTokens)
	if err != nil {
		t.Fatalf("querying prompt_metrics failed: %v", err)
	}
	if metricOutcome != understandingCmd.ValidationValidFirstTry {
		t.Errorf("expected validation outcome valid_first_try, got %s", metricOutcome)
	}
	if metricTokens != 330 {
		t.Errorf("expected 330 token usage recorded, got %d", metricTokens)
	}

	// 5. Verify outbox event emitted
	var outboxEvent string
	err = pool.QueryRow(ctx, `
		SELECT event_type
		FROM understanding.outbox_events
		WHERE aggregate_id = $1
	`, aiRes.ExtractionResultID).Scan(&outboxEvent)
	if err != nil {
		t.Fatalf("querying outbox_events failed: %v", err)
	}
	if outboxEvent != domain.EventFinancialEventCandidateCreated {
		t.Errorf("expected EventFinancialEventCandidateCreated outbox event, got %s", outboxEvent)
	}
}

// TestSprint5_TransparentFallbackFromBrokenTemplate verifies:
// Deliberately-broken/unmatched template transparently falls back to AI extractor,
// producing structurally identical CandidateFinancialEvent output (13_email_parser.md §7) [GATE].
func TestSprint5_TransparentFallbackFromBrokenTemplate(t *testing.T) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, getDatabaseURL())
	if err != nil {
		t.Skipf("skipping integration test, postgres unavailable: %v", err)
	}
	defer pool.Close()

	// AI mock response for fallback
	simulatedAI := `{
  "eventType": "Expense",
  "amount": { "value": "89.00", "currency": "USD", "confidence": 0.95 },
  "occurredAt": { "value": "2026-08-04T12:00:00Z", "confidence": 0.90 },
  "merchant": { "rawName": "Target Store", "normalizedName": "Target", "confidence": 0.90 },
  "paymentMethod": { "type": "card", "hint": "1234", "confidence": 0.80 },
  "categoryHint": { "value": "Shopping", "confidence": 0.70 }
}`

	mockLLM := &mockIntegrationLLM{responseJSON: simulatedAI}

	mod, err := understanding.NewModule(understanding.Config{
		DB:           pool,
		LLMExtractor: mockLLM,
	})
	if err != nil {
		t.Fatalf("failed to initialize understanding module: %v", err)
	}

	// 1. Publish a template that expects CSS selector `.non-existent-selector`
	brokenPattern := fmt.Sprintf("*@broken-%s.com", uuid.New().String()[:8])
	brokenRules := domain.ExtractionRules{
		SenderPattern: brokenPattern,
		Version:       1,
		Fields: map[string]domain.FieldRule{
			"amount": {
				SelectorType:       domain.SelectorCSS,
				SelectorExpression: ".non-existent-amount",
				Required:           true,
			},
		},
	}

	// We seed directly into DB as active template
	tmplID := uuid.New().String()
	brokenTmpl, _ := domain.NewSenderTemplate(tmplID, brokenPattern, 1, brokenRules, true, time.Now().UTC())
	_ = mod.Repo.PublishAtomic(ctx, brokenTmpl)

	userID := uuid.New().String()
	msgID := uuid.New().String()

	// Email matches sender pattern, but HTML does NOT contain the selector
	brokenEmail := domain.RawEmailMessage{
		MessageID:      msgID,
		GmailAccountID: uuid.New().String(),
		Sender:         "billing@" + brokenPattern[2:],
		Subject:        "Monthly Billing",
		BodyHTML:       "<div>Total amount was $89.00 at Target.</div>",
		ReceivedAt:     time.Now().UTC(),
	}

	// 2. Apply template -> template extraction fails -> triggers transparent AI fallback [GATE]
	res, err := mod.ApplyTemplateCmd.Handle(ctx, understandingCmd.ApplySenderTemplateCommand{
		MessageID: msgID,
		UserID:    userID,
		RawEmail:  brokenEmail,
	})
	if err != nil {
		t.Fatalf("expected transparent AI fallback to succeed, got error: %v", err)
	}

	if res.Candidate == nil {
		t.Fatal("expected non-nil Candidate from transparent fallback")
	}

	// Symmetrical output verification: CandidateFinancialEvent from AI has valid required fields
	cand := res.Candidate
	if cand.Amount.Value != 89.00 || cand.Amount.Currency != "USD" {
		t.Errorf("expected 89.00 USD from AI fallback, got %v", cand.Amount)
	}
	if cand.Provenance.ExtractionMethod != "ai" {
		t.Errorf("expected candidate provenance method 'ai', got %s", cand.Provenance.ExtractionMethod)
	}
	if err := cand.Validate(); err != nil {
		t.Errorf("candidate from AI fallback failed invariant validation: %v", err)
	}
}

// TestSprint5_RegressionGateInCI verifies the evaluation-set regression gate (18_testing_strategy.md §8) [GATE].
func TestSprint5_RegressionGateInCI(t *testing.T) {
	// Baseline metrics
	baseline := &validator.EvalMetrics{
		TotalFixtures:      7,
		SchemaValidityRate: 1.0,
		FieldLevelAccuracy: 0.95,
	}

	// Candidate with degraded accuracy
	regressed := &validator.EvalMetrics{
		TotalFixtures:      7,
		SchemaValidityRate: 0.85,
		FieldLevelAccuracy: 0.70,
	}

	err := validator.CompareAndGate(baseline, regressed)
	if err == nil {
		t.Fatal("expected regression gate to block regressed candidate prompt, got nil")
	}
}
