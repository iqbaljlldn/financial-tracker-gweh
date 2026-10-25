package domain

import (
	"context"
	"time"
)

// UserRepository defines the persistence contract for User aggregate roots.
// Per 16_security.md §6.2 and 20_coding_convention.md §9.2, every user-scoped
// query method includes userID as a mandatory parameter [GATE].
type UserRepository interface {
	// Save persists a new User aggregate.
	Save(ctx context.Context, user *User) error

	// FindByID retrieves a User by their unique identifier with mandatory userID check [GATE].
	FindByID(ctx context.Context, userID string) (*User, error)

	// FindByEmail retrieves a User by their normalized email address.
	FindByEmail(ctx context.Context, email string) (*User, error)

	// Update updates an existing User aggregate.
	Update(ctx context.Context, userID string, user *User) error
}

// OAuthTokenRepository defines the persistence contract for OAuth token entities.
type OAuthTokenRepository interface {
	// Save persists a new OAuthToken entity.
	Save(ctx context.Context, token *OAuthToken) error

	// FindByGmailAccountID retrieves the active OAuthToken for a given Gmail account.
	FindByGmailAccountID(ctx context.Context, gmailAccountID string) (*OAuthToken, error)

	// Update updates an existing OAuthToken entity with refreshed credentials.
	Update(ctx context.Context, token *OAuthToken) error
}

// RefreshTokenRepository defines the persistence contract for database-backed platform refresh tokens.
// Per 16_security.md §6.2 and 20_coding_convention.md §9.2, user-scoped operations require userID [GATE].
type RefreshTokenRepository interface {
	// Save stores a new hashed platform refresh token for a user with mandatory userID [GATE].
	Save(ctx context.Context, userID string, tokenHash string, expiresAt time.Time) error

	// FindValidByHash retrieves a non-revoked, non-expired refresh token record by its SHA-256 hash.
	FindValidByHash(ctx context.Context, tokenHash string) (userID string, err error)

	// Revoke marks a specific refresh token as revoked with mandatory userID [GATE].
	Revoke(ctx context.Context, userID string, tokenHash string) error

	// RevokeAllForUser revokes all platform refresh tokens for a user (e.g. on global sign-out) with mandatory userID [GATE].
	RevokeAllForUser(ctx context.Context, userID string) error
}
