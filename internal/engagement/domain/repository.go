package domain

import (
	"context"

	"github.com/google/uuid"
)

// NotificationJobRepository abstracts persistence of notification jobs.
type NotificationJobRepository interface {
	Save(ctx context.Context, job *NotificationJob) error
	Update(ctx context.Context, job *NotificationJob) error
	FindByID(ctx context.Context, id uuid.UUID) (*NotificationJob, error)
	FindByProviderMessageID(ctx context.Context, providerMsgID string) (*NotificationJob, error)
	FindLatestPendingReviewByUserID(ctx context.Context, userID uuid.UUID) (*NotificationJob, error)
	ListByUserID(ctx context.Context, userID uuid.UUID, limit, offset int32) ([]*NotificationJob, error)
}

// NotificationPreferenceRepository abstracts preference persistence.
type NotificationPreferenceRepository interface {
	GetByUserID(ctx context.Context, userID uuid.UUID) (*NotificationPreferences, error)
	SavePreferences(ctx context.Context, pref *NotificationPreferences) error
	FindByPhoneNumber(ctx context.Context, phone string) (*NotificationPreferences, error)
	ListUsersForSchedule(ctx context.Context, notificationType NotificationType) ([]UserScheduleTarget, error)
}

// UserScheduleTarget captures user id and phone for scheduled broadcast.
type UserScheduleTarget struct {
	UserID      uuid.UUID
	PhoneNumber string
}

// OutboxRepository abstracts saving events to engagement.outbox_events.
type OutboxRepository interface {
	SaveOutboxEvent(ctx context.Context, aggregateType string, aggregateID uuid.UUID, eventType string, payload any) error
}

// InboxRepository abstracts idempotency checking for incoming background tasks.
type InboxRepository interface {
	IsProcessed(ctx context.Context, taskID string) (bool, error)
	MarkProcessed(ctx context.Context, taskID string, summary any) error
}
