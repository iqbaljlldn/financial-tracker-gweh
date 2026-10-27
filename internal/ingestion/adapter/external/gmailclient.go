package external

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/identity/domain"
	ingestionDomain "github.com/iqbaljlldn/financial-tracker-gweh/internal/ingestion/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/observability"
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
	BaseBackoff   time.Duration        // Base backoff duration (default 500ms)
}

// GmailClient coordinates direct interactions with the Google Gmail REST API.
// Implements ingestionDomain.Connector per 12_gmail_connector.md §3.
type GmailClient struct {
	baseURL       string
	httpClient    *http.Client
	tokenProvider domain.TokenProvider
	pubSubTopic   string
	baseBackoff   time.Duration
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

	baseBackoff := cfg.BaseBackoff
	if baseBackoff <= 0 {
		baseBackoff = 500 * time.Millisecond
	}

	return &GmailClient{
		baseURL:       strings.TrimSuffix(baseURL, "/"),
		httpClient:    httpClient,
		tokenProvider: cfg.TokenProvider,
		pubSubTopic:   cfg.PubSubTopic,
		baseBackoff:   baseBackoff,
	}, nil
}

// Authenticate verifies connector credentials against the Gmail user profile.
func (c *GmailClient) Authenticate(ctx context.Context, creds ingestionDomain.ConnectorCredentials) (*ingestionDomain.ConnectorAccount, error) {
	token, err := c.tokenProvider.GetValidAccessToken(ctx, creds.AccountID)
	if err != nil {
		return nil, c.mapTokenError(err)
	}

	resp, err := c.doRequest(ctx, "profile", http.MethodGet, c.baseURL+"/me/profile", token, nil, creds.AccountID, 1)
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
		return nil, c.mapTokenError(err)
	}

	if windowDays <= 0 {
		windowDays = 90
	}
	afterDate := time.Now().UTC().AddDate(0, 0, -windowDays).Format("2006/01/02")

	// Pre-filter query: transactional keywords OR known financial domains, bounded to afterDate,
	// excluding obvious non-transactional subjects (OTP, verification, login alert, support tickets)
	queryFilter := fmt.Sprintf("(from:(*stripe* OR *paypal* OR *google* OR *apple* OR *amazon* OR *uber* OR *grab* OR *gojek* OR *tokopedia* OR *shopee* OR *bank* OR *receipt* OR *invoice*) OR (receipt OR invoice OR bill OR payment OR charged OR paid OR transfer OR subscription OR struk OR tagihan OR transaksi)) -subject:(otp OR verifikasi OR verification OR \"reset password\" OR \"welcome to\" OR \"selamat datang\" OR \"security alert\" OR \"login baru\" OR \"ticket #\" OR \"tiket\") after:%s", afterDate)

	endpoint := fmt.Sprintf("%s/me/messages?q=%s&maxResults=50", c.baseURL, url.QueryEscape(queryFilter))
	if pageToken != "" {
		endpoint += "&pageToken=" + url.QueryEscape(pageToken)
	}

	resp, err := c.doRequest(ctx, "messages.list", http.MethodGet, endpoint, token, nil, account.ID, 5)
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
		rawMsg, err := c.fetchMessageDetail(ctx, token, item.ID, account.ID)
		if err != nil {
			// Per 12_gmail_connector.md §9: single message fetch error should not fail the whole batch
			continue
		}
		if rawMsg.HistoryID != "" {
			latestHistoryID = rawMsg.HistoryID
		}
		rawMessages = append(rawMessages, rawMsg)
	}

	// If no message had a historyId (or list was empty), fetch baseline from profile
	if latestHistoryID == "" {
		if profHistID, err := c.getProfileHistoryID(ctx, token, account.ID); err == nil && profHistID != "" {
			latestHistoryID = profHistID
		}
	}

	return &ingestionDomain.SyncBatch{
		Messages:      rawMessages,
		NextPageToken: listResp.NextPageToken,
		HistoryID:     latestHistoryID,
		HasMore:       listResp.NextPageToken != "",
	}, nil
}

func (c *GmailClient) getProfileHistoryID(ctx context.Context, token string, accountID string) (string, error) {
	resp, err := c.doRequest(ctx, "profile", http.MethodGet, c.baseURL+"/me/profile", token, nil, accountID, 1)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("profile returned status %d", resp.StatusCode)
	}
	var prof struct {
		HistoryID string `json:"historyId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&prof); err != nil {
		return "", err
	}
	return prof.HistoryID, nil
}

// RegisterRealtimeWatch calls users.watch on the Gmail API to register a Google Cloud Pub/Sub topic per 12_gmail_connector.md §5.1.
func (c *GmailClient) RegisterRealtimeWatch(ctx context.Context, account *ingestionDomain.ConnectorAccount) (*ingestionDomain.WatchHandle, error) {
	token, err := c.tokenProvider.GetValidAccessToken(ctx, account.ID)
	if err != nil {
		return nil, c.mapTokenError(err)
	}

	bodyData, _ := json.Marshal(map[string]any{
		"topicName": c.pubSubTopic,
		"labelIds":  []string{"INBOX"},
	})

	resp, err := c.doRequest(ctx, "users.watch", http.MethodPost, c.baseURL+"/me/watch", token, bodyData, account.ID, 50)
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

type historyListResponse struct {
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

// FetchIncremental calls history.list to fetch delta updates since the last historyId per 12_gmail_connector.md §6.1.
func (c *GmailClient) FetchIncremental(
	ctx context.Context,
	account *ingestionDomain.ConnectorAccount,
	cursor string,
) (*ingestionDomain.IncrementalBatch, error) {
	cleanCursor := strings.TrimSpace(cursor)
	if cleanCursor == "" || (strings.Contains(c.baseURL, "googleapis.com") && !isNumericHistoryID(cleanCursor)) {
		return &ingestionDomain.IncrementalBatch{
			Messages:     nil,
			NewCursor:    cursor,
			ExpiredState: true,
		}, nil
	}

	token, err := c.tokenProvider.GetValidAccessToken(ctx, account.ID)
	if err != nil {
		return nil, c.mapTokenError(err)
	}

	endpoint := fmt.Sprintf("%s/me/history?startHistoryId=%s&historyTypes=messageAdded", c.baseURL, url.QueryEscape(cleanCursor))
	resp, err := c.doRequest(ctx, "history.list", http.MethodGet, endpoint, token, nil, account.ID, 2)
	if err != nil {
		return nil, fmt.Errorf("gmail history.list failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	batch, err := checkHistoryResponse(resp, cursor)
	if batch != nil || err != nil {
		return batch, err
	}

	var histResp historyListResponse
	if err := json.NewDecoder(resp.Body).Decode(&histResp); err != nil {
		return nil, fmt.Errorf("gmail history.list decode error: %w", err)
	}

	messages := c.extractHistoryMessages(ctx, token, account.ID, histResp.History)
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

func isNumericHistoryID(s string) bool {
	_, err := strconv.ParseUint(s, 10, 64)
	return err == nil
}

func checkHistoryResponse(resp *http.Response, cursor string) (*ingestionDomain.IncrementalBatch, error) {
	if resp.StatusCode == http.StatusNotFound {
		return &ingestionDomain.IncrementalBatch{
			Messages:     nil,
			NewCursor:    cursor,
			ExpiredState: true,
		}, nil
	}

	if resp.StatusCode == http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		bodyStr := string(body)
		if strings.Contains(bodyStr, "start_history_id") || strings.Contains(bodyStr, "historyId") || strings.Contains(bodyStr, "TYPE_UINT64") {
			return &ingestionDomain.IncrementalBatch{
				Messages:     nil,
				NewCursor:    cursor,
				ExpiredState: true,
			}, nil
		}
		return nil, fmt.Errorf("gmail history.list status %d: %s", resp.StatusCode, bodyStr)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gmail history.list status %d: %s", resp.StatusCode, string(body))
	}

	return nil, nil
}

func (c *GmailClient) extractHistoryMessages(ctx context.Context, token, accountID string, history []struct {
	ID            string `json:"id"`
	MessagesAdded []struct {
		Message struct {
			ID string `json:"id"`
		} `json:"message"`
	} `json:"messagesAdded"`
}) []*ingestionDomain.RawMessage {
	var messages []*ingestionDomain.RawMessage
	for _, h := range history {
		for _, added := range h.MessagesAdded {
			msg, err := c.fetchMessageDetail(ctx, token, added.Message.ID, accountID)
			if err != nil {
				continue
			}
			messages = append(messages, msg)
		}
	}
	return messages
}

// Disconnect deregisters watches using users.stop per 12_gmail_connector.md §3.1.
func (c *GmailClient) Disconnect(ctx context.Context, account *ingestionDomain.ConnectorAccount) error {
	token, err := c.tokenProvider.GetValidAccessToken(ctx, account.ID)
	if err != nil {
		return nil // Account already disconnected/revoked
	}

	resp, err := c.doRequest(ctx, "users.stop", http.MethodPost, c.baseURL+"/me/stop", token, nil, account.ID, 50)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	return nil
}

// FetchMessage fetches full details for a single Gmail message by ID.
func (c *GmailClient) FetchMessage(ctx context.Context, account *ingestionDomain.ConnectorAccount, messageID string) (*ingestionDomain.RawMessage, error) {
	if account == nil || account.ID == "" {
		return nil, errors.New("gmailclient: account is required")
	}
	token, err := c.tokenProvider.GetValidAccessToken(ctx, account.ID)
	if err != nil {
		return nil, c.mapTokenError(err)
	}
	return c.fetchMessageDetail(ctx, token, messageID, account.ID)
}

// doRequest executes an outbound HTTP request against Gmail API with truncated exponential backoff
// on 429 / 5xx responses and metrics instrumentation per 12_gmail_connector.md §8 and 17_observability.md §4.1.
func (c *GmailClient) doRequest(
	ctx context.Context,
	apiMethod, httpMethod, targetURL, token string,
	body []byte,
	accountID string,
	quotaUnits float64,
) (*http.Response, error) {
	const maxRetries = 4
	maxBackoff := 32 * time.Second

	for attempt := 0; attempt <= maxRetries; attempt++ {
		resp, err := c.executeSingleRequest(ctx, httpMethod, targetURL, token, body)
		if err != nil {
			if attempt == maxRetries {
				observability.GetMetrics().GmailAPICallsTotal.WithLabelValues(apiMethod, "network_error").Inc()
				return nil, err
			}
			if err := c.sleepBackoff(ctx, c.baseBackoff*(1<<attempt), maxBackoff); err != nil {
				return nil, err
			}
			continue
		}

		statusCode := resp.StatusCode
		observability.GetMetrics().GmailAPICallsTotal.WithLabelValues(apiMethod, strconv.Itoa(statusCode)).Inc()
		if accountID != "" && quotaUnits > 0 {
			observability.GetMetrics().GmailQuotaUnitsConsumedTotal.WithLabelValues(accountID).Add(quotaUnits)
		}

		if isRetryableStatus(statusCode) {
			_ = resp.Body.Close()
			if attempt == maxRetries {
				return resp, fmt.Errorf("gmail %s: rate limited or server error (%d) after %d retries", apiMethod, statusCode, attempt)
			}
			if err := c.sleepBackoff(ctx, c.baseBackoff*(1<<attempt), maxBackoff); err != nil {
				return nil, err
			}
			continue
		}

		return resp, nil
	}

	return nil, errors.New("request exceeded retries")
}

func (c *GmailClient) executeSingleRequest(ctx context.Context, method, targetURL, token string, body []byte) (*http.Response, error) {
	var bodyReader io.Reader
	if len(body) > 0 {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, targetURL, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.httpClient.Do(req)
}

func (c *GmailClient) sleepBackoff(ctx context.Context, backoff, maxBackoff time.Duration) error {
	if backoff > maxBackoff {
		backoff = maxBackoff
	}
	jitter := time.Duration(rand.Intn(100)) * time.Millisecond
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(backoff + jitter):
		return nil
	}
}

func isRetryableStatus(status int) bool {
	return status == http.StatusTooManyRequests || (status >= 500 && status <= 504)
}

func (c *GmailClient) fetchMessageDetail(ctx context.Context, token, messageID, accountID string) (*ingestionDomain.RawMessage, error) {
	url := fmt.Sprintf("%s/me/messages/%s?format=full", c.baseURL, messageID)
	resp, err := c.doRequest(ctx, "messages.get", http.MethodGet, url, token, nil, accountID, 5)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get message %s status %d: %s", messageID, resp.StatusCode, string(body))
	}

	var msgData messageDetailResponse
	if err := json.NewDecoder(resp.Body).Decode(&msgData); err != nil {
		return &ingestionDomain.RawMessage{ExternalID: messageID, ParseFailed: true}, nil
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

	bodyText, bodyHTML, parseFailed := extractBodies(msgData.Payload.Body.Data, msgData.Payload.Parts)
	if bodyText == "" && bodyHTML == "" && msgData.Snippet != "" {
		bodyText = msgData.Snippet
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
		ParseFailed:  parseFailed,
	}, nil
}

type messagePart struct {
	MimeType string `json:"mimeType"`
	Body     struct {
		Data string `json:"data"`
	} `json:"body"`
	Parts []messagePart `json:"parts"`
}

type messageDetailResponse struct {
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
		Parts []messagePart `json:"parts"`
	} `json:"payload"`
}

func extractBodies(mainBodyData string, parts []messagePart) (string, string, bool) {
	var parseFailed bool
	bodyText, err := decodeBase64URL(mainBodyData)
	if err != nil && mainBodyData != "" {
		parseFailed = true
	}
	var bodyHTML string
	walkParts(parts, &bodyText, &bodyHTML, &parseFailed)
	return bodyText, bodyHTML, parseFailed
}

func walkParts(parts []messagePart, bodyText, bodyHTML *string, parseFailed *bool) {
	for _, part := range parts {
		if strings.HasPrefix(part.MimeType, "text/plain") && *bodyText == "" && part.Body.Data != "" {
			txt, err := decodeBase64URL(part.Body.Data)
			if err != nil {
				*parseFailed = true
			} else {
				*bodyText = txt
			}
		} else if strings.HasPrefix(part.MimeType, "text/html") && *bodyHTML == "" && part.Body.Data != "" {
			html, err := decodeBase64URL(part.Body.Data)
			if err != nil {
				*parseFailed = true
			} else {
				*bodyHTML = html
			}
		}
		if len(part.Parts) > 0 {
			walkParts(part.Parts, bodyText, bodyHTML, parseFailed)
		}
	}
}

func (c *GmailClient) mapTokenError(err error) error {
	if errors.Is(err, domain.ErrRevocationDetected) || strings.Contains(strings.ToLower(err.Error()), "invalid_grant") {
		return domain.ErrRevocationDetected
	}
	return err
}

func decodeBase64URL(s string) (string, error) {
	if s == "" {
		return "", nil
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
		return "", err
	}
	return string(b), nil
}
