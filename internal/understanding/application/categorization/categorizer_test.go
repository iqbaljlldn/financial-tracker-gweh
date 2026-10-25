package categorization_test

import (
	"context"
	"testing"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/application/categorization"
)

func TestCategorizationHierarchy_StrictShortCircuit(t *testing.T) {
	ctx := context.Background()
	userStore := categorization.NewInMemoryUserHistoryStore()
	globalStore := categorization.NewInMemoryGlobalMappingStore()

	// Seed global mapping: "Starbucks" -> "Dining"
	_ = globalStore.SetCategory(ctx, "starbucks", "Dining", 0.75)

	engine := categorization.NewEngine(categorization.Config{
		UserHistory:   userStore,
		GlobalMapping: globalStore,
		AIThreshold:   0.50,
	})

	// Test 1: Tier 1 - User History overrides Global Mapping and AI Inference [GATE]
	// Suppose user prefers "Coffee Addiction" for Starbucks instead of "Dining"
	_ = userStore.SaveCategory(ctx, "user-1", "starbucks", "Coffee Addiction")

	res1 := engine.AssignCategory(ctx, categorization.Request{
		UserID:             "user-1",
		NormalizedMerchant: "Starbucks",
		CategoryHint:       "Food & Beverage", // from AI
		CategoryConfidence: 0.95,
	})
	if res1.Category != "Coffee Addiction" || res1.Source != categorization.SourceUserHistory {
		t.Fatalf("expected Tier 1 user_history short-circuit 'Coffee Addiction', got %s (%s)", res1.Category, res1.Source)
	}

	// Test 2: Tier 2 - Global Mapping used when user has no personal history
	res2 := engine.AssignCategory(ctx, categorization.Request{
		UserID:             "user-2", // different user without history
		NormalizedMerchant: "Starbucks",
		CategoryHint:       "Food & Beverage", // from AI
		CategoryConfidence: 0.95,
	})
	if res2.Category != "Dining" || res2.Source != categorization.SourceGlobalMapping {
		t.Fatalf("expected Tier 2 global_mapping short-circuit 'Dining', got %s (%s)", res2.Category, res2.Source)
	}

	// Test 3: Tier 3 - AI Inference used for unknown merchant
	res3 := engine.AssignCategory(ctx, categorization.Request{
		UserID:             "user-1",
		NormalizedMerchant: "New Fancy Boutique", // unknown to history and global
		CategoryHint:       "Apparel",
		CategoryConfidence: 0.80,
	})
	if res3.Category != "Apparel" || res3.Source != categorization.SourceAIInference {
		t.Fatalf("expected Tier 3 ai_inference short-circuit 'Apparel', got %s (%s)", res3.Category, res3.Source)
	}

	// Test 4: Tier 4 - Fallback to "Uncategorized" when AI confidence is below threshold
	res4 := engine.AssignCategory(ctx, categorization.Request{
		UserID:             "user-1",
		NormalizedMerchant: "Vague Store",
		CategoryHint:       "General",
		CategoryConfidence: 0.30, // below 0.50 threshold
	})
	if res4.Category != categorization.CategoryUncategorized || res4.Source != categorization.SourceDefault {
		t.Fatalf("expected Tier 4 fallback 'Uncategorized', got %s (%s)", res4.Category, res4.Source)
	}
}

func TestCategorization_FeedbackLoop(t *testing.T) {
	ctx := context.Background()
	userStore := categorization.NewInMemoryUserHistoryStore()
	globalStore := categorization.NewInMemoryGlobalMappingStore()

	engine := categorization.NewEngine(categorization.Config{
		UserHistory:   userStore,
		GlobalMapping: globalStore,
	})

	// User corrects an event's category to "Work Expenses"
	err := engine.RecordUserCorrection(ctx, "user-abc", "staples", "Work Expenses")
	if err != nil {
		t.Fatalf("unexpected error recording correction: %v", err)
	}

	// Future categorizations for that merchant now immediately return "Work Expenses"
	res := engine.AssignCategory(ctx, categorization.Request{
		UserID:             "user-abc",
		NormalizedMerchant: "Staples",
	})
	if res.Category != "Work Expenses" || res.Source != categorization.SourceUserHistory {
		t.Fatalf("expected feedback loop to update user_history, got %s (%s)", res.Category, res.Source)
	}
}
