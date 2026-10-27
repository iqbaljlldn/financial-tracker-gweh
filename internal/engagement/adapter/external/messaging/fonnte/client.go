package fonnte

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

// Config defines connection details for the Fonnte WhatsApp Gateway API.
type Config struct {
	BaseURL     string
	APIToken    string
	CountryCode string
	Timeout     time.Duration
}

// Client implements domain.MessagingClient using Fonnte API (https://api.fonnte.com).
type Client struct {
	cfg        Config
	httpClient *http.Client
}

// New constructs a new Fonnte messaging client.
func New(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.fonnte.com"
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.CountryCode == "" {
		cfg.CountryCode = "62"
	}
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

// ProviderName returns "fonnte".
func (c *Client) ProviderName() string {
	return "fonnte"
}

type fonnteSendRequest struct {
	Target      string `json:"target"`
	Message     string `json:"message"`
	CountryCode string `json:"countryCode"`
	Button      string `json:"button,omitempty"`
}

type fonnteSendResponse struct {
	Status  bool     `json:"status"`
	ID      []string `json:"id,omitempty"`
	Reason  string   `json:"reason,omitempty"`
	Detail  string   `json:"detail,omitempty"`
	Process string   `json:"process,omitempty"`
}

// SendMessage dispatches a WhatsApp message via Fonnte /send endpoint.
func (c *Client) SendMessage(ctx context.Context, msg domain.OutboundMessage) (*domain.SendResult, error) {
	if c.cfg.APIToken == "" {
		return nil, errors.New("fonnte: api token is not configured")
	}
	if msg.RecipientPhone == "" {
		return nil, errors.New("fonnte: recipient phone number is empty")
	}

	target := normalizePhoneNumber(msg.RecipientPhone, c.cfg.CountryCode)

	messageContent := msg.RenderedText
	if messageContent == "" && msg.Parameters != nil {
		messageContent = fmt.Sprintf("Notification: %s", msg.TemplateID)
	}

	reqBody := fonnteSendRequest{
		Target:      target,
		Message:     messageContent,
		CountryCode: c.cfg.CountryCode,
	}

	// If interactive buttons are specified, attach them in Fonnte format
	if len(msg.Buttons) > 0 {
		type fonnteButton struct {
			ID   string `json:"id"`
			Text string `json:"text"`
		}
		fButtons := make([]fonnteButton, len(msg.Buttons))
		for i, b := range msg.Buttons {
			fButtons[i] = fonnteButton{ID: b.ID, Text: b.Label}
		}
		rawButtons, err := json.Marshal(fButtons)
		if err == nil {
			reqBody.Button = string(rawButtons)
		}
	}

	rawJSON, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("fonnte: marshaling request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/send", c.cfg.BaseURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(rawJSON))
	if err != nil {
		return nil, fmt.Errorf("fonnte: creating request: %w", err)
	}

	httpReq.Header.Set("Authorization", c.cfg.APIToken)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("fonnte: executing request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("fonnte: reading response: %w", err)
	}

	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("fonnte: server error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var res fonnteSendResponse
	if err := json.Unmarshal(bodyBytes, &res); err != nil {
		return nil, fmt.Errorf("fonnte: unmarshaling response (%s): %w", string(bodyBytes), err)
	}

	if !res.Status {
		// Classify error: device disconnected, invalid number, blocked
		reason := res.Reason
		if reason == "" {
			reason = res.Detail
		}
		if reason == "" {
			reason = string(bodyBytes)
		}
		return nil, fmt.Errorf("fonnte: delivery rejected: %s", reason)
	}

	msgID := ""
	if len(res.ID) > 0 {
		msgID = res.ID[0]
	}

	return &domain.SendResult{
		ProviderMessageID: msgID,
		Status:            domain.DeliveryStatusPending,
		Provider:          "fonnte",
	}, nil
}

// normalizePhoneNumber ensures phone number format suitable for Indonesian & international WhatsApp.
func normalizePhoneNumber(phone, defaultCountryCode string) string {
	cleaned := strings.TrimSpace(phone)
	cleaned = strings.ReplaceAll(cleaned, "-", "")
	cleaned = strings.ReplaceAll(cleaned, " ", "")
	cleaned = strings.TrimPrefix(cleaned, "+")

	if strings.HasPrefix(cleaned, "0") && defaultCountryCode != "" {
		cleaned = defaultCountryCode + strings.TrimPrefix(cleaned, "0")
	}
	return cleaned
}
