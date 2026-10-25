package external

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/identity/domain"
	ingestionDomain "github.com/iqbaljlldn/financial-tracker-gweh/internal/ingestion/domain"
)

const (
	DefaultGmailAPIBase = "https://gmail.googleapis.com/gmail/v1/users"
)

// GmailClientConfig provides configuration for the external Gmail API connector adapter.
type GmailClientConfig struct {
	BaseURL       string // Configurable for mock server integration testing
	HTTPClient    *http.Client
	TokenProvider domain.TokenProvider // Sanctioned cross-module interface per 10_backend_blueprint.md §6
	PubSubTopic   string               // projects/{project}/topics/{topic} for users.watch
}

// GmailClient coordinates direct interactions with the Google Gmail REST API.
// Implements ingestionDomain.Connector per 12_gmail_connector.md §3.
type GmailClient struct {
	baseURL       string
	httpClient    *http.Client
	tokenProvider domain.TokenProvider
	pubSubTopic   string
}

// NewGmailClient constructs a new GmailClient.
func NewGmailClient(cfg GmailClientConfig) (*GmailClient, error) {
	if cfg.TokenProvider == nil {
		return nil, errors.New("gmailclient: TokenProvider is required")
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = DefaultGmailAPIBase
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	return &GmailClient{
		baseURL:       strings.TrimSuffix(baseURL, "/"),
		httpClient:    httpClient,
		tokenProvider: cfg.TokenProvider,
		pubSubTopic:   cfg.PubSubTopic,
	}, nil
}

// Authenticate verifies connector credentials against the Gmail user profile.
func (c *GmailClient) Authenticate(ctx context.Context, creds ingestionDomain.ConnectorCredentials) (*ingestionDomain.ConnectorAccount, error) {
	token, err := c.tokenProvider.GetValidAccessToken(ctx, creds.AccountID)
	if err != nil {
		return nil, fmt.Errorf("gmail authenticate: token retrieval failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/me/profile", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gmail authenticate: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gmail authenticate: google returned %d: %s", resp.StatusCode, string(body))
	}

	var profile struct {
		EmailAddress  string `json:"emailAddress"`
		HistoryID     string `json:"historyId"`
		MessagesTotal int    `json:"messagesTotal"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return nil, fmt.Errorf("gmail authenticate: decode profile failed: %w", err)
	}

	return &ingestionDomain.ConnectorAccount{
		ID:           creds.AccountID,
		EmailAddress: profile.EmailAddress,
		Provider:     "gmail",
	}, nil
}

// InitialSync fetches a paginated batch of messages within the bounded historical window (default 90 days).
// Uses search query pre-filtering per 12_gmail_connector.md §6.3 & §7.2 to avoid scanning non-financial mail.
func (c *GmailClient) InitialSync(
	ctx context.Context,
	account *ingestionDomain.ConnectorAccount,
	windowDays int,
	pageToken string,
) (*ingestionDomain.SyncBatch, error) {
	token, err := c.tokenProvider.GetValidAccessToken(ctx, account.ID)
	if err != nil {
		return nil, fmt.Errorf("gmail initial sync: token error: %w", err)
	}

	if windowDays <= 0 {
		windowDays = 90
	}
	afterDate := time.Now().UTC().AddDate(0, 0, -windowDays).Format("2006/01/02")

	// Pre-filter query: transactional keywords OR known financial domains, bounded to afterDate
	queryFilter := fmt.Sprintf("(from:(*stripe* OR *paypal* OR *google* OR *apple* OR *amazon* OR *uber* OR *grab* OR *gojek* OR *tokopedia* OR *shopee* OR *bank* OR *receipt* OR *invoice*) OR (receipt OR invoice OR bill OR payment OR charged OR paid OR transfer OR subscription OR struk OR tagihan OR transaksi)) after:%s", afterDate)

	endpoint := fmt.Sprintf("%s/me/messages?q=%s&maxResults=50", c.baseURL, url.QueryEscape(queryFilter))
	if pageToken != "" {
		endpoint += "&pageToken=" + url.QueryEscape(pageToken)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gmail messages.list failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gmail messages.list status %d: %s", resp.StatusCode, string(body))
	}

	var listResp struct {
		Messages []struct {
			ID       string `json:"id"`
			ThreadID string `json:"threadId"`
		} `json:"messages"`
		NextPageToken      string `json:"nextPageToken"`
		ResultSizeEstimate int    `json:"resultSizeEstimate"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, fmt.Errorf("gmail messages.list decode error: %w", err)
	}

	rawMessages := make([]*ingestionDomain.RawMessage, 0, len(listResp.Messages))
	latestHistoryID := ""

	for _, item := range listResp.Messages {
		rawMsg, err := c.fetchMessageDetail(ctx, token, item.ID)
		if err != nil {
			// Per 12_gmail_connector.md §9: single message fetch error should not fail the whole batch
			continue
		}
		if rawMsg.HistoryID != "" {
			latestHistoryID = rawMsg.HistoryID
		}
		rawMessages = append(rawMessages, rawMsg)
	}

	return &ingestionDomain.SyncBatch{
		Messages:      rawMessages,
		NextPageToken: listResp.NextPageToken,
		HistoryID:     latestHistoryID,
		HasMore:       listResp.NextPageToken != "",
	}, nil
}

// RegisterRealtimeWatch calls users.watch on the Gmail API to register a Google Cloud Pub/Sub topic per 12_gmail_connector.md §5.1.
func (c *GmailClient) RegisterRealtimeWatch(ctx context.Context, account *ingestionDomain.ConnectorAccount) (*ingestionDomain.WatchHandle, error) {
	token, err := c.tokenProvider.GetValidAccessToken(ctx, account.ID)
	if err != nil {
		return nil, fmt.Errorf("gmail watch: token error: %w", err)
	}

	bodyData, _ := json.Marshal(map[string]any{
		"topicName": c.pubSubTopic,
		"labelIds":  []string{"INBOX"},
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/me/watch", strings.NewReader(string(bodyData)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gmail watch request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gmail watch returned status %d: %s", resp.StatusCode, string(body))
	}

	var watchResp struct {
		HistoryID  string `json:"historyId"`
		Expiration string `json:"expiration"` // Milliseconds epoch as string
	}
	if err := json.NewDecoder(resp.Body).Decode(&watchResp); err != nil {
		return nil, fmt.Errorf("gmail watch decode error: %w", err)
	}

	expMillis, err := strconv.ParseInt(watchResp.Expiration, 10, 64)
	if err != nil {
		expMillis = time.Now().Add(6 * 24 * time.Hour).UnixMilli() // Fallback to 6 days safety margin
	}
	expirationTime := time.UnixMilli(expMillis).UTC()

	return &ingestionDomain.WatchHandle{
		Expiration: expirationTime,
		HistoryID:  watchResp.HistoryID,
	}, nil
}

// FetchIncremental calls history.list to fetch delta updates since the last historyId per 12_gmail_connector.md §6.1.
func (c *GmailClient) FetchIncremental(
	ctx context.Context,
	account *ingestionDomain.ConnectorAccount,
	cursor string,
) (*ingestionDomain.IncrementalBatch, error) {
	token, err := c.tokenProvider.GetValidAccessToken(ctx, account.ID)
	if err != nil {
		return nil, fmt.Errorf("gmail incremental sync: token error: %w", err)
	}

	endpoint := fmt.Sprintf("%s/me/history?startHistoryId=%s&historyTypes=messageAdded", c.baseURL, url.QueryEscape(cursor))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gmail history.list failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Handle historyId expiration (404) per 12_gmail_connector.md §6.2
	if resp.StatusCode == http.StatusNotFound {
		return &ingestionDomain.IncrementalBatch{
			Messages:     nil,
			NewCursor:    cursor,
			ExpiredState: true,
		}, nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gmail history.list status %d: %s", resp.StatusCode, string(body))
	}

	var histResp struct {
		History []struct {
			ID            string `json:"id"`
			MessagesAdded []struct {
				Message struct {
					ID string `json:"id"`
				} `json:"message"`
			} `json:"messagesAdded"`
		} `json:"history"`
		HistoryID     string `json:"historyId"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&histResp); err != nil {
		return nil, fmt.Errorf("gmail history.list decode error: %w", err)
	}

	var messages []*ingestionDomain.RawMessage
	for _, h := range histResp.History {
		for _, added := range h.MessagesAdded {
			msg, err := c.fetchMessageDetail(ctx, token, added.Message.ID)
			if err != nil {
				continue
			}
			messages = append(messages, msg)
		}
	}

	newCursor := histResp.HistoryID
	if newCursor == "" {
		newCursor = cursor
	}

	return &ingestionDomain.IncrementalBatch{
		Messages:     messages,
		NewCursor:    newCursor,
		ExpiredState: false,
	}, nil
}

// Disconnect deregisters watches using users.stop per 12_gmail_connector.md §3.1.
func (c *GmailClient) Disconnect(ctx context.Context, account *ingestionDomain.ConnectorAccount) error {
	token, err := c.tokenProvider.GetValidAccessToken(ctx, account.ID)
	if err != nil {
		return nil // Account already disconnected/revoked
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/me/stop", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	return nil
}

func (c *GmailClient) fetchMessageDetail(ctx context.Context, token, messageID string) (*ingestionDomain.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/me/messages/%s?format=full", c.baseURL, messageID), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get message %s status %d: %s", messageID, resp.StatusCode, string(body))
	}

	var msgData struct {
		ID           string `json:"id"`
		HistoryID    string `json:"historyId"`
		InternalDate string `json:"internalDate"`
		Snippet      string `json:"snippet"`
		Payload      struct {
			Headers []struct {
				Name  string `json:"name"`
				Value string `json:"value"`
			} `json:"headers"`
			Body struct {
				Data string `json:"data"`
			} `json:"body"`
			Parts []struct {
				MimeType string `json:"mimeType"`
				Body     struct {
					Data string `json:"data"`
				} `json:"body"`
			} `json:"parts"`
		} `json:"payload"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&msgData); err != nil {
		return nil, err
	}

	headers := make(map[string]string)
	for _, h := range msgData.Payload.Headers {
		headers[h.Name] = h.Value
	}

	dateMillis, _ := strconv.ParseInt(msgData.InternalDate, 10, 64)
	receivedAt := time.UnixMilli(dateMillis).UTC()
	if receivedAt.IsZero() {
		receivedAt = time.Now().UTC()
	}

	bodyText := decodeBase64URL(msgData.Payload.Body.Data)
	bodyHTML := ""
	for _, part := range msgData.Payload.Parts {
		if strings.HasPrefix(part.MimeType, "text/plain") && bodyText == "" {
			bodyText = decodeBase64URL(part.Body.Data)
		} else if strings.HasPrefix(part.MimeType, "text/html") {
			bodyHTML = decodeBase64URL(part.Body.Data)
		}
	}

	return &ingestionDomain.RawMessage{
		ExternalID:   msgData.ID,
		InternalDate: receivedAt,
		Snippet:      msgData.Snippet,
		Subject:      headers["Subject"],
		From:         headers["From"],
		To:           headers["To"],
		Headers:      headers,
		BodyText:     bodyText,
		BodyHTML:     bodyHTML,
		HistoryID:    msgData.HistoryID,
	}, nil
}

func decodeBase64URL(s string) string {
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "-", "+")
	s = strings.ReplaceAll(s, "_", "/")
	switch len(s) % 4 {
	case 2:
		s += "=="
	case 3:
		s += "="
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return ""
	}
	return string(b)
}
