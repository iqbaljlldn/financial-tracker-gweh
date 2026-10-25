-- +goose Up
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";
CREATE EXTENSION IF NOT EXISTS "citext";

CREATE SCHEMA IF NOT EXISTS administration;

-- Table: administration.audit_log (08_database_design.md §13.1, 16_security.md §8)
-- Append-only tamper-resistant audit trail. Exempt from GDPR erasure cascades.
CREATE TABLE IF NOT EXISTS administration.audit_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID,
    action TEXT NOT NULL,
    actor TEXT NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}',
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_audit_log_user_occurred ON administration.audit_log (user_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_log_action_occurred ON administration.audit_log (action, occurred_at DESC);

-- Table: administration.connector_health_metrics (08_database_design.md §13.2)
CREATE TABLE IF NOT EXISTS administration.connector_health_metrics (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    connector_type TEXT NOT NULL,
    connector_account_id UUID NOT NULL,
    status TEXT NOT NULL,
    error_count INT NOT NULL DEFAULT 0,
    latency_ms INT NOT NULL DEFAULT 0,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_connector_health_recorded ON administration.connector_health_metrics (connector_account_id, recorded_at DESC);

-- Table: administration.parser_metrics (08_database_design.md §13.2)
CREATE TABLE IF NOT EXISTS administration.parser_metrics (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    sender_pattern TEXT NOT NULL,
    template_version INT NOT NULL,
    total_parsed INT NOT NULL DEFAULT 0,
    failed_parsed INT NOT NULL DEFAULT 0,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Table: administration.prompt_metrics (08_database_design.md §13.2, 11_ai_blueprint.md §10)
CREATE TABLE IF NOT EXISTS administration.prompt_metrics (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    prompt_version TEXT NOT NULL,
    model_identifier TEXT NOT NULL,
    aggregate_confidence NUMERIC(4,3) NOT NULL,
    schema_validation_outcome TEXT NOT NULL,
    later_overridden BOOLEAN NOT NULL DEFAULT false,
    latency_ms INT NOT NULL,
    token_usage INT NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_prompt_metrics_version ON administration.prompt_metrics (prompt_version, recorded_at DESC);

-- GATE ENFORCEMENT (08_database_design.md §13.1, 16_security.md §8.2):
-- Provision administration_svc role with explicit SELECT and INSERT grants only.
-- DELETE and UPDATE are withheld to guarantee tamper resistance.
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'administration_svc') THEN
        CREATE ROLE administration_svc;
    END IF;
END $$;
-- +goose StatementEnd

GRANT USAGE ON SCHEMA administration TO administration_svc;
GRANT SELECT, INSERT ON administration.audit_log TO administration_svc;
GRANT SELECT, INSERT, UPDATE ON administration.connector_health_metrics TO administration_svc;
GRANT SELECT, INSERT, UPDATE ON administration.parser_metrics TO administration_svc;
GRANT SELECT, INSERT, UPDATE ON administration.prompt_metrics TO administration_svc;

REVOKE UPDATE, DELETE, TRUNCATE ON administration.audit_log FROM administration_svc;

-- +goose Down
DROP TABLE IF EXISTS administration.prompt_metrics;
DROP TABLE IF EXISTS administration.parser_metrics;
DROP TABLE IF EXISTS administration.connector_health_metrics;
DROP TABLE IF EXISTS administration.audit_log;
