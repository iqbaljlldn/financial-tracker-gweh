package ingestion

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"

	identityDomain "github.com/iqbaljlldn/financial-tracker-gweh/internal/identity/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/ingestion/adapter/external"
	ingestionHTTP "github.com/iqbaljlldn/financial-tracker-gweh/internal/ingestion/adapter/http"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/ingestion/adapter/outbox"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/ingestion/adapter/postgres"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/ingestion/application/command"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/ingestion/application/query"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/ingestion/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/security"
)

// Export command and query types for external callers.
type (
	StartInitialFullSyncCommand = command.StartInitialFullSyncCommand
	StartInitialFullSyncResult  = command.StartInitialFullSyncResult
	LinkGmailAccountCommand     = command.LinkGmailAccountCommand
	LinkGmailAccountResult      = command.LinkGmailAccountResult
	ListGmailAccountsQuery      = query.ListGmailAccountsQuery
)

// Config models dependencies for Ingestion module initialization.
type Config struct {
	DB             *pgxpool.Pool
	TokenProvider  identityDomain.TokenProvider // Sanctioned cross-module interface per 10_backend_blueprint.md §6 [GATE]
	Connector      domain.Connector             // Optional override (e.g. for mock integration testing)
	AsynqClient    *asynq.Client                // Optional for background outbox tasks
	SessionManager *security.SessionTokenManager
	GmailBaseURL   string // Optional override for mock testing
	PubSubTopic    string // projects/{project}/topics/{topic} for users.watch
	Logger         *slog.Logger
}

// Module encapsulates the Ingestion bounded context per 06_architecture.md §4.
type Module struct {
	HTTPHandler             *ingestionHTTP.Handler
	Repo                    *postgres.IngestionRepository
	Connector               domain.Connector
	LinkAccountCmd          *command.LinkGmailAccountHandler
	StartInitialFullSyncCmd *command.StartInitialFullSyncHandler
	ListAccountsQuery       *query.ListGmailAccountsHandler
	OutboxRelay             *outbox.IngestionOutboxRelay
	logger                  *slog.Logger
}

// NewModule constructs the Ingestion composition root.
func NewModule(cfg Config) (*Module, error) {
	if cfg.DB == nil {
		return nil, errors.New("ingestion: DB connection pool is required")
	}
	if cfg.TokenProvider == nil {
		return nil, errors.New("ingestion: TokenProvider is required")
	}
	if cfg.SessionManager == nil {
		return nil, errors.New("ingestion: SessionManager is required")
	}

	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// 1. Adapter Layer
	repo := postgres.NewIngestionRepository(cfg.DB)

	connector := cfg.Connector
	if connector == nil {
		gmailClient, err := external.NewGmailClient(external.GmailClientConfig{
			BaseURL:       cfg.GmailBaseURL,
			TokenProvider: cfg.TokenProvider,
			PubSubTopic:   cfg.PubSubTopic,
		})
		if err != nil {
			return nil, err
		}
		connector = gmailClient
	}

	classifier := domain.NewClassifier()

	// 2. Application Layer
	linkCmd := command.NewLinkGmailAccountHandler(repo, repo)
	syncCmd := command.NewStartInitialFullSyncHandler(repo, repo, connector, classifier, logger)
	listQuery := query.NewListGmailAccountsHandler(repo)

	// 3. Outbox Relay
	var relay *outbox.IngestionOutboxRelay
	if cfg.AsynqClient != nil {
		relay = outbox.NewIngestionOutboxRelay(cfg.DB, cfg.AsynqClient, 500*time.Millisecond, logger)
	}

	// 4. HTTP Adapter
	httpHandler := ingestionHTTP.NewHandler(ingestionHTTP.HandlerConfig{
		LinkCmd:        linkCmd,
		SyncCmd:        syncCmd,
		ListQuery:      listQuery,
		SessionManager: cfg.SessionManager,
	})

	return &Module{
		HTTPHandler:             httpHandler,
		Repo:                    repo,
		Connector:               connector,
		LinkAccountCmd:          linkCmd,
		StartInitialFullSyncCmd: syncCmd,
		ListAccountsQuery:       listQuery,
		OutboxRelay:             relay,
		logger:                  logger,
	}, nil
}

// RegisterHTTPRoutes mounts all Ingestion HTTP endpoints onto the Chi router.
func (m *Module) RegisterHTTPRoutes(r chi.Router) {
	m.HTTPHandler.RegisterRoutes(r)
}

// StartOutboxRelay starts background poller for unrelayed outbox events.
func (m *Module) StartOutboxRelay(ctx context.Context) error {
	if m.OutboxRelay == nil {
		return nil
	}
	return m.OutboxRelay.Start(ctx)
}

// StopOutboxRelay stops the background poller.
func (m *Module) StopOutboxRelay() {
	if m.OutboxRelay != nil {
		m.OutboxRelay.Stop()
	}
}

// PollOutboxOnce synchronously polls and relays outbox events (useful for tests).
func (m *Module) PollOutboxOnce(ctx context.Context) (int, error) {
	if m.OutboxRelay == nil {
		return 0, nil
	}
	return m.OutboxRelay.PollOnce(ctx)
}
