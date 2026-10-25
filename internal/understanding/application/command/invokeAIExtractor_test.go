package command_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/application/categorization"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/application/command"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/application/governance"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/application/prompt"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/domain"
)

type mockLLMExtractor struct {
	mu        sync.Mutex
	responses []*domain.StructuredLLMResponse
	callCount int
	err       error
}

func (m *mockLLMExtractor) Extract(_ context.Context, _ domain.ExtractionPromptRequest) (*domain.StructuredLLMResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCount++
	if m.err != nil {
		return nil, m.err
	}
	if len(m.responses) == 0 {
		return nil, errors.New("no mock responses configured")
	}
	idx := m.callCount - 1
	if idx >= len(m.responses) {
		idx = len(m.responses) - 1
	}
	return m.responses[idx], nil
}

type mockExtractionRepo struct {
	mu      sync.Mutex
	results []*domain.ExtractionResultRecord
}

func (m *mockExtractionRepo) SaveResult(_ context.Context, result *domain.ExtractionResultRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.results = append(m.results, result)
	return nil
}
func (m *mockExtractionRepo) ListRecent(_ context.Context, _ int) ([]*domain.ExtractionResultRecord, error) {
	return nil, nil
}
func (m *mockExtractionRepo) GetTemplateStatsInWindow(_ context.Context, _ string, _ time.Time) (int, int, error) {
	return 0, 0, nil
}

type mockPromptMetricsRepo struct {
	mu      sync.Mutex
	metrics []*domain.PromptMetricRecord
}

func (m *mockPromptMetricsRepo) RecordPromptMetric(_ context.Context, metric *domain.PromptMetricRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.metrics = append(m.metrics, metric)
	return nil
}

type mockOutboxRepo struct {
	mu     sync.Mutex
	events []string
}

func (m *mockOutboxRepo) RecordOutboxEvent(_ context.Context, _, _, eventType string, _ any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, eventType)
	return nil
}

func setupTestAIHandler(mockLLM *mockLLMExtractor, budgetCfg governance.BudgetConfig) (
	*command.InvokeAIExtractorHandler,
	*mockExtractionRepo,
	*mockPromptMetricsRepo,
	*mockOutboxRepo,
) {
	reg := prompt.NewRegistry()
	userStore := categorization.NewInMemoryUserHistoryStore()
	globalStore := categorization.NewInMemoryGlobalMappingStore()
	catEngine := categorization.NewEngine(categorization.Config{
		UserHistory:   userStore,
		GlobalMapping: globalStore,
		AIThreshold:   0.40,
	})
	gov := governance.NewDailyBudgetGovernor(budgetCfg)
	extRepo := &mockExtractionRepo{}
	metricsRepo := &mockPromptMetricsRepo{}
	outboxRepo := &mockOutboxRepo{}
	weights := domain.DefaultConfidenceWeights()

	handler := command.NewInvokeAIExtractorHandler(
		mockLLM,
		reg,
		catEngine,
		gov,
		extRepo,
		metricsRepo,
		outboxRepo,
		weights,
		nil,
	)

	return handler, extRepo, metricsRepo, outboxRepo
}

func TestInvokeAIExtractor_SuccessHappyPath(t *testing.T) {
	validJSON := `{
  "eventType": "Expense",
  "amount": { "value": "45.00", "currency": "USD", "confidence": 0.95 },
  "occurredAt": { "value": "2026-08-03T14:22:00Z", "confidence": 0.90 },
  "merchant": { "rawName": "Starbucks Coffee", "normalizedName": "Starbucks", "confidence": 0.90 },
  "paymentMethod": { "type": "card", "hint": "4821", "confidence": 0.80 },
  "categoryHint": { "value": "Dining", "confidence": 0.85 },
  "extractionNotes": "Clear receipt."
}`

	mockLLM := &mockLLMExtractor{
		responses: []*domain.StructuredLLMResponse{
			{
				RawJSON:         validJSON,
				ModelIdentifier: "test-model-v1",
				LatencyMs:       120,
				TokenUsage:      domain.LLMTokenUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
			},
		},
	}

	handler, extRepo, metricsRepo, outboxRepo := setupTestAIHandler(mockLLM, governance.BudgetConfig{
		MaxPerUserDaily:  10,
		MaxPlatformDaily: 100,
	})

	res, err := handler.Handle(context.Background(), command.InvokeAIExtractorCommand{
		MessageID:      "msg-123",
		GmailAccountID: "acc-456",
		UserID:         "user-789",
		RawEmail: domain.RawEmailMessage{
			Sender:   "receipts@starbucks.com",
			Subject:  "Your Starbucks Receipt",
			BodyText: "You spent $45.00 using Visa ending in 4111 2222 3333 4821 on Aug 3, 2026.",
		},
		TriggerReason: "unknown_sender",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Outcome != command.OutcomeSuccess {
		t.Errorf("expected outcome 'success', got %s", res.Outcome)
	}
	if res.Candidate == nil {
		t.Fatal("expected non-nil Candidate")
	}
	if res.Candidate.Amount.Value != 45.00 || res.Candidate.Amount.Currency != "USD" {
		t.Errorf("expected 45.00 USD, got %v", res.Candidate.Amount)
	}
	// Starbucks is in GlobalMapping -> Dining
	if res.Candidate.TypeSpecificPayload["category"] != "Dining" {
		t.Errorf("expected category Dining from global mapping, got %v", res.Candidate.TypeSpecificPayload["category"])
	}

	// Verify extraction_results persisted
	if len(extRepo.results) != 1 {
		t.Fatalf("expected 1 extraction result saved, got %d", len(extRepo.results))
	}
	if extRepo.results[0].Method != "ai" {
		t.Errorf("expected method 'ai', got %s", extRepo.results[0].Method)
	}

	// Verify prompt_metrics recorded
	if len(metricsRepo.metrics) != 1 {
		t.Fatalf("expected 1 prompt metric recorded, got %d", len(metricsRepo.metrics))
	}
	if metricsRepo.metrics[0].SchemaValidationOutcome != command.ValidationValidFirstTry {
		t.Errorf("expected valid_first_try, got %s", metricsRepo.metrics[0].SchemaValidationOutcome)
	}

	// Verify Outbox Event emitted (FinancialEventCandidateCreated)
	if len(outboxRepo.events) != 1 || outboxRepo.events[0] != domain.EventFinancialEventCandidateCreated {
		t.Errorf("expected EventFinancialEventCandidateCreated outbox event, got %v", outboxRepo.events)
	}
}

func TestInvokeAIExtractor_SingleRepromptOnSchemaFailure_Success(t *testing.T) {
	// First response is broken JSON, second response is valid JSON
	validJSON := `{
  "eventType": "Expense",
  "amount": { "value": "15.50", "currency": "USD", "confidence": 0.90 },
  "occurredAt": { "value": "2026-08-03T14:22:00Z", "confidence": 0.90 },
  "merchant": { "rawName": "Uber", "normalizedName": "Uber", "confidence": 0.90 }
}`

	mockLLM := &mockLLMExtractor{
		responses: []*domain.StructuredLLMResponse{
			{
				RawJSON:         "INVALID NON-JSON RESPONSE",
				ModelIdentifier: "test-model-v1",
				LatencyMs:       50,
			},
			{
				RawJSON:         validJSON,
				ModelIdentifier: "test-model-v1",
				LatencyMs:       60,
			},
		},
	}

	handler, _, metricsRepo, _ := setupTestAIHandler(mockLLM, governance.DefaultBudgetConfig())

	res, err := handler.Handle(context.Background(), command.InvokeAIExtractorCommand{
		MessageID: "msg-reprompt",
		UserID:    "user-1",
		RawEmail: domain.RawEmailMessage{
			Subject:  "Your Trip with Uber",
			BodyText: "Total: $15.50",
		},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Outcome != command.OutcomeRepromptSuccess {
		t.Errorf("expected OutcomeRepromptSuccess, got %s", res.Outcome)
	}
	if mockLLM.callCount != 2 {
		t.Errorf("expected exactly 2 LLM calls (1 attempt + 1 re-prompt), got %d", mockLLM.callCount)
	}
	if len(metricsRepo.metrics) != 1 || metricsRepo.metrics[0].SchemaValidationOutcome != command.ValidationValidAfterReprompt {
		t.Errorf("expected valid_after_reprompt recorded in prompt_metrics, got %v", metricsRepo.metrics)
	}
}

func TestInvokeAIExtractor_RepromptFails_RoutesToManualReview(t *testing.T) {
	// Both initial attempt and re-prompt return broken JSON -> routes to review queue [GATE]
	mockLLM := &mockLLMExtractor{
		responses: []*domain.StructuredLLMResponse{
			{RawJSON: "broken json 1", ModelIdentifier: "test-model"},
			{RawJSON: "broken json 2", ModelIdentifier: "test-model"},
		},
	}

	handler, extRepo, metricsRepo, outboxRepo := setupTestAIHandler(mockLLM, governance.DefaultBudgetConfig())

	res, err := handler.Handle(context.Background(), command.InvokeAIExtractorCommand{
		MessageID: "msg-bad-schema",
		UserID:    "user-1",
		RawEmail: domain.RawEmailMessage{
			Subject:  "Receipt",
			BodyText: "Unintelligible text",
		},
	})

	if err != nil {
		t.Fatalf("unexpected error, should gracefully handle schema invalid: %v", err)
	}
	if res.Outcome != command.OutcomeSchemaInvalid {
		t.Errorf("expected OutcomeSchemaInvalid, got %s", res.Outcome)
	}
	if res.ConfidenceAggregate != 0.0 {
		t.Errorf("expected 0.0 confidence for invalid schema, got %f", res.ConfidenceAggregate)
	}
	if mockLLM.callCount != 2 {
		t.Errorf("expected 2 calls before giving up, got %d", mockLLM.callCount)
	}
	// Check prompt_metrics logged invalid
	if len(metricsRepo.metrics) != 1 || metricsRepo.metrics[0].SchemaValidationOutcome != command.ValidationInvalid {
		t.Errorf("expected invalid in prompt_metrics, got %v", metricsRepo.metrics)
	}
	// Outbox event must still be emitted so financialcore enqueues review request (Diagram 4)
	if len(outboxRepo.events) != 1 || outboxRepo.events[0] != domain.EventFinancialEventCandidateCreated {
		t.Errorf("expected EventFinancialEventCandidateCreated for manual review, got %v", outboxRepo.events)
	}
	if len(extRepo.results) != 1 || extRepo.results[0].ConfidenceAggregate != 0.0 {
		t.Errorf("expected extraction_result saved with 0.0 confidence")
	}
}

func TestInvokeAIExtractor_BudgetExhaustion_RoutesToManualReview(t *testing.T) {
	mockLLM := &mockLLMExtractor{
		responses: []*domain.StructuredLLMResponse{
			{RawJSON: `{"eventType":"Expense","amount":{"value":10,"currency":"USD"}}`},
		},
	}

	// Daily budget 1 call
	handler, extRepo, _, outboxRepo := setupTestAIHandler(mockLLM, governance.BudgetConfig{
		MaxPerUserDaily:  1,
		MaxPlatformDaily: 100,
	})

	// First call succeeds
	_, err := handler.Handle(context.Background(), command.InvokeAIExtractorCommand{
		MessageID: "msg-1",
		UserID:    "user-budget",
		RawEmail:  domain.RawEmailMessage{Subject: "Order 1", BodyText: "Receipt $10"},
	})
	if err != nil {
		t.Fatalf("first call should succeed: %v", err)
	}

	// Second call exceeds budget -> circuit breaker trips
	res, err := handler.Handle(context.Background(), command.InvokeAIExtractorCommand{
		MessageID: "msg-2",
		UserID:    "user-budget",
		RawEmail:  domain.RawEmailMessage{Subject: "Order 2", BodyText: "Receipt $20"},
	})
	if err != nil {
		t.Fatalf("second call should be handled gracefully, got err: %v", err)
	}
	if res.Outcome != command.OutcomeBudgetExceeded {
		t.Errorf("expected OutcomeBudgetExceeded, got %s", res.Outcome)
	}
	if res.ConfidenceAggregate != 0.0 {
		t.Errorf("expected 0.0 confidence, got %f", res.ConfidenceAggregate)
	}
	// Must route to review queue (financialcore candidate created)
	if len(outboxRepo.events) != 2 {
		t.Fatalf("expected 2 outbox events, got %d", len(outboxRepo.events))
	}
	if len(extRepo.results) != 2 || extRepo.results[1].ConfidenceAggregate != 0.0 {
		t.Errorf("expected budget exceeded extraction result recorded")
	}
}

func TestInvokeAIExtractor_PIIMaskedInPrompt(t *testing.T) {
	var capturedEmail string
	mockLLM := &mockLLMExtractor{
		responses: []*domain.StructuredLLMResponse{
			{
				RawJSON: `{
  "eventType": "Expense",
  "amount": { "value": "100.00", "currency": "USD", "confidence": 0.95 },
  "occurredAt": { "value": "2026-08-03T14:22:00Z", "confidence": 0.90 },
  "merchant": { "rawName": "Apple Store", "normalizedName": "Apple", "confidence": 0.95 }
}`,
				ModelIdentifier: "test-model",
			},
		},
	}

	// Capture messages passed to LLM
	wrappingLLM := &capturingLLMExtractor{
		inner: mockLLM,
		onExtract: func(req domain.ExtractionPromptRequest) {
			for _, m := range req.Messages {
				if m.Role == "user" && strings.Contains(m.Content, "Subject:") {
					capturedEmail = m.Content
				}
			}
		},
	}

	reg := prompt.NewRegistry()
	userStore := categorization.NewInMemoryUserHistoryStore()
	globalStore := categorization.NewInMemoryGlobalMappingStore()
	catEngine := categorization.NewEngine(categorization.Config{UserHistory: userStore, GlobalMapping: globalStore})
	gov := governance.NewDailyBudgetGovernor(governance.DefaultBudgetConfig())
	extRepo := &mockExtractionRepo{}
	metricsRepo := &mockPromptMetricsRepo{}
	outboxRepo := &mockOutboxRepo{}

	handler := command.NewInvokeAIExtractorHandler(
		wrappingLLM,
		reg,
		catEngine,
		gov,
		extRepo,
		metricsRepo,
		outboxRepo,
		domain.DefaultConfidenceWeights(),
		nil,
	)

	_, err := handler.Handle(context.Background(), command.InvokeAIExtractorCommand{
		MessageID: "msg-pii",
		UserID:    "user-1",
		RawEmail: domain.RawEmailMessage{
			Subject:  "Apple Store Receipt",
			BodyText: "Charged $100.00 to Card: 4111 2222 3333 9876.\n---\nTo unsubscribe from marketing, click here.",
		},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify full credit card number was masked to ****9876 before reaching prompt
	if strings.Contains(capturedEmail, "4111 2222 3333 9876") {
		t.Errorf("PII LEAK: Full card number was not masked in prompt: %s", capturedEmail)
	}
	if !strings.Contains(capturedEmail, "****9876") {
		t.Errorf("expected masked card ****9876 in prompt: %s", capturedEmail)
	}
	// Verify footer stripped
	if strings.Contains(capturedEmail, "To unsubscribe") {
		t.Errorf("Boilerplate was not stripped in prompt: %s", capturedEmail)
	}
}

type capturingLLMExtractor struct {
	inner     domain.LLMExtractor
	onExtract func(req domain.ExtractionPromptRequest)
}

func (c *capturingLLMExtractor) Extract(ctx context.Context, req domain.ExtractionPromptRequest) (*domain.StructuredLLMResponse, error) {
	if c.onExtract != nil {
		c.onExtract(req)
	}
	return c.inner.Extract(ctx, req)
}
