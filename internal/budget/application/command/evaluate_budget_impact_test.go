package command_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/budget/application/command"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/budget/domain"
)

type mockBudgetRepo struct {
	budgets []*domain.Budget
}

func (m *mockBudgetRepo) Create(ctx context.Context, b *domain.Budget) error { return nil }
func (m *mockBudgetRepo) Update(ctx context.Context, b *domain.Budget) error { return nil }
func (m *mockBudgetRepo) GetByID(ctx context.Context, id, userID string) (*domain.Budget, error) {
	return nil, nil
}
func (m *mockBudgetRepo) ListByUserID(ctx context.Context, userID string, includeInactive bool) ([]*domain.Budget, error) {
	return m.budgets, nil
}
func (m *mockBudgetRepo) ListMatchingBudgets(ctx context.Context, userID, category string) ([]*domain.Budget, error) {
	var matching []*domain.Budget
	for _, b := range m.budgets {
		if b.MatchesScope(category) {
			matching = append(matching, b)
		}
	}
	return matching, nil
}
func (m *mockBudgetRepo) Delete(ctx context.Context, id, userID string) error { return nil }

type mockFiredAlertsRepo struct {
	fired map[string]bool
}

func newMockFiredAlertsRepo() *mockFiredAlertsRepo {
	return &mockFiredAlertsRepo{fired: make(map[string]bool)}
}

func (m *mockFiredAlertsRepo) ListFiredThresholds(ctx context.Context, budgetID string, periodStart time.Time) ([]float64, error) {
	var list []float64
	for key := range m.fired {
		var bID string
		var th float64
		_, _ = fmt.Sscanf(key, "%s_%f", &bID, &th)
		if bID == budgetID {
			list = append(list, th)
		}
	}
	return list, nil
}

func (m *mockFiredAlertsRepo) RecordFiredAlert(ctx context.Context, alertID, budgetID string, periodStart time.Time, threshold float64, firedAt time.Time) (bool, error) {
	key := fmt.Sprintf("%s_%.2f", budgetID, threshold)
	if m.fired[key] {
		return false, nil // conflict / already fired
	}
	m.fired[key] = true
	return true, nil
}

type mockOutboxRepo struct {
	events []string
}

func (m *mockOutboxRepo) RecordOutboxEvent(ctx context.Context, aggregateType, aggregateID, eventType string, payload any) error {
	m.events = append(m.events, eventType)
	return nil
}

type mockSpendProvider struct {
	spend float64
}

func (m *mockSpendProvider) GetCumulativeSpend(ctx context.Context, userID string, category string, periodStart, periodEnd time.Time) (float64, error) {
	return m.spend, nil
}

func (m *mockSpendProvider) GetNetCashFlow(ctx context.Context, userID string) (float64, error) {
	return 0, nil
}

func TestEvaluateBudgetImpact_ExpenseThresholdCrossing(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	b1, err := domain.NewBudget(
		"budget-1", "user-1", "global", "monthly",
		domain.Money{Value: 1000000, Currency: "IDR"},
		[]float64{0.80, 1.00},
		true, now,
	)
	if err != nil {
		t.Fatalf("unexpected error creating budget: %v", err)
	}

	budgetRepo := &mockBudgetRepo{budgets: []*domain.Budget{b1}}
	firedRepo := newMockFiredAlertsRepo()
	outboxRepo := &mockOutboxRepo{}
	spendProvider := &mockSpendProvider{spend: 850000} // 85% spend

	handler := command.NewEvaluateBudgetImpactHandler(budgetRepo, firedRepo, outboxRepo, spendProvider, nil)

	// Step 1: Expense crossing 80% threshold
	res, err := handler.Handle(ctx, command.EvaluateBudgetImpactCommand{
		UserID:     "user-1",
		EventID:    "evt-1",
		EventType:  "Expense",
		Amount:     domain.Money{Value: 850000, Currency: "IDR"},
		Category:   "Shopping",
		OccurredAt: now,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(res.AlertsFired) != 1 {
		t.Fatalf("expected 1 alert fired, got %d", len(res.AlertsFired))
	}
	if res.AlertsFired[0].Threshold != 0.80 {
		t.Errorf("expected 0.80 threshold, got %v", res.AlertsFired[0].Threshold)
	}
	if len(outboxRepo.events) != 1 || outboxRepo.events[0] != domain.EventBudgetThresholdCrossed {
		t.Errorf("expected 1 BudgetThresholdCrossed outbox event, got %v", outboxRepo.events)
	}

	// Step 2: Redelivery / Replay of identical event or another event at 85% spend
	res2, err := handler.Handle(ctx, command.EvaluateBudgetImpactCommand{
		UserID:     "user-1",
		EventID:    "evt-1-replay",
		EventType:  "Expense",
		Amount:     domain.Money{Value: 10000, Currency: "IDR"},
		Category:   "Shopping",
		OccurredAt: now,
	})
	if err != nil {
		t.Fatalf("unexpected error on replay: %v", err)
	}
	if len(res2.AlertsFired) != 0 {
		t.Fatalf("expected 0 alerts fired on replay (idempotency gate), got %d", len(res2.AlertsFired))
	}
	if len(outboxRepo.events) != 1 {
		t.Fatalf("expected outbox event count to remain 1, got %d", len(outboxRepo.events))
	}

	// Step 3: Spend increases to 1,050,000 (crosses 100% threshold)
	spendProvider.spend = 1050000
	res3, err := handler.Handle(ctx, command.EvaluateBudgetImpactCommand{
		UserID:     "user-1",
		EventID:    "evt-2",
		EventType:  "Expense",
		Amount:     domain.Money{Value: 200000, Currency: "IDR"},
		Category:   "Shopping",
		OccurredAt: now,
	})
	if err != nil {
		t.Fatalf("unexpected error on 100%% crossing: %v", err)
	}
	if len(res3.AlertsFired) != 1 || res3.AlertsFired[0].Threshold != 1.00 {
		t.Fatalf("expected 1.00 threshold alert, got %v", res3.AlertsFired)
	}
	if len(outboxRepo.events) != 2 {
		t.Fatalf("expected 2 outbox events total, got %d", len(outboxRepo.events))
	}
}

func TestEvaluateBudgetImpact_NonExpenseIgnored(t *testing.T) {
	ctx := context.Background()
	budgetRepo := &mockBudgetRepo{}
	firedRepo := newMockFiredAlertsRepo()
	outboxRepo := &mockOutboxRepo{}
	spendProvider := &mockSpendProvider{spend: 9999999}

	handler := command.NewEvaluateBudgetImpactHandler(budgetRepo, firedRepo, outboxRepo, spendProvider, nil)

	res, err := handler.Handle(ctx, command.EvaluateBudgetImpactCommand{
		UserID:    "user-1",
		EventID:   "evt-income",
		EventType: "Income",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.AlertsFired) != 0 || res.EvaluatedBudgets != 0 {
		t.Errorf("Income event should not trigger budget evaluation")
	}
}
