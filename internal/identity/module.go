package identity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-chi/chi/v5"

	adminDomain "github.com/iqbaljlldn/financial-tracker-gweh/internal/administration/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/identity/adapter/external"
	identityhttp "github.com/iqbaljlldn/financial-tracker-gweh/internal/identity/adapter/http"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/identity/adapter/postgres"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/identity/application/command"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/identity/application/query"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/identity/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/security"
)

// Config provides initialization parameters for the Identity module composition root.
type Config struct {
	DB                 postgres.DBTX
	AuditLogger        adminDomain.AuditLogger
	UserDataExporter   domain.UserDataExporter
	KMSClient          security.KMSClient
	KMSKeyID           string
	JWTSecret          []byte
	CSRFSecret         []byte
	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURI  string
	GoogleAuthURL      string
	GoogleTokenURL     string
	GoogleUserInfoURL  string
}

// Module encapsulates the Identity & Access bounded context.
type Module struct {
	HTTPHandler       *identityhttp.Handler
	UserRepo          domain.UserRepository
	TokenRepo         domain.OAuthTokenRepository
	RefreshRepo       domain.RefreshTokenRepository
	LinkAccountCmd    *command.LinkGoogleAccountHandler
	RefreshOAuthCmd   *command.RefreshOAuthTokenHandler
	UpdateSettingsCmd *command.UpdateUserSettingsHandler
	GetUserQuery      *query.GetUserByIDHandler
	SessionManager    *security.SessionTokenManager
	CSRFManager       *security.OAuthStateManager
	EnvelopeEncryptor *security.EnvelopeEncryptor
	GoogleOAuthClient *external.GoogleOAuthClient
}

// NewModule constructs and wires the Identity module using Clean Architecture manual dependency injection per 06_architecture.md §8.
func NewModule(cfg Config) (*Module, error) {
	if cfg.DB == nil {
		return nil, errors.New("identity: database handle is required")
	}
	if cfg.KMSClient == nil {
		return nil, errors.New("identity: KMSClient is required")
	}
	if cfg.KMSKeyID == "" {
		cfg.KMSKeyID = "default-kms-key"
	}
	if len(cfg.JWTSecret) < 32 {
		return nil, errors.New("identity: JWTSecret must be at least 32 bytes")
	}
	if len(cfg.CSRFSecret) < 16 {
		return nil, errors.New("identity: CSRFSecret must be at least 16 bytes")
	}

	// 1. Adapters & Infrastructure
	userRepo := postgres.NewUserRepository(cfg.DB)
	tokenRepo := postgres.NewOAuthTokenRepository(cfg.DB)
	refreshRepo := postgres.NewRefreshTokenRepository(cfg.DB)

	encryptor, err := security.NewEnvelopeEncryptor(cfg.KMSClient, cfg.KMSKeyID)
	if err != nil {
		return nil, err
	}

	sessionManager, err := security.NewSessionTokenManager(cfg.JWTSecret)
	if err != nil {
		return nil, err
	}

	csrfManager, err := security.NewOAuthStateManager(cfg.CSRFSecret, 10*time.Minute)
	if err != nil {
		return nil, err
	}

	oauthClient, err := external.NewGoogleOAuthClient(external.GoogleOAuthClientConfig{
		ClientID:     cfg.GoogleClientID,
		ClientSecret: cfg.GoogleClientSecret,
		RedirectURI:  cfg.GoogleRedirectURI,
		AuthURL:      cfg.GoogleAuthURL,
		TokenURL:     cfg.GoogleTokenURL,
		UserInfoURL:  cfg.GoogleUserInfoURL,
	})
	if err != nil {
		return nil, err
	}

	// 2. Application Layer Command & Query Handlers
	linkAccountCmd := command.NewLinkGoogleAccountHandler(
		userRepo,
		tokenRepo,
		refreshRepo,
		oauthClient,
		encryptor,
		sessionManager,
	)

	refreshOAuthCmd := command.NewRefreshOAuthTokenHandler(
		tokenRepo,
		oauthClient,
		encryptor,
	)

	updateSettingsCmd := command.NewUpdateUserSettingsHandler(userRepo)
	getUserQuery := query.NewGetUserByIDHandler(userRepo)

	// 3. Exporter
	exporter := cfg.UserDataExporter
	if exporter == nil && cfg.DB != nil {
		exporter = postgres.NewPostgresUserDataExporter(cfg.DB)
	}

	// 4. HTTP Handler
	httpHandler := identityhttp.NewHandler(identityhttp.HandlerConfig{
		LinkAccountCmd:    linkAccountCmd,
		UpdateSettingsCmd: updateSettingsCmd,
		GetUserQuery:      getUserQuery,
		RefreshRepo:       refreshRepo,
		UserRepo:          userRepo,
		UserDataExporter:  exporter,
		AuditLogger:       cfg.AuditLogger,
		OAuthClient:       oauthClient,
		CSRFManager:       csrfManager,
		SessionManager:    sessionManager,
	})

	return &Module{
		HTTPHandler:       httpHandler,
		UserRepo:          userRepo,
		TokenRepo:         tokenRepo,
		RefreshRepo:       refreshRepo,
		LinkAccountCmd:    linkAccountCmd,
		RefreshOAuthCmd:   refreshOAuthCmd,
		UpdateSettingsCmd: updateSettingsCmd,
		GetUserQuery:      getUserQuery,
		SessionManager:    sessionManager,
		CSRFManager:       csrfManager,
		EnvelopeEncryptor: encryptor,
		GoogleOAuthClient: oauthClient,
	}, nil
}

// RegisterHTTPRoutes mounts all Identity endpoints onto the provided Chi router.
func (m *Module) RegisterHTTPRoutes(r chi.Router) {
	m.HTTPHandler.RegisterRoutes(r)
}

// TokenProvider returns the sanctioned cross-module TokenProvider interface per 10_backend_blueprint.md §6.
func (m *Module) TokenProvider() domain.TokenProvider {
	return &tokenProviderAdapter{
		tokenRepo:  m.TokenRepo,
		refreshCmd: m.RefreshOAuthCmd,
		encryptor:  m.EnvelopeEncryptor,
	}
}

type tokenProviderAdapter struct {
	tokenRepo  domain.OAuthTokenRepository
	refreshCmd *command.RefreshOAuthTokenHandler
	encryptor  *security.EnvelopeEncryptor
}

func (p *tokenProviderAdapter) GetValidAccessToken(ctx context.Context, gmailAccountID string) (string, error) {
	token, err := p.tokenRepo.FindByGmailAccountID(ctx, gmailAccountID)
	if err != nil {
		return "", fmt.Errorf("token provider: token not found for account %s: %w", gmailAccountID, err)
	}

	// Proactively refresh if expired or expiring within 5 minutes
	if time.Now().UTC().Add(5 * time.Minute).After(token.AccessTokenExpiresAt()) {
		res, err := p.refreshCmd.Handle(ctx, command.RefreshOAuthTokenCommand{GmailAccountID: gmailAccountID})
		if err != nil {
			return "", fmt.Errorf("token provider: refresh failed: %w", err)
		}
		return res.AccessToken, nil
	}

	// Decrypt active access token
	decrypted, err := p.encryptor.Decrypt(ctx, token.EncryptedAccessToken())
	if err != nil {
		return "", fmt.Errorf("token provider: decrypt access token failed: %w", err)
	}
	return string(decrypted), nil
}
