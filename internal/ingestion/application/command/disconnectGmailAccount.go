package command

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/ingestion/domain"
)

var (
	ErrAccountForbidden = errors.New("forbidden: account does not belong to user")
)

// DisconnectGmailAccountCommand requests disconnecting a mailbox with explicit data retention choice per FR-USER-02.
type DisconnectGmailAccountCommand struct {
	AccountID string
	UserID    string // Mandatory row-level ownership parameter [GATE]
	Purge     bool   // Explicit user choice: keep existing transactions vs. GDPR purge
}

// DisconnectGmailAccountResult summarizes disconnection outcome.
type DisconnectGmailAccountResult struct {
	AccountID string
	Status    domain.AccountStatus
	Purged    bool
}

// DisconnectGmailAccountHandler stops watches, marks account disconnected, and triggers optional GDPR cascade.
type DisconnectGmailAccountHandler struct {
	uow         domain.UnitOfWork
	accountRepo domain.GmailAccountRepository
	connector   domain.Connector
	logger      *slog.Logger
}

// NewDisconnectGmailAccountHandler constructs a DisconnectGmailAccountHandler.
func NewDisconnectGmailAccountHandler(
	uow domain.UnitOfWork,
	accountRepo domain.GmailAccountRepository,
	connector domain.Connector,
	logger *slog.Logger,
) *DisconnectGmailAccountHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &DisconnectGmailAccountHandler{
		uow:         uow,
		accountRepo: accountRepo,
		connector:   connector,
		logger:      logger,
	}
}

// Handle executes account disconnection and outbox event persistence per 07_sequence_diagrams.md Diagram 8.
func (h *DisconnectGmailAccountHandler) Handle(ctx context.Context, cmd DisconnectGmailAccountCommand) (*DisconnectGmailAccountResult, error) {
	if strings.TrimSpace(cmd.AccountID) == "" || strings.TrimSpace(cmd.UserID) == "" {
		return nil, domain.ErrEmptyAccountFields
	}

	account, err := h.accountRepo.FindByID(ctx, cmd.AccountID)
	if err != nil {
		return nil, fmt.Errorf("find account for disconnect: %w", err)
	}

	// Mandatory row-level ownership convention [GATE]
	if account.UserID() != strings.TrimSpace(cmd.UserID) {
		return nil, ErrAccountForbidden
	}

	connectorAcct := &domain.ConnectorAccount{
		ID:           account.ID(),
		EmailAddress: account.EmailAddress(),
		Provider:     "gmail",
	}

	// Best-effort attempt to stop Gmail push watch
	if err := h.connector.Disconnect(ctx, connectorAcct); err != nil {
		h.logger.Warn("disconnect call to provider returned error, continuing with local transition",
			"accountId", account.ID(),
			"error", err,
		)
	}

	err = h.uow.ExecuteTx(ctx, func(tx *domain.TxRepositories) error {
		account.MarkDisconnected()
		if err := tx.Accounts.Update(ctx, account); err != nil {
			return fmt.Errorf("update account disconnected status: %w", err)
		}

		payload := domain.AccountDisconnectedPayload{
			GmailAccountID: account.ID(),
			UserID:         account.UserID(),
			Reason:         "user_requested",
			PurgeRequested: cmd.Purge,
			OccurredAt:     time.Now().UTC(),
		}
		if err := tx.Outbox.SaveEvent(ctx, "ingestion.gmail_account", account.ID(), domain.EventAccountDisconnected, payload); err != nil {
			return fmt.Errorf("save account disconnected outbox event: %w", err)
		}

		if cmd.Purge {
			// Trigger GDPR cascade per 07_sequence_diagrams.md Diagram 8
			purgePayload := domain.UserDataPurgeRequestedPayload{
				GmailAccountID: account.ID(),
				UserID:         account.UserID(),
				OccurredAt:     time.Now().UTC(),
			}
			if err := tx.Outbox.SaveEvent(ctx, "ingestion.gmail_account", account.ID(), domain.EventUserDataPurgeRequested, purgePayload); err != nil {
				return fmt.Errorf("save user data purge requested event: %w", err)
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	h.logger.Info("gmail account disconnected successfully",
		"accountId", account.ID(),
		"userId", account.UserID(),
		"purged", cmd.Purge,
	)

	return &DisconnectGmailAccountResult{
		AccountID: account.ID(),
		Status:    domain.StatusDisconnected,
		Purged:    cmd.Purge,
	}, nil
}
