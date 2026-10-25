package command

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/identity/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/security"
)

// LinkGoogleAccountCommand carries data required to complete Google OAuth sign-in / account linking.
type LinkGoogleAccountCommand struct {
	Code         string
	CodeVerifier string
}

// LinkGoogleAccountResult carries the session credentials and user profile returned to the caller.
type LinkGoogleAccountResult struct {
	SessionToken string
	RefreshToken string
	ExpiresAt    time.Time
	User         *domain.User
}

// LinkGoogleAccountHandler executes the Google OAuth account-linking command per 07_sequence_diagrams.md Diagram 1.
type LinkGoogleAccountHandler struct {
	userRepo       domain.UserRepository
	tokenRepo      domain.OAuthTokenRepository
	refreshRepo    domain.RefreshTokenRepository
	oauthClient    domain.OAuthClient
	encryptor      *security.EnvelopeEncryptor
	sessionManager *security.SessionTokenManager
}

// NewLinkGoogleAccountHandler creates a new LinkGoogleAccountHandler.
func NewLinkGoogleAccountHandler(
	userRepo domain.UserRepository,
	tokenRepo domain.OAuthTokenRepository,
	refreshRepo domain.RefreshTokenRepository,
	oauthClient domain.OAuthClient,
	encryptor *security.EnvelopeEncryptor,
	sessionManager *security.SessionTokenManager,
) *LinkGoogleAccountHandler {
	return &LinkGoogleAccountHandler{
		userRepo:       userRepo,
		tokenRepo:      tokenRepo,
		refreshRepo:    refreshRepo,
		oauthClient:    oauthClient,
		encryptor:      encryptor,
		sessionManager: sessionManager,
	}
}

// Handle executes the OAuth token exchange, envelope encryption, user matching, and platform session creation.
func (h *LinkGoogleAccountHandler) Handle(ctx context.Context, cmd LinkGoogleAccountCommand) (*LinkGoogleAccountResult, error) {
	// 1. Exchange authorization code with Google for tokens via PKCE
	tokenResp, err := h.oauthClient.ExchangeCode(ctx, cmd.Code, cmd.CodeVerifier)
	if err != nil {
		return nil, fmt.Errorf("link account: code exchange failed: %w", err)
	}

	// 2. Fetch user profile from Google UserInfo endpoint
	userInfo, err := h.oauthClient.FetchUserInfo(ctx, tokenResp.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("link account: fetch userinfo failed: %w", err)
	}

	// 3. Match or create User aggregate
	user, err := h.getOrCreateUser(ctx, userInfo)
	if err != nil {
		return nil, err
	}

	// 4. Envelope-encrypt and persist OAuth tokens per 16_security.md §4.3 [GATE]
	if err := h.persistOAuthTokens(ctx, userInfo.Email, tokenResp); err != nil {
		return nil, err
	}

	// 5. Issue short-lived platform session JWT per 16_security.md §5.1
	sessionToken, expiresAt, err := h.sessionManager.Issue(user.ID(), user.Email(), "user", 1*time.Hour)
	if err != nil {
		return nil, fmt.Errorf("link account: issue session token failed: %w", err)
	}

	// 6. Generate opaque, DB-backed, revocable refresh token per 16_security.md §5.1
	plainRefreshToken, tokenHash, err := security.GenerateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("link account: generate refresh token failed: %w", err)
	}

	// Refresh token persists with mandatory userID row-level ownership [GATE]
	refreshExpiresAt := time.Now().UTC().Add(30 * 24 * time.Hour) // 30-day refresh lifetime
	if err := h.refreshRepo.Save(ctx, user.ID(), tokenHash, refreshExpiresAt); err != nil {
		return nil, fmt.Errorf("link account: save refresh token failed: %w", err)
	}

	return &LinkGoogleAccountResult{
		SessionToken: sessionToken,
		RefreshToken: plainRefreshToken,
		ExpiresAt:    expiresAt,
		User:         user,
	}, nil
}

func (h *LinkGoogleAccountHandler) getOrCreateUser(ctx context.Context, userInfo *domain.UserInfoResult) (*domain.User, error) {
	user, err := h.userRepo.FindByEmail(ctx, userInfo.Email)
	if err != nil && err != domain.ErrUserNotFound {
		return nil, fmt.Errorf("link account: lookup user failed: %w", err)
	}
	if user != nil {
		return user, nil
	}

	userID := uuid.New().String()
	newUser, err := domain.NewUser(userID, userInfo.Email, userInfo.Name, "UTC")
	if err != nil {
		return nil, fmt.Errorf("link account: create user failed: %w", err)
	}
	if err := h.userRepo.Save(ctx, newUser); err != nil {
		return nil, fmt.Errorf("link account: save user failed: %w", err)
	}
	return newUser, nil
}

func (h *LinkGoogleAccountHandler) persistOAuthTokens(ctx context.Context, email string, tokenResp *domain.TokenExchangeResult) error {
	encryptedAccess, err := h.encryptor.Encrypt(ctx, []byte(tokenResp.AccessToken))
	if err != nil {
		return fmt.Errorf("link account: encrypt access token failed: %w", err)
	}

	encryptedRefresh, err := h.encryptor.Encrypt(ctx, []byte(tokenResp.RefreshToken))
	if err != nil {
		return fmt.Errorf("link account: encrypt refresh token failed: %w", err)
	}

	gmailAccountID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(email)).String()
	tokenExpiresAt := time.Now().UTC().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	existingToken, err := h.tokenRepo.FindByGmailAccountID(ctx, gmailAccountID)
	if err != nil && err != domain.ErrOAuthTokenNotFound {
		return fmt.Errorf("link account: check existing oauth token failed: %w", err)
	}

	if existingToken != nil {
		existingToken.UpdateTokens(encryptedAccess, encryptedRefresh, tokenExpiresAt)
		if err := h.tokenRepo.Update(ctx, existingToken); err != nil {
			return fmt.Errorf("link account: update oauth token failed: %w", err)
		}
		return nil
	}

	newToken, err := domain.NewOAuthToken(
		uuid.New().String(),
		gmailAccountID,
		encryptedAccess,
		encryptedRefresh,
		tokenExpiresAt,
		tokenResp.Scope,
	)
	if err != nil {
		return fmt.Errorf("link account: create oauth token entity failed: %w", err)
	}
	if err := h.tokenRepo.Save(ctx, newToken); err != nil {
		return fmt.Errorf("link account: save oauth token entity failed: %w", err)
	}
	return nil
}
