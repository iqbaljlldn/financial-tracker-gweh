package domain

import "time"

// PromptMetric models confidence distribution and volume per prompt version
// (08_database_design.md §13.2, 09_api_specification.md §6.6 FR-ADMIN-03).
type PromptMetric struct {
	ID                      string    `json:"id"`
	PromptVersion           string    `json:"promptVersion"`
	ModelIdentifier         string    `json:"modelIdentifier"`
	AggregateConfidence     float64   `json:"aggregateConfidence"`
	SchemaValidationOutcome string    `json:"schemaValidationOutcome"`
	LaterOverridden         bool      `json:"laterOverridden"`
	LatencyMs               int       `json:"latencyMs"`
	TokenUsage              int       `json:"tokenUsage"`
	RecordedAt              time.Time `json:"recordedAt"`
}

// PromptMetricsSummary models the aggregated metrics per prompt version and model.
type PromptMetricsSummary struct {
	PromptVersion   string  `json:"promptVersion"`
	ModelIdentifier string  `json:"modelIdentifier"`
	TotalCalls      int64   `json:"totalCalls"`
	AvgConfidence   float64 `json:"avgConfidence"`
	ValidCount      int64   `json:"validCount"`
	InvalidCount    int64   `json:"invalidCount"`
	OverriddenCount int64   `json:"overriddenCount"`
	AvgLatencyMs    float64 `json:"avgLatencyMs"`
	TotalTokens     int64   `json:"totalTokens"`
}
