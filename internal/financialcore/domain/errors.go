package domain

import "errors"

// Sentinel domain errors for the financialcore bounded context.
// Callers map these to HTTP codes / non-retryable task failures; they never wrap infrastructure errors.
var (
	// ErrConfidenceNotEvaluated is returned when a state transition is attempted without
	// an evaluated confidence score (INV-1, 04_domain_model.md §5.7).
	ErrConfidenceNotEvaluated = errors.New("financial event: confidence has not been evaluated against the gate threshold (INV-1)")

	// ErrBelowThreshold is returned when finalization is attempted below the confidence threshold
	// without explicit user confirmation (INV-1).
	ErrBelowThreshold = errors.New("financial event: confidence is below the gate threshold; user confirmation required (INV-1)")

	// ErrCurrencyImmutable is returned when an override attempts to change amount.currency (INV-2).
	ErrCurrencyImmutable = errors.New("financial event: amount.currency is immutable; cross-currency corrections require a disputed review case (INV-2)")

	// ErrEventArchived is returned when mutating an archived event (INV-4).
	ErrEventArchived = errors.New("financial event: event is archived and cannot be modified (INV-4)")

	// ErrInvalidStatusTransition is returned for illegal lifecycle transitions.
	ErrInvalidStatusTransition = errors.New("financial event: invalid status transition")

	// ErrInvalidEventType is returned for event types outside the taxonomy (04_domain_model.md §5.2).
	ErrInvalidEventType = errors.New("financial event: unsupported event type")

	// ErrInvalidAmount is returned for non-decimal or non-positive monetary amounts.
	ErrInvalidAmount = errors.New("financial event: amount must be a positive decimal string with at most 4 fractional digits")

	// ErrInvalidCurrency is returned for non ISO-4217-shaped currency codes.
	ErrInvalidCurrency = errors.New("financial event: currency must be a 3-letter uppercase ISO 4217 code")

	// ErrInvalidCandidate wraps any structurally invalid candidate; it is non-retryable.
	ErrInvalidCandidate = errors.New("financial event: invalid candidate")

	// ErrInvalidPayload is returned when type-specific payload attributes violate their documented shape.
	ErrInvalidPayload = errors.New("financial event: invalid type-specific payload")

	// ErrInvalidOverrideField is returned when an override targets an unsupported field.
	ErrInvalidOverrideField = errors.New("event override: unsupported field")

	// ErrInvalidOverrideValue is returned when an override value is malformed.
	ErrInvalidOverrideValue = errors.New("event override: invalid value")

	// ErrNoOverrideChange is returned when an override equals the current effective value.
	ErrNoOverrideChange = errors.New("event override: new value equals the current effective value")

	// ErrInvalidCategory is returned for category assignments with unsupported source or empty category.
	ErrInvalidCategory = errors.New("category assignment: invalid category or source")

	// ErrEventNotFound is returned when an event does not exist for the requesting user.
	ErrEventNotFound = errors.New("financial event: not found")

	// ErrNotPendingReview is returned when confirm is requested for an event that is not pending review.
	ErrNotPendingReview = errors.New("financial event: event is not pending review")

	// ErrUserIDRequired is returned when a query omits the mandatory userID scope.
	ErrUserIDRequired = errors.New("financial event: userID is required for every query")

	// ErrInvalidCursor is returned for malformed pagination cursors.
	ErrInvalidCursor = errors.New("financial event: invalid pagination cursor")
)
