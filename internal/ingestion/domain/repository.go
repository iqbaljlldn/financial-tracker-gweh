package domain

import (
	"context"
	"errors"
	"time"
)

var (
	ErrAccountNotFound    = errors.New("gmail account not found")
	ErrEmailAlreadyLinked = errors.New("email address is already linked to an account")
	ErrMessageNotFound    = errors.New("ingested message not found")
	ErrTransactionFailed  = errors.New("database transaction failed")
)

// GmailAccountRepository defines persistence operations for GmailAccount aggregates.
// Mandatory row-level ownership convention [GATE]: all user-scoped query methods require userID.
type GmailAccountRepository interface {
	Save(ctx context.Context, account *GmailAccount) error
	Update(ctx context.Context, account *GmailAccount) error
	FindByID(ctx context.Context, id string) (*GmailAccount, error)
	FindByEmail(ctx context.Context, email string) (*GmailAccount, error)
	ListByUserID(ctx context.Context, userID string) ([]*GmailAccount, error)
	ListActiveForWatchRenewal(ctx context.Context, before time.Time, limit int32) ([]*GmailAccount, error)
	ListActive(ctx context.Context) ([]*GmailAccount, error)
	RecordWatchLog(ctx context.Context, accountID string, event string) error
}

// IngestedMessageRepository defines persistence operations for IngestedMessage entities.
type IngestedMessageRepository interface {
	SaveBatch(ctx context.Context, messages []*IngestedMessage) error
	SaveIfNew(ctx context.Context, message *IngestedMessage) (bool, error)
	Exists(ctx context.Context, accountID, gmailMessageID string) (bool, error)
	FindByAccountAndMessageID(ctx context.Context, accountID, gmailMessageID string) (*IngestedMessage, error)
}

// OutboxRepository defines atomic event persistence for the Outbox pattern per 08_database_design.md §4.
type OutboxRepository interface {
	SaveEvent(ctx context.Context, aggregateType, aggregateID, eventType string, payload any) error
}

// UnitOfWork encapsulates transactional operations across multiple repository methods.
type UnitOfWork interface {
	ExecuteTx(ctx context.Context, fn func(txRepo *TxRepositories) error) error
}

// TxRepositories provides access to repositories bound to an active database transaction.
type TxRepositories struct {
	Accounts GmailAccountRepository
	Messages IngestedMessageRepository
	Outbox   OutboxRepository
}
