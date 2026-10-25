package domain

import (
	"errors"
	"time"
)

// OAuthToken represents the OAuth credential entity in the Identity & Access context per 04_domain_model.md §8.2.
// Sensitive token bytes are envelope-encrypted at rest per 16_security.md §4.3.
type OAuthToken struct {
	id                    string
	gmailAccountID        string
	encryptedAccessToken  []byte
	encryptedRefreshToken []byte
	accessTokenExpiresAt  time.Time
	scope                 string
	createdAt             time.Time
	updatedAt             time.Time
}

// NewOAuthToken constructs a new OAuthToken entity.
func NewOAuthToken(
	id string,
	gmailAccountID string,
	encryptedAccessToken []byte,
	encryptedRefreshToken []byte,
	expiresAt time.Time,
	scope string,
) (*OAuthToken, error) {
	if gmailAccountID == "" {
		return nil, errors.New("identity: gmailAccountID cannot be empty")
	}
	if len(encryptedAccessToken) == 0 {
		return nil, errors.New("identity: encryptedAccessToken cannot be empty")
	}
	if len(encryptedRefreshToken) == 0 {
		return nil, errors.New("identity: encryptedRefreshToken cannot be empty")
	}

	now := time.Now().UTC()
	return &OAuthToken{
		id:                    id,
		gmailAccountID:        gmailAccountID,
		encryptedAccessToken:  encryptedAccessToken,
		encryptedRefreshToken: encryptedRefreshToken,
		accessTokenExpiresAt:  expiresAt,
		scope:                 scope,
		createdAt:             now,
		updatedAt:             now,
	}, nil
}

// ReconstituteOAuthToken rebuilds an OAuthToken from storage without re-validation.
func ReconstituteOAuthToken(
	id string,
	gmailAccountID string,
	encryptedAccessToken []byte,
	encryptedRefreshToken []byte,
	expiresAt time.Time,
	scope string,
	createdAt, updatedAt time.Time,
) *OAuthToken {
	return &OAuthToken{
		id:                    id,
		gmailAccountID:        gmailAccountID,
		encryptedAccessToken:  encryptedAccessToken,
		encryptedRefreshToken: encryptedRefreshToken,
		accessTokenExpiresAt:  expiresAt,
		scope:                 scope,
		createdAt:             createdAt,
		updatedAt:             updatedAt,
	}
}

// ID returns the token record identifier.
func (t *OAuthToken) ID() string {
	return t.id
}

// GmailAccountID returns the associated Gmail account identifier.
func (t *OAuthToken) GmailAccountID() string {
	return t.gmailAccountID
}

// EncryptedAccessToken returns the envelope-encrypted access token bytes.
func (t *OAuthToken) EncryptedAccessToken() []byte {
	return t.encryptedAccessToken
}

// EncryptedRefreshToken returns the envelope-encrypted refresh token bytes.
func (t *OAuthToken) EncryptedRefreshToken() []byte {
	return t.encryptedRefreshToken
}

// AccessTokenExpiresAt returns the access token expiration time.
func (t *OAuthToken) AccessTokenExpiresAt() time.Time {
	return t.accessTokenExpiresAt
}

// Scope returns the OAuth scopes granted for this token.
func (t *OAuthToken) Scope() string {
	return t.scope
}

// CreatedAt returns the creation timestamp.
func (t *OAuthToken) CreatedAt() time.Time {
	return t.createdAt
}

// UpdatedAt returns the last updated timestamp.
func (t *OAuthToken) UpdatedAt() time.Time {
	return t.updatedAt
}

// IsExpired checks whether the access token has expired (with a 60-second clock skew buffer).
func (t *OAuthToken) IsExpired() bool {
	return time.Now().UTC().Add(60 * time.Second).After(t.accessTokenExpiresAt)
}

// UpdateTokens updates the access and refresh token ciphertexts after a successful token refresh.
func (t *OAuthToken) UpdateTokens(encryptedAccessToken, encryptedRefreshToken []byte, expiresAt time.Time) {
	t.encryptedAccessToken = encryptedAccessToken
	if len(encryptedRefreshToken) > 0 {
		t.encryptedRefreshToken = encryptedRefreshToken
	}
	t.accessTokenExpiresAt = expiresAt
	t.updatedAt = time.Now().UTC()
}
