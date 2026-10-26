package command

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/budget/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/observability"
)

// EvaluateBudgetImpactCommand represents the input to evaluate budget impact
// following a finalized Expense financial event (07_sequence_diagrams.md Diagram 6).
type EvaluateBudgetImpactCommand struct {
	UserID     string
	EventID    string
	EventType  string
	Amount     domain.Money
	Category   string
	OccurredAt time.Time
}

// EvaluateBudgetImpactResult summarizes the evaluation execution.
type EvaluateBudgetImpactResult struct {
	EvaluatedBudgets int
	AlertsFired      []domain.BudgetThresholdCrossedPayload
}

// EvaluateBudgetImpactHandler executes idempotent budget threshold evaluation [GATE].
type EvaluateBudgetImpactHandler struct {
	budgetRepo      domain.BudgetRepository
	firedAlertsRepo domain.FiredAlertRepository
	outboxRepo      domain.OutboxRepository
	spendProvider   domain.SpendProvider
	logger          *slog.Logger
}

// NewEvaluateBudgetImpactHandler initializes the EvaluateBudgetImpactHandler.
func NewEvaluateBudgetImpactHandler(
	budgetRepo domain.BudgetRepository,
	firedAlertsRepo domain.FiredAlertRepository,
	outboxRepo domain.OutboxRepository,
	spendProvider domain.SpendProvider,
	logger *slog.Logger,
) *EvaluateBudgetImpactHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &EvaluateBudgetImpactHandler{
		budgetRepo:      budgetRepo,
		firedAlertsRepo: firedAlertsRepo,
		outboxRepo:      outboxRepo,
		spendProvider:   spendProvider,
		logger:          logger,
	}
}

// Handle evaluates matching budgets upon finalized Expense events, recomputes cumulative spend,
// and idempotently fires BudgetThresholdCrossed events when not-yet-fired thresholds are crossed.
func (h *EvaluateBudgetImpactHandler) Handle(ctx context.Context, cmd EvaluateBudgetImpactCommand) (*EvaluateBudgetImpactResult, error) {
	// Policy filter: only Expense events trigger budget evaluation per 05_event_storming.md Phase 7
	if !strings.EqualFold(cmd.EventType, "Expense") {
		return &EvaluateBudgetImpactResult{}, nil
	}

	start := time.Now()
	defer func() {
		metrics := observability.GetMetrics()
		if metrics != nil && metrics.BudgetEvaluationDurationSeconds != nil {
			metrics.BudgetEvaluationDurationSeconds.Observe(time.Since(start).Seconds())
		}
	}()

	tracer := otel.GetTracerProvider().Tracer("budget")
	ctx, span := tracer.Start(ctx, "EvaluateBudgetImpact")
	defer span.End()

	span.SetAttributes(
		attribute.String("user.id", cmd.UserID),
		attribute.String("event.id", cmd.EventID),
	)

	occurredAt := cmd.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}

	// 1. Fetch matching active budgets for user (global and matching category)
	matchingBudgets, err := h.budgetRepo.ListMatchingBudgets(ctx, cmd.UserID, cmd.Category)
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to query matching budgets",
			"error", err,
			"userId", cmd.UserID,
			"category", cmd.Category,
		)
		return nil, fmt.Errorf("budget: list matching budgets: %w", err)
	}

	res := &EvaluateBudgetImpactResult{
		EvaluatedBudgets: len(matchingBudgets),
	}

	// 2. Evaluate each matching budget
	for _, b := range matchingBudgets {
		span.SetAttributes(attribute.String("budget.id", b.ID()))

		periodStart, periodEnd := b.CalculatePeriodBounds(occurredAt)

		// Determine category filter for spend calculation
		var spendCategory string
		if strings.HasPrefix(b.Scope(), "category:") {
			spendCategory = strings.TrimPrefix(b.Scope(), "category:")
		}

		// Recompute cumulative spend for current period
		currentSpend, err := h.spendProvider.GetCumulativeSpend(ctx, cmd.UserID, spendCategory, periodStart, periodEnd)
		if err != nil {
			h.logger.ErrorContext(ctx, "failed to compute cumulative spend for budget",
				"error", err,
				"budgetId", b.ID(),
				"userId", cmd.UserID,
				"scope", b.Scope(),
			)
			return nil, fmt.Errorf("budget: compute cumulative spend: %w", err)
		}

		// Query previously fired alert thresholds for this budget and period
		alreadyFired, err := h.firedAlertsRepo.ListFiredThresholds(ctx, b.ID(), periodStart)
		if err != nil {
			h.logger.ErrorContext(ctx, "failed to fetch fired alerts for budget",
				"error", err,
				"budgetId", b.ID(),
			)
			return nil, fmt.Errorf("budget: fetch fired alerts: %w", err)
		}

		// Check which thresholds have newly crossed
		newlyCrossed := b.EvaluateThresholds(currentSpend, alreadyFired)
		if len(newlyCrossed) == 0 {
			continue
		}

		// For each newly crossed threshold, attempt atomic idempotent insertion
		for _, threshold := range newlyCrossed {
			alertID := uuid.New().String()
			now := time.Now().UTC()

			inserted, err := h.firedAlertsRepo.RecordFiredAlert(ctx, alertID, b.ID(), periodStart, threshold, now)
			if err != nil {
				h.logger.ErrorContext(ctx, "failed to record fired alert",
					"error", err,
					"budgetId", b.ID(),
					"threshold", threshold,
				)
				return nil, fmt.Errorf("budget: record fired alert: %w", err)
			}

			// If already inserted (due to concurrent worker or replay), suppress duplicate alert [GATE]
			if !inserted {
				h.logger.InfoContext(ctx, "suppressed duplicate budget alert (already fired in period)",
					"budgetId", b.ID(),
					"periodStart", periodStart.Format("2006-01-02"),
					"threshold", threshold,
				)
				continue
			}

			percentUsed := (currentSpend / b.ThresholdAmount().Value) * 100.0

			payload := domain.BudgetThresholdCrossedPayload{
				BudgetID:     b.ID(),
				UserID:       cmd.UserID,
				Scope:        b.Scope(),
				PeriodStart:  periodStart.Format("2006-01-02"),
				Threshold:    threshold,
				CurrentSpend: domain.Money{Value: currentSpend, Currency: b.ThresholdAmount().Currency},
				BudgetLimit:  b.ThresholdAmount(),
				PercentUsed:  percentUsed,
				CrossedAt:    now,
			}

			// Emit BudgetThresholdCrossed via outbox
			if err := h.outboxRepo.RecordOutboxEvent(ctx, "budget", b.ID(), domain.EventBudgetThresholdCrossed, payload); err != nil {
				h.logger.ErrorContext(ctx, "failed to emit BudgetThresholdCrossed outbox event",
					"error", err,
					"budgetId", b.ID(),
				)
				return nil, fmt.Errorf("budget: record outbox event: %w", err)
			}

			// Increment Prometheus metric
			metrics := observability.GetMetrics()
			if metrics != nil && metrics.BudgetThresholdCrossedTotal != nil {
				metrics.BudgetThresholdCrossedTotal.WithLabelValues(fmt.Sprintf("%.2f", threshold)).Inc()
			}

			h.logger.InfoContext(ctx, "budget threshold crossed alert fired",
				"budgetId", b.ID(),
				"scope", b.Scope(),
				"threshold", threshold,
				"currentSpend", currentSpend,
				"limit", b.ThresholdAmount().Value,
				"percentUsed", percentUsed,
			)

			res.AlertsFired = append(res.AlertsFired, payload)
		}
	}

	return res, nil
}
