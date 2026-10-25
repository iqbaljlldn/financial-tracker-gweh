package governance

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/observability"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/domain"
)

// BudgetConfig defines daily AI call limits per 11_ai_blueprint.md §11.
type BudgetConfig struct {
	MaxPerUserDaily  int
	MaxPlatformDaily int
}

// DefaultBudgetConfig returns standard development limits.
func DefaultBudgetConfig() BudgetConfig {
	return BudgetConfig{
		MaxPerUserDaily:  100,
		MaxPlatformDaily: 5000,
	}
}

// DailyBudgetGovernor enforces AI cost governance and circuit breaking (11_ai_blueprint.md §11) [GATE].
type DailyBudgetGovernor struct {
	mu            sync.Mutex
	config        BudgetConfig
	currentDay    string
	userCalls     map[string]int
	platformCalls int
}

// NewDailyBudgetGovernor creates a new DailyBudgetGovernor.
func NewDailyBudgetGovernor(cfg BudgetConfig) *DailyBudgetGovernor {
	if cfg.MaxPerUserDaily <= 0 {
		cfg.MaxPerUserDaily = 100
	}
	if cfg.MaxPlatformDaily <= 0 {
		cfg.MaxPlatformDaily = 5000
	}

	return &DailyBudgetGovernor{
		config:     cfg,
		currentDay: time.Now().UTC().Format("2006-01-02"),
		userCalls:  make(map[string]int),
	}
}

// AcquireCall checks budget allowances and records an AI call.
// Returns domain.ErrLLMBudgetExceeded if either limit is reached, tripping the circuit breaker.
func (g *DailyBudgetGovernor) AcquireCall(_ context.Context, userID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	today := time.Now().UTC().Format("2006-01-02")
	if today != g.currentDay {
		// Reset counters on new UTC day
		g.currentDay = today
		g.userCalls = make(map[string]int)
		g.platformCalls = 0
	}

	// 1. Platform-wide circuit breaker check
	if g.platformCalls >= g.config.MaxPlatformDaily {
		g.updateMetrics(userID)
		return fmt.Errorf("%w: platform limit reached (%d/%d)",
			domain.ErrLLMBudgetExceeded, g.platformCalls, g.config.MaxPlatformDaily)
	}

	// 2. Per-user circuit breaker check
	userCount := g.userCalls[userID]
	if userID != "" && userCount >= g.config.MaxPerUserDaily {
		g.updateMetrics(userID)
		return fmt.Errorf("%w: user limit reached for %s (%d/%d)",
			domain.ErrLLMBudgetExceeded, userID, userCount, g.config.MaxPerUserDaily)
	}

	// Increment usage
	g.platformCalls++
	if userID != "" {
		g.userCalls[userID] = userCount + 1
	}

	g.updateMetrics(userID)
	return nil
}

func (g *DailyBudgetGovernor) updateMetrics(userID string) {
	metrics := observability.GetMetrics()
	if metrics == nil {
		return
	}

	platformRatio := float64(g.platformCalls) / float64(g.config.MaxPlatformDaily)
	metrics.UnderstandingAIDailyBudgetConsumedRatio.WithLabelValues("platform").Set(platformRatio)

	if userID != "" {
		userRatio := float64(g.userCalls[userID]) / float64(g.config.MaxPerUserDaily)
		metrics.UnderstandingAIDailyBudgetConsumedRatio.WithLabelValues("per_user").Set(userRatio)
	}
}
