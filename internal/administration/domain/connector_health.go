package domain

import "time"

// ConnectorHealth models health status and metrics for an external connector
// (08_database_design.md §13.2, 09_api_specification.md §6.6 FR-ADMIN-01).
type ConnectorHealth struct {
	ID                 string    `json:"id"`
	ConnectorType      string    `json:"connectorType"`
	ConnectorAccountID string    `json:"connectorAccountId"`
	Status             string    `json:"status"`
	ErrorCount         int       `json:"errorCount"`
	LatencyMs          int       `json:"latencyMs"`
	RecordedAt         time.Time `json:"recordedAt"`
}
