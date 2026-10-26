package command

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/financialcore/domain"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/platform/observability"
)

const (
	// DefaultPendingReviewSLA is 7 days per 17_observability.md §8 and sprint checklist.
	DefaultPendingReviewSLA = 7 * 24 * time.Hour
)

// CheckPendingReviewSLAHandler monitors the pending-review backlog and tracks SLA breaches.
type CheckPendingReviewSLAHandler struct {
	monitor domain.PendingReviewMonitor
	sla     time.Duration
	logger  *slog.Logger
}

// NewCheckPendingReviewSLAHandler constructs the SLA monitor handler.
func NewCheckPendingReviewSLAHandler(
	monitor domain.PendingReviewMonitor,
	sla time.Duration,
	logger *slog.Logger,
) *CheckPendingReviewSLAHandler {
	if sla <= 0 {
		sla = DefaultPendingReviewSLA
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &CheckPendingReviewSLAHandler{
		monitor: monitor,
		sla:     sla,
		logger:  logger,
	}
}

// Handle inspects the SLA boundary window and refreshes the pending gauge.
func (h *CheckPendingReviewSLAHandler) Handle(ctx context.Context, cmd CheckPendingReviewSLACommand) error {
	now := cmd.Now
	if now.IsZero() {
		now = time.Now()
	}

	// Breach window: events that crossed the SLA threshold during the last check interval (1 hour window)
	windowEnd := now.Add(-h.sla)
	windowStart := windowEnd.Add(-1 * time.Hour)

	breached, err := h.monitor.CountPendingReviewCrossedSLA(ctx, windowStart, windowEnd)
	if err != nil {
		return fmt.Errorf("checking pending review SLA breaches: %w", err)
	}

	if breached > 0 {
		observability.GetMetrics().FinancialCorePendingReviewSLABreachTotal.Add(float64(breached))
		h.logger.WarnContext(ctx, "financialcore pending review SLA breach detected",
			"breachedCount", breached,
			"slaDuration", h.sla.String(),
			"windowStart", windowStart.Format(time.RFC3339),
			"windowEnd", windowEnd.Format(time.RFC3339),
		)
	}

	totalPending, err := h.monitor.CountPendingReview(ctx)
	if err != nil {
		return fmt.Errorf("counting total pending review events: %w", err)
	}

	observability.GetMetrics().FinancialCoreEventsPendingReview.Set(float64(totalPending))
	return nil
}
