package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Status is the FinancialEvent lifecycle state (04_domain_model.md §5.1.1).
type Status string

const (
	StatusPendingReview Status = "pending_review"
	StatusFinalized     Status = "finalized"
	StatusArchived      Status = "archived"
	StatusDisputed      Status = "disputed"
)

// ParseStatus validates a status string.
func ParseStatus(s string) (Status, error) {
	switch st := Status(s); st {
	case StatusPendingReview, StatusFinalized, StatusArchived, StatusDisputed:
		return st, nil
	}
	return "", fmt.Errorf("financial event: unsupported status %q", s)
}

// FinancialEvent is the aggregate root of the ledger (04_domain_model.md §5.1): a common envelope plus
// a polymorphic type-specific payload. All state is unexported; every mutation goes through a method that
// enforces INV-1..INV-5. Original extracted values are never mutated — user corrections are recorded as
// EventOverrides and surfaced through the Effective* accessors (SRS-FR-LEDGER-02).
type FinancialEvent struct {
	id               string
	userID           string
	eventType        EventType
	amount           Money
	occurredAt       time.Time
	merchant         Merchant
	paymentMethod    PaymentMethod
	status           Status
	confidence       Confidence
	confidenceMethod ConfidenceMethod
	typeSpecific     map[string]any
	provenance       Provenance
	category         *CategoryAssignment
	overrides        []*EventOverride
	createdAt        time.Time
	updatedAt        time.Time

	rejectedAttributes []string // transient: documented payload attributes dropped during candidate normalization
}

// NewFromCandidate builds a FinancialEvent from an extractor candidate and a recorded gate decision.
//
// It enforces INV-1 (04_domain_model.md §5.7) at creation: an event cannot exist without an evaluated
// confidence — the decision must have been produced by EvaluateConfidenceGate for this exact candidate
// confidence. Passed decisions yield `finalized`; below-threshold decisions yield `pending_review`.
func NewFromCandidate(c Candidate, decision GateDecision, now time.Time) (*FinancialEvent, error) {
	if !decision.Evaluated || c.Confidence == nil || decision.Aggregate != c.Confidence.Aggregate {
		return nil, ErrConfidenceNotEvaluated
	}
	if _, err := uuid.Parse(c.UserID); err != nil {
		return nil, fmt.Errorf("%w: userId must be a UUID", ErrInvalidCandidate)
	}
	if c.OccurredAt.IsZero() {
		return nil, fmt.Errorf("%w: occurredAt is required", ErrInvalidCandidate)
	}

	eventType, err := ParseEventType(c.EventType)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidCandidate, err)
	}
	amount, err := MoneyFromFloat(c.Amount.Value, c.Amount.Currency)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidCandidate, err)
	}

	prov, err := NewProvenance(ProvenanceParams{
		ConnectorType:      c.Provenance.ConnectorType,
		ConnectorAccountID: c.Provenance.ConnectorAccountID,
		SourceMessageRef:   c.Provenance.SourceMessageRef,
		ExtractionMethod:   ExtractionMethod(c.Provenance.ExtractionMethod),
		TemplateVersion:    c.Provenance.TemplateVersion,
		PromptVersion:      c.Provenance.PromptVersion,
		ModelIdentifier:    c.Provenance.ModelIdentifier,
		ExtractedAt:        c.Provenance.ExtractedAt,
	})
	if err != nil {
		return nil, err
	}
	if prov.ExtractionMethod() == ExtractionMethodManual {
		return nil, fmt.Errorf("%w: candidates cannot claim manual extraction", ErrInvalidCandidate)
	}

	payload, rejected := NormalizePayload(eventType, c.attributesWithoutCategory())

	status := StatusPendingReview
	if decision.Passed {
		status = StatusFinalized
	}

	return &FinancialEvent{
		id:               uuid.New().String(),
		userID:           c.UserID,
		eventType:        eventType,
		amount:           amount,
		occurredAt:       c.OccurredAt.UTC(),
		merchant:         c.Merchant,
		paymentMethod:    paymentFromCandidate(c.PaymentMethod),
		status:           status,
		confidence:       cloneConfidence(*c.Confidence),
		confidenceMethod: MethodForExtraction(prov.ExtractionMethod()),
		typeSpecific:     payload,
		provenance:       prov,
		createdAt:        now.UTC(),
		updatedAt:        now.UTC(),

		rejectedAttributes: rejected,
	}, nil
}

// ManualEventParams carries user-entered ledger data for POST /financial-events (FR-LEDGER-03).
type ManualEventParams struct {
	UserID        string
	EventType     string
	Amount        Money
	OccurredAt    time.Time
	MerchantName  string
	PaymentMethod PaymentMethod
	Payload       map[string]any
}

// NewManual builds a user-entered event. Provenance.extractionMethod is forced to `manual` regardless of
// caller input (09_api_specification.md §6.2); confidence is 1.0 and the gate decision is still recorded.
func NewManual(p ManualEventParams, threshold float64, now time.Time) (*FinancialEvent, error) {
	if _, err := uuid.Parse(p.UserID); err != nil {
		return nil, fmt.Errorf("%w: userId must be a UUID", ErrInvalidCandidate)
	}
	if p.OccurredAt.IsZero() {
		return nil, fmt.Errorf("%w: occurredAt is required", ErrInvalidCandidate)
	}
	eventType, err := ParseEventType(p.EventType)
	if err != nil {
		return nil, err
	}
	if p.Amount.IsZero() {
		return nil, ErrInvalidAmount
	}
	payload, err := ValidatePayload(eventType, p.Payload)
	if err != nil {
		return nil, err
	}

	conf := Confidence{Aggregate: 1.0}
	decision, err := EvaluateConfidenceGate(&conf, threshold)
	if err != nil {
		return nil, err
	}

	prov, err := NewProvenance(ProvenanceParams{
		ConnectorType:      "manual",
		ConnectorAccountID: p.UserID,
		SourceMessageRef:   "manual:" + uuid.New().String(),
		ExtractionMethod:   ExtractionMethodManual,
		ExtractedAt:        now,
	})
	if err != nil {
		return nil, err
	}

	status := StatusPendingReview
	if decision.Passed {
		status = StatusFinalized
	}
	name := strings.TrimSpace(p.MerchantName)

	return &FinancialEvent{
		id:               uuid.New().String(),
		userID:           p.UserID,
		eventType:        eventType,
		amount:           p.Amount,
		occurredAt:       p.OccurredAt.UTC(),
		merchant:         Merchant{NormalizedName: name, RawName: name},
		paymentMethod:    p.PaymentMethod,
		status:           status,
		confidence:       conf,
		confidenceMethod: ConfidenceMethodManual,
		typeSpecific:     payload,
		provenance:       prov,
		createdAt:        now.UTC(),
		updatedAt:        now.UTC(),
	}, nil
}

func paymentFromCandidate(p CandidatePayment) PaymentMethod {
	hint := strings.TrimSpace(p.Detail)
	if last4 := strings.TrimSpace(p.Last4); last4 != "" {
		hint = strings.TrimSpace(hint + " **** " + last4)
	}
	return PaymentMethod{Type: strings.TrimSpace(p.Type), Hint: hint}
}

func cloneConfidence(c Confidence) Confidence {
	out := Confidence{Aggregate: c.Aggregate}
	if len(c.PerField) > 0 {
		out.PerField = make(map[string]float64, len(c.PerField))
		for k, v := range c.PerField {
			out.PerField[k] = v
		}
	}
	return out
}

// Snapshot is the persisted shape of a FinancialEvent used only by repository adapters for rehydration.
type Snapshot struct {
	ID               string
	UserID           string
	EventType        EventType
	Amount           Money
	OccurredAt       time.Time
	Merchant         Merchant
	PaymentMethod    PaymentMethod
	Status           Status
	Confidence       Confidence
	ConfidenceMethod ConfidenceMethod
	TypeSpecific     map[string]any
	Provenance       ProvenanceParams
	Category         *CategoryAssignment
	Overrides        []*EventOverride
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Rehydrate rebuilds an aggregate from persisted state without re-running creation-time validation:
// persisted rows were validated on the way in (and by DB constraints).
func Rehydrate(s Snapshot) *FinancialEvent {
	prov := Provenance{
		connectorType:      s.Provenance.ConnectorType,
		connectorAccountID: s.Provenance.ConnectorAccountID,
		sourceMessageRef:   s.Provenance.SourceMessageRef,
		extractionMethod:   s.Provenance.ExtractionMethod,
		templateVersion:    s.Provenance.TemplateVersion,
		promptVersion:      s.Provenance.PromptVersion,
		modelIdentifier:    s.Provenance.ModelIdentifier,
		extractedAt:        s.Provenance.ExtractedAt,
	}
	payload := s.TypeSpecific
	if payload == nil {
		payload = map[string]any{}
	}
	return &FinancialEvent{
		id:               s.ID,
		userID:           s.UserID,
		eventType:        s.EventType,
		amount:           s.Amount,
		occurredAt:       s.OccurredAt,
		merchant:         s.Merchant,
		paymentMethod:    s.PaymentMethod,
		status:           s.Status,
		confidence:       s.Confidence,
		confidenceMethod: s.ConfidenceMethod,
		typeSpecific:     payload,
		provenance:       prov,
		category:         s.Category,
		overrides:        s.Overrides,
		createdAt:        s.CreatedAt,
		updatedAt:        s.UpdatedAt,
	}
}

// --- Accessors (original, as-extracted values) ---

func (e *FinancialEvent) ID() string                         { return e.id }
func (e *FinancialEvent) UserID() string                     { return e.userID }
func (e *FinancialEvent) EventType() EventType               { return e.eventType }
func (e *FinancialEvent) Amount() Money                      { return e.amount }
func (e *FinancialEvent) OccurredAt() time.Time              { return e.occurredAt }
func (e *FinancialEvent) Merchant() Merchant                 { return e.merchant }
func (e *FinancialEvent) PaymentMethod() PaymentMethod       { return e.paymentMethod }
func (e *FinancialEvent) Status() Status                     { return e.status }
func (e *FinancialEvent) Confidence() Confidence             { return cloneConfidence(e.confidence) }
func (e *FinancialEvent) ConfidenceMethod() ConfidenceMethod { return e.confidenceMethod }
func (e *FinancialEvent) Provenance() Provenance             { return e.provenance }
func (e *FinancialEvent) Category() *CategoryAssignment      { return e.category }
func (e *FinancialEvent) CreatedAt() time.Time               { return e.createdAt }
func (e *FinancialEvent) UpdatedAt() time.Time               { return e.updatedAt }

// RejectedAttributes lists documented payload attributes dropped while normalizing a candidate (transient).
func (e *FinancialEvent) RejectedAttributes() []string { return e.rejectedAttributes }

// TypeSpecificPayload returns a copy of the type-specific attributes.
func (e *FinancialEvent) TypeSpecificPayload() map[string]any {
	out := make(map[string]any, len(e.typeSpecific))
	for k, v := range e.typeSpecific {
		out[k] = v
	}
	return out
}

// Overrides returns the override audit trail oldest-first.
func (e *FinancialEvent) Overrides() []*EventOverride {
	out := make([]*EventOverride, len(e.overrides))
	copy(out, e.overrides)
	return out
}

// --- Effective view: latest override per field, else the original extracted value (04_domain_model.md §5.5) ---

func (e *FinancialEvent) latestOverride(field string) *EventOverride {
	for i := len(e.overrides) - 1; i >= 0; i-- {
		if e.overrides[i].fieldName == field {
			return e.overrides[i]
		}
	}
	return nil
}

// EffectiveAmount returns the latest overridden amount, else the original.
func (e *FinancialEvent) EffectiveAmount() Money {
	if o := e.latestOverride(OverrideFieldAmount); o != nil {
		var m Money
		if err := m.UnmarshalJSON(o.newValue); err == nil {
			return m
		}
	}
	return e.amount
}

// EffectiveMerchant returns the merchant with its normalized name overridden when corrected.
func (e *FinancialEvent) EffectiveMerchant() Merchant {
	m := e.merchant
	if o := e.latestOverride(OverrideFieldMerchant); o != nil {
		var name string
		if err := jsonString(o.newValue, &name); err == nil && name != "" {
			m.NormalizedName = name
		}
	}
	return m
}

// EffectiveEventType returns the latest overridden event type, else the original.
func (e *FinancialEvent) EffectiveEventType() EventType {
	if o := e.latestOverride(OverrideFieldEventType); o != nil {
		var s string
		if err := jsonString(o.newValue, &s); err == nil {
			if t, perr := ParseEventType(s); perr == nil {
				return t
			}
		}
	}
	return e.eventType
}

// --- Lifecycle & invariants ---

// Finalize enforces INV-1 (04_domain_model.md §5.7): an event cannot transition from `pending_review` to
// `finalized` without a confidence value present and evaluated against the current threshold. A decision
// below the threshold finalizes only with explicit user confirmation (the web/WhatsApp confirm path).
func (e *FinancialEvent) Finalize(d GateDecision, userConfirmed bool, now time.Time) error {
	if e.status == StatusArchived {
		return ErrEventArchived
	}
	if e.status != StatusPendingReview {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidStatusTransition, e.status, StatusFinalized)
	}
	if !d.Evaluated || d.Aggregate != e.confidence.Aggregate {
		return ErrConfidenceNotEvaluated
	}
	if !d.Passed && !userConfirmed {
		return ErrBelowThreshold
	}
	e.status = StatusFinalized
	e.updatedAt = now.UTC()
	return nil
}

// Archive soft-deletes the event (INV-4, 04_domain_model.md §5.7). It reports whether the status changed;
// archiving an already-archived event is an idempotent no-op so purge cascades can safely be redelivered.
func (e *FinancialEvent) Archive(now time.Time) bool {
	if e.status == StatusArchived {
		return false
	}
	e.status = StatusArchived
	e.updatedAt = now.UTC()
	return true
}

// IsAnalyticsEligible enforces INV-4 (04_domain_model.md §5.7): archived events — and any event not yet
// finalized — are excluded from all Analytics and Budget computation.
func (e *FinancialEvent) IsAnalyticsEligible() bool {
	return e.status == StatusFinalized
}

func (e *FinancialEvent) guardMutable() error {
	if e.status == StatusArchived {
		return ErrEventArchived
	}
	return nil
}

// OverrideAmount records a correction of amount.value. It enforces INV-2 (04_domain_model.md §5.7):
// amount.currency is immutable once set from extraction, so a different currency is rejected with
// ErrCurrencyImmutable and never applied silently.
func (e *FinancialEvent) OverrideAmount(next Money, actor string, now time.Time) (*EventOverride, error) {
	if err := e.guardMutable(); err != nil {
		return nil, err
	}
	if next.IsZero() {
		return nil, ErrInvalidAmount
	}
	if next.Currency() != e.amount.Currency() {
		return nil, fmt.Errorf("%w: event is %s, override is %s", ErrCurrencyImmutable, e.amount.Currency(), next.Currency())
	}
	prev := e.EffectiveAmount()
	if prev.Equal(next) {
		return nil, ErrNoOverrideChange
	}
	return e.appendOverride(OverrideFieldAmount, prev, next, actor, now)
}

// OverrideMerchant records a correction of the merchant's normalized name.
func (e *FinancialEvent) OverrideMerchant(name, actor string, now time.Time) (*EventOverride, error) {
	if err := e.guardMutable(); err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("%w: merchant must not be empty", ErrInvalidOverrideValue)
	}
	prev := e.EffectiveMerchant().DisplayName()
	if prev == name {
		return nil, ErrNoOverrideChange
	}
	return e.appendOverride(OverrideFieldMerchant, prev, name, actor, now)
}

// OverrideEventType enforces INV-5 (04_domain_model.md §5.7): event_type is mutable only through an
// explicit EventOverride on the event_type field. There is no direct setter; the original type is retained
// and EffectiveEventType reflects the correction.
func (e *FinancialEvent) OverrideEventType(next EventType, actor string, now time.Time) (*EventOverride, error) {
	if err := e.guardMutable(); err != nil {
		return nil, err
	}
	if _, ok := typePayloadSpecs[next]; !ok {
		return nil, fmt.Errorf("%w: %q", ErrInvalidEventType, next)
	}
	prev := e.EffectiveEventType()
	if prev == next {
		return nil, ErrNoOverrideChange
	}
	return e.appendOverride(OverrideFieldEventType, prev, next, actor, now)
}

// OverrideCategory records a manual category correction and the superseding CategoryAssignment
// (source `manual`, confidence 1.0) that becomes the current effective category.
func (e *FinancialEvent) OverrideCategory(category, actor string, now time.Time) (*EventOverride, *CategoryAssignment, error) {
	if err := e.guardMutable(); err != nil {
		return nil, nil, err
	}
	category = strings.TrimSpace(category)
	prev := UncategorizedLabel
	if e.category != nil {
		prev = e.category.category
	}
	if prev == category {
		return nil, nil, ErrNoOverrideChange
	}
	assignment, err := NewCategoryAssignment(category, 1.0, CategorySourceManual, now)
	if err != nil {
		return nil, nil, err
	}
	override, err := e.appendOverride(OverrideFieldCategory, prev, category, actor, now)
	if err != nil {
		return nil, nil, err
	}
	e.AssignCategory(assignment)
	return override, assignment, nil
}

// AssignCategory installs a new current CategoryAssignment, superseding the previous one for audit.
func (e *FinancialEvent) AssignCategory(a *CategoryAssignment) {
	if e.category != nil {
		e.category.supersede(a.id)
	}
	e.category = a
}

func (e *FinancialEvent) appendOverride(field string, prev, next any, actor string, now time.Time) (*EventOverride, error) {
	o, err := NewEventOverride(e.id, field, prev, next, actor, now)
	if err != nil {
		return nil, err
	}
	e.overrides = append(e.overrides, o)
	e.updatedAt = now.UTC()
	return o, nil
}
