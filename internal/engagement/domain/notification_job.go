package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type NotificationType string

const (
	NotificationTypeDailySummary         NotificationType = "daily_summary"
	NotificationTypeWeeklySummary        NotificationType = "weekly_summary"
	NotificationTypeMonthlyReport        NotificationType = "monthly_report"
	NotificationTypeBudgetAlert          NotificationType = "budget_alert"
	NotificationTypeAiRecommendation     NotificationType = "ai_recommendation"
	NotificationTypeReviewRequest        NotificationType = "review_request"
	NotificationTypeTransactionFinalized NotificationType = "transaction_finalized"
)

func (t NotificationType) IsValid() bool {
	switch t {
	case NotificationTypeDailySummary,
		NotificationTypeWeeklySummary,
		NotificationTypeMonthlyReport,
		NotificationTypeBudgetAlert,
		NotificationTypeAiRecommendation,
		NotificationTypeReviewRequest,
		NotificationTypeTransactionFinalized:
		return true
	default:
		return false
	}
}

type DeliveryStatus string

const (
	DeliveryStatusPending   DeliveryStatus = "pending"
	DeliveryStatusDelivered DeliveryStatus = "delivered"
	DeliveryStatusFailed    DeliveryStatus = "failed"
	DeliveryStatusRetrying  DeliveryStatus = "retrying"
)

var (
	ErrInvalidNotificationType = errors.New("engagement: invalid notification type")
	ErrEmptyUserID             = errors.New("engagement: user id cannot be empty")
	ErrEmptyPayload            = errors.New("engagement: composed payload cannot be empty")
	ErrJobNotFound             = errors.New("engagement: notification job not found")
)

// NotificationJob represents a composed notification scheduled or delivered via WhatsApp.
type NotificationJob struct {
	id                  uuid.UUID
	userID              uuid.UUID
	notificationType    NotificationType
	composedPayload     []byte
	deliveryStatus      DeliveryStatus
	provider            string
	providerMessageID   string
	deliveryConfirmedAt *time.Time
	readAt              *time.Time
	createdAt           time.Time
}

// NewNotificationJob creates a new NotificationJob aggregate.
func NewNotificationJob(
	userID uuid.UUID,
	notificationType NotificationType,
	composedPayload []byte,
	provider string,
	createdAt time.Time,
) (*NotificationJob, error) {
	if userID == uuid.Nil {
		return nil, ErrEmptyUserID
	}
	if !notificationType.IsValid() {
		return nil, ErrInvalidNotificationType
	}
	if len(composedPayload) == 0 {
		return nil, ErrEmptyPayload
	}
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	if provider == "" {
		provider = "fonnte"
	}

	return &NotificationJob{
		id:               uuid.New(),
		userID:           userID,
		notificationType: notificationType,
		composedPayload:  composedPayload,
		deliveryStatus:   DeliveryStatusPending,
		provider:         provider,
		createdAt:        createdAt,
	}, nil
}

// ReconstituteJob recreates an existing job from repository persistence.
func ReconstituteJob(
	id uuid.UUID,
	userID uuid.UUID,
	notificationType NotificationType,
	composedPayload []byte,
	deliveryStatus DeliveryStatus,
	provider string,
	providerMessageID string,
	deliveryConfirmedAt *time.Time,
	readAt *time.Time,
	createdAt time.Time,
) *NotificationJob {
	return &NotificationJob{
		id:                  id,
		userID:              userID,
		notificationType:    notificationType,
		composedPayload:     composedPayload,
		deliveryStatus:      deliveryStatus,
		provider:            provider,
		providerMessageID:   providerMessageID,
		deliveryConfirmedAt: deliveryConfirmedAt,
		readAt:              readAt,
		createdAt:           createdAt,
	}
}

// ID returns the aggregate identifier.
func (j *NotificationJob) ID() uuid.UUID {
	return j.id
}

// UserID returns the recipient user ID.
func (j *NotificationJob) UserID() uuid.UUID {
	return j.userID
}

// NotificationType returns the notification category.
func (j *NotificationJob) NotificationType() NotificationType {
	return j.notificationType
}

// ComposedPayload returns the JSON rendered notification content.
func (j *NotificationJob) ComposedPayload() []byte {
	return j.composedPayload
}

// DeliveryStatus returns the current delivery state.
func (j *NotificationJob) DeliveryStatus() DeliveryStatus {
	return j.deliveryStatus
}

// Provider returns the messaging provider (e.g. fonnte, meta, mock).
func (j *NotificationJob) Provider() string {
	return j.provider
}

// ProviderMessageID returns the upstream WhatsApp message ID if provider accepted.
func (j *NotificationJob) ProviderMessageID() string {
	return j.providerMessageID
}

// DeliveryConfirmedAt returns the timestamp when delivery was confirmed.
func (j *NotificationJob) DeliveryConfirmedAt() *time.Time {
	return j.deliveryConfirmedAt
}

// ReadAt returns the timestamp when read receipt was confirmed.
func (j *NotificationJob) ReadAt() *time.Time {
	return j.readAt
}

// CreatedAt returns when the notification job was composed.
func (j *NotificationJob) CreatedAt() time.Time {
	return j.createdAt
}

// MarkProviderAccepted records provider acceptance and message ID.
func (j *NotificationJob) MarkProviderAccepted(providerMsgID string) {
	j.providerMessageID = providerMsgID
	j.deliveryStatus = DeliveryStatusPending
}

// MarkDelivered marks message as successfully delivered to user's device.
func (j *NotificationJob) MarkDelivered(at time.Time) {
	j.deliveryStatus = DeliveryStatusDelivered
	if at.IsZero() {
		at = time.Now().UTC()
	}
	j.deliveryConfirmedAt = &at
}

// MarkRead records that recipient has read the message.
func (j *NotificationJob) MarkRead(at time.Time) {
	if at.IsZero() {
		at = time.Now().UTC()
	}
	j.readAt = &at
}

// MarkRetrying sets status to retrying on transient provider failure.
func (j *NotificationJob) MarkRetrying() {
	j.deliveryStatus = DeliveryStatusRetrying
}

// MarkFailed sets status to failed on permanent provider failure.
func (j *NotificationJob) MarkFailed() {
	j.deliveryStatus = DeliveryStatusFailed
}
