-- name: InsertNotificationJob :one
INSERT INTO engagement.notification_jobs (
    id, user_id, notification_type, composed_payload, delivery_status, provider, provider_message_id, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING *;

-- name: GetNotificationJobByID :one
SELECT * FROM engagement.notification_jobs
WHERE id = $1;

-- name: GetNotificationJobByProviderMessageID :one
SELECT * FROM engagement.notification_jobs
WHERE provider_message_id = $1;

-- name: FindLatestPendingReviewJobByUserID :one
SELECT * FROM engagement.notification_jobs
WHERE user_id = $1 AND notification_type = 'review_request'
ORDER BY created_at DESC
LIMIT 1;

-- name: UpdateNotificationJobStatus :exec
UPDATE engagement.notification_jobs
SET delivery_status = $2,
    provider_message_id = COALESCE($3, provider_message_id)
WHERE id = $1;

-- name: UpdateNotificationJobDelivered :exec
UPDATE engagement.notification_jobs
SET delivery_status = 'delivered',
    delivery_confirmed_at = $2
WHERE id = $1;

-- name: UpdateNotificationJobRead :exec
UPDATE engagement.notification_jobs
SET read_at = $2
WHERE id = $1;

-- name: ListNotificationJobsByUserID :many
SELECT * FROM engagement.notification_jobs
WHERE user_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: GetNotificationPreferences :one
SELECT * FROM engagement.notification_preferences
WHERE user_id = $1;

-- name: UpsertNotificationPreferences :one
INSERT INTO engagement.notification_preferences (
    user_id, phone_number, daily_summary_enabled, weekly_summary_enabled,
    monthly_report_enabled, budget_alerts_enabled, ai_recommendations_enabled
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
)
ON CONFLICT (user_id) DO UPDATE SET
    phone_number = COALESCE(EXCLUDED.phone_number, engagement.notification_preferences.phone_number),
    daily_summary_enabled = EXCLUDED.daily_summary_enabled,
    weekly_summary_enabled = EXCLUDED.weekly_summary_enabled,
    monthly_report_enabled = EXCLUDED.monthly_report_enabled,
    budget_alerts_enabled = EXCLUDED.budget_alerts_enabled,
    ai_recommendations_enabled = EXCLUDED.ai_recommendations_enabled
RETURNING *;

-- name: FindUserPreferencesByPhoneNumber :one
SELECT * FROM engagement.notification_preferences
WHERE phone_number = $1
LIMIT 1;

-- name: ListUsersForScheduledNotification :many
SELECT np.user_id, np.phone_number
FROM engagement.notification_preferences np
WHERE (
    CASE 
        WHEN $1::text = 'daily_summary' THEN np.daily_summary_enabled
        WHEN $1::text = 'weekly_summary' THEN np.weekly_summary_enabled
        WHEN $1::text = 'monthly_report' THEN np.monthly_report_enabled
        ELSE false
    END
) = true
AND np.phone_number IS NOT NULL AND np.phone_number != '';

-- name: InsertEngagementOutbox :exec
INSERT INTO engagement.outbox_events (
    id, aggregate_type, aggregate_id, event_type, payload, payload_schema_version, created_at, relayed
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, false
);

-- name: ListUnrelayedEngagementOutbox :many
SELECT * FROM engagement.outbox_events
WHERE relayed = false
ORDER BY created_at ASC
LIMIT $1;

-- name: MarkEngagementOutboxRelayed :exec
UPDATE engagement.outbox_events
SET relayed = true, relayed_at = now()
WHERE id = ANY($1::uuid[]);

-- name: IsEngagementInboxProcessed :one
SELECT EXISTS (
    SELECT 1 FROM engagement.inbox_processed
    WHERE task_id = $1
);

-- name: RecordEngagementInboxProcessed :exec
INSERT INTO engagement.inbox_processed (task_id, processed_at, result_summary)
VALUES ($1, now(), $2)
ON CONFLICT (task_id) DO NOTHING;
