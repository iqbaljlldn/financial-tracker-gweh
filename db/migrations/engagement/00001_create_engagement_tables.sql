-- +goose Up
CREATE SCHEMA IF NOT EXISTS engagement;

-- Table: engagement.notification_jobs (08_database_design.md §12.1)
CREATE TABLE IF NOT EXISTS engagement.notification_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    notification_type TEXT NOT NULL CHECK (notification_type IN (
        'daily_summary', 'weekly_summary', 'monthly_report',
        'budget_alert', 'ai_recommendation', 'review_request'
    )),
    composed_payload JSONB NOT NULL,
    delivery_status TEXT NOT NULL DEFAULT 'pending' CHECK (delivery_status IN (
        'pending', 'delivered', 'failed', 'retrying'
    )),
    provider_message_id TEXT,
    delivery_confirmed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_notification_jobs_user_id ON engagement.notification_jobs (user_id);
CREATE INDEX IF NOT EXISTS idx_notification_jobs_provider_message_id ON engagement.notification_jobs (provider_message_id);

-- Table: engagement.notification_preferences (08_database_design.md §12.2)
CREATE TABLE IF NOT EXISTS engagement.notification_preferences (
    user_id UUID PRIMARY KEY,
    daily_summary_enabled BOOLEAN NOT NULL DEFAULT true,
    weekly_summary_enabled BOOLEAN NOT NULL DEFAULT true,
    monthly_report_enabled BOOLEAN NOT NULL DEFAULT true,
    budget_alerts_enabled BOOLEAN NOT NULL DEFAULT true,
    ai_recommendations_enabled BOOLEAN NOT NULL DEFAULT false
);

-- Table: engagement.inbox_processed (08_database_design.md §5)
CREATE TABLE IF NOT EXISTS engagement.inbox_processed (
    task_id TEXT PRIMARY KEY,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    result_summary JSONB
);

-- Table: engagement.outbox_events (08_database_design.md §4)
CREATE TABLE IF NOT EXISTS engagement.outbox_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    aggregate_type TEXT NOT NULL,
    aggregate_id UUID NOT NULL,
    event_type TEXT NOT NULL,
    payload JSONB NOT NULL,
    payload_schema_version SMALLINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    relayed BOOLEAN NOT NULL DEFAULT false,
    relayed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_engagement_outbox_unrelayed ON engagement.outbox_events (created_at) WHERE relayed = false;

-- Role provisioning scoped to engagement schema (06_architecture.md §5.3, 08_database_design.md §3)
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'engagement_svc') THEN
        CREATE ROLE engagement_svc;
    END IF;
END $$;
-- +goose StatementEnd

GRANT USAGE ON SCHEMA engagement TO engagement_svc;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA engagement TO engagement_svc;
ALTER DEFAULT PRIVILEGES IN SCHEMA engagement GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO engagement_svc;

-- +goose Down
DROP TABLE IF EXISTS engagement.outbox_events;
DROP TABLE IF EXISTS engagement.inbox_processed;
DROP TABLE IF EXISTS engagement.notification_preferences;
DROP TABLE IF EXISTS engagement.notification_jobs;
