package domain

import (
	"errors"
	"strings"
	"time"
)

type AccountStatus string

const (
	StatusLinking      AccountStatus = "linking"
	StatusInitialSync  AccountStatus = "initial_sync"
	StatusActive       AccountStatus = "active"
	StatusDisconnected AccountStatus = "disconnected"
	StatusError        AccountStatus = "error"
)

var (
	ErrInvalidAccountStatus = errors.New("invalid account status transition")
	ErrEmptyAccountFields   = errors.New("id, userID, and emailAddress cannot be empty")
	ErrInvalidSyncWindow    = errors.New("initialSyncWindowDays must be greater than zero")
)

// GmailAccount is the aggregate root representing a user's connected Gmail mailbox
// in the Ingestion module per 08_database_design.md §8.1.
type GmailAccount struct {
	id                    string
	userID                string
	emailAddress          string
	status                AccountStatus
	lastHistoryID         string
	watchExpiration       *time.Time
	initialSyncWindowDays int
	createdAt             time.Time
	updatedAt             time.Time
}

// NewGmailAccount constructs a new GmailAccount aggregate enforcing structural invariants.
func NewGmailAccount(id, userID, emailAddress string, syncWindowDays int) (*GmailAccount, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(userID) == "" || strings.TrimSpace(emailAddress) == "" {
		return nil, ErrEmptyAccountFields
	}
	if syncWindowDays <= 0 {
		syncWindowDays = 90 // Default to 90 days per 12_gmail_connector.md §6.3
	}

	now := time.Now().UTC()
	return &GmailAccount{
		id:                    strings.TrimSpace(id),
		userID:                strings.TrimSpace(userID),
		emailAddress:          strings.ToLower(strings.TrimSpace(emailAddress)),
		status:                StatusLinking,
		initialSyncWindowDays: syncWindowDays,
		createdAt:             now,
		updatedAt:             now,
	}, nil
}

// ReconstituteGmailAccount reconstructs an existing aggregate from persistence.
func ReconstituteGmailAccount(
	id, userID, emailAddress string,
	status AccountStatus,
	lastHistoryID string,
	watchExpiration *time.Time,
	initialSyncWindowDays int,
	createdAt, updatedAt time.Time,
) *GmailAccount {
	return &GmailAccount{
		id:                    id,
		userID:                userID,
		emailAddress:          emailAddress,
		status:                status,
		lastHistoryID:         lastHistoryID,
		watchExpiration:       watchExpiration,
		initialSyncWindowDays: initialSyncWindowDays,
		createdAt:             createdAt,
		updatedAt:             updatedAt,
	}
}

// Getters
func (a *GmailAccount) ID() string                  { return a.id }
func (a *GmailAccount) UserID() string              { return a.userID }
func (a *GmailAccount) EmailAddress() string        { return a.emailAddress }
func (a *GmailAccount) Status() AccountStatus       { return a.status }
func (a *GmailAccount) LastHistoryID() string       { return a.lastHistoryID }
func (a *GmailAccount) WatchExpiration() *time.Time { return a.watchExpiration }
func (a *GmailAccount) InitialSyncWindowDays() int  { return a.initialSyncWindowDays }
func (a *GmailAccount) CreatedAt() time.Time        { return a.createdAt }
func (a *GmailAccount) UpdatedAt() time.Time        { return a.updatedAt }

// State transitions

// StartInitialSync transitions account from linking to initial_sync.
func (a *GmailAccount) StartInitialSync() error {
	if a.status != StatusLinking && a.status != StatusDisconnected && a.status != StatusError {
		return ErrInvalidAccountStatus
	}
	a.status = StatusInitialSync
	a.updatedAt = time.Now().UTC()
	return nil
}

// CompleteInitialSync marks account as active and stores the final baseline historyId.
func (a *GmailAccount) CompleteInitialSync(finalHistoryID string) error {
	if a.status != StatusInitialSync && a.status != StatusLinking && a.status != StatusActive {
		return ErrInvalidAccountStatus
	}
	a.status = StatusActive
	if finalHistoryID != "" {
		a.lastHistoryID = finalHistoryID
	}
	a.updatedAt = time.Now().UTC()
	return nil
}

// UpdateHistoryID advances the incremental sync cursor.
func (a *GmailAccount) UpdateHistoryID(newHistoryID string) {
	if newHistoryID != "" {
		a.lastHistoryID = newHistoryID
		a.updatedAt = time.Now().UTC()
	}
}

// SetWatchExpiration records active push notification watch lease.
func (a *GmailAccount) SetWatchExpiration(exp time.Time, historyID string) {
	a.watchExpiration = &exp
	if historyID != "" {
		a.lastHistoryID = historyID
	}
	a.updatedAt = time.Now().UTC()
}

// MarkDisconnected records terminal OAuth consent revocation per 12_gmail_connector.md §4.3.
func (a *GmailAccount) MarkDisconnected() {
	a.status = StatusDisconnected
	a.watchExpiration = nil
	a.updatedAt = time.Now().UTC()
}

// MarkError marks connection in error state requiring user intervention.
func (a *GmailAccount) MarkError() {
	a.status = StatusError
	a.updatedAt = time.Now().UTC()
}
