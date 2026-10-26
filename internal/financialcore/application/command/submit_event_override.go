package command

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/financialcore/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/observability"
)

// SubmitEventOverrideHandler records manual corrections on a financial event without mutating
// the original extracted values (SRS-FR-LEDGER-02). Overrides and outbox rows commit in a single tx [GATE].
type SubmitEventOverrideHandler struct {
	uow      domain.UnitOfWork
	feedback domain.CategoryFeedback
	logger   *slog.Logger
	nowFn    func() time.Time
}

// NewSubmitEventOverrideHandler constructs the override handler.
func NewSubmitEventOverrideHandler(
	uow domain.UnitOfWork,
	feedback domain.CategoryFeedback,
	logger *slog.Logger,
) *SubmitEventOverrideHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &SubmitEventOverrideHandler{
		uow:      uow,
		feedback: feedback,
		logger:   logger,
		nowFn:    time.Now,
	}
}

// Handle applies overrides, commits the audit records and emits outbox events atomically.
func (h *SubmitEventOverrideHandler) Handle(ctx context.Context, cmd SubmitEventOverrideCommand) (*domain.FinancialEvent, []*domain.EventOverride, error) {
	if cmd.UserID == "" {
		return nil, nil, domain.ErrUserIDRequired
	}
	if cmd.EventID == "" {
		return nil, nil, domain.ErrEventNotFound
	}
	actorID := cmd.ActorUserID
	if actorID == "" {
		actorID = cmd.UserID
	}

	if !hasAnyOverrideField(cmd.Fields) {
		return nil, nil, domain.ErrNoOverrideChange
	}

	now := h.nowFn()
	var (
		event              *domain.FinancialEvent
		createdOverrides   []*domain.EventOverride
		categoryAssignment *domain.CategoryAssignment
	)

	err := h.uow.ExecuteTx(ctx, func(tx *domain.TxRepositories) error {
		var err error
		event, createdOverrides, categoryAssignment, err = executeSubmitOverrideTx(ctx, tx, cmd, actorID, now)
		return err
	})
	if err != nil {
		return nil, nil, err
	}

	for _, o := range createdOverrides {
		observability.GetMetrics().FinancialCoreOverridesSubmittedTotal.WithLabelValues(o.FieldName()).Inc()
	}

	if categoryAssignment != nil && h.feedback != nil {
		_ = h.feedback.RecordUserCorrection(ctx, cmd.UserID, event.EffectiveMerchant().NormalizedName, categoryAssignment.Category())
	}

	return event, createdOverrides, nil
}

func hasAnyOverrideField(f OverrideFields) bool {
	return f.Amount != nil || f.Merchant != nil || f.EventType != nil || f.Category != nil
}

func executeSubmitOverrideTx(ctx context.Context, tx *domain.TxRepositories, cmd SubmitEventOverrideCommand, actorID string, now time.Time) (*domain.FinancialEvent, []*domain.EventOverride, *domain.CategoryAssignment, error) {
	fe, err := tx.Events.FindByIDForUpdate(ctx, cmd.UserID, cmd.EventID)
	if err != nil {
		return nil, nil, nil, err
	}
	if fe.Status() == domain.StatusArchived {
		return nil, nil, nil, domain.ErrEventArchived
	}

	prevCategory := ""
	if fe.Category() != nil {
		prevCategory = fe.Category().Category()
	}

	createdOverrides, categoryAssignment, err := applyAllOverrides(ctx, tx, fe, cmd.Fields, actorID, now)
	if err != nil {
		return nil, nil, nil, err
	}
	if len(createdOverrides) == 0 {
		return nil, nil, nil, domain.ErrNoOverrideChange
	}

	if err := saveOverrideOutbox(ctx, tx, fe, createdOverrides, categoryAssignment, prevCategory, now); err != nil {
		return nil, nil, nil, err
	}

	return fe, createdOverrides, categoryAssignment, nil
}

func applyAllOverrides(ctx context.Context, tx *domain.TxRepositories, fe *domain.FinancialEvent, f OverrideFields, actorID string, now time.Time) ([]*domain.EventOverride, *domain.CategoryAssignment, error) {
	var createdOverrides []*domain.EventOverride

	if o, err := applyAmountOverride(ctx, tx, fe, f.Amount, actorID, now); err != nil {
		return nil, nil, err
	} else if o != nil {
		createdOverrides = append(createdOverrides, o)
	}

	if o, err := applyMerchantOverride(ctx, tx, fe, f.Merchant, actorID, now); err != nil {
		return nil, nil, err
	} else if o != nil {
		createdOverrides = append(createdOverrides, o)
	}

	if o, err := applyEventTypeOverride(ctx, tx, fe, f.EventType, actorID, now); err != nil {
		return nil, nil, err
	} else if o != nil {
		createdOverrides = append(createdOverrides, o)
	}

	o, asgn, err := applyCategoryOverride(ctx, tx, fe, f.Category, actorID, now)
	if err != nil {
		return nil, nil, err
	}
	if o != nil {
		createdOverrides = append(createdOverrides, o)
	}

	return createdOverrides, asgn, nil
}

func saveOverrideOutbox(ctx context.Context, tx *domain.TxRepositories, fe *domain.FinancialEvent, createdOverrides []*domain.EventOverride, categoryAssignment *domain.CategoryAssignment, prevCategory string, now time.Time) error {
	refs := make([]domain.OverrideRef, len(createdOverrides))
	for i, o := range createdOverrides {
		refs[i] = domain.OverrideRef{
			OverrideID:    o.ID(),
			FieldName:     o.FieldName(),
			PreviousValue: o.PreviousValue(),
			NewValue:      o.NewValue(),
		}
	}

	overriddenPayload := domain.FinancialEventOverriddenPayload{
		EventID:   fe.ID(),
		UserID:    fe.UserID(),
		Status:    string(fe.Status()),
		Overrides: refs,
		EmittedAt: now.UTC(),
	}
	if err := tx.Outbox.Save(ctx, domain.AggregateTypeFinancialEvent, fe.ID(), domain.EventFinancialEventOverridden, overriddenPayload); err != nil {
		return fmt.Errorf("saving overridden outbox event: %w", err)
	}

	if categoryAssignment != nil {
		catPayload := domain.NewCategoryChangedPayload(fe, prevCategory, now)
		if err := tx.Outbox.Save(ctx, domain.AggregateTypeFinancialEvent, fe.ID(), domain.EventCategoryAssignmentChanged, catPayload); err != nil {
			return fmt.Errorf("saving category changed outbox event: %w", err)
		}
	}
	return nil
}

func applyAmountOverride(ctx context.Context, tx *domain.TxRepositories, fe *domain.FinancialEvent, amt *AmountField, actorID string, now time.Time) (*domain.EventOverride, error) {
	if amt == nil {
		return nil, nil
	}
	m, err := domain.NewMoney(amt.Value, amt.Currency)
	if err != nil {
		return nil, err
	}
	override, err := fe.OverrideAmount(m, actorID, now)
	if err != nil {
		return nil, err
	}
	if err := tx.Events.InsertOverride(ctx, override, actorID); err != nil {
		return nil, fmt.Errorf("inserting amount override: %w", err)
	}
	return override, nil
}

func applyMerchantOverride(ctx context.Context, tx *domain.TxRepositories, fe *domain.FinancialEvent, merchant *string, actorID string, now time.Time) (*domain.EventOverride, error) {
	if merchant == nil {
		return nil, nil
	}
	override, err := fe.OverrideMerchant(*merchant, actorID, now)
	if err != nil {
		return nil, err
	}
	if err := tx.Events.InsertOverride(ctx, override, actorID); err != nil {
		return nil, fmt.Errorf("inserting merchant override: %w", err)
	}
	return override, nil
}

func applyEventTypeOverride(ctx context.Context, tx *domain.TxRepositories, fe *domain.FinancialEvent, etStr *string, actorID string, now time.Time) (*domain.EventOverride, error) {
	if etStr == nil {
		return nil, nil
	}
	et, err := domain.ParseEventType(*etStr)
	if err != nil {
		return nil, err
	}
	override, err := fe.OverrideEventType(et, actorID, now)
	if err != nil {
		return nil, err
	}
	if err := tx.Events.InsertOverride(ctx, override, actorID); err != nil {
		return nil, fmt.Errorf("inserting event_type override: %w", err)
	}
	return override, nil
}

func applyCategoryOverride(ctx context.Context, tx *domain.TxRepositories, fe *domain.FinancialEvent, cat *string, actorID string, now time.Time) (*domain.EventOverride, *domain.CategoryAssignment, error) {
	if cat == nil {
		return nil, nil, nil
	}
	override, assignment, err := fe.OverrideCategory(*cat, actorID, now)
	if err != nil {
		return nil, nil, err
	}
	if err := tx.Events.InsertOverride(ctx, override, actorID); err != nil {
		return nil, nil, fmt.Errorf("inserting category override: %w", err)
	}
	if err := tx.Events.SaveCategoryAssignment(ctx, fe.ID(), assignment); err != nil {
		return nil, nil, fmt.Errorf("saving category assignment: %w", err)
	}
	return override, assignment, nil
}
