package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/budget"
	budgetCmd "github.com/iqbaljlldn/financial-tracker-gweh/internal/budget/application/command"
	budgetQuery "github.com/iqbaljlldn/financial-tracker-gweh/internal/budget/application/query"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/budget/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/security"
)

// TestSprint7_Diagram6_BudgetAlertIdempotency verifies 07_sequence_diagrams.md Diagram 6:
// 1. Expense finalized -> EvaluateBudgetImpact computes spend against monthly budget.
// 2. Newly crossed threshold (e.g. 0.80) fires alert and records into budget_fired_alerts + outbox.
// 3. Redelivery / replay does NOT re-fire (idempotency gate via UNIQUE constraint).
// 4. Subsequent spend crossing 1.00 fires only the 1.00 threshold alert.
// 5. Cleaned up via TestCleaner on success.
func TestSprint7_Diagram6_BudgetAlertIdempotency(t *testing.T) {
	ctx := context.Background()
	pool, cleaner := setupTestDB(t)

	userID := uuid.New().String()
	cleaner.TrackUser(userID)

	budgetMod, err := budget.NewModule(budget.Config{
		DB: pool,
	})
	if err != nil {
		t.Fatalf("failed to initialize budget module: %v", err)
	}

	now := time.Now().UTC()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	// Step 1: Create a monthly global budget of 1,000,000 IDR with alert thresholds [0.80, 1.00]
	createdBudget, err := budgetMod.CreateBudgetCmd.Handle(ctx, budgetCmd.CreateBudgetCommand{
		UserID:          userID,
		Scope:           "global",
		PeriodType:      "monthly",
		ThresholdAmount: domain.Money{Value: 1000000, Currency: "IDR"},
		AlertThresholds: []float64{0.80, 1.00},
	})
	if err != nil {
		t.Fatalf("failed to create budget: %v", err)
	}
	budgetID := createdBudget.ID()
	if budgetID == "" {
		t.Fatalf("expected non-empty budget ID")
	}

	// Step 2: Insert a finalized Expense event of 850,000 IDR (85% of budget) in financialcore
	eventID1 := uuid.New().String()
	cleaner.TrackFinancialEvent(eventID1)
	insertTestFinancialEvent(t, pool, eventID1, userID, "Expense", 850000, "IDR", "Food", now)

	// Step 3: Evaluate budget impact -> 80% threshold should fire
	res1, err := budgetMod.EvaluateBudgetImpactCmd.Handle(ctx, budgetCmd.EvaluateBudgetImpactCommand{
		UserID:     userID,
		EventID:    eventID1,
		EventType:  "Expense",
		Amount:     domain.Money{Value: 850000, Currency: "IDR"},
		Category:   "Food",
		OccurredAt: now,
	})
	if err != nil {
		t.Fatalf("EvaluateBudgetImpact failed: %v", err)
	}

	if len(res1.AlertsFired) != 1 {
		t.Fatalf("expected 1 alert fired, got %d", len(res1.AlertsFired))
	}
	if res1.AlertsFired[0].Threshold != 0.80 {
		t.Errorf("expected 0.80 threshold, got %v", res1.AlertsFired[0].Threshold)
	}
	if res1.AlertsFired[0].CurrentSpend.Value != 850000 {
		t.Errorf("expected current spend 850000, got %v", res1.AlertsFired[0].CurrentSpend.Value)
	}

	// Verify budget_fired_alerts row in database
	var firedCount int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM budget.budget_fired_alerts 
		WHERE budget_id = $1::uuid AND period_start = $2 AND threshold = 0.80
	`, budgetID, startOfMonth).Scan(&firedCount)
	if err != nil {
		t.Fatalf("querying budget_fired_alerts: %v", err)
	}
	if firedCount != 1 {
		t.Errorf("expected 1 row in budget_fired_alerts, got %d", firedCount)
	}

	// Verify budget.outbox_events row
	var outboxCount int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM budget.outbox_events 
		WHERE aggregate_id = $1::uuid AND event_type = $2
	`, budgetID, domain.EventBudgetThresholdCrossed).Scan(&outboxCount)
	if err != nil {
		t.Fatalf("querying outbox_events: %v", err)
	}
	if outboxCount != 1 {
		t.Errorf("expected 1 outbox event, got %d", outboxCount)
	}

	// Step 4: Replay / redelivery of identical event -> MUST NOT fire duplicate alert (Idempotency GATE)
	resReplay, err := budgetMod.EvaluateBudgetImpactCmd.Handle(ctx, budgetCmd.EvaluateBudgetImpactCommand{
		UserID:     userID,
		EventID:    eventID1,
		EventType:  "Expense",
		Amount:     domain.Money{Value: 850000, Currency: "IDR"},
		Category:   "Food",
		OccurredAt: now,
	})
	if err != nil {
		t.Fatalf("EvaluateBudgetImpact replay failed: %v", err)
	}
	if len(resReplay.AlertsFired) != 0 {
		t.Fatalf("expected 0 alerts fired on replay (idempotency suppressed), got %d", len(resReplay.AlertsFired))
	}

	// Ensure outbox count did not increase
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM budget.outbox_events 
		WHERE aggregate_id = $1::uuid AND event_type = $2
	`, budgetID, domain.EventBudgetThresholdCrossed).Scan(&outboxCount)
	if err != nil {
		t.Fatalf("querying outbox_events after replay: %v", err)
	}
	if outboxCount != 1 {
		t.Errorf("expected outbox event count to remain 1, got %d", outboxCount)
	}

	// Step 5: Second expense of 200,000 IDR -> Cumulative spend becomes 1,050,000 IDR (105% of budget)
	eventID2 := uuid.New().String()
	cleaner.TrackFinancialEvent(eventID2)
	insertTestFinancialEvent(t, pool, eventID2, userID, "Expense", 200000, "IDR", "Food", now)

	res2, err := budgetMod.EvaluateBudgetImpactCmd.Handle(ctx, budgetCmd.EvaluateBudgetImpactCommand{
		UserID:     userID,
		EventID:    eventID2,
		EventType:  "Expense",
		Amount:     domain.Money{Value: 200000, Currency: "IDR"},
		Category:   "Food",
		OccurredAt: now,
	})
	if err != nil {
		t.Fatalf("EvaluateBudgetImpact second event failed: %v", err)
	}

	// Only 1.00 threshold should fire now (0.80 was already fired)
	if len(res2.AlertsFired) != 1 {
		t.Fatalf("expected 1 alert fired (1.00 threshold), got %d", len(res2.AlertsFired))
	}
	if res2.AlertsFired[0].Threshold != 1.00 {
		t.Errorf("expected 1.00 threshold, got %v", res2.AlertsFired[0].Threshold)
	}

	// Step 6: Verify ListBudgets query reflects current spend and percent used
	listRes, err := budgetMod.ListBudgetsQuery.Handle(ctx, budgetQuery.ListBudgetsQuery{
		UserID:          userID,
		IncludeInactive: false,
	})
	if err != nil {
		t.Fatalf("ListBudgets query failed: %v", err)
	}
	if len(listRes) != 1 {
		t.Fatalf("expected 1 budget in list, got %d", len(listRes))
	}
	if listRes[0].CurrentSpend.Value != 1050000 {
		t.Errorf("expected current spend 1050000, got %v", listRes[0].CurrentSpend.Value)
	}
	if listRes[0].PercentUsed != 105.0 {
		t.Errorf("expected percent used 105.0, got %v", listRes[0].PercentUsed)
	}
}

// TestSprint7_SavingsGoal_ProgressAndCRUD tests savings goal creation, progress calculation from net cash flow, and deletion.
func TestSprint7_SavingsGoal_ProgressAndCRUD(t *testing.T) {
	ctx := context.Background()
	pool, cleaner := setupTestDB(t)

	userID := uuid.New().String()
	cleaner.TrackUser(userID)

	budgetMod, err := budget.NewModule(budget.Config{DB: pool})
	if err != nil {
		t.Fatalf("failed to initialize budget module: %v", err)
	}

	targetDate := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)

	// 1. Create savings goal
	goal, err := budgetMod.CreateSavingsGoalCmd.Handle(ctx, budgetCmd.CreateSavingsGoalCommand{
		UserID:       userID,
		TargetAmount: domain.Money{Value: 10000000, Currency: "IDR"},
		TargetDate:   targetDate,
	})
	if err != nil {
		t.Fatalf("failed to create savings goal: %v", err)
	}
	goalID := goal.ID()
	if goalID == "" {
		t.Fatalf("expected non-empty goal ID")
	}

	// 2. Insert Income 15,000,000 IDR and Expense 5,000,000 IDR -> Net cash flow = 10,000,000 IDR (100% progress)
	incID := uuid.New().String()
	expID := uuid.New().String()
	cleaner.TrackFinancialEvent(incID)
	cleaner.TrackFinancialEvent(expID)

	insertTestFinancialEvent(t, pool, incID, userID, "Income", 15000000, "IDR", "Salary", time.Now().UTC())
	insertTestFinancialEvent(t, pool, expID, userID, "Expense", 5000000, "IDR", "Rent", time.Now().UTC())

	// 3. Query savings goals
	goalsList, err := budgetMod.ListSavingsGoalsQuery.Handle(ctx, budgetQuery.ListSavingsGoalsQuery{
		UserID: userID,
	})
	if err != nil {
		t.Fatalf("failed to list savings goals: %v", err)
	}
	if len(goalsList) != 1 {
		t.Fatalf("expected 1 goal, got %d", len(goalsList))
	}
	if goalsList[0].CurrentSavings.Value != 10000000 {
		t.Errorf("expected current savings 10000000, got %v", goalsList[0].CurrentSavings.Value)
	}
	if goalsList[0].ProgressPercentage != 100.0 {
		t.Errorf("expected progress 100%%, got %v", goalsList[0].ProgressPercentage)
	}

	// 4. Delete savings goal
	err = budgetMod.DeleteSavingsGoalCmd.Handle(ctx, budgetCmd.DeleteSavingsGoalCommand{
		ID:     goalID,
		UserID: userID,
	})
	if err != nil {
		t.Fatalf("failed to delete savings goal: %v", err)
	}

	goalsAfterDel, _ := budgetMod.ListSavingsGoalsQuery.Handle(ctx, budgetQuery.ListSavingsGoalsQuery{
		UserID: userID,
	})
	if len(goalsAfterDel) != 0 {
		t.Errorf("expected 0 goals after deletion, got %d", len(goalsAfterDel))
	}
}

// TestSprint7_HTTPRoutes tests the /budgets and /savings-goals REST endpoints.
func TestSprint7_HTTPRoutes(t *testing.T) {
	pool, cleaner := setupTestDB(t)

	userID := uuid.New().String()
	cleaner.TrackUser(userID)

	sessionMgr, err := security.NewSessionTokenManager([]byte("super-secret-session-key-32-bytes!"))
	if err != nil {
		t.Fatalf("failed to create session manager: %v", err)
	}

	budgetMod, err := budget.NewModule(budget.Config{
		DB:             pool,
		SessionManager: sessionMgr,
	})
	if err != nil {
		t.Fatalf("failed to initialize budget module: %v", err)
	}

	router := chi.NewRouter()
	budgetMod.RegisterHTTPRoutes(router)

	token, _, err := sessionMgr.Issue(userID, "test@example.com", "user", time.Hour)
	if err != nil {
		t.Fatalf("failed to generate session token: %v", err)
	}

	// 1. POST /budgets
	createBody := map[string]any{
		"scope": "category:Dining",
		"thresholdAmount": map[string]any{
			"value":    750000,
			"currency": "IDR",
		},
		"alertThresholds": []float64{0.75, 1.0},
	}
	bodyBytes, _ := json.Marshal(createBody)
	req := httptest.NewRequest("POST", "/budgets", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d (body: %s)", w.Code, w.Body.String())
	}

	var created struct {
		ID    string `json:"id"`
		Scope string `json:"scope"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	if created.ID == "" || created.Scope != "category:Dining" {
		t.Fatalf("unexpected created budget response: %s", w.Body.String())
	}

	// 2. GET /budgets
	getReq := httptest.NewRequest("GET", "/budgets", nil)
	getReq.Header.Set("Authorization", "Bearer "+token)
	wGet := httptest.NewRecorder()
	router.ServeHTTP(wGet, getReq)

	if wGet.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body: %s)", wGet.Code, wGet.Body.String())
	}

	// 3. DELETE /budgets/{id}
	delReq := httptest.NewRequest("DELETE", "/budgets/"+created.ID, nil)
	delReq.Header.Set("Authorization", "Bearer "+token)
	wDel := httptest.NewRecorder()
	router.ServeHTTP(wDel, delReq)

	if wDel.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d (body: %s)", wDel.Code, wDel.Body.String())
	}
}

func insertTestFinancialEvent(
	t *testing.T,
	pool *pgxpool.Pool,
	eventID, userID, eventType string,
	amount float64, currency, category string,
	occurredAt time.Time,
) {
	t.Helper()
	ctx := context.Background()

	var pgID, pgUserID pgtype.UUID
	_ = pgID.Scan(eventID)
	_ = pgUserID.Scan(userID)

	var numericAmount pgtype.Numeric
	_ = numericAmount.Scan(fmt.Sprintf("%.2f", amount))

	connAccountID := uuid.New().String()
	sourceRef := "msg_" + uuid.New().String()
	var pgConnID pgtype.UUID
	_ = pgConnID.Scan(connAccountID)

	// Insert finalized financial event into financialcore.financial_events
	_, err := pool.Exec(ctx, `
		INSERT INTO financialcore.financial_events (
			id, user_id, event_type, amount_value, amount_currency,
			occurred_at, merchant_raw_name, merchant_normalized_name,
			payment_method_type, payment_method_hint,
			status, confidence_aggregate, confidence_method,
			provenance_connector_type, provenance_connector_account_id,
			provenance_source_message_ref, provenance_extraction_method,
			provenance_extracted_at,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, 'Test Merchant', 'Test Merchant',
			'card', '1234',
			'finalized', 1.0, 'template_deterministic',
			'gmail', $7,
			$8, 'template',
			$6, $6, $6
		)
	`, pgID, pgUserID, eventType, numericAmount, currency, occurredAt, pgConnID, sourceRef)
	if err != nil {
		t.Fatalf("failed to insert test financial event: %v", err)
	}

	if category != "" {
		_, err = pool.Exec(ctx, `
			INSERT INTO financialcore.category_assignments (
				financial_event_id, category, confidence, source, assigned_at
			) VALUES ($1, $2, 1.0, 'manual', $3)
		`, pgID, category, occurredAt)
		if err != nil {
			t.Fatalf("failed to insert category assignment: %v", err)
		}
	}
}
