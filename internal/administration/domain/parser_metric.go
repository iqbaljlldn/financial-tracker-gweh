package domain

import "time"

// ParserMetric models success/failure rate per SenderTemplate
// (08_database_design.md §13.2, 09_api_specification.md §6.6 FR-ADMIN-02).
type ParserMetric struct {
	ID              string    `json:"id"`
	SenderPattern   string    `json:"senderPattern"`
	TemplateVersion int       `json:"templateVersion"`
	TotalParsed     int       `json:"totalParsed"`
	FailedParsed    int       `json:"failedParsed"`
	FailureRate     float64   `json:"failureRate"`
	SpikeAlert      bool      `json:"spikeAlert"`
	RecordedAt      time.Time `json:"recordedAt"`
}
