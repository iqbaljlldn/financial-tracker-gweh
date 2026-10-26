package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidBudgetID        = errors.New("budget: invalid budget ID")
	ErrInvalidUserID          = errors.New("budget: invalid user ID")
	ErrInvalidScope           = errors.New("budget: scope must be 'global' or 'category:<name>'")
	ErrInvalidPeriodType      = errors.New("budget: period type must be 'monthly'")
	ErrInvalidThresholdAmount = errors.New("budget: threshold amount must be a positive number")
	ErrInvalidCurrency        = errors.New("budget: threshold amount currency must be a 3-letter ISO code")
	ErrInvalidAlertThresholds = errors.New("budget: alert thresholds must contain at least one percentage between 0.01 and 5.0")
	ErrBudgetNotFound         = errors.New("budget: budget not found")
)

// Money represents a monetary value with currency.
type Money struct {
	Value    float64 `json:"value"`
	Currency string  `json:"currency"`
}

// UnmarshalJSON enables Money to unmarshal from either a numeric value or a decimal string.
func (m *Money) UnmarshalJSON(data []byte) error {
	var aux struct {
		Value    json.RawMessage `json:"value"`
		Currency string          `json:"currency"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	m.Currency = strings.ToUpper(strings.TrimSpace(aux.Currency))

	// Try unmarshaling as float64 first
	var f float64
	if err := json.Unmarshal(aux.Value, &f); err == nil {
		m.Value = f
		return nil
	}

	// Try unmarshaling as string then parse float
	var s string
	if err := json.Unmarshal(aux.Value, &s); err == nil {
		parsed, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if err != nil {
			return fmt.Errorf("money: invalid numeric value %q: %w", s, err)
		}
		m.Value = parsed
		return nil
	}

	return fmt.Errorf("money: could not unmarshal %s into float64 or string", string(aux.Value))
}

// Budget represents the Budget aggregate root per 04_domain_model.md §9.1 and 08_database_design.md §10.1.
type Budget struct {
	id              string
	userID          string
	scope           string
	periodType      string
	thresholdAmount Money
	alertThresholds []float64
	active          bool
	createdAt       time.Time
	updatedAt       time.Time
}

// NewBudget creates and validates a new Budget aggregate.
func NewBudget(
	id string,
	userID string,
	scope string,
	periodType string,
	thresholdAmount Money,
	alertThresholds []float64,
	active bool,
	now time.Time,
) (*Budget, error) {
	if strings.TrimSpace(id) == "" {
		return nil, ErrInvalidBudgetID
	}
	if strings.TrimSpace(userID) == "" {
		return nil, ErrInvalidUserID
	}
	scope = strings.TrimSpace(scope)
	if scope != "global" && !strings.HasPrefix(scope, "category:") {
		return nil, ErrInvalidScope
	}
	if strings.HasPrefix(scope, "category:") && strings.TrimSpace(strings.TrimPrefix(scope, "category:")) == "" {
		return nil, ErrInvalidScope
	}
	periodType = strings.ToLower(strings.TrimSpace(periodType))
	if periodType != "monthly" {
		return nil, ErrInvalidPeriodType
	}
	if thresholdAmount.Value <= 0 {
		return nil, ErrInvalidThresholdAmount
	}
	currency := strings.ToUpper(strings.TrimSpace(thresholdAmount.Currency))
	if len(currency) != 3 {
		return nil, ErrInvalidCurrency
	}
	thresholdAmount.Currency = currency

	if len(alertThresholds) == 0 {
		alertThresholds = []float64{0.80, 1.00}
	}
	for _, t := range alertThresholds {
		if t <= 0.0 || t > 5.0 {
			return nil, ErrInvalidAlertThresholds
		}
	}

	if now.IsZero() {
		now = time.Now().UTC()
	}

	return &Budget{
		id:              id,
		userID:          userID,
		scope:           scope,
		periodType:      periodType,
		thresholdAmount: thresholdAmount,
		alertThresholds: alertThresholds,
		active:          active,
		createdAt:       now,
		updatedAt:       now,
	}, nil
}

// Getters
func (b *Budget) ID() string                 { return b.id }
func (b *Budget) UserID() string             { return b.userID }
func (b *Budget) Scope() string              { return b.scope }
func (b *Budget) PeriodType() string         { return b.periodType }
func (b *Budget) ThresholdAmount() Money     { return b.thresholdAmount }
func (b *Budget) AlertThresholds() []float64 { return b.alertThresholds }
func (b *Budget) Active() bool               { return b.active }
func (b *Budget) CreatedAt() time.Time       { return b.createdAt }
func (b *Budget) UpdatedAt() time.Time       { return b.updatedAt }

// Update modifies budget attributes and marks updatedAt.
func (b *Budget) Update(scope string, periodType string, amount Money, thresholds []float64, active bool, now time.Time) error {
	scope = strings.TrimSpace(scope)
	if scope != "global" && !strings.HasPrefix(scope, "category:") {
		return ErrInvalidScope
	}
	if strings.HasPrefix(scope, "category:") && strings.TrimSpace(strings.TrimPrefix(scope, "category:")) == "" {
		return ErrInvalidScope
	}
	periodType = strings.ToLower(strings.TrimSpace(periodType))
	if periodType != "monthly" {
		return ErrInvalidPeriodType
	}
	if amount.Value <= 0 {
		return ErrInvalidThresholdAmount
	}
	currency := strings.ToUpper(strings.TrimSpace(amount.Currency))
	if len(currency) != 3 {
		return ErrInvalidCurrency
	}
	amount.Currency = currency

	if len(thresholds) == 0 {
		thresholds = []float64{0.80, 1.00}
	}
	for _, t := range thresholds {
		if t <= 0.0 || t > 5.0 {
			return ErrInvalidAlertThresholds
		}
	}

	if now.IsZero() {
		now = time.Now().UTC()
	}

	b.scope = scope
	b.periodType = periodType
	b.thresholdAmount = amount
	b.alertThresholds = thresholds
	b.active = active
	b.updatedAt = now
	return nil
}

// MatchesScope checks if a given category matches this budget's scope.
func (b *Budget) MatchesScope(category string) bool {
	if !b.active {
		return false
	}
	if b.scope == "global" {
		return true
	}
	expectedCategory := strings.TrimPrefix(b.scope, "category:")
	return strings.EqualFold(strings.TrimSpace(expectedCategory), strings.TrimSpace(category))
}

// CalculatePeriodBounds returns the start and end of the period for the given timestamp.
func (b *Budget) CalculatePeriodBounds(t time.Time) (time.Time, time.Time) {
	t = t.UTC()
	// Monthly period per 04_domain_model.md §9.1
	start := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	return start, end
}

// EvaluateThresholds determines which configured alert thresholds have newly crossed
// given current spend and previously fired thresholds.
func (b *Budget) EvaluateThresholds(currentSpend float64, alreadyFired []float64) []float64 {
	if !b.active || b.thresholdAmount.Value <= 0 {
		return nil
	}

	firedMap := make(map[string]bool)
	for _, f := range alreadyFired {
		firedMap[fmt.Sprintf("%.2f", f)] = true
	}

	var newlyCrossed []float64
	for _, th := range b.alertThresholds {
		key := fmt.Sprintf("%.2f", th)
		if firedMap[key] {
			continue // Already fired in this period
		}
		// Spend has reached or crossed threshold * limit
		if currentSpend >= (th * b.thresholdAmount.Value) {
			newlyCrossed = append(newlyCrossed, th)
		}
	}

	return newlyCrossed
}

func (b *Budget) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID              string    `json:"id"`
		UserID          string    `json:"userId"`
		Scope           string    `json:"scope"`
		PeriodType      string    `json:"periodType"`
		ThresholdAmount Money     `json:"thresholdAmount"`
		AlertThresholds []float64 `json:"alertThresholds"`
		Active          bool      `json:"active"`
		CreatedAt       time.Time `json:"createdAt"`
		UpdatedAt       time.Time `json:"updatedAt"`
	}{
		ID:              b.id,
		UserID:          b.userID,
		Scope:           b.scope,
		PeriodType:      b.periodType,
		ThresholdAmount: b.thresholdAmount,
		AlertThresholds: b.alertThresholds,
		Active:          b.active,
		CreatedAt:       b.createdAt,
		UpdatedAt:       b.updatedAt,
	})
}
