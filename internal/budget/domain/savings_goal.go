package domain

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"
)

var (
	ErrInvalidGoalID       = errors.New("savings_goal: invalid goal ID")
	ErrInvalidGoalUserID   = errors.New("savings_goal: invalid user ID")
	ErrInvalidTargetAmount = errors.New("savings_goal: target amount must be a positive number")
	ErrInvalidGoalCurrency = errors.New("savings_goal: currency must be a 3-letter ISO code")
	ErrInvalidTargetDate   = errors.New("savings_goal: target date must be set")
	ErrGoalNotFound        = errors.New("savings_goal: goal not found")
)

// SavingsGoal represents a savings goal entity per 04_domain_model.md §9.2 and 08_database_design.md §10.3.
type SavingsGoal struct {
	id           string
	userID       string
	targetAmount Money
	targetDate   time.Time
	createdAt    time.Time
	updatedAt    time.Time
}

// NewSavingsGoal creates and validates a new SavingsGoal entity.
func NewSavingsGoal(
	id string,
	userID string,
	targetAmount Money,
	targetDate time.Time,
	now time.Time,
) (*SavingsGoal, error) {
	if strings.TrimSpace(id) == "" {
		return nil, ErrInvalidGoalID
	}
	if strings.TrimSpace(userID) == "" {
		return nil, ErrInvalidGoalUserID
	}
	if targetAmount.Value <= 0 {
		return nil, ErrInvalidTargetAmount
	}
	currency := strings.ToUpper(strings.TrimSpace(targetAmount.Currency))
	if len(currency) != 3 {
		return nil, ErrInvalidGoalCurrency
	}
	targetAmount.Currency = currency

	if targetDate.IsZero() {
		return nil, ErrInvalidTargetDate
	}

	if now.IsZero() {
		now = time.Now().UTC()
	}

	return &SavingsGoal{
		id:           id,
		userID:       userID,
		targetAmount: targetAmount,
		targetDate:   targetDate.UTC(),
		createdAt:    now,
		updatedAt:    now,
	}, nil
}

// Getters
func (g *SavingsGoal) ID() string            { return g.id }
func (g *SavingsGoal) UserID() string        { return g.userID }
func (g *SavingsGoal) TargetAmount() Money   { return g.targetAmount }
func (g *SavingsGoal) TargetDate() time.Time { return g.targetDate }
func (g *SavingsGoal) CreatedAt() time.Time  { return g.createdAt }
func (g *SavingsGoal) UpdatedAt() time.Time  { return g.updatedAt }

// Update modifies goal parameters.
func (g *SavingsGoal) Update(targetAmount Money, targetDate time.Time, now time.Time) error {
	if targetAmount.Value <= 0 {
		return ErrInvalidTargetAmount
	}
	currency := strings.ToUpper(strings.TrimSpace(targetAmount.Currency))
	if len(currency) != 3 {
		return ErrInvalidGoalCurrency
	}
	targetAmount.Currency = currency

	if targetDate.IsZero() {
		return ErrInvalidTargetDate
	}

	if now.IsZero() {
		now = time.Now().UTC()
	}

	g.targetAmount = targetAmount
	g.targetDate = targetDate.UTC()
	g.updatedAt = now
	return nil
}

// CalculateProgress computes the percentage completion against net cash flow.
func (g *SavingsGoal) CalculateProgress(netCashFlow float64) float64 {
	if g.targetAmount.Value <= 0 {
		return 0.0
	}
	if netCashFlow <= 0 {
		return 0.0
	}
	pct := (netCashFlow / g.targetAmount.Value) * 100.0
	// Round to 2 decimal places
	return math.Round(pct*100) / 100
}

func (g *SavingsGoal) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID           string    `json:"id"`
		UserID       string    `json:"userId"`
		TargetAmount Money     `json:"targetAmount"`
		TargetDate   string    `json:"targetDate"`
		CreatedAt    time.Time `json:"createdAt"`
		UpdatedAt    time.Time `json:"updatedAt"`
	}{
		ID:           g.id,
		UserID:       g.userID,
		TargetAmount: g.targetAmount,
		TargetDate:   g.targetDate.Format("2006-01-02"),
		CreatedAt:    g.createdAt,
		UpdatedAt:    g.updatedAt,
	})
}
