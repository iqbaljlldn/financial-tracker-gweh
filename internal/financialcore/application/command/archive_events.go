package command

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/financialcore/domain"
)

// ArchiveFinancialEventsHandler archives financial events (INV-4) either by IDs or by connector account.
// Status change and outbox rows are persisted in a single transaction [GATE].
type ArchiveFinancialEventsHandler struct {
	uow    domain.UnitOfWork
	logger *slog.Logger
	nowFn  func() time.Time
}

// NewArchiveFinancialEventsHandler constructs the archive handler.
func NewArchiveFinancialEventsHandler(
	uow domain.UnitOfWork,
	logger *slog.Logger,
) *ArchiveFinancialEventsHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &ArchiveFinancialEventsHandler{
		uow:    uow,
		logger: logger,
		nowFn:  time.Now,
	}
}

// Handle executes the batch archiving.
func (h *ArchiveFinancialEventsHandler) Handle(ctx context.Context, cmd ArchiveFinancialEventsCommand) ([]string, error) {
	if cmd.UserID == "" {
		return nil, domain.ErrUserIDRequired
	}
	if len(cmd.EventIDs) == 0 && cmd.ConnectorAccountID == "" {
		return nil, nil
	}

	reason := cmd.Reason
	if reason == "" {
		reason = "manual_archive"
	}

	now := h.nowFn()
	var archivedIDs []string

	err := h.uow.ExecuteTx(ctx, func(tx *domain.TxRepositories) error {
		var (
			ids []string
			err error
		)
		if len(cmd.EventIDs) > 0 {
			ids, err = tx.Events.ArchiveByIDs(ctx, cmd.UserID, cmd.EventIDs, now)
		} else {
			ids, err = tx.Events.ArchiveByConnectorAccount(ctx, cmd.UserID, cmd.ConnectorAccountID, now)
		}
		if err != nil {
			return err
		}

		for _, id := range ids {
			payload := domain.FinancialEventArchivedPayload{
				EventID:   id,
				UserID:    cmd.UserID,
				Reason:    reason,
				EmittedAt: now.UTC(),
			}
			if err := tx.Outbox.Save(ctx, domain.AggregateTypeFinancialEvent, id, domain.EventFinancialEventArchived, payload); err != nil {
				return fmt.Errorf("saving archived outbox event for %s: %w", id, err)
			}
		}

		archivedIDs = ids
		return nil
	})
	if err != nil {
		return nil, err
	}

	return archivedIDs, nil
}
