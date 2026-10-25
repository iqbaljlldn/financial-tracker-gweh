package domain

import (
	"context"
	"time"
)

// ConnectorCredentials abstracts credentials needed to interact with an external data source.
type ConnectorCredentials struct {
	AccessToken  string
	RefreshToken string
	AccountID    string
	EmailAddress string
}

// ConnectorAccount represents an authenticated external account in a connector.
type ConnectorAccount struct {
	ID           string
	EmailAddress string
	Provider     string // "gmail", "outlook", etc.
}

// RawMessage contains unprocessed payload and headers extracted from an external message provider.
type RawMessage struct {
	ExternalID   string
	InternalDate time.Time
	Snippet      string
	Subject      string
	From         string
	To           string
	Headers      map[string]string
	BodyText     string
	BodyHTML     string
	HistoryID    string
}

// SyncBatch represents a single paginated batch of messages retrieved during initial full sync.
type SyncBatch struct {
	Messages      []*RawMessage
	NextPageToken string
	HistoryID     string
	HasMore       bool
}

// IncrementalBatch represents messages returned during delta/history sync.
type IncrementalBatch struct {
	Messages     []*RawMessage
	NewCursor    string
	ExpiredState bool // True if cursor (e.g. historyId) has expired on the provider side
}

// WatchHandle contains registration metadata for real-time webhooks/push signals.
type WatchHandle struct {
	Expiration time.Time
	HistoryID  string
}

// Connector defines the general, source-agnostic contract that every current and future
// transaction source (Gmail, Outlook, IMAP, etc.) must satisfy per 12_gmail_connector.md §3.1 [GATE].
// Provider-specific concepts (historyId, Pub/Sub, MIME) are strictly confined within implementations.
type Connector interface {
	// Authenticate establishes and validates a source-specific account connection.
	Authenticate(ctx context.Context, creds ConnectorCredentials) (*ConnectorAccount, error)

	// InitialSync performs a bounded historical message capture with pagination.
	InitialSync(ctx context.Context, account *ConnectorAccount, windowDays int, pageToken string) (*SyncBatch, error)

	// RegisterRealtimeWatch registers for real-time push notifications if supported.
	RegisterRealtimeWatch(ctx context.Context, account *ConnectorAccount) (*WatchHandle, error)

	// FetchIncremental fetches new signals since the given cursor/historyId.
	FetchIncremental(ctx context.Context, account *ConnectorAccount, cursor string) (*IncrementalBatch, error)

	// Disconnect terminates the connection and deregisters any active watches.
	Disconnect(ctx context.Context, account *ConnectorAccount) error
}
