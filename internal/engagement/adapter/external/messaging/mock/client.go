package mock

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/engagement/domain"
)

// Client is an in-memory messaging client for deterministic testing.
type Client struct {
	mu           sync.Mutex
	sentMessages []domain.OutboundMessage
	failNextN    int
	permanentErr bool
}

// New constructs a new Mock messaging client.
func New() *Client {
	return &Client{
		sentMessages: make([]domain.OutboundMessage, 0),
	}
}

var _ domain.MessagingClient = (*Client)(nil)

// ProviderName returns "mock".
func (c *Client) ProviderName() string {
	return "mock"
}

// SendMessage records the outbound message or returns configured error.
func (c *Client) SendMessage(ctx context.Context, msg domain.OutboundMessage) (*domain.SendResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.failNextN > 0 {
		c.failNextN--
		if c.permanentErr {
			return nil, errors.New("mock: permanent delivery failure (number blocked)")
		}
		return nil, errors.New("mock: transient network timeout")
	}

	c.sentMessages = append(c.sentMessages, msg)
	return &domain.SendResult{
		ProviderMessageID: fmt.Sprintf("mock_msg_%s", uuid.New().String()[:8]),
		Status:            domain.DeliveryStatusPending,
		Provider:          "mock",
	}, nil
}

// SentMessages returns a copy of all recorded sent messages.
func (c *Client) SentMessages() []domain.OutboundMessage {
	c.mu.Lock()
	defer c.mu.Unlock()
	copied := make([]domain.OutboundMessage, len(c.sentMessages))
	copy(copied, c.sentMessages)
	return copied
}

// Reset clears recorded messages and simulated errors.
func (c *Client) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sentMessages = nil
	c.failNextN = 0
	c.permanentErr = false
}

// SimulateFailure configures subsequent calls to fail.
func (c *Client) SimulateFailure(count int, permanent bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.failNextN = count
	c.permanentErr = permanent
}
