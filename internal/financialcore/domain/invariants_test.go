package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/financialcore/domain"
)

func validCandidate(confidence float64) domain.Candidate {
	return domain.Candidate{
		UserID:    uuid.New().String(),
		EventType: string(domain.EventTypeExpense),
		Amount: domain.CandidateAmount{
			Value:    125.50,
			Currency: "USD",
		},
		OccurredAt: time.Now().UTC(),
		Merchant: domain.Merchant{
			NormalizedName: "Target",
			RawName:        "TARGET T-1234",
		},
		PaymentMethod: domain.CandidatePayment{
			Type:   "card",
			Detail: "Visa",
			Last4:  "1234",
		},
		Confidence: &domain.Confidence{
			Aggregate: confidence,
		},
		Provenance: domain.CandidateProvenance{
			ConnectorType:      "gmail",
			ConnectorAccountID: uuid.New().String(),
			SourceMessageRef:   "msg_inv_test_" + uuid.New().String(),
			ExtractionMethod:   string(domain.ExtractionMethodTemplate),
			TemplateVersion:    "target-v1",
			ExtractedAt:        time.Now().UTC(),
		},
	}
}

// Test_INV1_ConfidenceGateEnforcement verifies INV-1 (04_domain_model.md §5.7):
// Cannot finalize without evaluated confidence; below-threshold -> pending_review,
// and below-threshold cannot transition to finalized without explicit user confirmation.
func Test_INV1_ConfidenceGateEnforcement(t *testing.T) {
	now := time.Now().UTC()
	threshold := 0.85

	t.Run("fails_when_decision_not_evaluated", func(t *testing.T) {
		c := validCandidate(0.95)
		unevaluatedDecision := domain.GateDecision{
			Evaluated: false,
			Aggregate: 0.95,
		}
		_, err := domain.NewFromCandidate(c, unevaluatedDecision, now)
		if err != domain.ErrConfidenceNotEvaluated {
			t.Fatalf("expected ErrConfidenceNotEvaluated, got %v", err)
		}
	})

	t.Run("fails_when_decision_aggregate_mismatches_candidate", func(t *testing.T) {
		c := validCandidate(0.95)
		mismatchedDecision := domain.GateDecision{
			Evaluated: true,
			Aggregate: 0.50,
			Threshold: threshold,
			Passed:    false,
		}
		_, err := domain.NewFromCandidate(c, mismatchedDecision, now)
		if err != domain.ErrConfidenceNotEvaluated {
			t.Fatalf("expected ErrConfidenceNotEvaluated, got %v", err)
		}
	})

	t.Run("below_threshold_produces_pending_review", func(t *testing.T) {
		c := validCandidate(0.60)
		decision, err := domain.EvaluateConfidenceGate(c.Confidence, threshold)
		if err != nil {
			t.Fatalf("EvaluateConfidenceGate failed: %v", err)
		}
		if decision.Passed {
			t.Fatalf("expected decision.Passed == false for 0.60 < 0.85")
		}

		fe, err := domain.NewFromCandidate(c, decision, now)
		if err != nil {
			t.Fatalf("NewFromCandidate failed: %v", err)
		}
		if fe.Status() != domain.StatusPendingReview {
			t.Fatalf("expected status %q, got %q", domain.StatusPendingReview, fe.Status())
		}

		// Attempting to finalize without user confirmation must fail with ErrBelowThreshold
		err = fe.Finalize(decision, false, now)
		if err != domain.ErrBelowThreshold {
			t.Fatalf("expected ErrBelowThreshold, got %v", err)
		}

		// Finalizing with explicit user confirmation succeeds
		err = fe.Finalize(decision, true, now)
		if err != nil {
			t.Fatalf("Finalize with user confirmation failed: %v", err)
		}
		if fe.Status() != domain.StatusFinalized {
			t.Fatalf("expected status %q, got %q", domain.StatusFinalized, fe.Status())
		}
	})

	t.Run("above_or_equal_threshold_produces_finalized", func(t *testing.T) {
		c := validCandidate(0.90)
		decision, err := domain.EvaluateConfidenceGate(c.Confidence, threshold)
		if err != nil {
			t.Fatalf("EvaluateConfidenceGate failed: %v", err)
		}
		if !decision.Passed {
			t.Fatalf("expected decision.Passed == true for 0.90 >= 0.85")
		}

		fe, err := domain.NewFromCandidate(c, decision, now)
		if err != nil {
			t.Fatalf("NewFromCandidate failed: %v", err)
		}
		if fe.Status() != domain.StatusFinalized {
			t.Fatalf("expected status %q, got %q", domain.StatusFinalized, fe.Status())
		}
	})
}

// Test_INV2_CurrencyImmutability verifies INV-2 (04_domain_model.md §5.7):
// amount.currency is immutable once set from extraction; overrides reject cross-currency changes with ErrCurrencyImmutable.
func Test_INV2_CurrencyImmutability(t *testing.T) {
	now := time.Now().UTC()
	c := validCandidate(0.95)
	decision, _ := domain.EvaluateConfidenceGate(c.Confidence, 0.70)
	fe, err := domain.NewFromCandidate(c, decision, now)
	if err != nil {
		t.Fatalf("NewFromCandidate failed: %v", err)
	}

	// Correcting value with same currency succeeds
	newAmountSameCurr, err := domain.NewMoney("150.00", "USD")
	if err != nil {
		t.Fatalf("NewMoney failed: %v", err)
	}
	override, err := fe.OverrideAmount(newAmountSameCurr, "user_1", now)
	if err != nil {
		t.Fatalf("OverrideAmount with same currency failed: %v", err)
	}
	if override.FieldName() != domain.OverrideFieldAmount {
		t.Fatalf("expected override field %q, got %q", domain.OverrideFieldAmount, override.FieldName())
	}
	if fe.EffectiveAmount().Value() != "150.00" {
		t.Fatalf("expected effective amount 150.00, got %s", fe.EffectiveAmount().Value())
	}

	// Attempting cross-currency change must be rejected with ErrCurrencyImmutable
	newAmountDiffCurr, err := domain.NewMoney("150.00", "IDR")
	if err != nil {
		t.Fatalf("NewMoney failed: %v", err)
	}
	_, err = fe.OverrideAmount(newAmountDiffCurr, "user_1", now)
	if err == nil {
		t.Fatalf("expected ErrCurrencyImmutable for cross-currency override, got nil")
	}
	if !errors.Is(err, domain.ErrCurrencyImmutable) {
		t.Fatalf("expected ErrCurrencyImmutable, got %v", err)
	}

	// Original amount currency must remain unchanged
	if fe.Amount().Currency() != "USD" {
		t.Fatalf("expected original currency USD, got %s", fe.Amount().Currency())
	}
}

// Test_INV3_ProvenanceUniquenessConstraint verifies INV-3 (04_domain_model.md §5.7):
// (connector_account_id, source_message_ref) pair is unique per provenance;
// provenance constructor validates required uniqueness identifiers.
func Test_INV3_ProvenanceUniquenessConstraint(t *testing.T) {
	accountID := uuid.New().String()
	sourceRef := "msg_12345"

	prov, err := domain.NewProvenance(domain.ProvenanceParams{
		ConnectorType:      "gmail",
		ConnectorAccountID: accountID,
		SourceMessageRef:   sourceRef,
		ExtractionMethod:   domain.ExtractionMethodTemplate,
		ExtractedAt:        time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("NewProvenance failed: %v", err)
	}
	if prov.ConnectorAccountID() != accountID {
		t.Fatalf("expected connectorAccountID %s, got %s", accountID, prov.ConnectorAccountID())
	}
	if prov.SourceMessageRef() != sourceRef {
		t.Fatalf("expected sourceMessageRef %s, got %s", sourceRef, prov.SourceMessageRef())
	}

	// Invalid empty connectorAccountID or sourceMessageRef
	_, err = domain.NewProvenance(domain.ProvenanceParams{
		ConnectorType:      "gmail",
		ConnectorAccountID: "",
		SourceMessageRef:   sourceRef,
		ExtractionMethod:   domain.ExtractionMethodTemplate,
	})
	if err == nil {
		t.Fatalf("expected error for empty connectorAccountID, got nil")
	}

	_, err = domain.NewProvenance(domain.ProvenanceParams{
		ConnectorType:      "gmail",
		ConnectorAccountID: accountID,
		SourceMessageRef:   "",
		ExtractionMethod:   domain.ExtractionMethodTemplate,
	})
	if err == nil {
		t.Fatalf("expected error for empty sourceMessageRef, got nil")
	}
}

// Test_INV4_ArchivedStatusExclusionFromAnalytics verifies INV-4 (04_domain_model.md §5.7):
// Archived status is excluded from Analytics and Budget eligibility (IsAnalyticsEligible() == false),
// and archiving is idempotent.
func Test_INV4_ArchivedStatusExclusionFromAnalytics(t *testing.T) {
	now := time.Now().UTC()
	c := validCandidate(0.95)
	decision, _ := domain.EvaluateConfidenceGate(c.Confidence, 0.70)
	fe, err := domain.NewFromCandidate(c, decision, now)
	if err != nil {
		t.Fatalf("NewFromCandidate failed: %v", err)
	}

	// Finalized event is eligible for analytics
	if !fe.IsAnalyticsEligible() {
		t.Fatalf("finalized event must be analytics eligible")
	}

	// Archive the event
	changed := fe.Archive(now)
	if !changed {
		t.Fatalf("expected Archive() to return true on first call")
	}
	if fe.Status() != domain.StatusArchived {
		t.Fatalf("expected status %q, got %q", domain.StatusArchived, fe.Status())
	}

	// Invariant INV-4 check: archived event is NOT eligible
	if fe.IsAnalyticsEligible() {
		t.Fatalf("archived event must NOT be analytics eligible (INV-4)")
	}

	// Idempotent re-archive
	changedAgain := fe.Archive(now)
	if changedAgain {
		t.Fatalf("expected Archive() on already-archived event to return false")
	}

	// Archived event cannot be mutated
	newAmount, _ := domain.NewMoney("200.00", "USD")
	_, err = fe.OverrideAmount(newAmount, "user_1", now)
	if err != domain.ErrEventArchived {
		t.Fatalf("expected ErrEventArchived on override, got %v", err)
	}
}

// Test_INV5_EventTypeMutabilityViaOverride verifies INV-5 (04_domain_model.md §5.7):
// event_type is mutable ONLY via EventOverride, never direct mutation. The original extracted
// event_type is immutable, and EffectiveEventType reflects the override.
func Test_INV5_EventTypeMutabilityViaOverride(t *testing.T) {
	now := time.Now().UTC()
	c := validCandidate(0.95)
	decision, _ := domain.EvaluateConfidenceGate(c.Confidence, 0.70)
	fe, err := domain.NewFromCandidate(c, decision, now)
	if err != nil {
		t.Fatalf("NewFromCandidate failed: %v", err)
	}

	if fe.EventType() != domain.EventTypeExpense {
		t.Fatalf("expected original EventType %q, got %q", domain.EventTypeExpense, fe.EventType())
	}
	if fe.EffectiveEventType() != domain.EventTypeExpense {
		t.Fatalf("expected initial EffectiveEventType %q, got %q", domain.EventTypeExpense, fe.EffectiveEventType())
	}

	// Override event_type to SubscriptionRenewal
	override, err := fe.OverrideEventType(domain.EventTypeSubscriptionRenewal, "user_1", now)
	if err != nil {
		t.Fatalf("OverrideEventType failed: %v", err)
	}
	if override.FieldName() != domain.OverrideFieldEventType {
		t.Fatalf("expected override field %q, got %q", domain.OverrideFieldEventType, override.FieldName())
	}

	// Invariant INV-5 check: original is unchanged, effective reflects latest override
	if fe.EventType() != domain.EventTypeExpense {
		t.Fatalf("original EventType must remain unchanged %q, got %q", domain.EventTypeExpense, fe.EventType())
	}
	if fe.EffectiveEventType() != domain.EventTypeSubscriptionRenewal {
		t.Fatalf("EffectiveEventType must reflect latest override %q, got %q", domain.EventTypeSubscriptionRenewal, fe.EffectiveEventType())
	}

	// Attempting no-op override returns ErrNoOverrideChange
	_, err = fe.OverrideEventType(domain.EventTypeSubscriptionRenewal, "user_1", now)
	if err != domain.ErrNoOverrideChange {
		t.Fatalf("expected ErrNoOverrideChange, got %v", err)
	}
}

func containsError(err, target error) bool {
	if err == nil {
		return false
	}
	if err == target {
		return true
	}
	return false
}
