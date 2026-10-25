package domain

import (
	"errors"
	"strings"
	"time"
)

type RoutingDecision string

const (
	RoutingTemplateParser RoutingDecision = "template_parser"
	RoutingAIExtractor    RoutingDecision = "ai_extractor"
	RoutingDiscarded      RoutingDecision = "discarded"
)

type ProcessingStatus string

const (
	ProcessingQueued     ProcessingStatus = "queued"
	ProcessingProcessing ProcessingStatus = "processing"
	ProcessingCompleted  ProcessingStatus = "completed"
	ProcessingFailed     ProcessingStatus = "failed"
)

var (
	ErrEmptyMessageFields = errors.New("id, accountID, and gmailMessageID cannot be empty")
)

// IngestedMessage represents an ingested raw message record per 08_database_design.md §8.2.
type IngestedMessage struct {
	id               string
	gmailAccountID   string
	gmailMessageID   string
	receivedAt       time.Time
	rawContentRef    string
	routingDecision  RoutingDecision
	processingStatus ProcessingStatus
	createdAt        time.Time
}

// NewIngestedMessage constructs a new IngestedMessage entity.
func NewIngestedMessage(
	id string,
	gmailAccountID string,
	gmailMessageID string,
	receivedAt time.Time,
	rawContentRef string,
	decision RoutingDecision,
) (*IngestedMessage, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(gmailAccountID) == "" || strings.TrimSpace(gmailMessageID) == "" {
		return nil, ErrEmptyMessageFields
	}

	return &IngestedMessage{
		id:               strings.TrimSpace(id),
		gmailAccountID:   strings.TrimSpace(gmailAccountID),
		gmailMessageID:   strings.TrimSpace(gmailMessageID),
		receivedAt:       receivedAt.UTC(),
		rawContentRef:    rawContentRef,
		routingDecision:  decision,
		processingStatus: ProcessingQueued,
		createdAt:        time.Now().UTC(),
	}, nil
}

// ReconstituteIngestedMessage reconstitutes an existing entity from persistence.
func ReconstituteIngestedMessage(
	id, gmailAccountID, gmailMessageID string,
	receivedAt time.Time,
	rawContentRef string,
	decision RoutingDecision,
	status ProcessingStatus,
	createdAt time.Time,
) *IngestedMessage {
	return &IngestedMessage{
		id:               id,
		gmailAccountID:   gmailAccountID,
		gmailMessageID:   gmailMessageID,
		receivedAt:       receivedAt,
		rawContentRef:    rawContentRef,
		routingDecision:  decision,
		processingStatus: status,
		createdAt:        createdAt,
	}
}

// Getters
func (m *IngestedMessage) ID() string                       { return m.id }
func (m *IngestedMessage) GmailAccountID() string           { return m.gmailAccountID }
func (m *IngestedMessage) GmailMessageID() string           { return m.gmailMessageID }
func (m *IngestedMessage) ReceivedAt() time.Time            { return m.receivedAt }
func (m *IngestedMessage) RawContentRef() string            { return m.rawContentRef }
func (m *IngestedMessage) RoutingDecision() RoutingDecision { return m.routingDecision }
func (m *IngestedMessage) ProcessingStatus() ProcessingStatus {
	return m.processingStatus
}
func (m *IngestedMessage) CreatedAt() time.Time { return m.createdAt }

func (m *IngestedMessage) MarkCompleted() {
	m.processingStatus = ProcessingCompleted
}

func (m *IngestedMessage) MarkFailed() {
	m.processingStatus = ProcessingFailed
}
