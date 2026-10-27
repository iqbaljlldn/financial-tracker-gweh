package domain

import (
	"context"
)

// AuditLogRepository defines append-only access to administration.audit_log.
// Tamper-resistant: never supports update or delete.
type AuditLogRepository interface {
	Insert(ctx context.Context, entry *AuditLogEntry) error
	List(ctx context.Context, filter AuditLogFilter) ([]*AuditLogEntry, int64, error)
}

// ConnectorHealthRepository defines persistence for connector health metrics.
type ConnectorHealthRepository interface {
	Insert(ctx context.Context, health *ConnectorHealth) error
	ListLatest(ctx context.Context) ([]*ConnectorHealth, error)
}

// ParserMetricsRepository defines persistence for parser metrics.
type ParserMetricsRepository interface {
	Insert(ctx context.Context, metric *ParserMetric) error
	GetAggregated(ctx context.Context) ([]*ParserMetric, error)
}

// PromptMetricsRepository defines persistence for prompt evaluation metrics.
type PromptMetricsRepository interface {
	Insert(ctx context.Context, metric *PromptMetric) error
	GetSummary(ctx context.Context) ([]*PromptMetricsSummary, error)
	ListRecent(ctx context.Context, limit int32) ([]*PromptMetric, error)
}

// AuditLogger defines the interface used by cross-module components and middleware
// to append tamper-resistant audit logs.
type AuditLogger interface {
	Log(ctx context.Context, userID *string, action, actor string, metadata map[string]any) error
}

// Ensure interface adherence
var _ AuditLogger = (AuditLogger)(nil)
