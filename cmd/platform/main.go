// Package main is the composition root for the Financial Event Platform.
// It provides a single, dual-mode binary running as either an HTTP API server
// or an asynchronous worker consumer per 06_architecture.md §11 and 21_project_structure.md §3.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/identity"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/config"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/httpserver"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/security"
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
		runWorkerStub(ctx, logger)
	}

	logger.Info("financial-event-platform shut down cleanly", slog.String("mode", mode))
}

func loadAppConfig(logger *slog.Logger) *config.Config {
	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "config/config.yaml"
	}
	cfg, err := config.Load(cfgPath)
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

	port := cfg.Platform.HTTP.Port
	if port <= 0 {
		port = 8080
	}

	server := httpserver.NewServer(httpserver.Config{
		Port:         port,
		ReadTimeout:  cfg.Platform.HTTP.ReadTimeout,
		WriteTimeout: cfg.Platform.HTTP.WriteTimeout,
	}, logger)

	identityMod.RegisterHTTPRoutes(server.Router())

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

func runWorkerStub(ctx context.Context, logger *slog.Logger) {
	logger.Info("Worker mode stub initialized (ready for Asynq task server wiring)")
	<-ctx.Done()
	logger.Info("received termination signal in Worker mode")
}
