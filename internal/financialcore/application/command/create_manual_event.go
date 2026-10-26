package command

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/financialcore/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/observability"
)

// CreateManualEventHandler handles manual creation of financial events (POST /financial-events).
// It forces extractionMethod=manual, records a gate decision, and commits event + outbox in one tx [GATE].
type CreateManualEventHandler struct {
	uow       domain.UnitOfWork
	threshold float64
	logger    *slog.Logger
	nowFn     func() time.Time
}

// NewCreateManualEventHandler constructs the manual event creation handler.
func NewCreateManualEventHandler(
	uow domain.UnitOfWork,
	threshold float64,
	logger *slog.Logger,
) *CreateManualEventHandler {
	if threshold <= 0 {
		threshold = domain.DefaultConfidenceThreshold
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &CreateManualEventHandler{
		uow:       uow,
		threshold: threshold,
		logger:    logger,
		nowFn:     time.Now,
	}
}

// Handle validates the manual input, creates the event aggregate, and commits it transactionally.
func (h *CreateManualEventHandler) Handle(ctx context.Context, cmd CreateManualEventCommand) (*domain.FinancialEvent, error) {
	if cmd.UserID == "" {
		return nil, domain.ErrUserIDRequired
	}

	amount, err := domain.NewMoney(cmd.AmountValue, cmd.AmountCurrency)
	if err != nil {
		return nil, err
	}

	now := h.nowFn()

	params := domain.ManualEventParams{
		UserID:       cmd.UserID,
		EventType:    cmd.EventType,
		Amount:       amount,
		OccurredAt:   cmd.OccurredAt,
		MerchantName: cmd.MerchantName,
		PaymentMethod: domain.PaymentMethod{
			Type: cmd.PaymentMethodType,
			Hint: cmd.PaymentMethodHint,
		},
		Payload: cmd.Payload,
	}

	fe, err := domain.NewManual(params, h.threshold, now)
	if err != nil {
		return nil, err
	}

	var categoryAssignment *domain.CategoryAssignment
	if cmd.Category != "" {
		assignment, err := domain.NewCategoryAssignment(cmd.Category, 1.0, domain.CategorySourceManual, now)
		if err != nil {
			return nil, err
		}
		fe.AssignCategory(assignment)
		categoryAssignment = assignment
	}

	err = h.uow.ExecuteTx(ctx, func(tx *domain.TxRepositories) error {
		inserted, err := tx.Events.InsertIfNew(ctx, fe)
		if err != nil {
			return fmt.Errorf("inserting manual financial event: %w", err)
		}
		if !inserted {
			// Extremely unlikely for manual events with uuid source ref, but handle cleanly
			observability.GetMetrics().FinancialCoreDedupConflictTotal.Inc()
			return nil
		}

		if categoryAssignment != nil {
			if err := tx.Events.SaveCategoryAssignment(ctx, fe.ID(), categoryAssignment); err != nil {
				return fmt.Errorf("saving manual category assignment: %w", err)
			}
		}

		finalizedPayload := domain.NewFinalizedPayload(fe, domain.FinalizedViaManualEntry, now)
		if err := tx.Outbox.Save(ctx, domain.AggregateTypeFinancialEvent, fe.ID(), domain.EventFinancialEventFinalized, finalizedPayload); err != nil {
			return fmt.Errorf("saving finalized outbox event: %w", err)
		}

		if categoryAssignment != nil {
			catPayload := domain.NewCategoryChangedPayload(fe, "", now)
			if err := tx.Outbox.Save(ctx, domain.AggregateTypeFinancialEvent, fe.ID(), domain.EventCategoryAssignmentChanged, catPayload); err != nil {
				return fmt.Errorf("saving category changed outbox event: %w", err)
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	observability.GetMetrics().FinancialCoreEventsFinalizedTotal.WithLabelValues(string(fe.EffectiveEventType()), string(fe.ConfidenceMethod())).Inc()

	return fe, nil
}
