package domain_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/financialcore/domain"
)

func TestMoney(t *testing.T) {
	t.Run("valid_creation", func(t *testing.T) {
		m, err := domain.NewMoney("100.50", "USD")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if m.Value() != "100.50" || m.Currency() != "USD" {
			t.Fatalf("unexpected value: %s %s", m.Value(), m.Currency())
		}
		if m.IsZero() {
			t.Fatalf("expected non-zero money")
		}

		m2, err := domain.MoneyFromFloat(100.5, "USD")
		if err != nil {
			t.Fatalf("unexpected error from float: %v", err)
		}
		if !m.Equal(m2) {
			t.Fatalf("expected equal money: %v vs %v", m, m2)
		}
	})

	t.Run("invalid_inputs", func(t *testing.T) {
		// Non-positive
		if _, err := domain.NewMoney("0.00", "USD"); err == nil {
			t.Fatalf("expected error for 0 amount")
		}
		if _, err := domain.NewMoney("-50.00", "USD"); err == nil {
			t.Fatalf("expected error for negative amount")
		}
		// Invalid currency
		if _, err := domain.NewMoney("50.00", "US"); err == nil {
			t.Fatalf("expected error for invalid currency length")
		}
		if _, err := domain.NewMoney("50.00", "usd"); err == nil {
			t.Fatalf("expected error for lowercase currency")
		}
		// Non-numeric
		if _, err := domain.NewMoney("abc", "USD"); err == nil {
			t.Fatalf("expected error for non-numeric value")
		}
		// More than 4 decimals
		if _, err := domain.NewMoney("12.34567", "USD"); err == nil {
			t.Fatalf("expected error for >4 decimals")
		}
	})

	t.Run("json_serialization", func(t *testing.T) {
		m, _ := domain.NewMoney("250.75", "EUR")
		b, err := json.Marshal(m)
		if err != nil {
			t.Fatalf("marshal error: %v", err)
		}
		var decoded domain.Money
		if err := json.Unmarshal(b, &decoded); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if !m.Equal(decoded) {
			t.Fatalf("expected %v, got %v", m, decoded)
		}

		// Rejects numeric float in JSON
		if err := json.Unmarshal([]byte(`{"value": 12.34, "currency": "EUR"}`), &decoded); err == nil {
			t.Fatalf("expected unmarshal to reject float number")
		}
	})
}

func TestConfidenceValueObject(t *testing.T) {
	c := &domain.Confidence{
		Aggregate: 0.85,
		PerField: map[string]float64{
			"amount": 0.9,
			"date":   0.8,
		},
	}
	dec, err := domain.EvaluateConfidenceGate(c, 0.80)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !dec.Passed || !dec.Evaluated || dec.Aggregate != 0.85 {
		t.Fatalf("unexpected decision: %+v", dec)
	}

	if _, err := domain.EvaluateConfidenceGate(nil, 0.80); err == nil {
		t.Fatalf("expected error for nil confidence")
	}
	if _, err := domain.EvaluateConfidenceGate(&domain.Confidence{Aggregate: 1.5}, 0.80); err == nil {
		t.Fatalf("expected error for aggregate > 1.0")
	}

	if m := domain.MethodForExtraction(domain.ExtractionMethodTemplate); m != domain.ConfidenceMethodTemplate {
		t.Fatalf("expected template deterministic method, got %v", m)
	}
	if m := domain.MethodForExtraction(domain.ExtractionMethodAI); m != domain.ConfidenceMethodAI {
		t.Fatalf("expected ai weighted method, got %v", m)
	}
	if m := domain.MethodForExtraction(domain.ExtractionMethodManual); m != domain.ConfidenceMethodManual {
		t.Fatalf("expected manual method, got %v", m)
	}
}

func TestEventTypesAndPayloads(t *testing.T) {
	allTypes := domain.AllEventTypes()
	if len(allTypes) != 15 {
		t.Fatalf("expected 15 event types, got %d", len(allTypes))
	}

	for _, et := range allTypes {
		parsed, err := domain.ParseEventType(string(et))
		if err != nil {
			t.Fatalf("failed to parse %s: %v", et, err)
		}
		if parsed != et {
			t.Fatalf("expected %s, got %s", et, parsed)
		}
		attrs := domain.PayloadAttributes(et)
		if len(attrs) == 0 && et != domain.EventTypeOthers {
			t.Fatalf("expected non-empty attributes for %s", et)
		}
	}

	if _, err := domain.ParseEventType("InvalidType"); err == nil {
		t.Fatalf("expected error parsing invalid event type")
	}

	// Payload validation
	// Transfer with valid direction
	validTransfer := map[string]any{
		"direction": "outgoing",
	}
	cleanTransfer, rejected := domain.NormalizePayload(domain.EventTypeTransfer, validTransfer)
	if len(rejected) > 0 || cleanTransfer["direction"] != "outgoing" {
		t.Fatalf("expected valid transfer normalization")
	}

	// Documented attributes with wrong shape are rejected
	withBadEnum := map[string]any{
		"direction": "sideways",
	}
	_, rejected = domain.NormalizePayload(domain.EventTypeTransfer, withBadEnum)
	if len(rejected) != 1 || rejected[0] != "direction" {
		t.Fatalf("expected direction rejected, got %v", rejected)
	}
}

func TestCategoryAssignment(t *testing.T) {
	now := time.Now().UTC()
	ca, err := domain.NewCategoryAssignment("Groceries", 0.95, domain.CategorySourceUserHistory, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ca.Category() != "Groceries" || ca.Confidence() != 0.95 || ca.Source() != domain.CategorySourceUserHistory {
		t.Fatalf("unexpected assignment values: %+v", ca)
	}
	if !ca.IsCurrent() {
		t.Fatalf("expected assignment to be current")
	}

	// Snapshot and rehydration
	snap := domain.CategoryAssignmentSnapshot{
		ID:         ca.ID(),
		Category:   ca.Category(),
		Confidence: ca.Confidence(),
		Source:     ca.Source(),
		AssignedAt: ca.AssignedAt(),
		IsCurrent:  true,
	}
	rehydrated := domain.RehydrateCategoryAssignment(snap)
	if rehydrated.ID() != ca.ID() || rehydrated.Category() != ca.Category() {
		t.Fatalf("rehydrated assignment does not match: %+v", rehydrated)
	}

	// Invalid sources
	if _, err := domain.NewCategoryAssignment("", 0.9, domain.CategorySourceUserHistory, now); err == nil {
		t.Fatalf("expected error for empty category")
	}
	if _, err := domain.NewCategoryAssignment("Food", 0.9, "unknown_source", now); err == nil {
		t.Fatalf("expected error for unknown source")
	}
	if _, err := domain.NewCategoryAssignment("Food", 1.5, domain.CategorySourceManual, now); err == nil {
		t.Fatalf("expected error for confidence > 1.0")
	}
}

func TestEventOverride(t *testing.T) {
	now := time.Now().UTC()
	evID := uuid.New().String()

	eo, err := domain.NewEventOverride(evID, domain.OverrideFieldMerchant, "Raw Store", "Clean Store", "user_1", now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if eo.FinancialEventID() != evID || eo.FieldName() != domain.OverrideFieldMerchant {
		t.Fatalf("unexpected override: %+v", eo)
	}

	// Unsupported field
	if _, err := domain.NewEventOverride(evID, "unsupported_field", "a", "b", "user_1", now); err == nil {
		t.Fatalf("expected error for unsupported field")
	}

	// Snapshot and rehydration
	snap := domain.EventOverrideSnapshot{
		ID:               eo.ID(),
		FinancialEventID: eo.FinancialEventID(),
		FieldName:        eo.FieldName(),
		PreviousValue:    eo.PreviousValue(),
		NewValue:         eo.NewValue(),
		ActorUserID:      eo.ActorUserID(),
		CorrectedAt:      eo.CorrectedAt(),
	}
	rehydrated := domain.RehydrateEventOverride(snap)
	if rehydrated.ID() != eo.ID() || rehydrated.FieldName() != eo.FieldName() {
		t.Fatalf("rehydrated override mismatch")
	}
}

func TestCandidateParsing(t *testing.T) {
	c := validCandidate(0.85)
	c.TypeSpecificPayload = map[string]any{
		"category":           "Utilities",
		"categorySource":     domain.CategorySourceAIInference,
		"categoryConfidence": 0.88,
	}

	sug, ok := c.SuggestedCategory()
	if !ok {
		t.Fatalf("expected valid suggested category")
	}
	if sug.Category != "Utilities" || sug.Source != domain.CategorySourceAIInference || sug.Confidence != 0.88 {
		t.Fatalf("unexpected suggestion: %+v", sug)
	}

	// Uncategorized / default fallback returns false
	c.TypeSpecificPayload["category"] = domain.UncategorizedLabel
	if _, ok := c.SuggestedCategory(); ok {
		t.Fatalf("expected Uncategorized to return ok=false")
	}

	raw, err := json.Marshal(domain.CandidateCreatedPayload{
		ExtractionResultID: uuid.New().String(),
		IngestedMessageID:  uuid.New().String(),
		Candidate:          c,
		OccurredAt:         time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	parsed, err := domain.ParseCandidatePayload(raw)
	if err != nil {
		t.Fatalf("failed to parse candidate payload: %v", err)
	}
	if parsed.Candidate.UserID != c.UserID {
		t.Fatalf("user id mismatch: %s vs %s", parsed.Candidate.UserID, c.UserID)
	}
}

func TestManualEventCreation(t *testing.T) {
	now := time.Now().UTC()
	amt, _ := domain.NewMoney("75.00", "USD")
	userID := uuid.New().String()

	params := domain.ManualEventParams{
		UserID:       userID,
		EventType:    string(domain.EventTypeExpense),
		Amount:       amt,
		OccurredAt:   now,
		MerchantName: "Manual Grocery",
		PaymentMethod: domain.PaymentMethod{
			Type: "cash",
			Hint: "cash on delivery",
		},
		Payload: map[string]any{
			"direction": "outflow",
		},
	}

	fe, err := domain.NewManual(params, 0.70, now)
	if err != nil {
		t.Fatalf("NewManual failed: %v", err)
	}
	if fe.Status() != domain.StatusFinalized {
		t.Fatalf("expected status %q, got %q", domain.StatusFinalized, fe.Status())
	}
	if fe.ConfidenceMethod() != domain.ConfidenceMethodManual {
		t.Fatalf("expected confidence method manual, got %v", fe.ConfidenceMethod())
	}
	if fe.Provenance().ExtractionMethod() != domain.ExtractionMethodManual {
		t.Fatalf("expected extraction method manual, got %v", fe.Provenance().ExtractionMethod())
	}
}

func TestAggregateOverrides(t *testing.T) {
	now := time.Now().UTC()
	c := validCandidate(0.95)
	dec, _ := domain.EvaluateConfidenceGate(c.Confidence, 0.70)
	fe, err := domain.NewFromCandidate(c, dec, now)
	if err != nil {
		t.Fatalf("NewFromCandidate failed: %v", err)
	}

	// Merchant override
	override, err := fe.OverrideMerchant("Clean Brand", "user_1", now)
	if err != nil {
		t.Fatalf("OverrideMerchant failed: %v", err)
	}
	if override.FieldName() != domain.OverrideFieldMerchant {
		t.Fatalf("expected merchant override field")
	}
	if fe.EffectiveMerchant().DisplayName() != "Clean Brand" {
		t.Fatalf("expected effective merchant 'Clean Brand', got %q", fe.EffectiveMerchant().DisplayName())
	}

	// Category override
	override, assignment, err := fe.OverrideCategory("Entertainment", "user_1", now)
	if err != nil {
		t.Fatalf("OverrideCategory failed: %v", err)
	}
	if override.FieldName() != domain.OverrideFieldCategory {
		t.Fatalf("expected category override field")
	}
	if fe.Category() == nil || fe.Category().Category() != "Entertainment" {
		t.Fatalf("expected effective category Entertainment, got %v", fe.Category())
	}
	if assignment.Source() != domain.CategorySourceManual {
		t.Fatalf("expected manual category source, got %s", assignment.Source())
	}

	// Overrides list
	allOverrides := fe.Overrides()
	if len(allOverrides) != 2 {
		t.Fatalf("expected 2 overrides, got %d", len(allOverrides))
	}
}

func TestCursorEncoding(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	c := domain.Cursor{
		OccurredAt: now,
		ID:         uuid.New().String(),
	}
	encoded := c.Encode()
	decoded, err := domain.DecodeCursor(encoded)
	if err != nil {
		t.Fatalf("DecodeCursor failed: %v", err)
	}
	if decoded.ID != c.ID || !decoded.OccurredAt.Equal(c.OccurredAt) {
		t.Fatalf("cursor mismatch: %+v vs %+v", decoded, c)
	}

	if _, err := domain.DecodeCursor("invalid-cursor!!!"); err == nil {
		t.Fatalf("expected error for malformed cursor")
	}
}

func TestDomainEventPayloadBuilders(t *testing.T) {
	now := time.Now().UTC()
	c := validCandidate(0.95)
	dec, _ := domain.EvaluateConfidenceGate(c.Confidence, 0.70)
	fe, _ := domain.NewFromCandidate(c, dec, now)

	finalizedPayload := domain.NewFinalizedPayload(fe, domain.FinalizedViaGate, now)
	if finalizedPayload.EventID != fe.ID() || finalizedPayload.FinalizedVia != domain.FinalizedViaGate {
		t.Fatalf("unexpected finalized payload: %+v", finalizedPayload)
	}

	pendingPayload := domain.NewPendingReviewPayload(fe, 0.85, now)
	if pendingPayload.EventID != fe.ID() || pendingPayload.Threshold != 0.85 {
		t.Fatalf("unexpected pending payload: %+v", pendingPayload)
	}

	asgn, _ := domain.NewCategoryAssignment("Dining", 0.9, domain.CategorySourceUserHistory, now)
	fe.AssignCategory(asgn)
	catChangedPayload := domain.NewCategoryChangedPayload(fe, "OldCategory", now)
	if catChangedPayload.Category != "Dining" || catChangedPayload.PreviousCategory != "OldCategory" {
		t.Fatalf("unexpected category changed payload: %+v", catChangedPayload)
	}
}

func TestRehydration(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	amt, _ := domain.NewMoney("300.00", "USD")
	id := uuid.New().String()
	userID := uuid.New().String()

	snap := domain.Snapshot{
		ID:         id,
		UserID:     userID,
		EventType:  domain.EventTypeExpense,
		Amount:     amt,
		OccurredAt: now,
		Merchant: domain.Merchant{
			NormalizedName: "BestBuy",
			RawName:        "BEST BUY #123",
		},
		Status: domain.StatusFinalized,
		Confidence: domain.Confidence{
			Aggregate: 0.99,
		},
		ConfidenceMethod: domain.ConfidenceMethodTemplate,
		Provenance: domain.ProvenanceParams{
			ConnectorType:      "gmail",
			ConnectorAccountID: uuid.New().String(),
			SourceMessageRef:   "msg_rehydrate_1",
			ExtractionMethod:   domain.ExtractionMethodTemplate,
			ExtractedAt:        now,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	fe := domain.Rehydrate(snap)
	if fe.ID() != id || fe.UserID() != userID || fe.Amount().Value() != "300.00" {
		t.Fatalf("rehydrated event aggregate mismatch: %+v", fe)
	}
}

func TestStatusParsing(t *testing.T) {
	validStatuses := []domain.Status{
		domain.StatusPendingReview,
		domain.StatusFinalized,
		domain.StatusArchived,
		domain.StatusDisputed,
	}
	for _, s := range validStatuses {
		parsed, err := domain.ParseStatus(string(s))
		if err != nil || parsed != s {
			t.Fatalf("failed to parse status %s: %v", s, err)
		}
	}
	if _, err := domain.ParseStatus("invalid_status"); err == nil {
		t.Fatalf("expected error for invalid status")
	}
}

func TestPayloadNormalizationComprehensive(t *testing.T) {
	// SubscriptionRenewal with renewal_cycle (enum), next_expected_date (date), subscription_name (string)
	subPayload := map[string]any{
		"subscription_name":  "Netflix",
		"renewal_cycle":      "monthly",
		"next_expected_date": "2026-11-01",
	}
	clean, rej := domain.NormalizePayload(domain.EventTypeSubscriptionRenewal, subPayload)
	if len(rej) > 0 || clean["renewal_cycle"] != "monthly" {
		t.Fatalf("expected valid subscription payload")
	}

	// ValidatePayload strict mode
	validStrict, err := domain.ValidatePayload(domain.EventTypeSubscriptionRenewal, subPayload)
	if err != nil || validStrict["subscription_name"] != "Netflix" {
		t.Fatalf("ValidatePayload failed: %v", err)
	}

	// ValidatePayload with invalid attribute
	badSub := map[string]any{
		"renewal_cycle": "weekly", // not in monthly, annual
	}
	if _, err := domain.ValidatePayload(domain.EventTypeSubscriptionRenewal, badSub); err == nil {
		t.Fatalf("expected ValidatePayload to fail for bad enum")
	}

	// ValidatePayload with unsupported event type
	if _, err := domain.ValidatePayload(domain.EventType("bad_type"), nil); err == nil {
		t.Fatalf("expected ValidatePayload to fail for unsupported event type")
	}

	// Test FailedPayment with attempted_amount (kindMoney float and string)
	failedPay := map[string]any{
		"failure_reason_hint": "insufficient_funds",
		"attempted_amount":    45.50,
	}
	cleanFP, rejFP := domain.NormalizePayload(domain.EventTypeFailedPayment, failedPay)
	if len(rejFP) > 0 || cleanFP["attempted_amount"] != "45.50" {
		t.Fatalf("expected valid failed payment normalization: %v", cleanFP)
	}

	// Test Transfer with linked_transfer_event_id (kindUUID)
	transferPayload := map[string]any{
		"linked_transfer_event_id": uuid.New().String(),
		"direction":                "incoming",
	}
	cleanTrans, rejTrans := domain.NormalizePayload(domain.EventTypeTransfer, transferPayload)
	if len(rejTrans) > 0 || cleanTrans["direction"] != "incoming" {
		t.Fatalf("expected valid transfer UUID normalization: %v", rejTrans)
	}

	// Bad UUID
	badUUIDTrans := map[string]any{
		"linked_transfer_event_id": "not-a-uuid",
	}
	_, rejBadUUID := domain.NormalizePayload(domain.EventTypeTransfer, badUUIDTrans)
	if len(rejBadUUID) != 1 {
		t.Fatalf("expected bad UUID to be rejected")
	}

	// Date format: RFC3339
	rfcDatePayload := map[string]any{
		"due_date": "2026-11-05T12:00:00Z",
	}
	cleanCC, rejCC := domain.NormalizePayload(domain.EventTypeCreditCardBill, rfcDatePayload)
	if len(rejCC) > 0 || cleanCC["due_date"] != "2026-11-05T12:00:00Z" {
		t.Fatalf("expected valid RFC3339 date normalization: %v", rejCC)
	}

	// Bad date string
	badDatePayload := map[string]any{
		"due_date": "invalid-date",
	}
	_, rejBadDate := domain.NormalizePayload(domain.EventTypeCreditCardBill, badDatePayload)
	if len(rejBadDate) != 1 {
		t.Fatalf("expected invalid date to be rejected")
	}
}

func TestDomainAccessorsAndEdgeCases(t *testing.T) {
	now := time.Now().UTC()
	c := validCandidate(0.95)
	dec, _ := domain.EvaluateConfidenceGate(c.Confidence, 0.70)
	fe, err := domain.NewFromCandidate(c, dec, now)
	if err != nil {
		t.Fatalf("NewFromCandidate failed: %v", err)
	}

	// Call accessors
	_ = fe.OccurredAt()
	_ = fe.Merchant()
	_ = fe.PaymentMethod()
	_ = fe.Confidence()
	_ = fe.CreatedAt()
	_ = fe.UpdatedAt()
	_ = fe.RejectedAttributes()
	_ = fe.TypeSpecificPayload()

	prov := fe.Provenance()
	_ = prov.ConnectorType()
	_ = prov.TemplateVersion()
	_ = prov.PromptVersion()
	_ = prov.ModelIdentifier()
	_ = prov.ExtractedAt()
	_ = prov.Params()

	// Merchant DisplayName fallback
	mFallback := domain.Merchant{RawName: "RAW STORE"}
	if mFallback.DisplayName() != "RAW STORE" {
		t.Fatalf("expected DisplayName fallback to raw name")
	}

	// NewFromCandidate validation errors
	badUser := c
	badUser.UserID = "invalid-uuid"
	if _, err := domain.NewFromCandidate(badUser, dec, now); err == nil {
		t.Fatalf("expected error for invalid UserID")
	}

	badOccurred := c
	badOccurred.OccurredAt = time.Time{}
	if _, err := domain.NewFromCandidate(badOccurred, dec, now); err == nil {
		t.Fatalf("expected error for zero OccurredAt")
	}

	badType := c
	badType.EventType = "invalid"
	if _, err := domain.NewFromCandidate(badType, dec, now); err == nil {
		t.Fatalf("expected error for invalid EventType")
	}

	badAmt := c
	badAmt.Amount.Value = -10
	if _, err := domain.NewFromCandidate(badAmt, dec, now); err == nil {
		t.Fatalf("expected error for negative Amount")
	}

	manualCand := c
	manualCand.Provenance.ExtractionMethod = string(domain.ExtractionMethodManual)
	if _, err := domain.NewFromCandidate(manualCand, dec, now); err == nil {
		t.Fatalf("expected error for manual candidate extraction")
	}

	// NewManual validation errors
	validMoney, _ := domain.NewMoney("10.00", "USD")
	manParams := domain.ManualEventParams{
		UserID:       "not-a-uuid",
		EventType:    string(domain.EventTypeExpense),
		Amount:       validMoney,
		OccurredAt:   now,
		MerchantName: "Store",
	}
	if _, err := domain.NewManual(manParams, 0.70, now); err == nil {
		t.Fatalf("expected error for invalid manual UserID")
	}

	manParams.UserID = uuid.New().String()
	manParams.OccurredAt = time.Time{}
	if _, err := domain.NewManual(manParams, 0.70, now); err == nil {
		t.Fatalf("expected error for zero manual OccurredAt")
	}

	manParams.OccurredAt = now
	manParams.EventType = "bad_type"
	if _, err := domain.NewManual(manParams, 0.70, now); err == nil {
		t.Fatalf("expected error for invalid manual EventType")
	}

	manParams.EventType = string(domain.EventTypeExpense)
	manParams.Amount = domain.Money{}
	if _, err := domain.NewManual(manParams, 0.70, now); err == nil {
		t.Fatalf("expected error for zero manual Amount")
	}

	// Lifecycle error branches
	// Finalizing non-pending review
	if err := fe.Finalize(dec, true, now); err == nil {
		t.Fatalf("expected error finalizing already finalized event")
	}

	// OverrideAmount errors
	if _, err := fe.OverrideAmount(domain.Money{}, "user", now); err == nil {
		t.Fatalf("expected error overriding zero amount")
	}
	if _, err := fe.OverrideAmount(fe.EffectiveAmount(), "user", now); err == nil {
		t.Fatalf("expected error overriding to identical amount")
	}

	// OverrideMerchant errors
	if _, err := fe.OverrideMerchant("", "user", now); err == nil {
		t.Fatalf("expected error overriding empty merchant")
	}
	if _, err := fe.OverrideMerchant(fe.EffectiveMerchant().DisplayName(), "user", now); err == nil {
		t.Fatalf("expected error overriding to identical merchant")
	}

	// OverrideEventType errors
	if _, err := fe.OverrideEventType(domain.EventType("bad"), "user", now); err == nil {
		t.Fatalf("expected error overriding invalid event type")
	}

	// OverrideCategory errors
	if _, _, err := fe.OverrideCategory("Entertainment", "user", now); err != nil {
		// first override
		_, _, _ = fe.OverrideCategory("Entertainment", "user", now)
	}

	// Provenance validation errors
	if _, err := domain.NewProvenance(domain.ProvenanceParams{
		ConnectorType:      "gmail",
		ConnectorAccountID: uuid.New().String(),
		SourceMessageRef:   "ref",
		ExtractionMethod:   domain.ExtractionMethod("invalid_method"),
	}); err == nil {
		t.Fatalf("expected error for invalid extraction method")
	}

	if _, err := domain.NewProvenance(domain.ProvenanceParams{
		ConnectorType:      "",
		ConnectorAccountID: uuid.New().String(),
		SourceMessageRef:   "ref",
		ExtractionMethod:   domain.ExtractionMethodTemplate,
	}); err == nil {
		t.Fatalf("expected error for empty connector type")
	}
}

func TestCoverageBoost(t *testing.T) {
	now := time.Now().UTC()

	// MoneyFromFloat negative
	if _, err := domain.MoneyFromFloat(-10.5, "USD"); err == nil {
		t.Fatalf("expected error for negative MoneyFromFloat")
	}

	// normalizeAttribute edge cases
	// kindString with non-string
	_, rej := domain.NormalizePayload(domain.EventTypeIncome, map[string]any{"income_source": 123})
	if len(rej) != 1 {
		t.Fatalf("expected non-string to be rejected")
	}

	// kindBool with valid and non-bool
	clean, rej := domain.NormalizePayload(domain.EventTypeIncome, map[string]any{"is_recurring": true})
	if len(rej) > 0 || clean["is_recurring"] != true {
		t.Fatalf("expected valid bool")
	}
	_, rej = domain.NormalizePayload(domain.EventTypeIncome, map[string]any{"is_recurring": "yes"})
	if len(rej) != 1 {
		t.Fatalf("expected string rejected for bool")
	}

	// kindDate with non-string, ISO date, invalid ISO date
	clean, _ = domain.NormalizePayload(domain.EventTypeSubscriptionRenewal, map[string]any{"next_expected_date": "2026-10-26"})
	if clean["next_expected_date"] != "2026-10-26" {
		t.Fatalf("expected valid ISO date")
	}
	_, rej = domain.NormalizePayload(domain.EventTypeSubscriptionRenewal, map[string]any{"next_expected_date": 123})
	if len(rej) != 1 {
		t.Fatalf("expected non-string date rejected")
	}
	_, rej = domain.NormalizePayload(domain.EventTypeSubscriptionRenewal, map[string]any{"next_expected_date": "2026-02-31"})
	if len(rej) != 1 {
		t.Fatalf("expected invalid calendar date rejected")
	}

	// kindMoney with bad string, bad float, non-number
	_, rej = domain.NormalizePayload(domain.EventTypeCreditCardBill, map[string]any{"total_due": "abc"})
	if len(rej) != 1 {
		t.Fatalf("expected bad money string rejected")
	}
	_, rej = domain.NormalizePayload(domain.EventTypeCreditCardBill, map[string]any{"total_due": -5.0})
	if len(rej) != 1 {
		t.Fatalf("expected negative money float rejected")
	}
	_, rej = domain.NormalizePayload(domain.EventTypeCreditCardBill, map[string]any{"total_due": true})
	if len(rej) != 1 {
		t.Fatalf("expected non-number money rejected")
	}

	// kindUUID with non-string
	_, rej = domain.NormalizePayload(domain.EventTypeTransfer, map[string]any{"linked_transfer_event_id": 123})
	if len(rej) != 1 {
		t.Fatalf("expected non-string UUID rejected")
	}

	// kindEnum with non-string
	_, rej = domain.NormalizePayload(domain.EventTypeTransfer, map[string]any{"direction": 123})
	if len(rej) != 1 {
		t.Fatalf("expected non-string enum rejected")
	}

	// Category assignment supersede
	c := validCandidate(0.95)
	dec, _ := domain.EvaluateConfidenceGate(c.Confidence, 0.70)
	fe, _ := domain.NewFromCandidate(c, dec, now)
	cat1, _ := domain.NewCategoryAssignment("Dining", 0.9, domain.CategorySourceUserHistory, now)
	fe.AssignCategory(cat1)
	cat2, _ := domain.NewCategoryAssignment("Food", 0.9, domain.CategorySourceUserHistory, now)
	fe.AssignCategory(cat2) // supersedes cat1
	if cat1.IsCurrent() || cat1.SupersededBy() != cat2.ID() {
		t.Fatalf("expected cat1 to be superseded by cat2")
	}

	// Override category from nil category
	fe2, _ := domain.NewFromCandidate(c, dec, now)
	_, _, err := fe2.OverrideCategory("Travel", "user_1", now)
	if err != nil {
		t.Fatalf("OverrideCategory on uncategorized event failed: %v", err)
	}
	// Override to same category
	_, _, err = fe2.OverrideCategory("Travel", "user_1", now)
	if err != domain.ErrNoOverrideChange {
		t.Fatalf("expected ErrNoOverrideChange, got %v", err)
	}

	// Cursor decode errors
	if _, err := domain.DecodeCursor("e30="); err == nil {
		t.Fatalf("expected error for empty cursor JSON")
	}
}
