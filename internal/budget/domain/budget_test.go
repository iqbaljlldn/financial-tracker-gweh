package domain_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/budget/domain"
)

func TestNewBudget_Validation(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name        string
		id          string
		userID      string
		scope       string
		periodType  string
		amount      domain.Money
		thresholds  []float64
		expectedErr error
	}{
		{
			name:        "valid global budget",
			id:          "b1",
			userID:      "u1",
			scope:       "global",
			periodType:  "monthly",
			amount:      domain.Money{Value: 1000000, Currency: "IDR"},
			thresholds:  []float64{0.8, 1.0},
			expectedErr: nil,
		},
		{
			name:        "valid category budget",
			id:          "b2",
			userID:      "u1",
			scope:       "category:Food & Dining",
			periodType:  "monthly",
			amount:      domain.Money{Value: 500000, Currency: "idr"},
			thresholds:  []float64{0.5, 0.8, 1.0},
			expectedErr: nil,
		},
		{
			name:        "empty budget id",
			id:          "",
			userID:      "u1",
			scope:       "global",
			periodType:  "monthly",
			amount:      domain.Money{Value: 1000, Currency: "USD"},
			expectedErr: domain.ErrInvalidBudgetID,
		},
		{
			name:        "empty user id",
			id:          "b1",
			userID:      "",
			scope:       "global",
			periodType:  "monthly",
			amount:      domain.Money{Value: 1000, Currency: "USD"},
			expectedErr: domain.ErrInvalidUserID,
		},
		{
			name:        "invalid scope",
			id:          "b1",
			userID:      "u1",
			scope:       "random_scope",
			periodType:  "monthly",
			amount:      domain.Money{Value: 1000, Currency: "USD"},
			expectedErr: domain.ErrInvalidScope,
		},
		{
			name:        "empty category in scope",
			id:          "b1",
			userID:      "u1",
			scope:       "category:",
			periodType:  "monthly",
			amount:      domain.Money{Value: 1000, Currency: "USD"},
			expectedErr: domain.ErrInvalidScope,
		},
		{
			name:        "invalid period type",
			id:          "b1",
			userID:      "u1",
			scope:       "global",
			periodType:  "weekly",
			amount:      domain.Money{Value: 1000, Currency: "USD"},
			expectedErr: domain.ErrInvalidPeriodType,
		},
		{
			name:        "non-positive threshold amount",
			id:          "b1",
			userID:      "u1",
			scope:       "global",
			periodType:  "monthly",
			amount:      domain.Money{Value: 0, Currency: "USD"},
			expectedErr: domain.ErrInvalidThresholdAmount,
		},
		{
			name:        "invalid currency code length",
			id:          "b1",
			userID:      "u1",
			scope:       "global",
			periodType:  "monthly",
			amount:      domain.Money{Value: 1000, Currency: "US"},
			expectedErr: domain.ErrInvalidCurrency,
		},
		{
			name:        "invalid threshold percentage",
			id:          "b1",
			userID:      "u1",
			scope:       "global",
			periodType:  "monthly",
			amount:      domain.Money{Value: 1000, Currency: "USD"},
			thresholds:  []float64{0.0},
			expectedErr: domain.ErrInvalidAlertThresholds,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, err := domain.NewBudget(tc.id, tc.userID, tc.scope, tc.periodType, tc.amount, tc.thresholds, true, now)
			if tc.expectedErr != nil {
				if err != tc.expectedErr {
					t.Fatalf("expected error %v, got %v", tc.expectedErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if b.ThresholdAmount().Currency != "IDR" && b.ThresholdAmount().Currency != "USD" {
				t.Fatalf("expected uppercase currency, got %s", b.ThresholdAmount().Currency)
			}
		})
	}
}

func TestBudget_MatchesScope(t *testing.T) {
	now := time.Now().UTC()

	globalBudget, _ := domain.NewBudget("b1", "u1", "global", "monthly", domain.Money{Value: 1000, Currency: "USD"}, nil, true, now)
	if !globalBudget.MatchesScope("Food") {
		t.Errorf("global budget should match any category")
	}
	if !globalBudget.MatchesScope("") {
		t.Errorf("global budget should match empty category")
	}

	categoryBudget, _ := domain.NewBudget("b2", "u1", "category:Groceries", "monthly", domain.Money{Value: 500, Currency: "USD"}, nil, true, now)
	if !categoryBudget.MatchesScope("Groceries") {
		t.Errorf("category budget should match exact category")
	}
	if !categoryBudget.MatchesScope("groceries") {
		t.Errorf("category budget should match case-insensitively")
	}
	if categoryBudget.MatchesScope("Utilities") {
		t.Errorf("category budget should not match different category")
	}

	inactiveBudget, _ := domain.NewBudget("b3", "u1", "global", "monthly", domain.Money{Value: 1000, Currency: "USD"}, nil, false, now)
	if inactiveBudget.MatchesScope("Food") {
		t.Errorf("inactive budget should not match")
	}
}

func TestBudget_CalculatePeriodBounds(t *testing.T) {
	b, _ := domain.NewBudget("b1", "u1", "global", "monthly", domain.Money{Value: 1000, Currency: "USD"}, nil, true, time.Now())

	timestamp := time.Date(2026, 10, 24, 15, 30, 0, 0, time.UTC)
	start, end := b.CalculatePeriodBounds(timestamp)

	expectedStart := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	expectedEnd := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

	if !start.Equal(expectedStart) {
		t.Errorf("expected start %v, got %v", expectedStart, start)
	}
	if !end.Equal(expectedEnd) {
		t.Errorf("expected end %v, got %v", expectedEnd, end)
	}
}

func TestBudget_EvaluateThresholds(t *testing.T) {
	b, _ := domain.NewBudget(
		"b1", "u1", "global", "monthly",
		domain.Money{Value: 1000, Currency: "USD"},
		[]float64{0.50, 0.80, 1.00},
		true, time.Now(),
	)

	// Case 1: Spend is 400 (< 50%) -> no threshold crossed
	crossed := b.EvaluateThresholds(400, nil)
	if len(crossed) != 0 {
		t.Fatalf("expected 0 crossed thresholds, got %v", crossed)
	}

	// Case 2: Spend reaches 850 (crosses 0.50 and 0.80) -> returns both
	crossed = b.EvaluateThresholds(850, nil)
	if len(crossed) != 2 || crossed[0] != 0.50 || crossed[1] != 0.80 {
		t.Fatalf("expected [0.50, 0.80], got %v", crossed)
	}

	// Case 3: Already fired 0.50, spend is 850 -> only returns 0.80
	crossed = b.EvaluateThresholds(850, []float64{0.50})
	if len(crossed) != 1 || crossed[0] != 0.80 {
		t.Fatalf("expected [0.80], got %v", crossed)
	}

	// Case 4: Already fired 0.80 and 0.50, spend is 850 -> returns empty (idempotency)
	crossed = b.EvaluateThresholds(850, []float64{0.50, 0.80})
	if len(crossed) != 0 {
		t.Fatalf("expected 0 crossed thresholds on replay, got %v", crossed)
	}

	// Case 5: Spend reaches 1050 (crosses 1.00) -> returns [1.00]
	crossed = b.EvaluateThresholds(1050, []float64{0.50, 0.80})
	if len(crossed) != 1 || crossed[0] != 1.00 {
		t.Fatalf("expected [1.00], got %v", crossed)
	}
}

func TestSavingsGoal_CalculateProgress(t *testing.T) {
	targetDate := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	goal, err := domain.NewSavingsGoal("g1", "u1", domain.Money{Value: 10000000, Currency: "IDR"}, targetDate, time.Now())
	if err != nil {
		t.Fatalf("unexpected error creating savings goal: %v", err)
	}

	// Negative or zero net cash flow
	if p := goal.CalculateProgress(-1000); p != 0.0 {
		t.Errorf("expected 0 for negative net cash flow, got %v", p)
	}
	if p := goal.CalculateProgress(0); p != 0.0 {
		t.Errorf("expected 0 for zero net cash flow, got %v", p)
	}

	// 5,000,000 of 10,000,000 -> 50%
	if p := goal.CalculateProgress(5000000); p != 50.0 {
		t.Errorf("expected 50%%, got %v", p)
	}

	// 12,000,000 of 10,000,000 -> 120%
	if p := goal.CalculateProgress(12000000); p != 120.0 {
		t.Errorf("expected 120%%, got %v", p)
	}
}

func TestMoney_UnmarshalJSON(t *testing.T) {
	// Case 1: unmarshaling from decimal string (as emitted by financialcore ledger)
	stringJSON := []byte(`{"value":"15000.00","currency":"IDR"}`)
	var m1 domain.Money
	if err := json.Unmarshal(stringJSON, &m1); err != nil {
		t.Fatalf("failed to unmarshal string money: %v", err)
	}
	if m1.Value != 15000.00 || m1.Currency != "IDR" {
		t.Errorf("expected 15000.00 IDR, got %v %s", m1.Value, m1.Currency)
	}

	// Case 2: unmarshaling from float number
	numberJSON := []byte(`{"value":15000.50,"currency":"idr"}`)
	var m2 domain.Money
	if err := json.Unmarshal(numberJSON, &m2); err != nil {
		t.Fatalf("failed to unmarshal number money: %v", err)
	}
	if m2.Value != 15000.50 || m2.Currency != "IDR" {
		t.Errorf("expected 15000.50 IDR, got %v %s", m2.Value, m2.Currency)
	}
}
