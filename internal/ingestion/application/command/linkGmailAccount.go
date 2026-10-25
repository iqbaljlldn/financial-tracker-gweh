package command

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/ingestion/domain"
)

// LinkGmailAccountCommand carries parameters needed to link a user's Gmail mailbox per 07_sequence_diagrams.md Diagram 1.
type LinkGmailAccountCommand struct {
	AccountID             string
	UserID                string
	EmailAddress          string
	InitialSyncWindowDays int
}

// LinkGmailAccountResult contains the persisted GmailAccount representation.
type LinkGmailAccountResult struct {
	Account *domain.GmailAccount
}

// LinkGmailAccountHandler coordinates account registration and outbox event persistence.
type LinkGmailAccountHandler struct {
	uow         domain.UnitOfWork
	accountRepo domain.GmailAccountRepository
}

// NewLinkGmailAccountHandler creates a new LinkGmailAccountHandler.
func NewLinkGmailAccountHandler(uow domain.UnitOfWork, accountRepo domain.GmailAccountRepository) *LinkGmailAccountHandler {
	return &LinkGmailAccountHandler{
		uow:         uow,
		accountRepo: accountRepo,
	}
}

// Handle executes account creation and atomic outbox publishing in a single transaction [GATE].
func (h *LinkGmailAccountHandler) Handle(ctx context.Context, cmd LinkGmailAccountCommand) (*LinkGmailAccountResult, error) {
	if strings.TrimSpace(cmd.UserID) == "" || strings.TrimSpace(cmd.EmailAddress) == "" {
		return nil, domain.ErrEmptyAccountFields
	}

	cleanEmail := strings.ToLower(strings.TrimSpace(cmd.EmailAddress))
	accountID := strings.TrimSpace(cmd.AccountID)
	if accountID == "" {
		// Use deterministic UUID based on email to align with Identity module's OAuth token store
		accountID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(cleanEmail)).String()
	}

	windowDays := cmd.InitialSyncWindowDays
	if windowDays <= 0 {
		windowDays = 90 // Default 90-day lookback window per 12_gmail_connector.md §6.3
	}

	account, err := domain.NewGmailAccount(accountID, cmd.UserID, cleanEmail, windowDays)
	if err != nil {
		return nil, fmt.Errorf("create gmail account aggregate: %w", err)
	}

	// Transactional persistence: insert account + outbox event atomically [GATE]
	err = h.uow.ExecuteTx(ctx, func(tx *domain.TxRepositories) error {
		if err := tx.Accounts.Save(ctx, account); err != nil {
			return err
		}

		payload := domain.GmailAccountLinkedPayload{
			GmailAccountID: account.ID(),
			UserID:         account.UserID(),
			EmailAddress:   account.EmailAddress(),
			OccurredAt:     time.Now().UTC(),
		}

		if err := tx.Outbox.SaveEvent(ctx, "ingestion.gmail_account", account.ID(), domain.EventGmailAccountLinked, payload); err != nil {
			return fmt.Errorf("save outbox event: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &LinkGmailAccountResult{
		Account: account,
	}, nil
}
