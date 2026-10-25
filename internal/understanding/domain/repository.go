package domain

import (
	"context"
	"time"
)

// RawEmailMessage encapsulates raw email payload needed for template or AI extraction.
type RawEmailMessage struct {
	MessageID      string
	GmailAccountID string
	Sender         string
	Subject        string
	Headers        map[string]string
	BodyHTML       string
	BodyText       string
	ReceivedAt     time.Time
}

// ExtractionResultRecord models understanding.extraction_results row (08_database_design.md §9.2).
type ExtractionResultRecord struct {
	ID                  string
	IngestedMessageID   string
	Method              string
	TemplateVersion     *string
	PromptVersion       *string
	ModelIdentifier     *string
	CandidatePayload    CandidateFinancialEvent
	ConfidenceAggregate float64
	ConfidencePerField  map[string]float64
	CreatedAt           time.Time
}

// SenderTemplateRepository defines persistence operations for SenderTemplate aggregates.
type SenderTemplateRepository interface {
	FindActiveByPattern(ctx context.Context, pattern string) (*SenderTemplate, error)
	ListActive(ctx context.Context) ([]*SenderTemplate, error)
	FindByID(ctx context.Context, id string) (*SenderTemplate, error)
	GetLatestVersionForPattern(ctx context.Context, pattern string) (int, error)
	// PublishAtomic atomically inserts the new template with active=true and deactivates any existing
	// active template for the same senderPattern within a single transaction.
	// Old versions are never deleted per 13_email_parser.md §6.2 [GATE].
	PublishAtomic(ctx context.Context, newTemplate *SenderTemplate) error
}

// ExtractionResultRepository defines persistence operations for extraction_results and monitoring queries.
type ExtractionResultRepository interface {
	SaveResult(ctx context.Context, result *ExtractionResultRecord) error
	ListRecent(ctx context.Context, limit int) ([]*ExtractionResultRecord, error)
	GetTemplateStatsInWindow(ctx context.Context, templateVersion string, since time.Time) (total int, failed int, err error)
}

// OutboxRepository defines transactional outbox publishing for understanding domain events.
type OutboxRepository interface {
	RecordOutboxEvent(ctx context.Context, aggregateType, aggregateID, eventType string, payload any) error
}

// PromptMetricRecord models an entry in administration.prompt_metrics (08_database_design.md §13.2, 11_ai_blueprint.md §10).
type PromptMetricRecord struct {
	ID                      string
	PromptVersion           string
	ModelIdentifier         string
	AggregateConfidence     float64
	SchemaValidationOutcome string // "valid_first_try" | "valid_after_reprompt" | "invalid"
	LaterOverridden         bool
	LatencyMs               int
	TokenUsage              int
	RecordedAt              time.Time
}

// PromptMetricsRepository defines persistence for AI prompt performance tracking.
type PromptMetricsRepository interface {
	RecordPromptMetric(ctx context.Context, metric *PromptMetricRecord) error
}

// RawMessageProvider retrieves the raw email message content for extraction.
type RawMessageProvider interface {
	GetRawMessage(ctx context.Context, messageID string) (*RawEmailMessage, error)
}
