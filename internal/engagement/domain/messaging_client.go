package domain

import (
	"context"
)

// ActionButton models an interactive button (e.g. Quick Reply) on WhatsApp.
type ActionButton struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// OutboundMessage carries the platform-agnostic notification payload to deliver.
type OutboundMessage struct {
	RecipientPhone   string            `json:"recipientPhone"`
	NotificationType NotificationType  `json:"notificationType"`
	TemplateID       string            `json:"templateId"`
	Parameters       map[string]string `json:"parameters"`
	RenderedText     string            `json:"renderedText"`
	Buttons          []ActionButton    `json:"buttons,omitempty"`
}

// SendResult captures the outcome returned by the messaging provider.
type SendResult struct {
	ProviderMessageID string
	Status            DeliveryStatus
	Provider          string
}

// MessagingClient is the platform-agnostic port interface for delivering WhatsApp messages.
// Adapters exist for Fonnte, Meta WhatsApp Cloud API, and Mock.
type MessagingClient interface {
	SendMessage(ctx context.Context, msg OutboundMessage) (*SendResult, error)
	ProviderName() string
}
