package domain

import "context"

// TokenExchangeResult represents the token payload returned by the OAuth provider.
type TokenExchangeResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
	Scope        string
}

// UserInfoResult represents the user identity returned by the OAuth provider.
type UserInfoResult struct {
	Email string
	Name  string
	Sub   string
}

// OAuthClient defines the port for communicating with an external OAuth 2.0 provider.
// Application layer command handlers depend on this interface, maintaining Clean Architecture isolation.
type OAuthClient interface {
	ExchangeCode(ctx context.Context, code, codeVerifier string) (*TokenExchangeResult, error)
	RefreshToken(ctx context.Context, refreshToken string) (*TokenExchangeResult, error)
	FetchUserInfo(ctx context.Context, accessToken string) (*UserInfoResult, error)
}
