package validator_test

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/application/prompt"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/application/validator"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/domain"
)

// mockSimulatedLLMExtractor simulates model behavior based on whether prompt version is good or regressed.
type mockSimulatedLLMExtractor struct {
	regressed bool
}

func (m *mockSimulatedLLMExtractor) Extract(_ context.Context, req domain.ExtractionPromptRequest) (*domain.StructuredLLMResponse, error) {
	// Look at the last message content
	lastMsg := req.Messages[len(req.Messages)-1].Content

	if m.regressed {
		// Regressed model extracts incorrect amount and wrong event type
		return &domain.StructuredLLMResponse{
			RawJSON: `{
  "eventType": "Others",
  "amount": { "value": "1.00", "currency": "XXX", "confidence": 0.20 },
  "occurredAt": { "value": "2026-08-01T00:00:00Z", "confidence": 0.20 },
  "merchant": { "rawName": "Unknown", "normalizedName": "Unknown", "confidence": 0.20 }
}`,
			ModelIdentifier: "regressed-model",
		}, nil
	}

	// Accurate baseline model
	lower := strings.ToLower(lastMsg)
	var jsonResp string
	switch {
	case strings.Contains(lower, "amazon"):
		jsonResp = `{
  "eventType": "Expense",
  "amount": { "value": "45.00", "currency": "USD", "confidence": 0.98 },
  "occurredAt": { "value": "2026-08-03T14:22:00Z", "confidence": 0.95 },
  "merchant": { "rawName": "Amazon.com", "normalizedName": "Amazon", "confidence": 0.95 }
}`
	case strings.Contains(lower, "gopay") || strings.Contains(lower, "kopi kenangan"):
		jsonResp = `{
  "eventType": "Expense",
  "amount": { "value": "28500", "currency": "IDR", "confidence": 0.99 },
  "occurredAt": { "value": "2026-08-04T10:15:00Z", "confidence": 0.95 },
  "merchant": { "rawName": "Kopi Kenangan", "normalizedName": "Kopi Kenangan", "confidence": 0.95 }
}`
	case strings.Contains(lower, "netflix"):
		jsonResp = `{
  "eventType": "SubscriptionRenewal",
  "amount": { "value": "22.99", "currency": "USD", "confidence": 0.98 },
  "occurredAt": { "value": "2026-08-05T08:00:00Z", "confidence": 0.95 },
  "merchant": { "rawName": "Netflix", "normalizedName": "Netflix", "confidence": 0.95 }
}`
	case strings.Contains(lower, "acme corp") || strings.Contains(lower, "payroll"):
		jsonResp = `{
  "eventType": "Salary",
  "amount": { "value": "4250.00", "currency": "EUR", "confidence": 0.95 },
  "occurredAt": { "value": "2026-07-31T09:00:00Z", "confidence": 0.95 },
  "merchant": { "rawName": "Acme Corp Europe GmbH", "normalizedName": "Acme Corp", "confidence": 0.90 }
}`
	case strings.Contains(lower, "klikbca") || strings.Contains(lower, "m-bca"):
		jsonResp = `{
  "eventType": "Transfer",
  "amount": { "value": "500000", "currency": "IDR", "confidence": 0.99 },
  "occurredAt": { "value": "2026-08-02T07:10:05Z", "confidence": 0.90 },
  "merchant": { "rawName": "KlikBCA", "normalizedName": "BCA", "confidence": 0.90 }
}`
	case strings.Contains(lower, "grab") || strings.Contains(lower, "refunded"):
		jsonResp = `{
  "eventType": "Refund",
  "amount": { "value": "14.50", "currency": "SGD", "confidence": 0.95 },
  "occurredAt": { "value": "2026-08-01T04:30:00Z", "confidence": 0.90 },
  "merchant": { "rawName": "Grab", "normalizedName": "Grab", "confidence": 0.90 }
}`
	case strings.Contains(lower, "bankbsi") || strings.Contains(lower, "masita"):
		jsonResp = `{
  "eventType": "Expense",
  "amount": { "value": "15000", "currency": "IDR", "confidence": 0.98 },
  "occurredAt": { "value": "2026-10-24T10:40:22Z", "confidence": 0.95 },
  "merchant": { "rawName": "Masita (Ricebowl Salad &", "normalizedName": "Masita", "confidence": 0.95 },
  "paymentMethod": { "type": "QRIS", "hint": "1309", "confidence": 0.90 }
}`
	default: // Target
		jsonResp = `{
  "eventType": "Expense",
  "amount": { "value": "30.60", "currency": "USD", "confidence": 0.92 },
  "occurredAt": { "value": "2026-08-02T19:00:00Z", "confidence": 0.90 },
  "merchant": { "rawName": "Target", "normalizedName": "Target", "confidence": 0.95 }
}`
	}

	return &domain.StructuredLLMResponse{
		RawJSON:         jsonResp,
		ModelIdentifier: "baseline-model",
	}, nil
}

func TestAIEvalHarness_RunEvaluationSet(t *testing.T) {
	fixturesDir := filepath.Join("..", "..", "..", "..", "test", "fixtures", "ai-eval-set")
	fixtures, err := validator.LoadEvalFixtures(fixturesDir)
	if err != nil {
		t.Fatalf("failed to load evaluation fixtures: %v", err)
	}

	if len(fixtures) < 7 {
		t.Fatalf("expected at least 7 eval fixtures covering event types and currencies, got %d", len(fixtures))
	}

	ctx := context.Background()
	pv := prompt.GetV1PromptVersion()
	extractor := &mockSimulatedLLMExtractor{regressed: false}

	metrics, err := validator.EvaluatePromptVersion(ctx, pv, extractor, fixtures)
	if err != nil {
		t.Fatalf("evaluation failed: %v", err)
	}

	if metrics.SchemaValidityRate != 1.0 {
		t.Errorf("expected 100%% schema validity rate, got %f", metrics.SchemaValidityRate)
	}
	if metrics.FieldLevelAccuracy < 0.90 {
		t.Errorf("expected high field level accuracy (>=0.90), got %f", metrics.FieldLevelAccuracy)
	}
	if metrics.CalibrationScore <= 0 {
		t.Errorf("expected positive calibration score, got %f", metrics.CalibrationScore)
	}

	fmt.Printf("Baseline Metrics: Fixtures=%d, SchemaValidity=%.2f, FieldAccuracy=%.2f, CalibrationScore=%.2f\n",
		metrics.TotalFixtures, metrics.SchemaValidityRate, metrics.FieldLevelAccuracy, metrics.CalibrationScore)
}

func TestAIEvalHarness_RegressionGateRejectsRegressedPrompt(t *testing.T) {
	// [GATE]: Demonstrate that a deliberately-regressed candidate prompt fails promotion gate (18_testing_strategy.md §8.3)
	fixturesDir := filepath.Join("..", "..", "..", "..", "test", "fixtures", "ai-eval-set")
	fixtures, err := validator.LoadEvalFixtures(fixturesDir)
	if err != nil {
		t.Fatalf("failed to load fixtures: %v", err)
	}

	ctx := context.Background()
	v1Prompt := prompt.GetV1PromptVersion()

	// 1. Compute baseline scores for active prompt (v1)
	baselineMetrics, err := validator.EvaluatePromptVersion(ctx, v1Prompt, &mockSimulatedLLMExtractor{regressed: false}, fixtures)
	if err != nil {
		t.Fatalf("baseline eval failed: %v", err)
	}

	// 2. Candidate prompt (v2) with degraded extraction accuracy
	v2Prompt := &prompt.PromptVersion{
		VersionID:         "v2-regressed",
		SystemInstruction: "Degraded instructions that misclassify transactions",
		SchemaDefinition:  v1Prompt.SchemaDefinition,
	}
	regressedMetrics, err := validator.EvaluatePromptVersion(ctx, v2Prompt, &mockSimulatedLLMExtractor{regressed: true}, fixtures)
	if err != nil {
		t.Fatalf("candidate eval failed: %v", err)
	}

	// 3. CompareAndGate MUST return error and block activation
	gateErr := validator.CompareAndGate(baselineMetrics, regressedMetrics)
	if gateErr == nil {
		t.Fatal("expected CompareAndGate to reject regressed prompt version, got nil [GATE FAILED]")
	}
	if !strings.Contains(gateErr.Error(), "field-level accuracy regressed") {
		t.Errorf("expected field-level accuracy regressed error, got %v", gateErr)
	}

	// 4. Candidate prompt with identical or improved metrics PASSES promotion gate
	candidateMetrics, err := validator.EvaluatePromptVersion(ctx, v1Prompt, &mockSimulatedLLMExtractor{regressed: false}, fixtures)
	if err != nil {
		t.Fatalf("candidate eval failed: %v", err)
	}
	if err := validator.CompareAndGate(baselineMetrics, candidateMetrics); err != nil {
		t.Fatalf("expected non-regressed candidate to pass promotion gate, got %v", err)
	}
}
