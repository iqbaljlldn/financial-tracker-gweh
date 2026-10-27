package http

import "time"

// AuditLogItemDTO represents an audit log entry in the API response.
type AuditLogItemDTO struct {
	ID         string         `json:"id"`
	UserID     *string        `json:"userId,omitempty"`
	Action     string         `json:"action"`
	Actor      string         `json:"actor"`
	Metadata   map[string]any `json:"metadata"`
	OccurredAt string         `json:"occurredAt"`
}

// AuditLogListResponse represents GET /admin/audit-log response.
type AuditLogListResponse struct {
	Items      []AuditLogItemDTO  `json:"items"`
	Pagination PaginationResponse `json:"pagination"`
}

type PaginationResponse struct {
	Total  int64 `json:"total"`
	Limit  int   `json:"limit"`
	Offset int   `json:"offset"`
}

// ConnectorHealthDTO represents GET /admin/connectors/health item.
type ConnectorHealthDTO struct {
	ID                 string    `json:"id"`
	ConnectorType      string    `json:"connectorType"`
	ConnectorAccountID string    `json:"connectorAccountId"`
	Status             string    `json:"status"`
	ErrorCount         int       `json:"errorCount"`
	LatencyMs          int       `json:"latencyMs"`
	RecordedAt         time.Time `json:"recordedAt"`
}

// ParserMetricDTO represents GET /admin/parsers/metrics item.
type ParserMetricDTO struct {
	SenderPattern   string    `json:"senderPattern"`
	TemplateVersion int       `json:"templateVersion"`
	TotalParsed     int       `json:"totalParsed"`
	FailedParsed    int       `json:"failedParsed"`
	FailureRate     float64   `json:"failureRate"`
	SpikeAlert      bool      `json:"spikeAlert"`
	LastRecordedAt  time.Time `json:"lastRecordedAt"`
}

// PromptMetricSummaryDTO represents GET /admin/prompts/metrics item.
type PromptMetricSummaryDTO struct {
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

// ErrorResponse follows 09_api_specification.md §7.2 standard error envelope.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}
