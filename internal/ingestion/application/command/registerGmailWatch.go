package command

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	identityDomain "github.com/iqbaljlldn/financial-tracker-gweh/internal/identity/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/ingestion/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/observability"
)

// RegisterGmailWatchCommand requests setting up or extending real-time push watch per 12_gmail_connector.md §5.1.
type RegisterGmailWatchCommand struct {
	AccountID string
}

// RegisterGmailWatchResult provides status of watch registration.
type RegisterGmailWatchResult struct {
	AccountID       string
	WatchExpiration time.Time
	HistoryID       string
	PollOnlyMode    bool
}

// RegisterGmailWatchHandler registers or renews Gmail users.watch with Pub/Sub.
type RegisterGmailWatchHandler struct {
	uow         domain.UnitOfWork
	accountRepo domain.GmailAccountRepository
	connector   domain.Connector
	logger      *slog.Logger
}

// NewRegisterGmailWatchHandler constructs a RegisterGmailWatchHandler.
func NewRegisterGmailWatchHandler(
	uow domain.UnitOfWork,
	accountRepo domain.GmailAccountRepository,
	connector domain.Connector,
	logger *slog.Logger,
) *RegisterGmailWatchHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &RegisterGmailWatchHandler{
		uow:         uow,
		accountRepo: accountRepo,
		connector:   connector,
		logger:      logger,
	}
}

// Handle executes watch registration against Gmail API.
// Idempotent: re-calling before expiration extends watch lease per §5.2.
// On failure: falls back to poll-only mode per §9, logging elevated alert [GATE].
func (h *RegisterGmailWatchHandler) Handle(ctx context.Context, cmd RegisterGmailWatchCommand) (*RegisterGmailWatchResult, error) {
	if strings.TrimSpace(cmd.AccountID) == "" {
		return nil, domain.ErrEmptyAccountFields
	}

	account, err := h.accountRepo.FindByID(ctx, cmd.AccountID)
	if err != nil {
		return nil, fmt.Errorf("find account: %w", err)
	}

	connectorAcct := &domain.ConnectorAccount{
		ID:           account.ID(),
		EmailAddress: account.EmailAddress(),
		Provider:     "gmail",
	}

	handle, err := h.connector.RegisterRealtimeWatch(ctx, connectorAcct)
	if err != nil {
		return h.handleWatchFailure(ctx, account, err)
	}

	// Watch succeeded
	isRenewal := account.WatchExpiration() != nil
	account.SetWatchExpiration(handle.Expiration, handle.HistoryID)

	if err := h.accountRepo.Update(ctx, account); err != nil {
		return nil, fmt.Errorf("update account watch expiration: %w", err)
	}

	logEvent := "registered"
	if isRenewal {
		logEvent = "renewed"
	}
	_ = h.accountRepo.RecordWatchLog(ctx, account.ID(), logEvent)
	observability.GetMetrics().WatchRenewalSuccessTotal.Inc()

	h.logger.Info("gmail watch registered successfully",
		"accountId", account.ID(),
		"expiration", handle.Expiration,
		"event", logEvent,
	)

	return &RegisterGmailWatchResult{
		AccountID:       account.ID(),
		WatchExpiration: handle.Expiration,
		HistoryID:       handle.HistoryID,
		PollOnlyMode:    false,
	}, nil
}

func (h *RegisterGmailWatchHandler) handleWatchFailure(ctx context.Context, account *domain.GmailAccount, watchErr error) (*RegisterGmailWatchResult, error) {
	_ = h.accountRepo.RecordWatchLog(ctx, account.ID(), "renewal_failed")
	observability.GetMetrics().WatchRenewalFailureTotal.Inc()

	// Check if failure is due to OAuth revocation
	if errors.Is(watchErr, identityDomain.ErrRevocationDetected) || strings.Contains(strings.ToLower(watchErr.Error()), "invalid_grant") {
		h.logger.Warn("oauth revocation detected during watch registration, disconnecting account",
			"accountId", account.ID(),
			"error", watchErr,
		)
		err := h.uow.ExecuteTx(ctx, func(tx *domain.TxRepositories) error {
			account.MarkDisconnected()
			if err := tx.Accounts.Update(ctx, account); err != nil {
				return err
			}
			payload := domain.AccountDisconnectedPayload{
				GmailAccountID: account.ID(),
				UserID:         account.UserID(),
				Reason:         "oauth_revoked",
				PurgeRequested: false,
				OccurredAt:     time.Now().UTC(),
			}
			return tx.Outbox.SaveEvent(ctx, "ingestion.gmail_account", account.ID(), domain.EventAccountDisconnected, payload)
		})
		if err != nil {
			return nil, fmt.Errorf("disconnect revoked account: %w", err)
		}
		return &RegisterGmailWatchResult{AccountID: account.ID(), PollOnlyMode: false}, nil
	}

	// Elevated severity alert: fallback to poll-only mode per 12_gmail_connector.md §9
	h.logger.Error("gmail watch registration failed, falling back to poll-only mode",
		"accountId", account.ID(),
		"error", watchErr,
	)

	return &RegisterGmailWatchResult{
		AccountID:    account.ID(),
		PollOnlyMode: true,
	}, nil
}
