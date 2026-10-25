package domain

import "context"

// TokenProvider defines the sanctioned cross-module interface per 10_backend_blueprint.md §6.
// Consumed exclusively by the Ingestion module to fetch a valid, decrypted, and refreshed OAuth access token.
type TokenProvider interface {
	GetValidAccessToken(ctx context.Context, gmailAccountID string) (string, error)
}
