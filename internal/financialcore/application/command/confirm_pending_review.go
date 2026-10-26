package command

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/financialcore/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/observability"
)

// ConfirmPendingReviewHandler transitions an event from pending_review to finalized with user confirmation.
// It is protected by inbox_processed idempotency (double-tap safety) and persists state changes
// and outbox rows in a single transaction [GATE].
type ConfirmPendingReviewHandler struct {
	uow       domain.UnitOfWork
	threshold float64
	feedback  domain.CategoryFeedback
	logger    *slog.Logger
	nowFn     func() time.Time
}

// NewConfirmPendingReviewHandler constructs the confirm handler.
func NewConfirmPendingReviewHandler(
	uow domain.UnitOfWork,
	threshold float64,
	feedback domain.CategoryFeedback,
	logger *slog.Logger,
) *ConfirmPendingReviewHandler {
	if threshold <= 0 {
		threshold = domain.DefaultConfidenceThreshold
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &ConfirmPendingReviewHandler{
		uow:       uow,
		threshold: threshold,
		feedback:  feedback,
		logger:    logger,
		nowFn:     time.Now,
	}
}

// Handle executes the confirmation and any accompanying overrides.
func (h *ConfirmPendingReviewHandler) Handle(ctx context.Context, cmd ConfirmPendingReviewCommand) (*domain.FinancialEvent, error) {
	if cmd.UserID == "" {
		return nil, domain.ErrUserIDRequired
	}
	if cmd.EventID == "" {
		return nil, domain.ErrEventNotFound
	}
	actorID := cmd.ActorUserID
	if actorID == "" {
		actorID = cmd.UserID
	}

	now := h.nowFn()
	var (
		event              *domain.FinancialEvent
		firstAttempt       bool
		categoryOverridden bool
	)

	err := h.uow.ExecuteTx(ctx, func(tx *domain.TxRepositories) error {
		var err error
		event, firstAttempt, categoryOverridden, err = h.executeConfirmTx(ctx, tx, cmd, actorID, now)
		return err
	})
	if err != nil {
		return nil, err
	}

	if firstAttempt {
		observability.GetMetrics().FinancialCoreEventsPendingReview.Dec()
		observability.GetMetrics().FinancialCoreEventsFinalizedTotal.WithLabelValues(string(event.EffectiveEventType()), "manual").Inc()

		if categoryOverridden && h.feedback != nil {
			_ = h.feedback.RecordUserCorrection(ctx, cmd.UserID, event.EffectiveMerchant().NormalizedName, event.Category().Category())
		}
	}

	return event, nil
}

func (h *ConfirmPendingReviewHandler) executeConfirmTx(ctx context.Context, tx *domain.TxRepositories, cmd ConfirmPendingReviewCommand, actorID string, now time.Time) (*domain.FinancialEvent, bool, bool, error) {
	taskID := "confirm:" + cmd.EventID
	if cmd.IdempotencyKey != "" {
		taskID = "confirm:" + cmd.IdempotencyKey
	}

	first, err := tx.Inbox.TryRecord(ctx, taskID, map[string]string{
		"eventId": cmd.EventID,
		"userId":  cmd.UserID,
	})
	if err != nil {
		return nil, false, false, fmt.Errorf("checking inbox idempotency: %w", err)
	}
	if !first {
		fe, err := tx.Events.FindByIDForUpdate(ctx, cmd.UserID, cmd.EventID)
		if err != nil {
			return nil, false, false, err
		}
		return fe, false, false, nil
	}

	fe, err := tx.Events.FindByIDForUpdate(ctx, cmd.UserID, cmd.EventID)
	if err != nil {
		return nil, false, false, err
	}
	if fe.Status() == domain.StatusFinalized {
		return fe, true, false, nil
	}
	if fe.Status() != domain.StatusPendingReview {
		return nil, false, false, domain.ErrNotPendingReview
	}

	categoryOverridden, prevCategory, err := applyConfirmCorrections(ctx, tx, fe, cmd.Corrections, actorID, now)
	if err != nil {
		return nil, false, false, err
	}

	decision := domain.GateDecision{
		Evaluated: true,
		Aggregate: fe.Confidence().Aggregate,
		Threshold: h.threshold,
		Passed:    fe.Confidence().Aggregate >= h.threshold,
	}
	if err := fe.Finalize(decision, true, now); err != nil {
		return nil, false, false, err
	}

	if err := tx.Events.UpdateStatus(ctx, fe); err != nil {
		return nil, false, false, fmt.Errorf("updating event status: %w", err)
	}

	if err := saveConfirmOutbox(ctx, tx, fe, categoryOverridden, prevCategory, now); err != nil {
		return nil, false, false, err
	}

	return fe, true, categoryOverridden, nil
}

func applyConfirmCorrections(ctx context.Context, tx *domain.TxRepositories, fe *domain.FinancialEvent, c *OverrideFields, actorID string, now time.Time) (bool, string, error) {
	if c == nil {
		return false, "", nil
	}
	if _, err := applyAmountOverride(ctx, tx, fe, c.Amount, actorID, now); err != nil {
		return false, "", err
	}
	if _, err := applyMerchantOverride(ctx, tx, fe, c.Merchant, actorID, now); err != nil {
		return false, "", err
	}
	if _, err := applyEventTypeOverride(ctx, tx, fe, c.EventType, actorID, now); err != nil {
		return false, "", err
	}
	var prevCategory string
	if fe.Category() != nil {
		prevCategory = fe.Category().Category()
	}
	o, _, err := applyCategoryOverride(ctx, tx, fe, c.Category, actorID, now)
	if err != nil {
		return false, "", err
	}
	return o != nil, prevCategory, nil
}

func saveConfirmOutbox(ctx context.Context, tx *domain.TxRepositories, fe *domain.FinancialEvent, categoryOverridden bool, prevCategory string, now time.Time) error {
	finalizedPayload := domain.NewFinalizedPayload(fe, domain.FinalizedViaConfirmation, now)
	if err := tx.Outbox.Save(ctx, domain.AggregateTypeFinancialEvent, fe.ID(), domain.EventFinancialEventFinalized, finalizedPayload); err != nil {
		return fmt.Errorf("saving finalized outbox event: %w", err)
	}

	if categoryOverridden {
		catPayload := domain.NewCategoryChangedPayload(fe, prevCategory, now)
		if err := tx.Outbox.Save(ctx, domain.AggregateTypeFinancialEvent, fe.ID(), domain.EventCategoryAssignmentChanged, catPayload); err != nil {
			return fmt.Errorf("saving category changed outbox event: %w", err)
		}
	}
	return nil
}
