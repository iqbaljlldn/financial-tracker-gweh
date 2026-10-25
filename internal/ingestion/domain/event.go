package domain

import (
	"time"
)

// Event names
const (
	EventGmailAccountLinked  = "GmailAccountLinked"
	EventMessageIngested     = "MessageIngested"
	EventSyncCompleted       = "SyncCompleted"
	EventAccountDisconnected = "AccountDisconnected"
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
	OccurredAt     time.Time `json:"occurredAt"`
}
