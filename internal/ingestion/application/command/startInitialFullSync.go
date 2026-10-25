package command

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/ingestion/domain"
)

// StartInitialFullSyncCommand executes the historical email synchronization per 12_gmail_connector.md §6.3.
type StartInitialFullSyncCommand struct {
	AccountID  string
	WindowDays int
	PageToken  string
}

// StartInitialFullSyncResult reports sync completion statistics.
type StartInitialFullSyncResult struct {
	AccountID             string
	MessagesIngestedCount int
	FinalHistoryID        string
	Status                domain.AccountStatus
}

type messageEntry struct {
	msg      *domain.IngestedMessage
	decision domain.RoutingDecision
}

// StartInitialFullSyncHandler coordinates bounded pagination, classification, and transactional persistence.
type StartInitialFullSyncHandler struct {
	uow         domain.UnitOfWork
	accountRepo domain.GmailAccountRepository
	connector   domain.Connector
	classifier  domain.SenderClassifier
	logger      *slog.Logger
}

// NewStartInitialFullSyncHandler creates a new StartInitialFullSyncHandler.
func NewStartInitialFullSyncHandler(
	uow domain.UnitOfWork,
	accountRepo domain.GmailAccountRepository,
	connector domain.Connector,
	classifier domain.SenderClassifier,
	logger *slog.Logger,
) *StartInitialFullSyncHandler {
	if classifier == nil {
		classifier = domain.NewClassifier()
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &StartInitialFullSyncHandler{
		uow:         uow,
		accountRepo: accountRepo,
		connector:   connector,
		classifier:  classifier,
		logger:      logger,
	}
}

// Handle executes the bounded initial sync with resumable per-page persistence [GATE].
func (h *StartInitialFullSyncHandler) Handle(ctx context.Context, cmd StartInitialFullSyncCommand) (*StartInitialFullSyncResult, error) {
	account, err := h.accountRepo.FindByID(ctx, cmd.AccountID)
	if err != nil {
		return nil, fmt.Errorf("lookup account %s: %w", cmd.AccountID, err)
	}

	windowDays := cmd.WindowDays
	if windowDays <= 0 {
		windowDays = account.InitialSyncWindowDays()
	}
	if windowDays <= 0 {
		windowDays = 90
	}

	// 1. Transition status: linking -> initial_sync
	if err := account.StartInitialSync(); err == nil {
		if err := h.accountRepo.Update(ctx, account); err != nil {
			h.logger.Warn("could not update account status to initial_sync", "error", err)
		}
	}

	connectorAccount := &domain.ConnectorAccount{
		ID:           account.ID(),
		EmailAddress: account.EmailAddress(),
		Provider:     "gmail",
	}

	totalIngested := 0
	lastHistoryID := account.LastHistoryID()
	pageToken := cmd.PageToken

	h.logger.Info("starting initial full sync",
		"account_id", account.ID(),
		"email", account.EmailAddress(),
		"window_days", windowDays,
	)

	// 2. Paginated message enumeration loop
	for {
		batch, err := h.connector.InitialSync(ctx, connectorAccount, windowDays, pageToken)
		if err != nil {
			return nil, fmt.Errorf("connector initial sync page error: %w", err)
		}

		if batch.HistoryID != "" {
			lastHistoryID = batch.HistoryID
		}

		entries := h.preparePageEntries(batch, account.ID())

		// 3. Transactional per-page persistence + outbox insertion [GATE]
		pageInsertedCount, err := h.persistPageEntries(ctx, account.ID(), entries)
		if err != nil {
			return nil, fmt.Errorf("commit page batch transaction: %w", err)
		}

		totalIngested += pageInsertedCount

		if !batch.HasMore || batch.NextPageToken == "" {
			break
		}
		pageToken = batch.NextPageToken
	}

	// 4. On completion: store final historyId baseline and transition status to active [GATE]
	if err := h.finalizeSync(ctx, account, totalIngested, lastHistoryID); err != nil {
		return nil, fmt.Errorf("finalize sync state: %w", err)
	}

	h.logger.Info("initial full sync completed successfully",
		"account_id", account.ID(),
		"messages_ingested", totalIngested,
		"final_history_id", lastHistoryID,
		"status", account.Status(),
	)

	return &StartInitialFullSyncResult{
		AccountID:             account.ID(),
		MessagesIngestedCount: totalIngested,
		FinalHistoryID:        lastHistoryID,
		Status:                account.Status(),
	}, nil
}

func (h *StartInitialFullSyncHandler) preparePageEntries(batch *domain.SyncBatch, accountID string) []messageEntry {
	entries := make([]messageEntry, 0, len(batch.Messages))
	for _, raw := range batch.Messages {
		// Sender classification with permissive-on-ambiguity rule [GATE]
		decision := h.classifier.Classify(raw, false)
		if decision == domain.RoutingDiscarded {
			continue
		}

		ingestedMsg, err := domain.NewIngestedMessage(
			uuid.New().String(),
			accountID,
			raw.ExternalID,
			raw.InternalDate,
			"",
			decision,
		)
		if err != nil {
			continue
		}

		entries = append(entries, messageEntry{msg: ingestedMsg, decision: decision})
	}
	return entries
}

func (h *StartInitialFullSyncHandler) persistPageEntries(ctx context.Context, accountID string, entries []messageEntry) (int, error) {
	pageInsertedCount := 0
	err := h.uow.ExecuteTx(ctx, func(tx *domain.TxRepositories) error {
		for _, entry := range entries {
			inserted, err := tx.Messages.SaveIfNew(ctx, entry.msg)
			if err != nil {
				return fmt.Errorf("save ingested message %s: %w", entry.msg.GmailMessageID(), err)
			}

			// Only emit MessageIngested if this message was newly recorded
			if inserted {
				pageInsertedCount++
				payload := domain.MessageIngestedPayload{
					MessageID:       entry.msg.ID(),
					GmailAccountID:  accountID,
					GmailMessageID:  entry.msg.GmailMessageID(),
					ReceivedAt:      entry.msg.ReceivedAt(),
					RoutingDecision: entry.decision,
					RawContentRef:   entry.msg.RawContentRef(),
					OccurredAt:      time.Now().UTC(),
				}

				if err := tx.Outbox.SaveEvent(ctx, "ingestion.ingested_message", entry.msg.ID(), domain.EventMessageIngested, payload); err != nil {
					return fmt.Errorf("save outbox event for message %s: %w", entry.msg.GmailMessageID(), err)
				}
			}
		}
		return nil
	})
	return pageInsertedCount, err
}

func (h *StartInitialFullSyncHandler) finalizeSync(ctx context.Context, account *domain.GmailAccount, totalIngested int, lastHistoryID string) error {
	if err := account.CompleteInitialSync(lastHistoryID); err != nil {
		h.logger.Warn("could not complete initial sync transition", "error", err)
	}

	return h.uow.ExecuteTx(ctx, func(tx *domain.TxRepositories) error {
		if err := tx.Accounts.Update(ctx, account); err != nil {
			return err
		}

		syncCompletedPayload := domain.SyncCompletedPayload{
			GmailAccountID: account.ID(),
			UserID:         account.UserID(),
			MessagesCount:  totalIngested,
			FinalHistoryID: lastHistoryID,
			OccurredAt:     time.Now().UTC(),
		}

		return tx.Outbox.SaveEvent(ctx, "ingestion.gmail_account", account.ID(), domain.EventSyncCompleted, syncCompletedPayload)
	})
}
