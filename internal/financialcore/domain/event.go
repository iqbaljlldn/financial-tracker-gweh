package domain

import (
	"encoding/json"
	"time"
)

func jsonString(raw json.RawMessage, out *string) error {
	return json.Unmarshal(raw, out)
}

// Domain event names (04_domain_model.md §5.8).
const (
	// EventFinancialEventCandidateCreated is consumed (emitted by understanding); never emitted here.
	EventFinancialEventCandidateCreated = "FinancialEventCandidateCreated"

	EventFinancialEventPendingReview = "FinancialEventPendingReview"
	EventFinancialEventFinalized     = "FinancialEventFinalized"
	EventFinancialEventOverridden    = "FinancialEventOverridden"
	EventFinancialEventArchived      = "FinancialEventArchived"
	EventCategoryAssignmentChanged   = "CategoryAssignmentChanged"
)

// Aggregate type labels used in outbox_events.aggregate_type.
const (
	AggregateTypeFinancialEvent = "financialcore.financial_event"
)

// Finalization routes recorded on FinancialEventFinalized.
const (
	FinalizedViaGate         = "confidence_gate"
	FinalizedViaConfirmation = "user_confirmation"
	FinalizedViaManualEntry  = "manual_entry"
)

// FinancialEventPendingReviewPayload is emitted when confidence is below threshold; consumed by
// Notification (review request) and Administration. It carries the candidate-time category suggestion (H5).
type FinancialEventPendingReviewPayload struct {
	EventID             string    `json:"eventId"`
	UserID              string    `json:"userId"`
	EventType           string    `json:"eventType"`
	Amount              Money     `json:"amount"`
	OccurredAt          time.Time `json:"occurredAt"`
	Merchant            string    `json:"merchant"`
	SuggestedCategory   string    `json:"suggestedCategory,omitempty"`
	ConfidenceAggregate float64   `json:"confidenceAggregate"`
	Threshold           float64   `json:"threshold"`
	EmittedAt           time.Time `json:"emittedAt"`
}

// FinancialEventFinalizedPayload is emitted when an event reaches `finalized`; consumed by Analytics,
// Budget and Notification. It carries effective (override-aware) values.
type FinancialEventFinalizedPayload struct {
	EventID             string    `json:"eventId"`
	UserID              string    `json:"userId"`
	EventType           string    `json:"eventType"`
	Amount              Money     `json:"amount"`
	OccurredAt          time.Time `json:"occurredAt"`
	Merchant            string    `json:"merchant"`
	Category            string    `json:"category,omitempty"`
	ConfidenceMethod    string    `json:"confidenceMethod"`
	ConfidenceAggregate float64   `json:"confidenceAggregate"`
	FinalizedVia        string    `json:"finalizedVia"`
	EmittedAt           time.Time `json:"emittedAt"`
}

// OverrideRef summarizes one override inside FinancialEventOverriddenPayload.
type OverrideRef struct {
	OverrideID    string          `json:"overrideId"`
	FieldName     string          `json:"fieldName"`
	PreviousValue json.RawMessage `json:"previousValue"`
	NewValue      json.RawMessage `json:"newValue"`
}

// FinancialEventOverriddenPayload is emitted when a user submits a manual correction.
type FinancialEventOverriddenPayload struct {
	EventID   string        `json:"eventId"`
	UserID    string        `json:"userId"`
	Status    string        `json:"status"`
	Overrides []OverrideRef `json:"overrides"`
	EmittedAt time.Time     `json:"emittedAt"`
}

// FinancialEventArchivedPayload is emitted when an event is soft-deleted (INV-4).
type FinancialEventArchivedPayload struct {
	EventID   string    `json:"eventId"`
	UserID    string    `json:"userId"`
	Reason    string    `json:"reason"`
	EmittedAt time.Time `json:"emittedAt"`
}

// CategoryAssignmentChangedPayload is emitted whenever a new current category is applied.
type CategoryAssignmentChangedPayload struct {
	EventID          string    `json:"eventId"`
	UserID           string    `json:"userId"`
	Category         string    `json:"category"`
	PreviousCategory string    `json:"previousCategory,omitempty"`
	Source           string    `json:"source"`
	Confidence       float64   `json:"confidence"`
	Merchant         string    `json:"merchant"`
	EmittedAt        time.Time `json:"emittedAt"`
}

// NewFinalizedPayload builds a FinancialEventFinalized payload from the effective view.
func NewFinalizedPayload(e *FinancialEvent, via string, now time.Time) FinancialEventFinalizedPayload {
	p := FinancialEventFinalizedPayload{
		EventID:             e.id,
		UserID:              e.userID,
		EventType:           string(e.EffectiveEventType()),
		Amount:              e.EffectiveAmount(),
		OccurredAt:          e.occurredAt,
		Merchant:            e.EffectiveMerchant().DisplayName(),
		ConfidenceMethod:    string(e.confidenceMethod),
		ConfidenceAggregate: e.confidence.Aggregate,
		FinalizedVia:        via,
		EmittedAt:           now.UTC(),
	}
	if e.category != nil {
		p.Category = e.category.category
	}
	return p
}

// NewPendingReviewPayload builds a FinancialEventPendingReview payload.
func NewPendingReviewPayload(e *FinancialEvent, threshold float64, now time.Time) FinancialEventPendingReviewPayload {
	p := FinancialEventPendingReviewPayload{
		EventID:             e.id,
		UserID:              e.userID,
		EventType:           string(e.eventType),
		Amount:              e.amount,
		OccurredAt:          e.occurredAt,
		Merchant:            e.merchant.DisplayName(),
		ConfidenceAggregate: e.confidence.Aggregate,
		Threshold:           threshold,
		EmittedAt:           now.UTC(),
	}
	if e.category != nil {
		p.SuggestedCategory = e.category.category
	}
	return p
}

// NewCategoryChangedPayload builds a CategoryAssignmentChanged payload.
func NewCategoryChangedPayload(e *FinancialEvent, previous string, now time.Time) CategoryAssignmentChangedPayload {
	return CategoryAssignmentChangedPayload{
		EventID:          e.id,
		UserID:           e.userID,
		Category:         e.category.category,
		PreviousCategory: previous,
		Source:           e.category.source,
		Confidence:       e.category.confidence,
		Merchant:         e.EffectiveMerchant().DisplayName(),
		EmittedAt:        now.UTC(),
	}
}
