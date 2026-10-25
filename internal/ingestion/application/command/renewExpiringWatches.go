package command

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/ingestion/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/observability"
)

// RenewExpiringWatchesCommand sweeps active Gmail accounts whose watch leases are within 24h of expiry.
// Per 12_gmail_connector.md §5.2, scheduled renewal fires at <= 6 days from registration (1-day safety margin) [GATE].
type RenewExpiringWatchesCommand struct {
	SafetyMargin time.Duration
	BatchLimit   int32
}

// RenewExpiringWatchesResult summarizes renewal outcomes.
type RenewExpiringWatchesResult struct {
	AccountsExamined int
	RenewedCount     int
	FailedCount      int
}

// RenewExpiringWatchesHandler executes periodic watch renewal.
type RenewExpiringWatchesHandler struct {
	accountRepo   domain.GmailAccountRepository
	registerWatch *RegisterGmailWatchHandler
	logger        *slog.Logger
}

// NewRenewExpiringWatchesHandler constructs a RenewExpiringWatchesHandler.
func NewRenewExpiringWatchesHandler(
	accountRepo domain.GmailAccountRepository,
	registerWatch *RegisterGmailWatchHandler,
	logger *slog.Logger,
) *RenewExpiringWatchesHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &RenewExpiringWatchesHandler{
		accountRepo:   accountRepo,
		registerWatch: registerWatch,
		logger:        logger,
	}
}

// Handle executes the scheduled renewal task.
func (h *RenewExpiringWatchesHandler) Handle(ctx context.Context, cmd RenewExpiringWatchesCommand) (*RenewExpiringWatchesResult, error) {
	safetyMargin := cmd.SafetyMargin
	if safetyMargin <= 0 {
		safetyMargin = 24 * time.Hour // 1-day safety margin (fires at <= 6 days of 7-day Gmail lease) [GATE]
	}

	limit := cmd.BatchLimit
	if limit <= 0 {
		limit = 100
	}

	expiryThreshold := time.Now().UTC().Add(safetyMargin)
	accounts, err := h.accountRepo.ListActiveForWatchRenewal(ctx, expiryThreshold, limit)
	if err != nil {
		return nil, fmt.Errorf("list accounts for watch renewal: %w", err)
	}

	observability.GetMetrics().WatchExpiringSoon.Set(float64(len(accounts)))

	result := &RenewExpiringWatchesResult{
		AccountsExamined: len(accounts),
	}

	for _, acc := range accounts {
		watchRes, err := h.registerWatch.Handle(ctx, RegisterGmailWatchCommand{AccountID: acc.ID()})
		if err != nil || (watchRes != nil && watchRes.PollOnlyMode) {
			result.FailedCount++
			if err != nil && strings.Contains(strings.ToLower(err.Error()), "oauth token not found") {
				h.logger.Warn("watch renewal skipped: account has no active OAuth credentials",
					slog.String("accountId", acc.ID()),
					slog.String("email", acc.EmailAddress()),
					slog.String("hint", "Complete Google OAuth flow or remove stale account"),
				)
			} else {
				h.logger.Warn("renewal failed for account, proceeding with batch",
					slog.String("accountId", acc.ID()),
					slog.Any("error", err),
				)
			}
			continue
		}
		result.RenewedCount++
	}

	h.logger.Info("watch renewal sweep completed",
		"examined", result.AccountsExamined,
		"renewed", result.RenewedCount,
		"failed", result.FailedCount,
	)

	return result, nil
}
