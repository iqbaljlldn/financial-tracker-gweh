package command

import (
	"context"
	"fmt"
	"time"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/identity/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/security"
)

// RefreshOAuthTokenCommand carries the target Gmail account ID to refresh tokens for.
type RefreshOAuthTokenCommand struct {
	GmailAccountID string
}

// RefreshOAuthTokenResult contains the unwrapped active access token and expiration.
type RefreshOAuthTokenResult struct {
	AccessToken string
	ExpiresAt   time.Time
}

// RefreshOAuthTokenHandler coordinates token unwrapping, Google refresh, and ciphertext re-encryption.
type RefreshOAuthTokenHandler struct {
	tokenRepo   domain.OAuthTokenRepository
	oauthClient domain.OAuthClient
	encryptor   *security.EnvelopeEncryptor
}

// NewRefreshOAuthTokenHandler creates a new RefreshOAuthTokenHandler.
func NewRefreshOAuthTokenHandler(
	tokenRepo domain.OAuthTokenRepository,
	oauthClient domain.OAuthClient,
	encryptor *security.EnvelopeEncryptor,
) *RefreshOAuthTokenHandler {
	return &RefreshOAuthTokenHandler{
		tokenRepo:   tokenRepo,
		oauthClient: oauthClient,
		encryptor:   encryptor,
	}
}

// Handle executes token decryption, remote refresh against Google, and re-encryption per 16_security.md §4.3.
func (h *RefreshOAuthTokenHandler) Handle(ctx context.Context, cmd RefreshOAuthTokenCommand) (*RefreshOAuthTokenResult, error) {
	// 1. Fetch token record
	token, err := h.tokenRepo.FindByGmailAccountID(ctx, cmd.GmailAccountID)
	if err != nil {
		return nil, fmt.Errorf("refresh oauth: token lookup failed: %w", err)
	}

	// 2. Unwrap refresh token using KMS envelope decryptor
	decryptedRefresh, err := h.encryptor.Decrypt(ctx, token.EncryptedRefreshToken())
	if err != nil {
		return nil, fmt.Errorf("refresh oauth: decrypt refresh token failed: %w", err)
	}
	defer security.SecureWipe(decryptedRefresh)

	// 3. Exchange refresh token with Google
	tokenResp, err := h.oauthClient.RefreshToken(ctx, string(decryptedRefresh))
	if err != nil {
		return nil, fmt.Errorf("refresh oauth: google refresh request failed: %w", err)
	}

	// 4. Envelope-encrypt fresh access token
	newEncryptedAccess, err := h.encryptor.Encrypt(ctx, []byte(tokenResp.AccessToken))
	if err != nil {
		return nil, fmt.Errorf("refresh oauth: encrypt new access token failed: %w", err)
	}

	var newEncryptedRefresh []byte
	if tokenResp.RefreshToken != "" {
		newEncryptedRefresh, err = h.encryptor.Encrypt(ctx, []byte(tokenResp.RefreshToken))
		if err != nil {
			return nil, fmt.Errorf("refresh oauth: encrypt new refresh token failed: %w", err)
		}
	}

	expiresAt := time.Now().UTC().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	token.UpdateTokens(newEncryptedAccess, newEncryptedRefresh, expiresAt)

	// 5. Persist updated ciphertexts
	if err := h.tokenRepo.Update(ctx, token); err != nil {
		return nil, fmt.Errorf("refresh oauth: persist updated tokens failed: %w", err)
	}

	return &RefreshOAuthTokenResult{
		AccessToken: tokenResp.AccessToken,
		ExpiresAt:   expiresAt,
	}, nil
}
