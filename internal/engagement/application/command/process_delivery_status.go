package command

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/engagement/domain"
)

// DeliveryStatusCommand encapsulates provider status webhook payload.
type DeliveryStatusCommand struct {
	ProviderMessageID string    `json:"providerMessageId"`
	Status            string    `json:"status"` // "sent", "delivered", "read", "failed"
	Timestamp         time.Time `json:"timestamp"`
}

// ProcessDeliveryStatusHandler updates two-tier delivery confirmation per Diagram 7.
type ProcessDeliveryStatusHandler struct {
	jobRepo domain.NotificationJobRepository
	logger  *slog.Logger
}

// NewProcessDeliveryStatusHandler constructs a delivery status handler.
func NewProcessDeliveryStatusHandler(
	jobRepo domain.NotificationJobRepository,
	logger *slog.Logger,
) *ProcessDeliveryStatusHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &ProcessDeliveryStatusHandler{
		jobRepo: jobRepo,
		logger:  logger,
	}
}

// Handle updates delivery tier on notification job.
// Read tier is strictly for observability, never used as a functional gate [GATE].
func (h *ProcessDeliveryStatusHandler) Handle(ctx context.Context, cmd DeliveryStatusCommand) error {
	if cmd.ProviderMessageID == "" {
		return fmt.Errorf("status: provider message id is required")
	}

	job, err := h.jobRepo.FindByProviderMessageID(ctx, cmd.ProviderMessageID)
	if err != nil {
		h.logger.WarnContext(ctx, "status: notification job not found for provider message id",
			"providerMessageId", cmd.ProviderMessageID,
			"status", cmd.Status,
		)
		return nil // don't fail webhook on unknown provider IDs
	}

	ts := cmd.Timestamp
	if ts.IsZero() {
		ts = time.Now().UTC()
	}

	statusLower := strings.ToLower(strings.TrimSpace(cmd.Status))
	switch statusLower {
	case "delivered":
		job.MarkDelivered(ts)
	case "read":
		job.MarkRead(ts)
	case "failed":
		job.MarkFailed()
	}

	if err := h.jobRepo.Update(ctx, job); err != nil {
		return fmt.Errorf("status: updating job %s: %w", job.ID(), err)
	}

	h.logger.InfoContext(ctx, "updated notification delivery status",
		"jobId", job.ID(),
		"status", statusLower,
	)
	return nil
}
