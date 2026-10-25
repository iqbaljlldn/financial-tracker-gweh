package http

import (
	"time"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/domain"
)

// TemplateDTO represents the wire format for a SenderTemplate.
type TemplateDTO struct {
	ID            string                 `json:"id"`
	SenderPattern string                 `json:"senderPattern"`
	Version       int                    `json:"version"`
	Rules         domain.ExtractionRules `json:"rules"`
	Active        bool                   `json:"active"`
	CreatedAt     time.Time              `json:"createdAt"`
}

// ExtractionResultDTO represents the wire format for an ExtractionResult record.
type ExtractionResultDTO struct {
	ID                  string                          `json:"id"`
	IngestedMessageID   string                          `json:"ingestedMessageId"`
	Method              string                          `json:"method"`
	TemplateVersion     *string                         `json:"templateVersion,omitempty"`
	Candidate           *domain.CandidateFinancialEvent `json:"candidate,omitempty"`
	ConfidenceAggregate float64                         `json:"confidenceAggregate"`
	ConfidencePerField  map[string]float64              `json:"confidencePerField,omitempty"`
	CreatedAt           time.Time                       `json:"createdAt"`
}

// PublishTemplateRequest represents the payload to publish or update a template.
type PublishTemplateRequest struct {
	SenderPattern   string                 `json:"senderPattern"`
	Version         int                    `json:"version,omitempty"`
	ExtractionRules domain.ExtractionRules `json:"extractionRules"`
}

// ParserMetricsDTO represents the wire format for rolling failure rate metrics.
type ParserMetricsDTO struct {
	ExaminedTemplates int                 `json:"examinedTemplates"`
	Rollups           []TemplateRollupDTO `json:"rollups"`
	SpikeAlerts       []string            `json:"spikeAlerts,omitempty"`
}

// TemplateRollupDTO represents failure rate metrics for one template.
type TemplateRollupDTO struct {
	SenderPattern string  `json:"senderPattern"`
	Version       int     `json:"version"`
	Total1h       int     `json:"total1h"`
	Failed1h      int     `json:"failed1h"`
	Rate1h        float64 `json:"rate1h"`
	Total24h      int     `json:"total24h"`
	Failed24h     int     `json:"failed24h"`
	Rate24h       float64 `json:"rate24h"`
}
