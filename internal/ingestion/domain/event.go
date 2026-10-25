package domain

import (
	"time"
)

// Event names per 04_domain_model.md §6.4 & 07_sequence_diagrams.md
const (
	EventGmailAccountLinked           = "GmailAccountLinked"
	EventMessageIngested              = "MessageIngested"
	EventSyncCompleted                = "SyncCompleted"
	EventAccountDisconnected          = "AccountDisconnected"
	EventHistorySyncFallbackTriggered = "HistorySyncFallbackTriggered"
	EventUserDataPurgeRequested       = "UserDataPurgeRequested"
)

// GmailAccountLinkedPayload models EventGmailAccountLinked payload for outbox events.
type GmailAccountLinkedPayload struct {
	GmailAccountID string    `json:"gmailAccountId"`
	UserID         string    `json:"userId"`
	EmailAddress   string    `json:"emailAddress"`
	OccurredAt     time.Time `json:"occurredAt"`
}

// MessageIngestedPayload models EventMessageIngested payload for outbox events.
type MessageIngestedPayload struct {
	MessageID       string          `json:"messageId"`
	GmailAccountID  string          `json:"gmailAccountId"`
	GmailMessageID  string          `json:"gmailMessageId"`
	ReceivedAt      time.Time       `json:"receivedAt"`
	RoutingDecision RoutingDecision `json:"routingDecision"`
	RawContentRef   string          `json:"rawContentRef"`
	OccurredAt      time.Time       `json:"occurredAt"`
}

// SyncCompletedPayload models EventSyncCompleted payload.
type SyncCompletedPayload struct {
	GmailAccountID string    `json:"gmailAccountId"`
	UserID         string    `json:"userId"`
	MessagesCount  int       `json:"messagesCount"`
	FinalHistoryID string    `json:"finalHistoryId"`
	OccurredAt     time.Time `json:"occurredAt"`
}

// AccountDisconnectedPayload models EventAccountDisconnected payload.
type AccountDisconnectedPayload struct {
	GmailAccountID string    `json:"gmailAccountId"`
	UserID         string    `json:"userId"`
	Reason         string    `json:"reason"`
	PurgeRequested bool      `json:"purgeRequested"`
	OccurredAt     time.Time `json:"occurredAt"`
}

// HistorySyncFallbackTriggeredPayload models EventHistorySyncFallbackTriggered payload per 12_gmail_connector.md §6.2.
type HistorySyncFallbackTriggeredPayload struct {
	GmailAccountID string    `json:"gmailAccountId"`
	UserID         string    `json:"userId"`
	Reason         string    `json:"reason"`
	LookbackDays   int       `json:"lookbackDays"`
	OccurredAt     time.Time `json:"occurredAt"`
}

// UserDataPurgeRequestedPayload models EventUserDataPurgeRequested payload per 07_sequence_diagrams.md Diagram 8.
type UserDataPurgeRequestedPayload struct {
	GmailAccountID string    `json:"gmailAccountId"`
	UserID         string    `json:"userId"`
	OccurredAt     time.Time `json:"occurredAt"`
}
