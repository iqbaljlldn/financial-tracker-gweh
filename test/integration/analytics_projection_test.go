package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/analytics"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/analytics/application/projection"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/security"
)

// Helper to create test finalized event payload
func makeFinalizedPayload(eventID, userID, eventType, value, currency, merchant, category string, occurredAt time.Time) []byte {
	p := map[string]any{
		"eventId":   eventID,
		"userId":    userID,
		"eventType": eventType,
		"amount": map[string]string{
			"value":    value,
			"currency": currency,
		},
		"occurredAt": occurredAt.Format(time.RFC3339),
		"merchant":   merchant,
		"category":   category,
		"emittedAt":  occurredAt.Format(time.RFC3339),
	}
	b, _ := json.Marshal(p)
	return b
}

// TestSprint8_ProjectionMatchesHandComputedAggregates verifies:
// 1. Incremental projection of Income & Expense events
// 2. Read models match hand-computed expected aggregates (CashFlow, CategoryBreakdown, MerchantRanking, Trend)
// 3. Category change updates CategoryBreakdown & percentage shares correctly
// 4. Archive reverses event contribution accurately
func TestSprint8_ProjectionMatchesHandComputedAggregates(t *testing.T) {
	ctx := context.Background()
	pool, cleaner := setupTestDB(t)

	userID := uuid.New().String()
	cleaner.TrackUser(userID)

	mod := analytics.NewModule(analytics.ModuleConfig{
		DBPool: pool,
	})
	proj := mod.Projector()

	date15 := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	date16 := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	date20 := time.Date(2026, 7, 20, 14, 0, 0, 0, time.UTC)

	eventID1 := uuid.New().String() // Income 5000.00 USD
	eventID2 := uuid.New().String() // Expense 1500.00 USD (Dining, Starbucks)
	eventID3 := uuid.New().String() // Expense 500.00 USD (Dining, Starbucks)
	eventID4 := uuid.New().String() // Expense 1000.00 USD (Groceries, Whole Foods)

	cleaner.TrackFinancialEvent(eventID1)
	cleaner.TrackFinancialEvent(eventID2)
	cleaner.TrackFinancialEvent(eventID3)
	cleaner.TrackFinancialEvent(eventID4)

	// Step 1: Project 4 events
	err := proj.ProjectAnalyticsReadModels(ctx, uuid.New().String(), "FinancialEventFinalized",
		makeFinalizedPayload(eventID1, userID, "Income", "5000.00", "USD", "Employer", "", date15), date15)
	if err != nil {
		t.Fatalf("project event 1: %v", err)
	}

	err = proj.ProjectAnalyticsReadModels(ctx, uuid.New().String(), "FinancialEventFinalized",
		makeFinalizedPayload(eventID2, userID, "Expense", "1500.00", "USD", "Starbucks", "Dining", date15), date15)
	if err != nil {
		t.Fatalf("project event 2: %v", err)
	}

	err = proj.ProjectAnalyticsReadModels(ctx, uuid.New().String(), "FinancialEventFinalized",
		makeFinalizedPayload(eventID3, userID, "Expense", "500.00", "USD", "Starbucks", "Dining", date16), date16)
	if err != nil {
		t.Fatalf("project event 3: %v", err)
	}

	err = proj.ProjectAnalyticsReadModels(ctx, uuid.New().String(), "FinancialEventFinalized",
		makeFinalizedPayload(eventID4, userID, "Expense", "1000.00", "USD", "Whole Foods", "Groceries", date20), date20)
	if err != nil {
		t.Fatalf("project event 4: %v", err)
	}

	// Step 2: Verify Cash Flow Monthly (Expected: Income = 5000.00, Expense = 3000.00)
	cfRes, err := mod.Repo().GetCashFlow(ctx, userID, "monthly", nil, nil, nil)
	if err != nil {
		t.Fatalf("get cash flow: %v", err)
	}
	if len(cfRes) != 1 {
		t.Fatalf("expected 1 monthly cash flow row, got %d", len(cfRes))
	}
	if cfRes[0].TotalIncome != "5000.00" || cfRes[0].TotalExpense != "3000.00" {
		t.Errorf("expected Income 5000.00 and Expense 3000.00, got Income %s, Expense %s", cfRes[0].TotalIncome, cfRes[0].TotalExpense)
	}

	// Step 3: Verify Trend Monthly (Expected: NetFlow = 2000.00)
	trRes, err := mod.Repo().GetTrend(ctx, userID, "monthly", nil, nil, nil)
	if err != nil {
		t.Fatalf("get trend: %v", err)
	}
	if len(trRes) != 1 {
		t.Fatalf("expected 1 trend row, got %d", len(trRes))
	}
	if trRes[0].NetFlow != "2000.00" {
		t.Errorf("expected NetFlow 2000.00, got %s", trRes[0].NetFlow)
	}

	// Step 4: Verify Category Breakdown
	// Total expenses = 3000.00. Dining = 2000.00 (66.67%), Groceries = 1000.00 (33.33%)
	catRes, err := mod.Repo().GetCategoryBreakdown(ctx, userID, nil, nil, nil)
	if err != nil {
		t.Fatalf("get category breakdown: %v", err)
	}
	if len(catRes) != 2 {
		t.Fatalf("expected 2 categories, got %d", len(catRes))
	}
	if catRes[0].Category != "Dining" || catRes[0].TotalValue != "2000.00" || catRes[0].PercentageShare < 66.6 {
		t.Errorf("expected Dining 2000.00 (~66.67%%), got %+v", catRes[0])
	}
	if catRes[1].Category != "Groceries" || catRes[1].TotalValue != "1000.00" || catRes[1].PercentageShare < 33.3 {
		t.Errorf("expected Groceries 1000.00 (~33.33%%), got %+v", catRes[1])
	}

	// Step 5: Verify Merchant Ranking
	// Starbucks = 2000.00 (count 2), Whole Foods = 1000.00 (count 1)
	merchRes, err := mod.Repo().GetMerchantRanking(ctx, userID, nil, nil, nil, 10)
	if err != nil {
		t.Fatalf("get merchant ranking: %v", err)
	}
	if len(merchRes) != 2 {
		t.Fatalf("expected 2 merchants, got %d", len(merchRes))
	}
	if merchRes[0].MerchantNormalizedName != "Starbucks" || merchRes[0].TotalValue != "2000.00" || merchRes[0].TransactionCount != 2 {
		t.Errorf("expected Starbucks 2000.00 (count 2), got %+v", merchRes[0])
	}

	// Step 6: Test Category Change (Move Event 4 from Groceries to Utilities)
	catChangePayload, _ := json.Marshal(map[string]any{
		"eventId":          eventID4,
		"userId":           userID,
		"category":         "Utilities",
		"previousCategory": "Groceries",
		"source":           "user_correction",
		"confidence":       1.0,
		"merchant":         "Whole Foods",
		"emittedAt":        time.Now().UTC().Format(time.RFC3339),
	})
	err = proj.ProjectAnalyticsReadModels(ctx, uuid.New().String(), "CategoryAssignmentChanged", catChangePayload, time.Now().UTC())
	if err != nil {
		t.Fatalf("project category change: %v", err)
	}

	catResAfterChange, _ := mod.Repo().GetCategoryBreakdown(ctx, userID, nil, nil, nil)
	foundUtilities := false
	for _, c := range catResAfterChange {
		if c.Category == "Groceries" && c.TotalValue != "0.00" {
			t.Errorf("expected Groceries total to be 0, got %s", c.TotalValue)
		}
		if c.Category == "Utilities" && c.TotalValue == "1000.00" {
			foundUtilities = true
		}
	}
	if !foundUtilities {
		t.Errorf("expected Utilities category with total 1000.00, got %+v", catResAfterChange)
	}

	// Step 7: Test Archive (Archive Event 3: Expense 500.00 USD)
	archivePayload, _ := json.Marshal(map[string]any{
		"eventId":   eventID3,
		"userId":    userID,
		"reason":    "duplicate",
		"emittedAt": time.Now().UTC().Format(time.RFC3339),
	})
	err = proj.ProjectAnalyticsReadModels(ctx, uuid.New().String(), "FinancialEventArchived", archivePayload, time.Now().UTC())
	if err != nil {
		t.Fatalf("project archive: %v", err)
	}

	// Cash flow expense should drop from 3000 to 2500
	cfAfterArchive, _ := mod.Repo().GetCashFlow(ctx, userID, "monthly", nil, nil, nil)
	if len(cfAfterArchive) != 1 || cfAfterArchive[0].TotalExpense != "2500.00" {
		t.Errorf("expected total expense 2500.00 after archive, got %+v", cfAfterArchive)
	}

	// Starbucks should drop from 2000 (count 2) to 1500 (count 1)
	merchAfterArchive, _ := mod.Repo().GetMerchantRanking(ctx, userID, nil, nil, nil, 10)
	if len(merchAfterArchive) < 1 || merchAfterArchive[0].TotalValue != "1500.00" || merchAfterArchive[0].TransactionCount != 1 {
		t.Errorf("expected Starbucks 1500.00 (count 1), got %+v", merchAfterArchive)
	}
}

// TestSprint8_MultiCurrencyPartitioning verifies:
// Multi-currency events are strictly partitioned by composite primary key and never blended [GATE].
func TestSprint8_MultiCurrencyPartitioning(t *testing.T) {
	ctx := context.Background()
	pool, cleaner := setupTestDB(t)

	userID := uuid.New().String()
	cleaner.TrackUser(userID)

	mod := analytics.NewModule(analytics.ModuleConfig{
		DBPool: pool,
	})
	proj := mod.Projector()

	now := time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC)
	eventUSD := uuid.New().String()
	eventIDR := uuid.New().String()

	cleaner.TrackFinancialEvent(eventUSD)
	cleaner.TrackFinancialEvent(eventIDR)

	// Finalize 100.00 USD Expense
	_ = proj.ProjectAnalyticsReadModels(ctx, uuid.New().String(), "FinancialEventFinalized",
		makeFinalizedPayload(eventUSD, userID, "Expense", "100.00", "USD", "DigitalOcean", "Hosting", now), now)

	// Finalize 1,500,000.00 IDR Expense
	_ = proj.ProjectAnalyticsReadModels(ctx, uuid.New().String(), "FinancialEventFinalized",
		makeFinalizedPayload(eventIDR, userID, "Expense", "1500000.00", "IDR", "PLN", "Utilities", now), now)

	// Query Cash Flow without currency filter -> MUST return 2 distinct rows
	cfAll, err := mod.Repo().GetCashFlow(ctx, userID, "monthly", nil, nil, nil)
	if err != nil {
		t.Fatalf("get cash flow: %v", err)
	}
	if len(cfAll) != 2 {
		t.Fatalf("expected exactly 2 partitioned currency rows, got %d", len(cfAll))
	}

	currencies := map[string]string{}
	for _, row := range cfAll {
		currencies[row.Currency] = row.TotalExpense
	}

	if currencies["USD"] != "100.00" {
		t.Errorf("expected USD 100.00, got %s", currencies["USD"])
	}
	if currencies["IDR"] != "1500000.00" {
		t.Errorf("expected IDR 1500000.00, got %s", currencies["IDR"])
	}

	// Query Cash Flow filtered by USD
	currUSD := "USD"
	cfUSD, err := mod.Repo().GetCashFlow(ctx, userID, "monthly", nil, nil, &currUSD)
	if err != nil {
		t.Fatalf("get cash flow USD: %v", err)
	}
	if len(cfUSD) != 1 || cfUSD[0].Currency != "USD" || cfUSD[0].TotalExpense != "100.00" {
		t.Errorf("expected single USD row of 100.00, got %+v", cfUSD)
	}

	// Query Category Breakdown without currency filter -> separate per currency
	catAll, err := mod.Repo().GetCategoryBreakdown(ctx, userID, nil, nil, nil)
	if err != nil {
		t.Fatalf("get category breakdown: %v", err)
	}
	if len(catAll) != 2 {
		t.Fatalf("expected 2 category breakdown rows across currencies, got %d", len(catAll))
	}
}

// TestSprint8_FullRebuildFromReplay verifies:
// Replaying all outbox events from scratch produces identical state to incremental projection [GATE].
func TestSprint8_FullRebuildFromReplay(t *testing.T) {
	ctx := context.Background()
	pool, cleaner := setupTestDB(t)

	userID := uuid.New().String()
	cleaner.TrackUser(userID)

	mod := analytics.NewModule(analytics.ModuleConfig{
		DBPool: pool,
	})
	proj := mod.Projector()
	rebuilder := mod.Rebuilder()
	t.Cleanup(func() {
		_ = rebuilder.RebuildFromDB(context.Background(), pool)
	})

	d1 := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, 9, 2, 11, 0, 0, 0, time.UTC)
	d3 := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)

	e1 := uuid.New().String()
	e2 := uuid.New().String()
	e3 := uuid.New().String()

	cleaner.TrackFinancialEvent(e1)
	cleaner.TrackFinancialEvent(e2)
	cleaner.TrackFinancialEvent(e3)

	events := []projection.ReplayOutboxEvent{
		{
			ID:        uuid.New().String(),
			EventType: "FinancialEventFinalized",
			Payload:   makeFinalizedPayload(e1, userID, "Income", "3000.00", "USD", "Employer", "", d1),
			CreatedAt: d1,
		},
		{
			ID:        uuid.New().String(),
			EventType: "FinancialEventFinalized",
			Payload:   makeFinalizedPayload(e2, userID, "Expense", "500.00", "USD", "Uber", "Transport", d2),
			CreatedAt: d2,
		},
		{
			ID:        uuid.New().String(),
			EventType: "FinancialEventFinalized",
			Payload:   makeFinalizedPayload(e3, userID, "Expense", "250.00", "USD", "Target", "Shopping", d3),
			CreatedAt: d3,
		},
	}

	// 1. Incremental projection
	for _, evt := range events {
		if err := proj.ProjectAnalyticsReadModels(ctx, evt.ID, evt.EventType, evt.Payload, evt.CreatedAt); err != nil {
			t.Fatalf("incremental project %s: %v", evt.ID, err)
		}
	}

	// Capture incremental state
	cfOrig, _ := mod.Repo().GetCashFlow(ctx, userID, "monthly", nil, nil, nil)
	catOrig, _ := mod.Repo().GetCategoryBreakdown(ctx, userID, nil, nil, nil)
	merchOrig, _ := mod.Repo().GetMerchantRanking(ctx, userID, nil, nil, nil, 10)
	trendOrig, _ := mod.Repo().GetTrend(ctx, userID, "monthly", nil, nil, nil)

	if len(cfOrig) == 0 {
		t.Fatalf("expected non-empty cash flow before rebuild")
	}

	// 2. Full wipe-and-rebuild from replay
	if err := rebuilder.Rebuild(ctx, events); err != nil {
		t.Fatalf("rebuilder.Rebuild failed: %v", err)
	}

	// Capture rebuilt state
	cfRebuilt, _ := mod.Repo().GetCashFlow(ctx, userID, "monthly", nil, nil, nil)
	catRebuilt, _ := mod.Repo().GetCategoryBreakdown(ctx, userID, nil, nil, nil)
	merchRebuilt, _ := mod.Repo().GetMerchantRanking(ctx, userID, nil, nil, nil, 10)
	trendRebuilt, _ := mod.Repo().GetTrend(ctx, userID, "monthly", nil, nil, nil)

	// 3. Verify exact parity between incremental and rebuilt state (excluding UpdatedAt)
	if len(cfOrig) != len(cfRebuilt) {
		t.Fatalf("cash flow count mismatch: orig %d, rebuilt %d", len(cfOrig), len(cfRebuilt))
	}
	for i := range cfOrig {
		if cfOrig[i].TotalIncome != cfRebuilt[i].TotalIncome || cfOrig[i].TotalExpense != cfRebuilt[i].TotalExpense {
			t.Errorf("cash flow mismatch at index %d: orig %+v, rebuilt %+v", i, cfOrig[i], cfRebuilt[i])
		}
	}

	if len(catOrig) != len(catRebuilt) {
		t.Fatalf("category breakdown count mismatch: orig %d, rebuilt %d", len(catOrig), len(catRebuilt))
	}
	for i := range catOrig {
		if catOrig[i].Category != catRebuilt[i].Category || catOrig[i].TotalValue != catRebuilt[i].TotalValue || catOrig[i].PercentageShare != catRebuilt[i].PercentageShare {
			t.Errorf("category breakdown mismatch at %d: orig %+v, rebuilt %+v", i, catOrig[i], catRebuilt[i])
		}
	}

	if len(merchOrig) != len(merchRebuilt) {
		t.Fatalf("merchant ranking count mismatch: orig %d, rebuilt %d", len(merchOrig), len(merchRebuilt))
	}
	for i := range merchOrig {
		if merchOrig[i].MerchantNormalizedName != merchRebuilt[i].MerchantNormalizedName || merchOrig[i].TotalValue != merchRebuilt[i].TotalValue || merchOrig[i].TransactionCount != merchRebuilt[i].TransactionCount {
			t.Errorf("merchant ranking mismatch at %d: orig %+v, rebuilt %+v", i, merchOrig[i], merchRebuilt[i])
		}
	}

	if !reflect.DeepEqual(len(trendOrig), len(trendRebuilt)) {
		t.Fatalf("trend count mismatch: orig %d, rebuilt %d", len(trendOrig), len(trendRebuilt))
	}
	for i := range trendOrig {
		if trendOrig[i].NetFlow != trendRebuilt[i].NetFlow {
			t.Errorf("trend mismatch at %d: orig %+v, rebuilt %+v", i, trendOrig[i], trendRebuilt[i])
		}
	}
}

// TestSprint8_HTTPQueryEndpoints verifies:
// 1. GET /analytics/cash-flow
// 2. GET /analytics/category-breakdown
// 3. GET /analytics/merchant-ranking
// 4. GET /analytics/trend
// All return 200 OK with projectionFreshness.
func TestSprint8_HTTPQueryEndpoints(t *testing.T) {
	ctx := context.Background()
	pool, cleaner := setupTestDB(t)

	userID := uuid.New().String()
	cleaner.TrackUser(userID)

	sessionMgr, err := security.NewSessionTokenManager([]byte("integration-test-secret-32-bytes!"))
	if err != nil {
		t.Fatalf("new session manager: %v", err)
	}
	token, _, err := sessionMgr.Issue(userID, "user@example.com", "user", 1*time.Hour)
	if err != nil {
		t.Fatalf("issue session token: %v", err)
	}

	mod := analytics.NewModule(analytics.ModuleConfig{
		DBPool:         pool,
		SessionManager: sessionMgr,
	})

	// Seed one event
	eID := uuid.New().String()
	cleaner.TrackFinancialEvent(eID)
	now := time.Now().UTC()
	_ = mod.Projector().ProjectAnalyticsReadModels(ctx, uuid.New().String(), "FinancialEventFinalized",
		makeFinalizedPayload(eID, userID, "Expense", "42.50", "USD", "Coffee Shop", "Dining", now), now)

	router := chi.NewRouter()
	mod.RegisterHTTPRoutes(router)

	testCases := []struct {
		name       string
		path       string
		checkField string
	}{
		{"Cash Flow", "/api/v1/analytics/cash-flow?granularity=monthly", "buckets"},
		{"Category Breakdown", "/api/v1/analytics/category-breakdown", "items"},
		{"Merchant Ranking", "/api/v1/analytics/merchant-ranking?limit=5", "items"},
		{"Trend", "/api/v1/analytics/trend?granularity=monthly", "items"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.Header.Set("Authorization", "Bearer "+token)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
			}

			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("parse response json: %v", err)
			}

			if _, ok := body[tc.checkField]; !ok {
				t.Errorf("missing expected field %q in response: %s", tc.checkField, rec.Body.String())
			}

			freshness, ok := body["projectionFreshness"]
			if !ok || freshness == nil {
				t.Errorf("expected non-nil projectionFreshness, got %v", freshness)
			}
		})
	}
}
