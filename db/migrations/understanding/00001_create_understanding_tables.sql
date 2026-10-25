-- +goose Up
CREATE SCHEMA IF NOT EXISTS understanding;

-- Table: understanding.sender_templates (08_database_design.md §9.1)
CREATE TABLE IF NOT EXISTS understanding.sender_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    sender_pattern TEXT NOT NULL,
    version INTEGER NOT NULL,
    extraction_rules JSONB NOT NULL,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_sender_templates UNIQUE (sender_pattern, version)
);

CREATE INDEX IF NOT EXISTS idx_sender_templates_active ON understanding.sender_templates (active)
WHERE active = true;

-- Table: understanding.extraction_results (08_database_design.md §9.2)
CREATE TABLE IF NOT EXISTS understanding.extraction_results (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ingested_message_id UUID NOT NULL,
    method TEXT NOT NULL CHECK (method IN ('template', 'ai')),
    template_version TEXT,
    prompt_version TEXT,
    model_identifier TEXT,
    candidate_payload JSONB NOT NULL,
    confidence_aggregate NUMERIC(4,3) NOT NULL,
    confidence_per_field JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_extraction_results_ingested_message_id ON understanding.extraction_results (ingested_message_id);

-- Table: understanding.outbox_events (08_database_design.md §4)
CREATE TABLE IF NOT EXISTS understanding.outbox_events (
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

CREATE INDEX IF NOT EXISTS idx_understanding_outbox_unrelayed ON understanding.outbox_events (created_at) WHERE relayed = false;

-- Role provisioning scoped to understanding schema (06_architecture.md §5.3, 08_database_design.md §3)
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'understanding_svc') THEN
        CREATE ROLE understanding_svc;
    END IF;
END $$;
-- +goose StatementEnd

GRANT USAGE ON SCHEMA understanding TO understanding_svc;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA understanding TO understanding_svc;
ALTER DEFAULT PRIVILEGES IN SCHEMA understanding GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO understanding_svc;

-- +goose Down
DROP TABLE IF EXISTS understanding.outbox_events;
DROP TABLE IF EXISTS understanding.extraction_results;
DROP TABLE IF EXISTS understanding.sender_templates;
