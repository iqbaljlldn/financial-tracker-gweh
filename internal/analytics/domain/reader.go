package domain

import (
	"context"
	"time"
)

// AnalyticsSnapshotReader defines the sanctioned cross-module read interface per 10_backend_blueprint.md §6.
// Consuming modules (e.g., engagement) import this domain interface to fetch read models
// without violating Clean Architecture or module boundary rules (06_architecture.md §5.2).
type AnalyticsSnapshotReader interface {
	GetCashFlow(ctx context.Context, userID, granularity string, from, to *time.Time, currency *string) ([]CashFlowBucket, error)
	GetCategoryBreakdown(ctx context.Context, userID string, from, to *time.Time, currency *string) ([]CategoryBreakdownItem, error)
	GetTrend(ctx context.Context, userID, granularity string, from, to *time.Time, currency *string) ([]TrendItem, error)
}
