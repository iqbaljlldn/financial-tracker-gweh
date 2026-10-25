package command

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	identityDomain "github.com/iqbaljlldn/financial-tracker-gweh/internal/identity/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/ingestion/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/observability"
)

// RunIncrementalSyncCommand carries parameters for delta synchronization.
// Symmetrically triggered by both Pub/Sub push and poll-fallback scheduler per 12_gmail_connector.md §10 [GATE].
type RunIncrementalSyncCommand struct {
	AccountID string
	Trigger   string // "push" or "poll_fallback"
}

// RunIncrementalSyncResult summarizes processed messages and cursor progression.
type RunIncrementalSyncResult struct {
	AccountID             string
	MessagesExaminedCount int
	MessagesIngestedCount int
	NewHistoryID          string
	FallbackTriggered     bool
}

type syncEntry struct {
	msg      *domain.IngestedMessage
	decision domain.RoutingDecision
	isFailed bool
}

// RunIncrementalSyncHandler executes incremental sync against Gmail History API.
type RunIncrementalSyncHandler struct {
	uow                  domain.UnitOfWork
	accountRepo          domain.GmailAccountRepository
	connector            domain.Connector
	classifier           domain.SenderClassifier
	fallbackLookbackDays int
	logger               *slog.Logger
}

// NewRunIncrementalSyncHandler constructs a RunIncrementalSyncHandler.
func NewRunIncrementalSyncHandler(
	uow domain.UnitOfWork,
	accountRepo domain.GmailAccountRepository,
	connector domain.Connector,
	classifier domain.SenderClassifier,
	fallbackDays int,
	logger *slog.Logger,
) *RunIncrementalSyncHandler {
	if classifier == nil {
		classifier = domain.NewClassifier()
	}
	if fallbackDays <= 0 {
		fallbackDays = 3 // Bounded fallback re-sync (default 3 days per 12_gmail_connector.md §6.2)
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &RunIncrementalSyncHandler{
		uow:                  uow,
		accountRepo:          accountRepo,
		connector:            connector,
		classifier:           classifier,
		fallbackLookbackDays: fallbackDays,
		logger:               logger,
	}
}

// Handle executes incremental sync for a Gmail mailbox.
func (h *RunIncrementalSyncHandler) Handle(ctx context.Context, cmd RunIncrementalSyncCommand) (*RunIncrementalSyncResult, error) {
	if strings.TrimSpace(cmd.AccountID) == "" {
		return nil, domain.ErrEmptyAccountFields
	}

	account, err := h.accountRepo.FindByID(ctx, cmd.AccountID)
	if err != nil {
		return nil, fmt.Errorf("find account for incremental sync: %w", err)
	}

	if account.Status() != domain.StatusActive {
		h.logger.Debug("skipping incremental sync: account not active",
			"accountId", account.ID(),
			"status", account.Status(),
		)
		return &RunIncrementalSyncResult{AccountID: account.ID()}, nil
	}

	connectorAcct := &domain.ConnectorAccount{
		ID:           account.ID(),
		EmailAddress: account.EmailAddress(),
		Provider:     "gmail",
	}

	batch, err := h.connector.FetchIncremental(ctx, connectorAcct, account.LastHistoryID())
	if err != nil {
		return nil, h.handleFetchError(ctx, account, err)
	}

	var rawMessages []*domain.RawMessage
	var newCursor string
	var fallbackTriggered bool

	if batch.ExpiredState {
		// 404 / expired historyId: bounded fallback re-sync, NOT full 90-day re-scan per §6.2 [GATE]
		observability.GetMetrics().HistorySyncFallbackTotal.WithLabelValues("historyid_expired").Inc()
		fallbackTriggered = true

		h.logger.Warn("historyId expired, executing bounded fallback re-sync",
			"accountId", account.ID(),
			"lastHistoryId", account.LastHistoryID(),
			"fallbackDays", h.fallbackLookbackDays,
		)

		fallbackBatch, err := h.connector.InitialSync(ctx, connectorAcct, h.fallbackLookbackDays, "")
		if err != nil {
			return nil, fmt.Errorf("bounded fallback sync failed: %w", err)
		}

		rawMessages = fallbackBatch.Messages
		newCursor = fallbackBatch.HistoryID
	} else {
		rawMessages = batch.Messages
		newCursor = batch.NewCursor
	}

	entries := h.prepareSyncEntries(account, rawMessages)
	ingestedCount, err := h.persistSyncEntries(ctx, account, entries, newCursor, fallbackTriggered)
	if err != nil {
		return nil, fmt.Errorf("persist sync batch: %w", err)
	}

	return &RunIncrementalSyncResult{
		AccountID:             account.ID(),
		MessagesExaminedCount: len(rawMessages),
		MessagesIngestedCount: ingestedCount,
		NewHistoryID:          newCursor,
		FallbackTriggered:     fallbackTriggered,
	}, nil
}

func (h *RunIncrementalSyncHandler) prepareSyncEntries(
	account *domain.GmailAccount,
	rawMessages []*domain.RawMessage,
) []syncEntry {
	entries := make([]syncEntry, 0, len(rawMessages))
	for _, raw := range rawMessages {
		if raw.ParseFailed {
			// Malformed MIME structure isolated per 12_gmail_connector.md §9 [GATE]
			msg, err := domain.NewIngestedMessage(
				uuid.New().String(),
				account.ID(),
				raw.ExternalID,
				raw.InternalDate,
				"",
				domain.RoutingDiscarded,
			)
			if err == nil {
				msg.MarkFailed()
				entries = append(entries, syncEntry{msg: msg, isFailed: true})
			}
			continue
		}

		decision := h.classifier.Classify(raw, false)
		msg, err := domain.NewIngestedMessage(
			uuid.New().String(),
			account.ID(),
			raw.ExternalID,
			raw.InternalDate,
			raw.Snippet,
			decision,
		)
		if err != nil {
			continue
		}
		if decision == domain.RoutingDiscarded {
			msg.MarkCompleted()
		}
		entries = append(entries, syncEntry{msg: msg, decision: decision, isFailed: false})
	}
	return entries
}

func (h *RunIncrementalSyncHandler) persistSyncEntries(
	ctx context.Context,
	account *domain.GmailAccount,
	entries []syncEntry,
	newCursor string,
	fallbackTriggered bool,
) (int, error) {
	ingestedCount := 0

	// Transactional persistence: updates last_history_id ONLY after full batch is recorded [GATE]
	err := h.uow.ExecuteTx(ctx, func(tx *domain.TxRepositories) error {
		if fallbackTriggered {
			payload := domain.HistorySyncFallbackTriggeredPayload{
				GmailAccountID: account.ID(),
				UserID:         account.UserID(),
				Reason:         "historyid_expired",
				LookbackDays:   h.fallbackLookbackDays,
				OccurredAt:     time.Now().UTC(),
			}
			if err := tx.Outbox.SaveEvent(ctx, "ingestion.gmail_account", account.ID(), domain.EventHistorySyncFallbackTriggered, payload); err != nil {
				return fmt.Errorf("save fallback outbox event: %w", err)
			}
		}

		for _, entry := range entries {
			isNew, err := tx.Messages.SaveIfNew(ctx, entry.msg)
			if err != nil {
				return fmt.Errorf("save ingested message %s: %w", entry.msg.GmailMessageID(), err)
			}
			if !isNew || entry.isFailed || entry.decision == domain.RoutingDiscarded {
				continue
			}

			payload := domain.MessageIngestedPayload{
				MessageID:       entry.msg.ID(),
				GmailAccountID:  account.ID(),
				GmailMessageID:  entry.msg.GmailMessageID(),
				ReceivedAt:      entry.msg.ReceivedAt(),
				RoutingDecision: entry.decision,
				RawContentRef:   entry.msg.RawContentRef(),
				OccurredAt:      time.Now().UTC(),
			}

			if err := tx.Outbox.SaveEvent(ctx, "ingestion.ingested_message", entry.msg.ID(), domain.EventMessageIngested, payload); err != nil {
				return fmt.Errorf("save message ingested event: %w", err)
			}

			observability.GetMetrics().MessagesIngestedTotal.WithLabelValues(string(entry.decision)).Inc()
			ingestedCount++

			// Observe sync lag
			lag := time.Since(entry.msg.ReceivedAt()).Seconds()
			if lag > 0 {
				observability.GetMetrics().SyncLagSeconds.WithLabelValues(account.ID()).Observe(lag)
			}
		}

		account.UpdateHistoryID(newCursor)
		return tx.Accounts.Update(ctx, account)
	})

	return ingestedCount, err
}

func (h *RunIncrementalSyncHandler) handleFetchError(ctx context.Context, account *domain.GmailAccount, err error) error {
	if errors.Is(err, identityDomain.ErrRevocationDetected) || strings.Contains(strings.ToLower(err.Error()), "invalid_grant") {
		h.logger.Warn("oauth grant revoked during incremental sync", "accountId", account.ID())
		_ = h.uow.ExecuteTx(ctx, func(tx *domain.TxRepositories) error {
			account.MarkDisconnected()
			if updateErr := tx.Accounts.Update(ctx, account); updateErr != nil {
				return updateErr
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
	} else if strings.Contains(strings.ToLower(err.Error()), "oauth token not found") {
		h.logger.Warn("incremental sync aborted: no OAuth credentials found for account",
			slog.String("accountId", account.ID()),
			slog.String("email", account.EmailAddress()),
			slog.String("reason", "Account has not completed Google OAuth authentication or token has been purged"),
		)
	}
	return err
}
