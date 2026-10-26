// Package main is the composition root for the Financial Event Platform.
// It provides a single, dual-mode binary running as either an HTTP API server
// or an asynchronous worker consumer per 06_architecture.md §11 and 21_project_structure.md §3.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/financialcore"
	fcCmd "github.com/iqbaljlldn/financial-tracker-gweh/internal/financialcore/application/command"
	fcDomain "github.com/iqbaljlldn/financial-tracker-gweh/internal/financialcore/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/identity"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/ingestion"
	ingestionCmd "github.com/iqbaljlldn/financial-tracker-gweh/internal/ingestion/application/command"
	ingestionDomain "github.com/iqbaljlldn/financial-tracker-gweh/internal/ingestion/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/config"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/httpserver"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/observability"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/security"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/taskserver"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/application/categorization"
	understandingCmd "github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/application/command"
	understandingDomain "github.com/iqbaljlldn/financial-tracker-gweh/internal/understanding/domain"
)

var (
	// Version is populated at build time via -ldflags.
	Version = "dev"
	// Commit is populated at build time via -ldflags.
	Commit = "none"
)

const (
	modeAPI    = "api"
	modeWorker = "worker"
)

func main() {
	var mode string
	flag.StringVar(&mode, "mode", "", "Execution mode: 'api' for HTTP serving or 'worker' for background task processing")
	flag.Parse()

	// Environment variable fallback if flag is not provided
	if mode == "" {
		mode = os.Getenv("PLATFORM_MODE")
	}
	if mode == "" {
		mode = modeAPI
	}

	mode = strings.ToLower(strings.TrimSpace(mode))

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	if mode != modeAPI && mode != modeWorker {
		logger.Error("invalid platform mode specified",
			slog.String("mode", mode),
			slog.String("expected", "api|worker"),
		)
		flag.Usage()
		os.Exit(1)
	}

	logger.Info("starting financial-event-platform",
		slog.String("mode", mode),
		slog.String("version", Version),
		slog.String("commit", Commit),
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch mode {
	case modeAPI:
		runAPI(ctx, logger)
	case modeWorker:
		runWorker(ctx, logger)
	}

	logger.Info("financial-event-platform shut down cleanly", slog.String("mode", mode))
}

func loadAppConfig(logger *slog.Logger) *config.Config {
	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "config/config.dev.yaml"
	}
	cfg, err := config.Load(cfgPath, "config/secrets.env", ".env", "secrets.env")
	if err != nil {
		logger.Warn("could not load YAML config, falling back to defaults/env", slog.String("error", err.Error()))
		return &config.Config{}
	}
	return cfg
}

func resolveDatabaseURL(cfg *config.Config) string {
	if cfg.Platform.DatabaseURL != "" {
		return cfg.Platform.DatabaseURL
	}
	if envURL := os.Getenv("DATABASE_URL"); envURL != "" {
		return envURL
	}
	return "postgres://postgres:postgres@localhost:5433/financial_tracker?sslmode=disable"
}

func resolveRedisURL(cfg *config.Config) string {
	if cfg.Platform.RedisURL != "" {
		return cfg.Platform.RedisURL
	}
	if envURL := os.Getenv("REDIS_URL"); envURL != "" {
		return envURL
	}
	return "redis://localhost:6380/0"
}

func initKMSClient(cfg *config.Config) (security.KMSClient, string, error) {
	kmsKeyID := cfg.Platform.KMSKeyID
	if kmsKeyID == "" {
		kmsKeyID = "primary-kms-key"
	}
	masterSecret := os.Getenv("KMS_MASTER_KEY_SECRET")
	var masterKey []byte
	if len(masterSecret) >= 32 {
		masterKey = []byte(masterSecret)[:32]
	} else {
		masterKey = []byte("financial-tracker-32byte-kms-key")
	}
	client, err := security.NewLocalKMSClient(kmsKeyID, masterKey)
	return client, kmsKeyID, err
}

func initSecrets(cfg *config.Config) ([]byte, []byte) {
	jwtSecret := []byte(cfg.Platform.JWTSecret)
	if len(jwtSecret) < 32 {
		jwtSecret = []byte("financial-tracker-session-secret-key-32bytes!")
	}
	csrfSecret := []byte(os.Getenv("CSRF_SECRET"))
	if len(csrfSecret) < 16 {
		csrfSecret = []byte("csrf-secret-16bytes!")
	}
	return jwtSecret, csrfSecret
}

type categorySuggesterBridge struct {
	engine *categorization.Engine
}

func (b *categorySuggesterBridge) Suggest(ctx context.Context, userID, normalizedMerchant string) (fcDomain.CategorySuggestion, bool) {
	if b.engine == nil {
		return fcDomain.CategorySuggestion{}, false
	}
	assign := b.engine.AssignCategory(ctx, categorization.Request{
		UserID:             userID,
		NormalizedMerchant: normalizedMerchant,
	})
	if assign.Source == categorization.SourceDefault || assign.Category == categorization.CategoryUncategorized {
		return fcDomain.CategorySuggestion{}, false
	}
	return fcDomain.CategorySuggestion{
		Category:   assign.Category,
		Confidence: assign.Confidence,
		Source:     assign.Source,
	}, true
}

type categoryFeedbackBridge struct {
	engine *categorization.Engine
}

func (b *categoryFeedbackBridge) RecordUserCorrection(ctx context.Context, userID, normalizedMerchant, category string) error {
	if b.engine == nil {
		return nil
	}
	return b.engine.RecordUserCorrection(ctx, userID, normalizedMerchant, category)
}

func runAPI(ctx context.Context, logger *slog.Logger) {
	cfg := loadAppConfig(logger)
	dbPool, err := pgxpool.New(ctx, resolveDatabaseURL(cfg))
	if err != nil {
		logger.Error("failed to connect to postgres", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer dbPool.Close()

	kmsClient, kmsKeyID, err := initKMSClient(cfg)
	if err != nil {
		logger.Error("failed to initialize KMS client", slog.String("error", err.Error()))
		os.Exit(1)
	}

	jwtSecret, csrfSecret := initSecrets(cfg)

	identityMod, err := identity.NewModule(identity.Config{
		DB:                 dbPool,
		KMSClient:          kmsClient,
		KMSKeyID:           kmsKeyID,
		JWTSecret:          jwtSecret,
		CSRFSecret:         csrfSecret,
		GoogleClientID:     cfg.Identity.GoogleClientID,
		GoogleClientSecret: cfg.Identity.GoogleClientSecret,
		GoogleRedirectURI:  cfg.Identity.GoogleRedirectURL,
	})
	if err != nil {
		logger.Error("failed to initialize identity module", slog.String("error", err.Error()))
		os.Exit(1)
	}

	sessionManager, err := security.NewSessionTokenManager(jwtSecret)
	if err != nil {
		logger.Error("failed to initialize session manager", slog.String("error", err.Error()))
		os.Exit(1)
	}

	redisURL := resolveRedisURL(cfg)
	asynqClient, err := taskserver.NewClient(redisURL)
	if err != nil {
		logger.Warn("could not connect to Redis, background outbox tasks disabled", slog.String("error", err.Error()))
	}

	ingestionMod, err := ingestion.NewModule(ingestion.Config{
		DB:                       dbPool,
		TokenProvider:            identityMod.TokenProvider(), // Sanctioned cross-module interface [GATE]
		AsynqClient:              asynqClient,
		SessionManager:           sessionManager,
		PubSubTopic:              cfg.Ingestion.PubSubTopic,
		PubSubVerificationSecret: cfg.Ingestion.PubSubVerificationSecret,
		PollFallbackInterval:     cfg.Ingestion.PollFallbackInterval,
		HistoryFallbackDays:      cfg.Ingestion.HistoryFallbackLookbackDays,
		Logger:                   logger,
	})
	if err != nil {
		logger.Error("failed to initialize ingestion module", slog.String("error", err.Error()))
		os.Exit(1)
	}

	port := cfg.Platform.HTTP.Port
	if port <= 0 {
		port = 8080
	}

	server := httpserver.NewServer(httpserver.Config{
		Port:         port,
		ReadTimeout:  cfg.Platform.HTTP.ReadTimeout,
		WriteTimeout: cfg.Platform.HTTP.WriteTimeout,
	}, logger)

	// Mount Prometheus metrics endpoint per 17_observability.md §4
	server.Router().Handle("/metrics", observability.GetMetrics().Handler())

	understandingMod, err := understanding.NewModule(understanding.Config{
		DB:             dbPool,
		AsynqClient:    asynqClient,
		SessionManager: sessionManager,
		AIConfig: understanding.AIConfig{
			BaseURL:      cfg.Understanding.ResolvedBaseURL,
			APIKey:       cfg.Understanding.ResolvedAPIKey,
			DefaultModel: cfg.Understanding.Model,
			Timeout:      cfg.Understanding.AIRequestTimeout,
		},
		Logger: logger,
	})
	if err != nil {
		logger.Error("failed to initialize understanding module", slog.String("error", err.Error()))
		os.Exit(1)
	}
	financialcoreMod, err := financialcore.NewModule(financialcore.Config{
		DB:                  dbPool,
		SessionManager:      sessionManager,
		ConfidenceThreshold: cfg.FinancialCore.ConfidenceGateThreshold,
		PendingReviewSLA:    cfg.FinancialCore.PendingReviewSLA,
		CategorySuggester:   &categorySuggesterBridge{engine: understandingMod.Categorizer},
		CategoryFeedback:    &categoryFeedbackBridge{engine: understandingMod.Categorizer},
		Logger:              logger,
	})
	if err != nil {
		logger.Error("failed to initialize financialcore module", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// Mount Identity, Ingestion, Understanding, and FinancialCore routes
	identityMod.RegisterHTTPRoutes(server.Router())
	ingestionMod.RegisterHTTPRoutes(server.Router())
	understandingMod.RegisterHTTPRoutes(server.Router())
	financialcoreMod.RegisterHTTPRoutes(server.Router())

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.Start()
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutting down HTTP API server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	case err := <-serverErr:
		if err != nil {
			logger.Error("HTTP server exited unexpectedly", slog.String("error", err.Error()))
		}
	}
}

func runWorker(ctx context.Context, logger *slog.Logger) {
	cfg := loadAppConfig(logger)
	dbPool, err := pgxpool.New(ctx, resolveDatabaseURL(cfg))
	if err != nil {
		logger.Error("failed to connect to postgres in worker mode", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer dbPool.Close()

	kmsClient, kmsKeyID, err := initKMSClient(cfg)
	if err != nil {
		logger.Error("failed to initialize KMS client", slog.String("error", err.Error()))
		os.Exit(1)
	}

	jwtSecret, csrfSecret := initSecrets(cfg)

	identityMod, err := identity.NewModule(identity.Config{
		DB:                 dbPool,
		KMSClient:          kmsClient,
		KMSKeyID:           kmsKeyID,
		JWTSecret:          jwtSecret,
		CSRFSecret:         csrfSecret,
		GoogleClientID:     cfg.Identity.GoogleClientID,
		GoogleClientSecret: cfg.Identity.GoogleClientSecret,
		GoogleRedirectURI:  cfg.Identity.GoogleRedirectURL,
	})
	if err != nil {
		logger.Error("failed to initialize identity module", slog.String("error", err.Error()))
		os.Exit(1)
	}

	sessionManager, err := security.NewSessionTokenManager(jwtSecret)
	if err != nil {
		logger.Error("failed to initialize session manager", slog.String("error", err.Error()))
		os.Exit(1)
	}

	redisURL := resolveRedisURL(cfg)
	asynqClient, err := taskserver.NewClient(redisURL)
	if err != nil {
		logger.Error("failed to connect to Redis", slog.String("error", err.Error()))
		os.Exit(1)
	}

	ingestionMod, err := ingestion.NewModule(ingestion.Config{
		DB:                       dbPool,
		TokenProvider:            identityMod.TokenProvider(),
		AsynqClient:              asynqClient,
		SessionManager:           sessionManager,
		PubSubTopic:              cfg.Ingestion.PubSubTopic,
		PubSubVerificationSecret: cfg.Ingestion.PubSubVerificationSecret,
		PollFallbackInterval:     cfg.Ingestion.PollFallbackInterval,
		HistoryFallbackDays:      cfg.Ingestion.HistoryFallbackLookbackDays,
		Logger:                   logger,
	})
	if err != nil {
		logger.Error("failed to initialize ingestion module", slog.String("error", err.Error()))
		os.Exit(1)
	}

	understandingMod, err := understanding.NewModule(understanding.Config{
		DB:             dbPool,
		AsynqClient:    asynqClient,
		SessionManager: sessionManager,
		AIConfig: understanding.AIConfig{
			BaseURL:      cfg.Understanding.ResolvedBaseURL,
			APIKey:       cfg.Understanding.ResolvedAPIKey,
			DefaultModel: cfg.Understanding.Model,
			Timeout:      cfg.Understanding.AIRequestTimeout,
		},
		Logger: logger,
	})
	if err != nil {
		logger.Error("failed to initialize understanding module in worker", slog.String("error", err.Error()))
		os.Exit(1)
	}
	_ = understandingMod.SeedCuratedTemplates(ctx)

	financialcoreMod, err := financialcore.NewModule(financialcore.Config{
		DB:                  dbPool,
		SessionManager:      sessionManager,
		ConfidenceThreshold: cfg.FinancialCore.ConfidenceGateThreshold,
		PendingReviewSLA:    cfg.FinancialCore.PendingReviewSLA,
		CategorySuggester:   &categorySuggesterBridge{engine: understandingMod.Categorizer},
		CategoryFeedback:    &categoryFeedbackBridge{engine: understandingMod.Categorizer},
		Logger:              logger,
	})
	if err != nil {
		logger.Error("failed to initialize financialcore module in worker", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// Start Ingestion Outbox Relay
	if err := ingestionMod.StartOutboxRelay(ctx); err != nil {
		logger.Error("failed to start ingestion outbox relay", slog.String("error", err.Error()))
	}
	defer ingestionMod.StopOutboxRelay()

	// Start Understanding Outbox Relay
	if err := understandingMod.StartOutboxRelay(ctx); err != nil {
		logger.Error("failed to start understanding outbox relay", slog.String("error", err.Error()))
	}
	defer understandingMod.StopOutboxRelay()

	// Start Poll-Fallback Safety Net Scheduler [GATE - H2 Resolution]
	ingestionMod.PollFallbackScheduler.Start(ctx)
	defer ingestionMod.PollFallbackScheduler.Stop()

	// Initialize Asynq task server
	server, err := taskserver.NewServer(taskserver.ServerConfig{
		RedisURI:    redisURL,
		Concurrency: 10,
		Logger:      logger,
	})
	if err != nil {
		logger.Error("failed to initialize asynq worker server", slog.String("error", err.Error()))
		os.Exit(1)
	}

	mux := asynq.NewServeMux()
	registerWorkerTaskHandlers(mux, ingestionMod, understandingMod, financialcoreMod, logger)

	// Periodic watch renewal and parser metrics background loop
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, _ = ingestionMod.RenewWatchesCmd.Handle(ctx, ingestionCmd.RenewExpiringWatchesCommand{
					SafetyMargin: 24 * time.Hour,
				})
				_, _ = understandingMod.RollupMetricsCmd.Handle(ctx, understandingCmd.RollupParserMetricsCommand{
					Trigger: "scheduled",
				})
				_ = financialcoreMod.CheckSLACmd.Handle(ctx, fcCmd.CheckPendingReviewSLACommand{
					Now: time.Now(),
				})
			}
		}
	}()

	logger.Info("Asynq worker server started listening to queues")
	go func() {
		if err := server.Run(mux); err != nil {
			logger.Error("asynq worker server stopped with error", slog.String("error", err.Error()))
		}
	}()

	<-ctx.Done()
	logger.Info("stopping Asynq worker server...")
	server.Stop()
}

func registerWorkerTaskHandlers(
	mux *asynq.ServeMux,
	ingestionMod *ingestion.Module,
	understandingMod *understanding.Module,
	financialcoreMod *financialcore.Module,
	logger *slog.Logger,
) {
	mux.HandleFunc(taskserver.TaskStartInitialFullSync, func(ctx context.Context, t *asynq.Task) error {
		var payload struct {
			AccountID  string `json:"accountId"`
			WindowDays int    `json:"windowDays"`
		}
		if err := json.Unmarshal(t.Payload(), &payload); err != nil {
			return err
		}
		logger.Info("processing TaskStartInitialFullSync", "account_id", payload.AccountID)
		_, err := ingestionMod.StartInitialFullSyncCmd.Handle(ctx, ingestionCmd.StartInitialFullSyncCommand{
			AccountID:  payload.AccountID,
			WindowDays: payload.WindowDays,
		})
		return err
	})

	mux.HandleFunc(taskserver.TaskRunIncrementalSync, func(ctx context.Context, t *asynq.Task) error {
		var payload struct {
			AccountID string `json:"accountId"`
			Trigger   string `json:"trigger"`
		}
		if err := json.Unmarshal(t.Payload(), &payload); err != nil {
			return err
		}
		logger.Info("processing TaskRunIncrementalSync", "account_id", payload.AccountID, "trigger", payload.Trigger)
		_, err := ingestionMod.IncrementalSyncCmd.Handle(ctx, ingestionCmd.RunIncrementalSyncCommand{
			AccountID: payload.AccountID,
			Trigger:   payload.Trigger,
		})
		return err
	})

	mux.HandleFunc(taskserver.TaskRegisterGmailWatch, func(ctx context.Context, t *asynq.Task) error {
		var payload struct {
			AccountID string `json:"accountId"`
		}
		if err := json.Unmarshal(t.Payload(), &payload); err != nil {
			return err
		}
		logger.Info("processing TaskRegisterGmailWatch", "account_id", payload.AccountID)
		_, err := ingestionMod.RegisterWatchCmd.Handle(ctx, ingestionCmd.RegisterGmailWatchCommand{
			AccountID: payload.AccountID,
		})
		return err
	})

	mux.HandleFunc(taskserver.TaskRenewExpiringWatches, func(ctx context.Context, t *asynq.Task) error {
		logger.Info("processing TaskRenewExpiringWatches")
		_, err := ingestionMod.RenewWatchesCmd.Handle(ctx, ingestionCmd.RenewExpiringWatchesCommand{
			SafetyMargin: 24 * time.Hour,
		})
		return err
	})

	mux.HandleFunc(taskserver.TaskMessageIngested, func(ctx context.Context, t *asynq.Task) error {
		var payload ingestionDomain.MessageIngestedPayload
		if err := json.Unmarshal(t.Payload(), &payload); err != nil {
			return fmt.Errorf("unmarshaling MessageIngestedPayload: %w", err)
		}

		logger.Info("processing TaskMessageIngested",
			"message_id", payload.MessageID,
			"gmail_account_id", payload.GmailAccountID,
			"gmail_message_id", payload.GmailMessageID,
			"decision", payload.RoutingDecision,
		)

		if payload.RoutingDecision == ingestionDomain.RoutingDiscarded {
			_ = ingestionMod.Repo.UpdateMessageProcessingStatus(ctx, payload.MessageID, "completed")
			return nil
		}

		account, err := ingestionMod.Repo.FindByID(ctx, payload.GmailAccountID)
		if err != nil {
			return fmt.Errorf("finding gmail account %s: %w", payload.GmailAccountID, err)
		}
		if account == nil {
			return fmt.Errorf("gmail account %s not found", payload.GmailAccountID)
		}

		connAccount := &ingestionDomain.ConnectorAccount{
			ID:           account.ID(),
			EmailAddress: account.EmailAddress(),
			Provider:     "gmail",
		}
		rawMsg, err := ingestionMod.Connector.FetchMessage(ctx, connAccount, payload.GmailMessageID)
		if err != nil {
			logger.Warn("failed to fetch message detail from connector",
				"gmail_message_id", payload.GmailMessageID,
				"error", err.Error(),
			)
			_ = ingestionMod.Repo.UpdateMessageProcessingStatus(ctx, payload.MessageID, "failed")
			return fmt.Errorf("fetching message detail %s: %w", payload.GmailMessageID, err)
		}

		rawEmail := understandingDomain.RawEmailMessage{
			MessageID:      payload.MessageID,
			GmailAccountID: payload.GmailAccountID,
			Sender:         rawMsg.From,
			Subject:        rawMsg.Subject,
			Headers:        rawMsg.Headers,
			BodyHTML:       rawMsg.BodyHTML,
			BodyText:       rawMsg.BodyText,
			ReceivedAt:     rawMsg.InternalDate,
		}

		routeRes, err := understandingMod.ClassifyAndRouteCmd.Handle(ctx, understandingCmd.ClassifyAndRouteMessageCommand{
			MessageID:      payload.MessageID,
			GmailAccountID: payload.GmailAccountID,
			UserID:         account.UserID(),
			RawEmail:       rawEmail,
		})
		if err != nil {
			logger.Error("failed to classify and route message",
				"message_id", payload.MessageID,
				"error", err.Error(),
			)
			_ = ingestionMod.Repo.UpdateMessageProcessingStatus(ctx, payload.MessageID, "failed")
			return fmt.Errorf("classifying and routing message %s: %w", payload.MessageID, err)
		}

		logger.Info("routed ingested message successfully",
			"message_id", payload.MessageID,
			"route", routeRes.Route,
		)
		_ = ingestionMod.Repo.UpdateMessageProcessingStatus(ctx, payload.MessageID, "completed")
		return nil
	})

	understandingMod.RegisterWorkerTasks(mux)
	financialcoreMod.RegisterWorkerTasks(mux)
}
