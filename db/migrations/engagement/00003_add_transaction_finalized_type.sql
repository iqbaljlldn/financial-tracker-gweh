-- +goose Up
ALTER TABLE engagement.notification_jobs DROP CONSTRAINT IF EXISTS notification_jobs_notification_type_check;
ALTER TABLE engagement.notification_jobs ADD CONSTRAINT notification_jobs_notification_type_check CHECK (notification_type IN (
    'daily_summary', 'weekly_summary', 'monthly_report',
    'budget_alert', 'ai_recommendation', 'review_request',
    'transaction_finalized'
));

-- +goose Down
ALTER TABLE engagement.notification_jobs DROP CONSTRAINT IF EXISTS notification_jobs_notification_type_check;
ALTER TABLE engagement.notification_jobs ADD CONSTRAINT notification_jobs_notification_type_check CHECK (notification_type IN (
    'daily_summary', 'weekly_summary', 'monthly_report',
    'budget_alert', 'ai_recommendation', 'review_request'
));
