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
		runAPIStub(ctx, logger)
	case modeWorker:
		runWorkerStub(ctx, logger)
	}

	logger.Info("financial-event-platform shut down cleanly", slog.String("mode", mode))
}

func runAPIStub(ctx context.Context, logger *slog.Logger) {
	logger.Info("API mode stub initialized (ready for HTTP router wiring)")
	<-ctx.Done()
	logger.Info("received termination signal in API mode")
}

func runWorkerStub(ctx context.Context, logger *slog.Logger) {
	logger.Info("Worker mode stub initialized (ready for Asynq task server wiring)")
	<-ctx.Done()
	logger.Info("received termination signal in Worker mode")
}
