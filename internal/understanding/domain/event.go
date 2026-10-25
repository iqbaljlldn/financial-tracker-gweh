package domain

import (
	"time"
)

// Domain event names per 04_domain_model.md §7.3 & 07_sequence_diagrams.md Diagram 3
const (
	EventTemplateExtractionSucceeded    = "TemplateExtractionSucceeded"
	EventTemplateExtractionFailed       = "TemplateExtractionFailed"
	EventFinancialEventCandidateCreated = "FinancialEventCandidateCreated"
)

// TemplateExtractionSucceededPayload models EventTemplateExtractionSucceeded payload.
type TemplateExtractionSucceededPayload struct {
	IngestedMessageID string    `json:"ingestedMessageId"`
	TemplateID        string    `json:"templateId"`
	SenderPattern     string    `json:"senderPattern"`
	TemplateVersion   int       `json:"templateVersion"`
	Confidence        float64   `json:"confidence"`
	OccurredAt        time.Time `json:"occurredAt"`
}

// TemplateExtractionFailedPayload models EventTemplateExtractionFailed payload.
// Emitted on required-field extraction failure and consumed by Parser Monitoring & AI fallback trigger.
type TemplateExtractionFailedPayload struct {
	IngestedMessageID string    `json:"ingestedMessageId"`
	TemplateID        string    `json:"templateId,omitempty"`
	SenderPattern     string    `json:"senderPattern"`
	TemplateVersion   int       `json:"templateVersion,omitempty"`
	Reason            string    `json:"reason"`
	MissingFields     []string  `json:"missingFields,omitempty"`
	OccurredAt        time.Time `json:"occurredAt"`
}

// FinancialEventCandidateCreatedPayload models EventFinancialEventCandidateCreated payload.
// Outbox event dequeued by Financial Core's EvaluateConfidenceGate policy per Diagram 3.
type FinancialEventCandidateCreatedPayload struct {
	ExtractionResultID string                  `json:"extractionResultId"`
	IngestedMessageID  string                  `json:"ingestedMessageId"`
	Candidate          CandidateFinancialEvent `json:"candidate"`
	OccurredAt         time.Time               `json:"occurredAt"`
}
