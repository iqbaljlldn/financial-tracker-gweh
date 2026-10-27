package command

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/engagement/domain"
)

// DeliverNotificationCommand encapsulates parameters for delivering a composed notification.
type DeliverNotificationCommand struct {
	JobID string `json:"jobId"`
}

// DeliverNotificationHandler executes delivery via the configured MessagingClient.
type DeliverNotificationHandler struct {
	jobRepo         domain.NotificationJobRepository
	prefRepo        domain.NotificationPreferenceRepository
	messagingClient domain.MessagingClient
	logger          *slog.Logger
}

// NewDeliverNotificationHandler constructs a new delivery handler.
func NewDeliverNotificationHandler(
	jobRepo domain.NotificationJobRepository,
	prefRepo domain.NotificationPreferenceRepository,
	messagingClient domain.MessagingClient,
	logger *slog.Logger,
) *DeliverNotificationHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &DeliverNotificationHandler{
		jobRepo:         jobRepo,
		prefRepo:        prefRepo,
		messagingClient: messagingClient,
		logger:          logger,
	}
}

// Handle executes the delivery command.
func (h *DeliverNotificationHandler) Handle(ctx context.Context, cmd DeliverNotificationCommand) error {
	jobID, err := uuid.Parse(cmd.JobID)
	if err != nil {
		return fmt.Errorf("delivery: invalid job id: %w", err)
	}

	job, err := h.jobRepo.FindByID(ctx, jobID)
	if err != nil {
		return fmt.Errorf("delivery: fetching job %s: %w", jobID, err)
	}

	if job.DeliveryStatus() == domain.DeliveryStatusDelivered {
		h.logger.InfoContext(ctx, "delivery: job already delivered, skipping", "jobId", jobID)
		return nil
	}

	var msg domain.OutboundMessage
	if err := json.Unmarshal(job.ComposedPayload(), &msg); err != nil {
		return fmt.Errorf("delivery: unmarshaling composed payload for job %s: %w", jobID, err)
	}

	// Deliver via MessagingClient
	res, err := h.messagingClient.SendMessage(ctx, msg)
	if err != nil {
		isPermanent := isPermanentFailure(err)
		if isPermanent {
			job.MarkFailed()
			_ = h.jobRepo.Update(ctx, job)
			h.logger.ErrorContext(ctx, "delivery: permanent failure",
				"jobId", jobID,
				"provider", h.messagingClient.ProviderName(),
				"error", err,
			)
			return nil // do not retry permanent failures
		}

		job.MarkRetrying()
		_ = h.jobRepo.Update(ctx, job)
		h.logger.WarnContext(ctx, "delivery: transient failure, will retry",
			"jobId", jobID,
			"provider", h.messagingClient.ProviderName(),
			"error", err,
		)
		return err // triggers Asynq retry with exponential backoff
	}

	job.MarkProviderAccepted(res.ProviderMessageID)
	// Some providers (like Fonnte when synchronous) already treat accepted as delivered
	now := time.Now().UTC()
	job.MarkDelivered(now)

	if err := h.jobRepo.Update(ctx, job); err != nil {
		return fmt.Errorf("delivery: updating job %s after delivery: %w", jobID, err)
	}

	h.logger.InfoContext(ctx, "delivery: message delivered successfully",
		"jobId", jobID,
		"provider", h.messagingClient.ProviderName(),
		"providerMsgId", res.ProviderMessageID,
	)
	return nil
}

func isPermanentFailure(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "blocked") ||
		strings.Contains(s, "invalid phone") ||
		strings.Contains(s, "not registered") ||
		strings.Contains(s, "permanent") ||
		strings.Contains(s, "unsubscribed")
}
