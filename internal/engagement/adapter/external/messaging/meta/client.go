package meta

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/engagement/domain"
)

// Config holds credentials for Meta WhatsApp Cloud API.
type Config struct {
	BaseURL            string
	PhoneNumberID      string
	AccessToken        string
	AppSecret          string
	WebhookVerifyToken string
	Timeout            time.Duration
}

// Client implements domain.MessagingClient using Meta WhatsApp Cloud API.
type Client struct {
	cfg        Config
	httpClient *http.Client
}

// New constructs a Meta Cloud API client.
func New(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://graph.facebook.com/v20.0"
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

var _ domain.MessagingClient = (*Client)(nil)

// ProviderName returns "meta".
func (c *Client) ProviderName() string {
	return "meta"
}

// SendMessage delivers message via Meta WhatsApp Graph API.
func (c *Client) SendMessage(ctx context.Context, msg domain.OutboundMessage) (*domain.SendResult, error) {
	if c.cfg.AccessToken == "" {
		return nil, errors.New("meta: access token is empty")
	}
	if c.cfg.PhoneNumberID == "" {
		return nil, errors.New("meta: phone number id is empty")
	}
	if msg.RecipientPhone == "" {
		return nil, errors.New("meta: recipient phone is empty")
	}

	target := strings.TrimPrefix(msg.RecipientPhone, "+")

	// Payload construction: text or interactive button
	var payload map[string]any
	if len(msg.Buttons) > 0 {
		buttons := make([]map[string]any, len(msg.Buttons))
		for i, b := range msg.Buttons {
			buttons[i] = map[string]any{
				"type": "reply",
				"reply": map[string]string{
					"id":    b.ID,
					"title": b.Label,
				},
			}
		}
		payload = map[string]any{
			"messaging_product": "whatsapp",
			"recipient_type":    "individual",
			"to":                target,
			"type":              "interactive",
			"interactive": map[string]any{
				"type": "button",
				"body": map[string]string{
					"text": msg.RenderedText,
				},
				"action": map[string]any{
					"buttons": buttons,
				},
			},
		}
	} else {
		payload = map[string]any{
			"messaging_product": "whatsapp",
			"recipient_type":    "individual",
			"to":                target,
			"type":              "text",
			"text": map[string]string{
				"body": msg.RenderedText,
			},
		}
	}

	rawJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("meta: marshaling payload: %w", err)
	}

	url := fmt.Sprintf("%s/%s/messages", c.cfg.BaseURL, c.cfg.PhoneNumberID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(rawJSON))
	if err != nil {
		return nil, fmt.Errorf("meta: creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.cfg.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("meta: executing request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("meta: reading response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("meta: API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var res struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(bodyBytes, &res); err != nil {
		return nil, fmt.Errorf("meta: parsing response: %w", err)
	}

	msgID := ""
	if len(res.Messages) > 0 {
		msgID = res.Messages[0].ID
	}

	return &domain.SendResult{
		ProviderMessageID: msgID,
		Status:            domain.DeliveryStatusPending,
		Provider:          "meta",
	}, nil
}
