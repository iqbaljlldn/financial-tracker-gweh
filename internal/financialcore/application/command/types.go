package command

import (
	"time"
)

// EvaluateConfidenceGateCommand carries a raw FinancialEventCandidateCreated outbox payload.
type EvaluateConfidenceGateCommand struct {
	Payload []byte
}

// OverrideFields specifies values to correct via EventOverride.
type OverrideFields struct {
	Amount    *AmountField
	Merchant  *string
	EventType *string
	Category  *string
}

// AmountField carries amount correction data.
type AmountField struct {
	Value    string
	Currency string
}

// ConfirmPendingReviewCommand requests finalization of a pending-review event with optional corrections.
type ConfirmPendingReviewCommand struct {
	UserID         string
	EventID        string
	IdempotencyKey string
	ActorUserID    string
	Corrections    *OverrideFields
}

// SubmitEventOverrideCommand records one or more manual field corrections without mutating original values.
type SubmitEventOverrideCommand struct {
	UserID      string
	EventID     string
	ActorUserID string
	Fields      OverrideFields
}

// ArchiveFinancialEventsCommand requests soft-deletion of events by IDs or connector account.
type ArchiveFinancialEventsCommand struct {
	UserID             string
	EventIDs           []string
	ConnectorAccountID string
	Reason             string
}

// CreateManualEventCommand carries data for user-entered transactions (POST /financial-events).
type CreateManualEventCommand struct {
	UserID            string
	EventType         string
	AmountValue       string
	AmountCurrency    string
	OccurredAt        time.Time
	MerchantName      string
	PaymentMethodType string
	PaymentMethodHint string
	Category          string
	Payload           map[string]any
}

// CheckPendingReviewSLACommand drives the SLA breach detector and backlog gauge.
type CheckPendingReviewSLACommand struct {
	Now time.Time
}
