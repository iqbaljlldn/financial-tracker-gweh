-- +goose Up
-- Enable necessary extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";
CREATE EXTENSION IF NOT EXISTS "citext";

CREATE SCHEMA IF NOT EXISTS identity;

-- Table: identity.users (08_database_design.md §6.1)
CREATE TABLE IF NOT EXISTS identity.users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email CITEXT UNIQUE NOT NULL,
    display_name TEXT,
    timezone TEXT NOT NULL DEFAULT 'UTC',
    notification_daily_time TIME,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Table: identity.oauth_tokens (08_database_design.md §6.2)
-- Stores envelope-encrypted credentials. No cross-schema FK to ingestion.gmail_accounts per 06_architecture.md §5.2.
CREATE TABLE IF NOT EXISTS identity.oauth_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    gmail_account_id UUID NOT NULL,
    encrypted_access_token BYTEA NOT NULL,
    encrypted_refresh_token BYTEA NOT NULL,
    access_token_expires_at TIMESTAMPTZ NOT NULL,
    scope TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_oauth_tokens_gmail_account_id ON identity.oauth_tokens (gmail_account_id);

-- Table: identity.refresh_tokens (16_security.md §5.1)
-- Stores database-backed, revocable platform session refresh tokens.
CREATE TABLE IF NOT EXISTS identity.refresh_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES identity.users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    revoked BOOLEAN NOT NULL DEFAULT false,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user_id ON identity.refresh_tokens (user_id);

-- Table: identity.outbox_events (08_database_design.md §4)
-- Reference implementation of Transactional Outbox pattern.
CREATE TABLE IF NOT EXISTS identity.outbox_events (
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

CREATE INDEX IF NOT EXISTS idx_identity_outbox_unrelayed ON identity.outbox_events (created_at) WHERE relayed = false;

-- Role provisioning scoped to identity schema (06_architecture.md §5.3, 08_database_design.md §3)
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'identity_svc') THEN
        CREATE ROLE identity_svc;
    END IF;
END $$;
-- +goose StatementEnd

GRANT USAGE ON SCHEMA identity TO identity_svc;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA identity TO identity_svc;
ALTER DEFAULT PRIVILEGES IN SCHEMA identity GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO identity_svc;

-- +goose Down
DROP TABLE IF EXISTS identity.outbox_events;
DROP TABLE IF EXISTS identity.refresh_tokens;
DROP TABLE IF EXISTS identity.oauth_tokens;
DROP TABLE IF EXISTS identity.users;
