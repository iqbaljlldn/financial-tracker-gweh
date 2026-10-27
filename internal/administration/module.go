package administration

import (
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	adminHTTP "github.com/iqbaljlldn/financial-tracker-gweh/internal/administration/adapter/http"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/administration/adapter/postgres"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/administration/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/security"
)

// Config holds composition-root dependencies for the Administration module.
type Config struct {
	DB             *pgxpool.Pool
	SessionManager *security.SessionTokenManager
	Logger         *slog.Logger
}

// Module encapsulates the Administration bounded context per 06_architecture.md §4.
type Module struct {
	AuditLogger   domain.AuditLogger
	AuditRepo     domain.AuditLogRepository
	ConnectorRepo domain.ConnectorHealthRepository
	ParserRepo    domain.ParserMetricsRepository
	PromptRepo    domain.PromptMetricsRepository
	HTTPHandler   *adminHTTP.Handler
	logger        *slog.Logger
}

// NewModule constructs the Administration module and initializes its components.
func NewModule(cfg Config) (*Module, error) {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	auditRepo := postgres.NewAuditLogRepository(cfg.DB)
	connectorRepo := postgres.NewConnectorHealthRepository(cfg.DB)
	parserRepo := postgres.NewParserMetricsRepository(cfg.DB)
	promptRepo := postgres.NewPromptMetricsRepository(cfg.DB)

	httpHandler := adminHTTP.NewHandler(adminHTTP.HandlerConfig{
		AuditRepo:     auditRepo,
		AuditLogger:   auditRepo,
		ConnectorRepo: connectorRepo,
		ParserRepo:    parserRepo,
		PromptRepo:    promptRepo,
		SessionMgr:    cfg.SessionManager,
	})

	return &Module{
		AuditLogger:   auditRepo,
		AuditRepo:     auditRepo,
		ConnectorRepo: connectorRepo,
		ParserRepo:    parserRepo,
		PromptRepo:    promptRepo,
		HTTPHandler:   httpHandler,
		logger:        logger,
	}, nil
}

// RegisterHTTPRoutes mounts all administrative endpoints onto the Chi router.
func (m *Module) RegisterHTTPRoutes(r chi.Router) {
	m.HTTPHandler.RegisterRoutes(r)
}
