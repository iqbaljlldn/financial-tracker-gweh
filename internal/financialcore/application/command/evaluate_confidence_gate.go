package command

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/financialcore/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/observability"
)

// EvaluateConfidenceGateHandler runs the confidence gate against candidate events, enforces INV-1,
// wires candidate-time categorization (H5), and commits the event with outbox rows in a single tx [GATE].
type EvaluateConfidenceGateHandler struct {
	uow       domain.UnitOfWork
	threshold float64
	suggester domain.CategorySuggester
	logger    *slog.Logger
	nowFn     func() time.Time
}

// NewEvaluateConfidenceGateHandler constructs the gating handler.
func NewEvaluateConfidenceGateHandler(
	uow domain.UnitOfWork,
	threshold float64,
	suggester domain.CategorySuggester,
	logger *slog.Logger,
) *EvaluateConfidenceGateHandler {
	if threshold <= 0 {
		threshold = domain.DefaultConfidenceThreshold
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &EvaluateConfidenceGateHandler{
		uow:       uow,
		threshold: threshold,
		suggester: suggester,
		logger:    logger,
		nowFn:     time.Now,
	}
}

// Handle executes the confidence gate evaluation and persistence.
func (h *EvaluateConfidenceGateHandler) Handle(ctx context.Context, cmd EvaluateConfidenceGateCommand) (*domain.FinancialEvent, error) {
	parsed, err := domain.ParseCandidatePayload(cmd.Payload)
	if err != nil {
		return nil, err
	}

	// Propagate trace context embedded in the candidate payload [GATE: 17_observability.md §5.2]
	ctx = observability.ExtractTraceContext(ctx, observability.TraceContextFromPayload(cmd.Payload))

	cand := parsed.Candidate
	if cand.Confidence == nil {
		return nil, domain.ErrConfidenceNotEvaluated
	}

	decision, err := domain.EvaluateConfidenceGate(cand.Confidence, h.threshold)
	if err != nil {
		return nil, err
	}

	now := h.nowFn()

	fe, err := domain.NewFromCandidate(cand, decision, now)
	if err != nil {
		return nil, err
	}

	fe.AssignCategory(h.resolveCategory(ctx, cand, now))

	var inserted bool
	err = h.uow.ExecuteTx(ctx, func(tx *domain.TxRepositories) error {
		ins, err := tx.Events.InsertIfNew(ctx, fe)
		if err != nil {
			return err
		}
		if !ins {
			inserted = false
			return nil // Idempotency: redelivery is a successful no-op [GATE]
		}
		inserted = true

		if fe.Category() != nil {
			if err := tx.Events.SaveCategoryAssignment(ctx, fe.ID(), fe.Category()); err != nil {
				return fmt.Errorf("saving candidate category assignment: %w", err)
			}
		}

		return saveCandidateOutbox(ctx, tx, fe, h.threshold, now)
	})
	if err != nil {
		return nil, err
	}

	if !inserted {
		// Idempotent redelivery absorbed by DB constraint
		observability.GetMetrics().FinancialCoreDedupConflictTotal.Inc()
		return fe, nil
	}

	// Observability & metrics
	if fe.Status() == domain.StatusFinalized {
		observability.GetMetrics().FinancialCoreEventsFinalizedTotal.WithLabelValues(string(fe.EffectiveEventType()), string(fe.ConfidenceMethod())).Inc()
	} else if fe.Status() == domain.StatusPendingReview {
		observability.GetMetrics().FinancialCoreEventsPendingReview.Inc()
	}

	if rejected := fe.RejectedAttributes(); len(rejected) > 0 {
		h.logger.WarnContext(ctx, "candidate normalized with dropped payload attributes",
			"eventId", fe.ID(),
			"eventType", string(fe.EventType()),
			"rejectedAttributes", rejected,
		)
	}

	return fe, nil
}

func (h *EvaluateConfidenceGateHandler) resolveCategory(ctx context.Context, cand domain.Candidate, now time.Time) *domain.CategoryAssignment {
	if sug, ok := cand.SuggestedCategory(); ok {
		if assignment, err := domain.NewCategoryAssignment(sug.Category, sug.Confidence, sug.Source, now); err == nil {
			return assignment
		}
	} else if h.suggester != nil {
		if sug, ok := h.suggester.Suggest(ctx, cand.UserID, cand.Merchant.NormalizedName); ok {
			if assignment, err := domain.NewCategoryAssignment(sug.Category, sug.Confidence, sug.Source, now); err == nil {
				return assignment
			}
		}
	}
	return nil
}

func saveCandidateOutbox(ctx context.Context, tx *domain.TxRepositories, fe *domain.FinancialEvent, threshold float64, now time.Time) error {
	if fe.Status() == domain.StatusFinalized {
		payload := domain.NewFinalizedPayload(fe, domain.FinalizedViaGate, now)
		if err := tx.Outbox.Save(ctx, domain.AggregateTypeFinancialEvent, fe.ID(), domain.EventFinancialEventFinalized, payload); err != nil {
			return fmt.Errorf("saving finalized outbox event: %w", err)
		}
	} else if fe.Status() == domain.StatusPendingReview {
		payload := domain.NewPendingReviewPayload(fe, threshold, now)
		if err := tx.Outbox.Save(ctx, domain.AggregateTypeFinancialEvent, fe.ID(), domain.EventFinancialEventPendingReview, payload); err != nil {
			return fmt.Errorf("saving pending review outbox event: %w", err)
		}
	}

	if fe.Category() != nil {
		payload := domain.NewCategoryChangedPayload(fe, "", now)
		if err := tx.Outbox.Save(ctx, domain.AggregateTypeFinancialEvent, fe.ID(), domain.EventCategoryAssignmentChanged, payload); err != nil {
			return fmt.Errorf("saving category changed outbox event: %w", err)
		}
	}

	return nil
}
