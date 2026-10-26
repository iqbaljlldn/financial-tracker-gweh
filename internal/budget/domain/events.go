package domain

import "time"

const (
	// EventBudgetThresholdCrossed is emitted when cumulative spend crosses a configured,
	// not-yet-fired threshold (04_domain_model.md §9.3).
	EventBudgetThresholdCrossed = "BudgetThresholdCrossed"

	// EventSavingsGoalProgressUpdated is emitted when savings goal progress is recomputed.
	EventSavingsGoalProgressUpdated = "SavingsGoalProgressUpdated"
)

// BudgetThresholdCrossedPayload carries payload data for BudgetThresholdCrossed event.
type BudgetThresholdCrossedPayload struct {
	BudgetID     string    `json:"budgetId"`
	UserID       string    `json:"userId"`
	Scope        string    `json:"scope"`
	PeriodStart  string    `json:"periodStart"` // ISO date e.g. "2026-10-01"
	Threshold    float64   `json:"threshold"`   // e.g. 0.8, 1.0
	CurrentSpend Money     `json:"currentSpend"`
	BudgetLimit  Money     `json:"budgetLimit"`
	PercentUsed  float64   `json:"percentUsed"` // e.g. 82.5 (%)
	CrossedAt    time.Time `json:"crossedAt"`
}

// SavingsGoalProgressUpdatedPayload carries payload data for SavingsGoalProgressUpdated event.
type SavingsGoalProgressUpdatedPayload struct {
	GoalID             string    `json:"goalId"`
	UserID             string    `json:"userId"`
	CurrentSavings     Money     `json:"currentSavings"`
	TargetAmount       Money     `json:"targetAmount"`
	ProgressPercentage float64   `json:"progressPercentage"`
	UpdatedAt          time.Time `json:"updatedAt"`
}
