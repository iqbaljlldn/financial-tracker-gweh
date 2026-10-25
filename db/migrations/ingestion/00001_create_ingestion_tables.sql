-- +goose Up
CREATE EXTENSION IF NOT EXISTS "citext";
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE SCHEMA IF NOT EXISTS ingestion;

-- Table: ingestion.gmail_accounts (08_database_design.md §8.1)
CREATE TABLE IF NOT EXISTS ingestion.gmail_accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    email_address CITEXT UNIQUE NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('linking', 'initial_sync', 'active', 'disconnected', 'error')),
    last_history_id TEXT,
    watch_expiration TIMESTAMPTZ,
    initial_sync_window_days SMALLINT NOT NULL DEFAULT 90,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_gmail_accounts_user_id ON ingestion.gmail_accounts (user_id);

-- Table: ingestion.ingested_messages (08_database_design.md §8.2)
CREATE TABLE IF NOT EXISTS ingestion.ingested_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    gmail_account_id UUID NOT NULL REFERENCES ingestion.gmail_accounts(id) ON DELETE CASCADE,
    gmail_message_id TEXT NOT NULL,
    received_at TIMESTAMPTZ NOT NULL,
    raw_content_ref TEXT,
    routing_decision TEXT CHECK (routing_decision IN ('template_parser', 'ai_extractor', 'discarded')),
    processing_status TEXT NOT NULL DEFAULT 'queued' CHECK (processing_status IN ('queued', 'processing', 'completed', 'failed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_ingested_messages UNIQUE (gmail_account_id, gmail_message_id)
);

CREATE INDEX IF NOT EXISTS idx_ingested_messages_status ON ingestion.ingested_messages (processing_status)
WHERE processing_status IN ('queued', 'failed');

-- Table: ingestion.sender_watch_log (08_database_design.md §8.3)
CREATE TABLE IF NOT EXISTS ingestion.sender_watch_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    gmail_account_id UUID NOT NULL REFERENCES ingestion.gmail_accounts(id) ON DELETE CASCADE,
    event TEXT NOT NULL CHECK (event IN ('registered', 'renewed', 'renewal_failed', 'expired')),
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Table: ingestion.outbox_events (08_database_design.md §4)
CREATE TABLE IF NOT EXISTS ingestion.outbox_events (
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

CREATE INDEX IF NOT EXISTS idx_ingestion_outbox_unrelayed ON ingestion.outbox_events (created_at) WHERE relayed = false;

-- Role provisioning scoped to ingestion schema (06_architecture.md §5.3, 08_database_design.md §3)
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'ingestion_svc') THEN
        CREATE ROLE ingestion_svc;
    END IF;
END $$;
-- +goose StatementEnd

GRANT USAGE ON SCHEMA ingestion TO ingestion_svc;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA ingestion TO ingestion_svc;
ALTER DEFAULT PRIVILEGES IN SCHEMA ingestion GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO ingestion_svc;

-- +goose Down
DROP TABLE IF EXISTS ingestion.outbox_events;
DROP TABLE IF EXISTS ingestion.sender_watch_log;
DROP TABLE IF EXISTS ingestion.ingested_messages;
DROP TABLE IF EXISTS ingestion.gmail_accounts;
