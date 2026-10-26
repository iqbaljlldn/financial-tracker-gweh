package domain

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Overridable field names (04_domain_model.md §5.5).
const (
	OverrideFieldAmount    = "amount"
	OverrideFieldMerchant  = "merchant"
	OverrideFieldEventType = "eventType"
	OverrideFieldCategory  = "category"
)

// IsOverridableField reports whether a field name may be corrected via EventOverride.
func IsOverridableField(field string) bool {
	switch field {
	case OverrideFieldAmount, OverrideFieldMerchant, OverrideFieldEventType, OverrideFieldCategory:
		return true
	}
	return false
}

// EventOverride is the append-only audit entity for a single manual correction (04_domain_model.md §5.5).
// It never deletes or overwrites the original extracted value; the effective value is the latest
// override for a field, else the original (SRS-FR-LEDGER-02).
type EventOverride struct {
	id             string
	financialEvent string
	fieldName      string
	previousValue  json.RawMessage
	newValue       json.RawMessage
	actorUserID    string
	correctedAt    time.Time
}

// EventOverrideSnapshot carries persisted state for rehydration.
type EventOverrideSnapshot struct {
	ID               string
	FinancialEventID string
	FieldName        string
	PreviousValue    json.RawMessage
	NewValue         json.RawMessage
	ActorUserID      string
	CorrectedAt      time.Time
}

// NewEventOverride builds an override record; snapshots must be valid JSON.
func NewEventOverride(eventID, field string, previous, next any, actor string, now time.Time) (*EventOverride, error) {
	if !IsOverridableField(field) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidOverrideField, field)
	}
	prev, err := json.Marshal(previous)
	if err != nil {
		return nil, fmt.Errorf("%w: previous value: %v", ErrInvalidOverrideValue, err)
	}
	nxt, err := json.Marshal(next)
	if err != nil {
		return nil, fmt.Errorf("%w: new value: %v", ErrInvalidOverrideValue, err)
	}
	return &EventOverride{
		id:             uuid.New().String(),
		financialEvent: eventID,
		fieldName:      field,
		previousValue:  prev,
		newValue:       nxt,
		actorUserID:    actor,
		correctedAt:    now.UTC(),
	}, nil
}

// RehydrateEventOverride rebuilds an override from persisted state.
func RehydrateEventOverride(s EventOverrideSnapshot) *EventOverride {
	return &EventOverride{
		id:             s.ID,
		financialEvent: s.FinancialEventID,
		fieldName:      s.FieldName,
		previousValue:  s.PreviousValue,
		newValue:       s.NewValue,
		actorUserID:    s.ActorUserID,
		correctedAt:    s.CorrectedAt,
	}
}

func (o *EventOverride) ID() string                     { return o.id }
func (o *EventOverride) FinancialEventID() string       { return o.financialEvent }
func (o *EventOverride) FieldName() string              { return o.fieldName }
func (o *EventOverride) PreviousValue() json.RawMessage { return o.previousValue }
func (o *EventOverride) NewValue() json.RawMessage      { return o.newValue }
func (o *EventOverride) ActorUserID() string            { return o.actorUserID }
func (o *EventOverride) CorrectedAt() time.Time         { return o.correctedAt }
