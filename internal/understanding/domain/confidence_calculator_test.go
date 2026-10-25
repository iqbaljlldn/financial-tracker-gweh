package domain_test

import (
	"testing"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/domain"
)

func floatPtr(f float64) *float64 {
	return &f
}

func TestComputeAggregateConfidence_FullConfidence(t *testing.T) {
	weights := domain.DefaultConfidenceWeights()
	fc := domain.FieldConfidences{
		Amount:        floatPtr(1.0),
		Merchant:      floatPtr(1.0),
		EventType:     floatPtr(1.0),
		OccurredAt:    floatPtr(1.0),
		PaymentMethod: floatPtr(1.0),
		CategoryHint:  floatPtr(1.0),
	}

	agg, perField := domain.ComputeAggregateConfidence(fc, weights)
	if agg != 1.0 {
		t.Fatalf("expected 1.0 aggregate confidence, got %f", agg)
	}
	if perField["amount"] != 1.0 || perField["merchant"] != 1.0 {
		t.Errorf("expected 1.0 per field, got %v", perField)
	}
}

func TestComputeAggregateConfidence_MissingAmountDepressesScore(t *testing.T) {
	// 11_ai_blueprint.md §6.2: Missing amount (weight 0.35) sharply depresses aggregate score
	weights := domain.DefaultConfidenceWeights()
	fc := domain.FieldConfidences{
		Amount:        nil, // missing amount -> 0.0
		Merchant:      floatPtr(1.0),
		EventType:     floatPtr(1.0),
		OccurredAt:    floatPtr(1.0),
		PaymentMethod: floatPtr(1.0),
		CategoryHint:  floatPtr(1.0),
	}

	agg, perField := domain.ComputeAggregateConfidence(fc, weights)
	// Expected: 1.0 - 0.35 = 0.65
	if agg != 0.65 {
		t.Fatalf("expected 0.65 aggregate confidence when amount is missing, got %f", agg)
	}
	if perField["amount"] != 0.0 {
		t.Errorf("expected 0.0 for missing amount, got %f", perField["amount"])
	}
}

func TestComputeAggregateConfidence_BlueprintExample(t *testing.T) {
	// From 11_ai_blueprint.md §5:
	// amount: 0.95 (0.95 * 0.35 = 0.3325)
	// occurredAt: 0.90 (0.90 * 0.15 = 0.1350)
	// merchant: 0.85 (0.85 * 0.20 = 0.1700)
	// eventType: 0.90 (assumed eventType conf = 0.90 * 0.20 = 0.1800)
	// paymentMethod: 0.70 (0.70 * 0.05 = 0.0350)
	// categoryHint: 0.60 (0.60 * 0.05 = 0.0300)
	// Total = 0.3325 + 0.1350 + 0.1700 + 0.1800 + 0.0350 + 0.0300 = 0.8825 -> 0.883
	weights := domain.DefaultConfidenceWeights()
	fc := domain.FieldConfidences{
		Amount:        floatPtr(0.95),
		OccurredAt:    floatPtr(0.90),
		Merchant:      floatPtr(0.85),
		EventType:     floatPtr(0.90),
		PaymentMethod: floatPtr(0.70),
		CategoryHint:  floatPtr(0.60),
	}

	agg, _ := domain.ComputeAggregateConfidence(fc, weights)
	if agg != 0.883 {
		t.Fatalf("expected 0.883 aggregate confidence, got %f", agg)
	}
}

func TestComputeAggregateConfidence_ReviewableConfigOverride(t *testing.T) {
	// Verifies that weights can be adjusted via config without hardcoding
	customWeights := domain.ConfidenceWeights{
		Amount:        0.50,
		Merchant:      0.10,
		EventType:     0.10,
		OccurredAt:    0.10,
		PaymentMethod: 0.10,
		CategoryHint:  0.10,
	}

	fc := domain.FieldConfidences{
		Amount:        floatPtr(1.0),
		Merchant:      nil,
		EventType:     nil,
		OccurredAt:    nil,
		PaymentMethod: nil,
		CategoryHint:  nil,
	}

	agg, _ := domain.ComputeAggregateConfidence(fc, customWeights)
	if agg != 0.50 {
		t.Fatalf("expected 0.50 aggregate confidence with custom weights, got %f", agg)
	}
}
