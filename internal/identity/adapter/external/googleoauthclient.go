package external

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/identity/domain"
)

const (
	// Mandated Google OAuth Scopes per 12_gmail_connector.md §4.1 and 16_security.md §5.1 [GATE].
	// Strictly read-only; never request write, send, compose, or settings scopes.
	ScopeGmailReadOnly = "https://www.googleapis.com/auth/gmail.readonly"
	ScopeOpenID        = "openid"
	ScopeEmail         = "email"
	ScopeProfile       = "profile"

	DefaultAuthURL     = "https://accounts.google.com/o/oauth2/v2/auth"
	DefaultTokenURL    = "https://oauth2.googleapis.com/token"
	DefaultUserInfoURL = "https://www.googleapis.com/oauth2/v3/userinfo"
)

// GoogleOAuthClientConfig provides parameters for initializing the Google OAuth 2.0 PKCE client.
type GoogleOAuthClientConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
	AuthURL      string // Configurable for mock server integration testing
	TokenURL     string // Configurable for mock server integration testing
	UserInfoURL  string // Configurable for mock server integration testing
	HTTPClient   *http.Client
}

// TokenResponse models the OAuth 2.0 token response returned by Google.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	IDToken      string `json:"id_token"`
}

// GoogleUserInfo models user identity attributes returned from Google's userinfo endpoint.
type GoogleUserInfo struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
}

// GoogleOAuthClient coordinates Google OAuth 2.0 Authorization Code Flow with PKCE.
type GoogleOAuthClient struct {
	clientID     string
	clientSecret string
	redirectURI  string
	authURL      string
	tokenURL     string
	userInfoURL  string
	httpClient   *http.Client
}

// NewGoogleOAuthClient creates a new Google OAuth client enforcing least-privilege scopes and consent parameters.
func NewGoogleOAuthClient(cfg GoogleOAuthClientConfig) (*GoogleOAuthClient, error) {
	if cfg.ClientID == "" {
		return nil, errors.New("googleoauth: client_id is required")
	}
	if cfg.RedirectURI == "" {
		return nil, errors.New("googleoauth: redirect_uri is required")
	}

	authURL := cfg.AuthURL
	if authURL == "" {
		authURL = DefaultAuthURL
	}
	tokenURL := cfg.TokenURL
	if tokenURL == "" {
		tokenURL = DefaultTokenURL
	}
	userInfoURL := cfg.UserInfoURL
	if userInfoURL == "" {
		userInfoURL = DefaultUserInfoURL
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}

	return &GoogleOAuthClient{
		clientID:     cfg.ClientID,
		clientSecret: cfg.ClientSecret,
		redirectURI:  cfg.RedirectURI,
		authURL:      authURL,
		tokenURL:     tokenURL,
		userInfoURL:  userInfoURL,
		httpClient:   httpClient,
	}, nil
}

// GeneratePKCE generates a cryptographically secure PKCE code verifier and code challenge (S256).
func GeneratePKCE() (codeVerifier string, codeChallenge string, err error) {
	raw := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return "", "", fmt.Errorf("googleoauth: failed to generate pkce verifier: %w", err)
	}

	codeVerifier = base64.RawURLEncoding.EncodeToString(raw)
	h := sha256.Sum256([]byte(codeVerifier))
	codeChallenge = base64.RawURLEncoding.EncodeToString(h[:])
	return codeVerifier, codeChallenge, nil
}

// BuildAuthorizationURL constructs the Google consent screen URL with all mandated parameters [GATE].
// Per 12_gmail_connector.md §4.2:
// - access_type=offline (mandatory to obtain refresh token)
// - prompt=consent (guarantees refresh token issuance on repeat authorization)
// - include_granted_scopes=true (supports incremental authorization)
// - scopes limited strictly to gmail.readonly, openid, email, profile (least privilege)
func (c *GoogleOAuthClient) BuildAuthorizationURL(state, codeChallenge string) string {
	q := url.Values{}
	q.Set("client_id", c.clientID)
	q.Set("redirect_uri", c.redirectURI)
	q.Set("response_type", "code")
	q.Set("state", state)
	q.Set("code_challenge", codeChallenge)
	q.Set("code_challenge_method", "S256")

	// Consent parameters (12_gmail_connector.md §4.2)
	q.Set("access_type", "offline")
	q.Set("prompt", "consent")
	q.Set("include_granted_scopes", "true")

	// Mandated Scopes [GATE: never request write scopes]
	scopes := []string{
		ScopeGmailReadOnly,
		ScopeOpenID,
		ScopeEmail,
		ScopeProfile,
	}
	q.Set("scope", strings.Join(scopes, " "))

	return fmt.Sprintf("%s?%s", c.authURL, q.Encode())
}

var _ domain.OAuthClient = (*GoogleOAuthClient)(nil)

// ExchangeCode exchanges the authorization code for OAuth tokens using PKCE verifier.
func (c *GoogleOAuthClient) ExchangeCode(ctx context.Context, code, codeVerifier string) (*domain.TokenExchangeResult, error) {
	data := url.Values{}
	data.Set("client_id", c.clientID)
	if c.clientSecret != "" {
		data.Set("client_secret", c.clientSecret)
	}
	data.Set("code", code)
	data.Set("code_verifier", codeVerifier)
	data.Set("redirect_uri", c.redirectURI)
	data.Set("grant_type", "authorization_code")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("googleoauth: failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("googleoauth: token request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("googleoauth: failed to read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errPayload struct {
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
		}
		_ = json.Unmarshal(body, &errPayload)
		return nil, fmt.Errorf("googleoauth: token endpoint returned status %d: %s (%s)",
			resp.StatusCode, errPayload.Error, errPayload.ErrorDescription)
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("googleoauth: failed to parse token response: %w", err)
	}

	return &domain.TokenExchangeResult{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresIn:    tokenResp.ExpiresIn,
		Scope:        tokenResp.Scope,
	}, nil
}

// RefreshToken exchanges an existing refresh token for a fresh access token.
// Per 12_gmail_connector.md §4.3: invalid_grant is classified as terminal revocation.
func (c *GoogleOAuthClient) RefreshToken(ctx context.Context, refreshToken string) (*domain.TokenExchangeResult, error) {
	data := url.Values{}
	data.Set("client_id", c.clientID)
	if c.clientSecret != "" {
		data.Set("client_secret", c.clientSecret)
	}
	data.Set("refresh_token", refreshToken)
	data.Set("grant_type", "refresh_token")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("googleoauth: failed to create refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("googleoauth: refresh request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("googleoauth: failed to read refresh response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errPayload struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &errPayload)

		// Terminal revocation detection per 12_gmail_connector.md §4.3
		if errPayload.Error == "invalid_grant" {
			return nil, domain.ErrRevocationDetected
		}
		return nil, fmt.Errorf("googleoauth: refresh failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("googleoauth: failed to parse refresh response: %w", err)
	}

	return &domain.TokenExchangeResult{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresIn:    tokenResp.ExpiresIn,
		Scope:        tokenResp.Scope,
	}, nil
}

// FetchUserInfo retrieves user profile details from Google's UserInfo API using the access token.
func (c *GoogleOAuthClient) FetchUserInfo(ctx context.Context, accessToken string) (*domain.UserInfoResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.userInfoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("googleoauth: failed to create userinfo request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("googleoauth: userinfo request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("googleoauth: failed to read userinfo response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("googleoauth: userinfo returned status %d: %s", resp.StatusCode, string(body))
	}

	var userInfo GoogleUserInfo
	if err := json.Unmarshal(body, &userInfo); err != nil {
		return nil, fmt.Errorf("googleoauth: failed to parse userinfo: %w", err)
	}

	return &domain.UserInfoResult{
		Email: userInfo.Email,
		Name:  userInfo.Name,
		Sub:   userInfo.Sub,
	}, nil
}
