package domain

import (
	"context"
	"time"
)

// Repository defines the storage port for the Analytics module.
// Analytics reads and writes exclusively within its own schema (06_architecture.md §7).
type Repository interface {
	// Cash Flow
	UpsertCashFlow(ctx context.Context, userID string, bucket time.Time, granularity string, income, expense float64, currency string, updatedAt time.Time) error
	GetCashFlow(ctx context.Context, userID, granularity string, from, to *time.Time, currency *string) ([]CashFlowBucket, error)

	// Category Breakdown
	UpsertCategoryBreakdown(ctx context.Context, userID string, bucket time.Time, category string, value float64, currency string, updatedAt time.Time) error
	RecalculateCategoryPercentageShares(ctx context.Context, userID string, bucket time.Time, currency string, updatedAt time.Time) error
	GetCategoryBreakdown(ctx context.Context, userID string, from, to *time.Time, currency *string) ([]CategoryBreakdownItem, error)

	// Merchant Ranking
	UpsertMerchantRanking(ctx context.Context, userID string, bucket time.Time, merchant string, value float64, count int32, currency string, updatedAt time.Time) error
	GetMerchantRanking(ctx context.Context, userID string, from, to *time.Time, currency *string, limit int32) ([]MerchantRankingItem, error)

	// Trend
	UpsertTrend(ctx context.Context, userID string, bucket time.Time, granularity string, netFlow float64, currency string, updatedAt time.Time) error
	GetTrend(ctx context.Context, userID, granularity string, from, to *time.Time, currency *string) ([]TrendItem, error)

	// Projection Checkpoint
	UpsertProjectionCheckpoint(ctx context.Context, sourceModule, outboxEventID string, checkpointAt time.Time) error
	GetProjectionCheckpoint(ctx context.Context, sourceModule string) (*ProjectionCheckpoint, error)

	// Projected Events tracking
	SaveProjectedEvent(ctx context.Context, event ProjectedEvent) error
	GetProjectedEvent(ctx context.Context, eventID string) (*ProjectedEvent, error)

	// Wiping (for full rebuild & test cleanup)
	WipeAnalyticsData(ctx context.Context) error

	// Transaction support
	ExecTx(ctx context.Context, fn func(repo Repository) error) error
}
