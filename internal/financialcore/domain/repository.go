package domain

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

// EventRepository is the mutating persistence port for the FinancialEvent aggregate.
// Every method is invoked inside a UnitOfWork transaction together with the Outbox write.
type EventRepository interface {
	// InsertIfNew persists a new event with ON CONFLICT (connector_account_id, source_message_ref) DO NOTHING.
	// It returns inserted=false (and no error) when INV-3's DB uniqueness constraint absorbs a redelivery.
	InsertIfNew(ctx context.Context, e *FinancialEvent) (inserted bool, err error)
	// FindByIDForUpdate loads the aggregate (with overrides and current category) under a row lock.
	FindByIDForUpdate(ctx context.Context, userID, id string) (*FinancialEvent, error)
	// UpdateStatus persists status and updated_at of an already-loaded aggregate.
	UpdateStatus(ctx context.Context, e *FinancialEvent) error
	// InsertOverride appends an override audit record.
	InsertOverride(ctx context.Context, o *EventOverride, actorUserID string) error
	// SaveCategoryAssignment inserts a new current assignment and supersedes any previous current one.
	SaveCategoryAssignment(ctx context.Context, eventID string, a *CategoryAssignment) error
	// ArchiveByIDs archives the user's non-archived events among ids and returns the archived IDs.
	ArchiveByIDs(ctx context.Context, userID string, ids []string, now time.Time) ([]string, error)
	// ArchiveByConnectorAccount archives all of the user's non-archived events from one connector account.
	ArchiveByConnectorAccount(ctx context.Context, userID, connectorAccountID string, now time.Time) ([]string, error)
}

// OutboxRepository writes outbox rows in the caller's transaction. Implementations must embed the
// trace context of ctx into every payload at write time [GATE: 17_observability.md §5.2].
type OutboxRepository interface {
	Save(ctx context.Context, aggregateType, aggregateID, eventType string, payload any) error
}

// InboxRepository implements the inbox_processed idempotency guard (06_architecture.md §6.2).
type InboxRepository interface {
	// TryRecord inserts the task ID; first=false means it was already processed (duplicate delivery or double-tap).
	TryRecord(ctx context.Context, taskID string, summary any) (first bool, err error)
}

// TxRepositories groups repositories bound to one active transaction.
type TxRepositories struct {
	Events EventRepository
	Outbox OutboxRepository
	Inbox  InboxRepository
}

// UnitOfWork runs fn in one database transaction: a command's state change and its Outbox rows
// commit or roll back together [GATE].
type UnitOfWork interface {
	ExecuteTx(ctx context.Context, fn func(tx *TxRepositories) error) error
}

// CategorySuggester is the port to the categorization policy (AssignCategory, 11_ai_blueprint.md §7).
// It is implemented at the composition root; financialcore never imports the understanding module.
type CategorySuggester interface {
	Suggest(ctx context.Context, userID, normalizedMerchant string) (CategorySuggestion, bool)
}

// CategoryFeedback is the port that feeds manual category corrections back into the user_history tier.
type CategoryFeedback interface {
	RecordUserCorrection(ctx context.Context, userID, normalizedMerchant, category string) error
}

// ListFilter parameterizes the ledger listing. UserID is mandatory.
type ListFilter struct {
	UserID    string
	From      *time.Time
	To        *time.Time
	Status    string
	EventType string
	Category  string
	Merchant  string
	Cursor    *Cursor
	Limit     int
}

// Cursor is the keyset pagination position (occurred_at DESC, id DESC).
type Cursor struct {
	OccurredAt time.Time `json:"t"`
	ID         string    `json:"id"`
}

// Encode renders an opaque cursor token.
func (c Cursor) Encode() string {
	raw, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(raw)
}

// DecodeCursor parses an opaque cursor token.
func DecodeCursor(token string) (*Cursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidCursor, err)
	}
	var c Cursor
	if err := json.Unmarshal(raw, &c); err != nil || c.ID == "" || c.OccurredAt.IsZero() {
		return nil, ErrInvalidCursor
	}
	return &c, nil
}

// ListResult is one cursor page of ledger events.
type ListResult struct {
	Items      []*FinancialEvent
	NextCursor string
	HasMore    bool
}

// EventReader is the READ-ONLY query port. It deliberately has no mutating methods so query handlers
// cannot perform writes (CQRS discipline; asserted by a reflection test).
type EventReader interface {
	FindByID(ctx context.Context, userID, id string) (*FinancialEvent, error)
	List(ctx context.Context, f ListFilter) (*ListResult, error)
	ListOverrides(ctx context.Context, userID, eventID string) ([]*EventOverride, error)
}

// PendingReviewMonitor is the read-only port backing the pending-review SLA monitor.
type PendingReviewMonitor interface {
	CountPendingReview(ctx context.Context) (int, error)
	// CountPendingReviewCrossedSLA counts pending events created in [notBefore, notAfter).
	CountPendingReviewCrossedSLA(ctx context.Context, notBefore, notAfter time.Time) (int, error)
}
