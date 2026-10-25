package scheduler

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/ingestion/application/command"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/ingestion/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/observability"
)

// PollFallbackScheduler runs the H2-resolving safety net periodically per 12_gmail_connector.md §5.3.
// Independently sweeps all active Gmail accounts and triggers symmetric incremental sync [GATE].
type PollFallbackScheduler struct {
	accountRepo domain.GmailAccountRepository
	syncHandler *command.RunIncrementalSyncHandler
	interval    time.Duration
	logger      *slog.Logger
	stopChan    chan struct{}
	wg          sync.WaitGroup
}

// NewPollFallbackScheduler constructs a new PollFallbackScheduler.
func NewPollFallbackScheduler(
	accountRepo domain.GmailAccountRepository,
	syncHandler *command.RunIncrementalSyncHandler,
	interval time.Duration,
	logger *slog.Logger,
) *PollFallbackScheduler {
	if interval <= 0 {
		interval = 15 * time.Minute // Default 15 min per 12_gmail_connector.md §5.3
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &PollFallbackScheduler{
		accountRepo: accountRepo,
		syncHandler: syncHandler,
		interval:    interval,
		logger:      logger,
		stopChan:    make(chan struct{}),
	}
}

// Start launches the background scheduler loop.
func (s *PollFallbackScheduler) Start(ctx context.Context) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.logger.Info("starting poll-fallback safety net scheduler",
			"interval", s.interval.String(),
		)

		// Immediate initial heartbeat and sweep
		s.RunOnce(ctx)

		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		for {
			select {
			case <-s.stopChan:
				s.logger.Info("stopping poll-fallback safety net scheduler")
				return
			case <-ctx.Done():
				s.logger.Info("poll-fallback scheduler context cancelled")
				return
			case <-ticker.C:
				s.RunOnce(ctx)
			}
		}
	}()
}

// Stop signals the scheduler loop to stop and waits for completion.
func (s *PollFallbackScheduler) Stop() {
	close(s.stopChan)
	s.wg.Wait()
}

// RunOnce performs a single execution cycle across all active accounts and records the liveness heartbeat [GATE].
func (s *PollFallbackScheduler) RunOnce(ctx context.Context) {
	nowUnix := float64(time.Now().Unix())
	observability.GetMetrics().PollFallbackLiveness.Set(nowUnix) // [GATE: "monitor the monitor"]

	accounts, err := s.accountRepo.ListActive(ctx)
	if err != nil {
		s.logger.Error("poll-fallback: failed to list active accounts", "error", err)
		return
	}

	for _, acc := range accounts {
		if ctx.Err() != nil {
			return
		}

		cmd := command.RunIncrementalSyncCommand{
			AccountID: acc.ID(),
			Trigger:   "poll_fallback",
		}

		res, err := s.syncHandler.Handle(ctx, cmd)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "oauth token not found") {
				s.logger.Warn("poll-fallback sync skipped: no OAuth token found for account (unauthenticated or leftover test account)",
					slog.String("accountId", acc.ID()),
					slog.String("email", acc.EmailAddress()),
					slog.String("hint", "Complete Google OAuth flow to link this account, or remove stale test records from ingestion.gmail_accounts"),
				)
			} else {
				s.logger.Warn("poll-fallback sync failed for account",
					slog.String("accountId", acc.ID()),
					slog.String("email", acc.EmailAddress()),
					slog.String("error", err.Error()),
				)
			}
			continue
		}

		if res != nil && res.MessagesIngestedCount > 0 {
			s.logger.Info("poll-fallback successfully ingested new mail (H2 safety-net in action)",
				"accountId", acc.ID(),
				"ingestedCount", res.MessagesIngestedCount,
				"newHistoryId", res.NewHistoryID,
			)
		}
	}
}
