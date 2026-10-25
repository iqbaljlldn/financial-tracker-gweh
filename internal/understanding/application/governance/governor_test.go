package governance_test

import (
	"context"
	"errors"
	"testing"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/application/governance"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/domain"
)

func TestDailyBudgetGovernor_PerUserLimit(t *testing.T) {
	ctx := context.Background()
	gov := governance.NewDailyBudgetGovernor(governance.BudgetConfig{
		MaxPerUserDaily:  3,
		MaxPlatformDaily: 100,
	})

	for i := 0; i < 3; i++ {
		if err := gov.AcquireCall(ctx, "user-1"); err != nil {
			t.Fatalf("call %d should succeed, got %v", i+1, err)
		}
	}

	// 4th call should trip circuit breaker
	err := gov.AcquireCall(ctx, "user-1")
	if err == nil {
		t.Fatal("expected ErrLLMBudgetExceeded, got nil")
	}
	if !errors.Is(err, domain.ErrLLMBudgetExceeded) {
		t.Fatalf("expected ErrLLMBudgetExceeded, got %v", err)
	}

	// Another user should still be able to make calls
	if err := gov.AcquireCall(ctx, "user-2"); err != nil {
		t.Fatalf("user-2 should not be blocked by user-1, got %v", err)
	}
}

func TestDailyBudgetGovernor_PlatformLimit(t *testing.T) {
	ctx := context.Background()
	gov := governance.NewDailyBudgetGovernor(governance.BudgetConfig{
		MaxPerUserDaily:  10,
		MaxPlatformDaily: 2,
	})

	if err := gov.AcquireCall(ctx, "u1"); err != nil {
		t.Fatalf("u1 call should succeed: %v", err)
	}
	if err := gov.AcquireCall(ctx, "u2"); err != nil {
		t.Fatalf("u2 call should succeed: %v", err)
	}

	// 3rd call exceeds platform limit
	err := gov.AcquireCall(ctx, "u3")
	if !errors.Is(err, domain.ErrLLMBudgetExceeded) {
		t.Fatalf("expected ErrLLMBudgetExceeded on platform limit, got %v", err)
	}
}
