package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iqbaljlldn/financial-tracker-gweh/internal/engagement/adapter/postgres/gen"
	"github.com/iqbaljlldn/financial-tracker-gweh/internal/engagement/domain"
)

// Repository implements engagement domain repositories backed by PostgreSQL.
type Repository struct {
	pool    *pgxpool.Pool
	queries *gen.Queries
}

// NewRepository constructs an engagement repository instance.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{
		pool:    pool,
		queries: gen.New(pool),
	}
}

// Ensure interface implementations.
var (
	_ domain.NotificationJobRepository        = (*Repository)(nil)
	_ domain.NotificationPreferenceRepository = (*Repository)(nil)
	_ domain.OutboxRepository                 = (*Repository)(nil)
	_ domain.InboxRepository                  = (*Repository)(nil)
)

// Save persists a new NotificationJob.
func (r *Repository) Save(ctx context.Context, job *domain.NotificationJob) error {
	var providerMsgID pgtype.Text
	if job.ProviderMessageID() != "" {
		providerMsgID = pgtype.Text{String: job.ProviderMessageID(), Valid: true}
	}

	_, err := r.queries.InsertNotificationJob(ctx, gen.InsertNotificationJobParams{
		ID:                uuidToPgtype(job.ID()),
		UserID:            uuidToPgtype(job.UserID()),
		NotificationType:  string(job.NotificationType()),
		ComposedPayload:   job.ComposedPayload(),
		DeliveryStatus:    string(job.DeliveryStatus()),
		Provider:          pgtype.Text{String: job.Provider(), Valid: true},
		ProviderMessageID: providerMsgID,
		CreatedAt:         timeToPgtype(job.CreatedAt()),
	})
	return err
}

// Update saves state updates (delivery status, timestamps) to an existing NotificationJob.
func (r *Repository) Update(ctx context.Context, job *domain.NotificationJob) error {
	var providerMsgID pgtype.Text
	if job.ProviderMessageID() != "" {
		providerMsgID = pgtype.Text{String: job.ProviderMessageID(), Valid: true}
	}

	err := r.queries.UpdateNotificationJobStatus(ctx, gen.UpdateNotificationJobStatusParams{
		ID:                uuidToPgtype(job.ID()),
		DeliveryStatus:    string(job.DeliveryStatus()),
		ProviderMessageID: providerMsgID,
	})
	if err != nil {
		return err
	}

	if job.DeliveryConfirmedAt() != nil {
		err = r.queries.UpdateNotificationJobDelivered(ctx, gen.UpdateNotificationJobDeliveredParams{
			ID:                  uuidToPgtype(job.ID()),
			DeliveryConfirmedAt: timeToPgtype(*job.DeliveryConfirmedAt()),
		})
		if err != nil {
			return err
		}
	}

	if job.ReadAt() != nil {
		err = r.queries.UpdateNotificationJobRead(ctx, gen.UpdateNotificationJobReadParams{
			ID:     uuidToPgtype(job.ID()),
			ReadAt: timeToPgtype(*job.ReadAt()),
		})
		if err != nil {
			return err
		}
	}

	return nil
}

// FindByID finds a notification job by ID.
func (r *Repository) FindByID(ctx context.Context, id uuid.UUID) (*domain.NotificationJob, error) {
	row, err := r.queries.GetNotificationJobByID(ctx, uuidToPgtype(id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrJobNotFound
		}
		return nil, err
	}
	return toDomainJob(row), nil
}

// FindByProviderMessageID finds a notification job by external provider message ID.
func (r *Repository) FindByProviderMessageID(ctx context.Context, providerMsgID string) (*domain.NotificationJob, error) {
	row, err := r.queries.GetNotificationJobByProviderMessageID(ctx, pgtype.Text{String: providerMsgID, Valid: true})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrJobNotFound
		}
		return nil, err
	}
	return toDomainJob(row), nil
}

// FindLatestPendingReviewByUserID retrieves the most recent review_request notification for a user.
func (r *Repository) FindLatestPendingReviewByUserID(ctx context.Context, userID uuid.UUID) (*domain.NotificationJob, error) {
	row, err := r.queries.FindLatestPendingReviewJobByUserID(ctx, uuidToPgtype(userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrJobNotFound
		}
		return nil, err
	}
	return toDomainJob(row), nil
}

// ListByUserID retrieves paginated notification jobs for a user.
func (r *Repository) ListByUserID(ctx context.Context, userID uuid.UUID, limit, offset int32) ([]*domain.NotificationJob, error) {
	rows, err := r.queries.ListNotificationJobsByUserID(ctx, gen.ListNotificationJobsByUserIDParams{
		UserID: uuidToPgtype(userID),
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return nil, err
	}

	result := make([]*domain.NotificationJob, len(rows))
	for i, row := range rows {
		result[i] = toDomainJob(row)
	}
	return result, nil
}

// GetByUserID retrieves notification preferences for a user, returning default preferences if none exist.
func (r *Repository) GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.NotificationPreferences, error) {
	row, err := r.queries.GetNotificationPreferences(ctx, uuidToPgtype(userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.DefaultPreferences(userID), nil
		}
		return nil, err
	}
	return toDomainPreferences(row), nil
}

// SavePreferences upserts notification preferences.
func (r *Repository) SavePreferences(ctx context.Context, pref *domain.NotificationPreferences) error {
	var phone pgtype.Text
	if pref.PhoneNumber != "" {
		phone = pgtype.Text{String: pref.PhoneNumber, Valid: true}
	}

	_, err := r.queries.UpsertNotificationPreferences(ctx, gen.UpsertNotificationPreferencesParams{
		UserID:                   uuidToPgtype(pref.UserID),
		PhoneNumber:              phone,
		DailySummaryEnabled:      pref.DailySummaryEnabled,
		WeeklySummaryEnabled:     pref.WeeklySummaryEnabled,
		MonthlyReportEnabled:     pref.MonthlyReportEnabled,
		BudgetAlertsEnabled:      pref.BudgetAlertsEnabled,
		AiRecommendationsEnabled: pref.AiRecommendationsEnabled,
	})
	return err
}

// FindByPhoneNumber finds user preferences associated with a phone number (for inbound reply routing).
func (r *Repository) FindByPhoneNumber(ctx context.Context, phone string) (*domain.NotificationPreferences, error) {
	row, err := r.queries.FindUserPreferencesByPhoneNumber(ctx, pgtype.Text{String: phone, Valid: true})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return toDomainPreferences(row), nil
}

// ListUsersForSchedule returns all users eligible to receive a scheduled summary.
func (r *Repository) ListUsersForSchedule(ctx context.Context, notificationType domain.NotificationType) ([]domain.UserScheduleTarget, error) {
	rows, err := r.queries.ListUsersForScheduledNotification(ctx, string(notificationType))
	if err != nil {
		return nil, err
	}

	targets := make([]domain.UserScheduleTarget, len(rows))
	for i, row := range rows {
		targets[i] = domain.UserScheduleTarget{
			UserID:      uuid.UUID(row.UserID.Bytes),
			PhoneNumber: row.PhoneNumber.String,
		}
	}
	return targets, nil
}

// SaveOutboxEvent records an event in engagement.outbox_events.
func (r *Repository) SaveOutboxEvent(ctx context.Context, aggregateType string, aggregateID uuid.UUID, eventType string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	return r.queries.InsertEngagementOutbox(ctx, gen.InsertEngagementOutboxParams{
		ID:                   uuidToPgtype(uuid.New()),
		AggregateType:        aggregateType,
		AggregateID:          uuidToPgtype(aggregateID),
		EventType:            eventType,
		Payload:              raw,
		PayloadSchemaVersion: 1,
		CreatedAt:            timeToPgtype(time.Now().UTC()),
	})
}

// IsProcessed checks whether an incoming background task ID has already been executed.
func (r *Repository) IsProcessed(ctx context.Context, taskID string) (bool, error) {
	return r.queries.IsEngagementInboxProcessed(ctx, taskID)
}

// MarkProcessed marks a background task as processed in engagement.inbox_processed.
func (r *Repository) MarkProcessed(ctx context.Context, taskID string, summary any) error {
	var summaryBytes []byte
	if summary != nil {
		summaryBytes, _ = json.Marshal(summary)
	}

	return r.queries.RecordEngagementInboxProcessed(ctx, gen.RecordEngagementInboxProcessedParams{
		TaskID:        taskID,
		ResultSummary: summaryBytes,
	})
}

// Helper conversions
func uuidToPgtype(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: id != uuid.Nil}
}

func timeToPgtype(t time.Time) pgtype.Timestamptz {
	if t.IsZero() {
		return pgtype.Timestamptz{Valid: false}
	}
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func toDomainJob(row gen.EngagementNotificationJob) *domain.NotificationJob {
	var deliveryConfirmedAt *time.Time
	if row.DeliveryConfirmedAt.Valid {
		deliveryConfirmedAt = &row.DeliveryConfirmedAt.Time
	}

	var readAt *time.Time
	if row.ReadAt.Valid {
		readAt = &row.ReadAt.Time
	}

	provider := "fonnte"
	if row.Provider.Valid && row.Provider.String != "" {
		provider = row.Provider.String
	}

	return domain.ReconstituteJob(
		uuid.UUID(row.ID.Bytes),
		uuid.UUID(row.UserID.Bytes),
		domain.NotificationType(row.NotificationType),
		row.ComposedPayload,
		domain.DeliveryStatus(row.DeliveryStatus),
		provider,
		row.ProviderMessageID.String,
		deliveryConfirmedAt,
		readAt,
		row.CreatedAt.Time,
	)
}

func toDomainPreferences(row gen.EngagementNotificationPreference) *domain.NotificationPreferences {
	return &domain.NotificationPreferences{
		UserID:                   uuid.UUID(row.UserID.Bytes),
		PhoneNumber:              row.PhoneNumber.String,
		DailySummaryEnabled:      row.DailySummaryEnabled,
		WeeklySummaryEnabled:     row.WeeklySummaryEnabled,
		MonthlyReportEnabled:     row.MonthlyReportEnabled,
		BudgetAlertsEnabled:      row.BudgetAlertsEnabled,
		AiRecommendationsEnabled: row.AiRecommendationsEnabled,
	}
}
