package domain

import (
	"context"
	"time"
)

// BudgetRepository defines storage operations for the Budget aggregate root.
type BudgetRepository interface {
	Create(ctx context.Context, b *Budget) error
	Update(ctx context.Context, b *Budget) error
	GetByID(ctx context.Context, id, userID string) (*Budget, error)
	ListByUserID(ctx context.Context, userID string, includeInactive bool) ([]*Budget, error)
	ListMatchingBudgets(ctx context.Context, userID, category string) ([]*Budget, error)
	Delete(ctx context.Context, id, userID string) error
}

// FiredAlertRepository manages idempotent alert tracking in budget_fired_alerts table.
type FiredAlertRepository interface {
	// ListFiredThresholds returns all thresholds already fired for a given budget and period_start.
	ListFiredThresholds(ctx context.Context, budgetID string, periodStart time.Time) ([]float64, error)

	// RecordFiredAlert atomically inserts a fired alert record using UNIQUE constraint.
	// Returns true if inserted, false if already exists (idempotency deduplication).
	RecordFiredAlert(ctx context.Context, alertID, budgetID string, periodStart time.Time, threshold float64, firedAt time.Time) (bool, error)
}

// SavingsGoalRepository defines storage operations for SavingsGoal entities.
type SavingsGoalRepository interface {
	Create(ctx context.Context, g *SavingsGoal) error
	Update(ctx context.Context, g *SavingsGoal) error
	GetByID(ctx context.Context, id, userID string) (*SavingsGoal, error)
	ListByUserID(ctx context.Context, userID string) ([]*SavingsGoal, error)
	Delete(ctx context.Context, id, userID string) error
}

// OutboxRepository records budget domain events into budget.outbox_events.
type OutboxRepository interface {
	RecordOutboxEvent(ctx context.Context, aggregateType, aggregateID, eventType string, payload any) error
}

// SpendProvider provides read access to cumulative expenses and net cash flow from the ledger.
type SpendProvider interface {
	// GetCumulativeSpend computes the sum of finalized expenses for the user in the period [start, end).
	// If category is provided, filters by that category; otherwise computes global spend.
	GetCumulativeSpend(ctx context.Context, userID string, category string, periodStart, periodEnd time.Time) (float64, error)

	// GetNetCashFlow calculates total finalized Income minus total finalized Expense for the user.
	GetNetCashFlow(ctx context.Context, userID string) (float64, error)
}
