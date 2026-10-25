-- +goose Up
CREATE SCHEMA IF NOT EXISTS financialcore;

-- Table: financialcore.financial_events (08_database_design.md §7.1)
-- Canonical ledger aggregate root.
CREATE TABLE IF NOT EXISTS financialcore.financial_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    event_type TEXT NOT NULL CHECK (event_type IN (
        'Income', 'Expense', 'Transfer', 'Refund', 'Cashback',
        'SubscriptionRenewal', 'FailedPayment', 'Salary', 'Investment',
        'Dividend', 'Loan', 'CreditCardBill', 'Tax', 'Donation', 'Others'
    )),
    amount_value NUMERIC(18,4) NOT NULL,
    amount_currency CHAR(3) NOT NULL CHECK (amount_currency ~ '^[A-Z]{3}$'),
    occurred_at TIMESTAMPTZ NOT NULL,
    merchant_raw_name TEXT,
    merchant_normalized_name TEXT,
    payment_method_type TEXT,
    payment_method_hint TEXT,
    status TEXT NOT NULL DEFAULT 'pending_review' CHECK (status IN (
        'pending_review', 'finalized', 'archived', 'disputed'
    )),
    confidence_aggregate NUMERIC(4,3) NOT NULL CHECK (confidence_aggregate BETWEEN 0 AND 1),
    confidence_method TEXT NOT NULL CHECK (confidence_method IN (
        'template_deterministic', 'ai_weighted', 'manual'
    )),
    type_specific_payload JSONB NOT NULL DEFAULT '{}',
    provenance_connector_type TEXT NOT NULL,
    provenance_connector_account_id UUID NOT NULL,
    provenance_source_message_ref TEXT NOT NULL,
    provenance_extraction_method TEXT NOT NULL CHECK (provenance_extraction_method IN (
        'template', 'ai', 'manual'
    )),
    provenance_template_version TEXT,
    provenance_prompt_version TEXT,
    provenance_model_identifier TEXT,
    provenance_extracted_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Invariant INV-3: Dedup uniqueness constraint per source message (04_domain_model.md §5.7)
    CONSTRAINT uq_financial_events_provenance UNIQUE (provenance_connector_account_id, provenance_source_message_ref)
);

CREATE INDEX IF NOT EXISTS idx_financial_events_user_occurred ON financialcore.financial_events (user_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_financial_events_user_status ON financialcore.financial_events (user_id, status);
CREATE INDEX IF NOT EXISTS idx_financial_events_merchant ON financialcore.financial_events (user_id, merchant_normalized_name);
CREATE INDEX IF NOT EXISTS idx_financial_events_payload_gin ON financialcore.financial_events USING GIN (type_specific_payload);

-- Table: financialcore.event_overrides (08_database_design.md §7.2)
CREATE TABLE IF NOT EXISTS financialcore.event_overrides (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    financial_event_id UUID NOT NULL REFERENCES financialcore.financial_events(id) ON DELETE CASCADE,
    field_name TEXT NOT NULL,
    previous_value JSONB NOT NULL,
    new_value JSONB NOT NULL,
    actor_user_id UUID NOT NULL,
    corrected_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_event_overrides_financial_event_id ON financialcore.event_overrides (financial_event_id);

-- Table: financialcore.category_assignments (08_database_design.md §7.3)
CREATE TABLE IF NOT EXISTS financialcore.category_assignments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    financial_event_id UUID NOT NULL REFERENCES financialcore.financial_events(id) ON DELETE CASCADE,
    category TEXT NOT NULL,
    confidence NUMERIC(4,3) NOT NULL,
    source TEXT NOT NULL CHECK (source IN ('user_history', 'global_mapping', 'ai_inference', 'manual')),
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    superseded_by UUID REFERENCES financialcore.category_assignments(id),
    is_current BOOLEAN NOT NULL DEFAULT true
);

CREATE INDEX IF NOT EXISTS idx_category_assignments_current ON financialcore.category_assignments (financial_event_id)
WHERE is_current = true;

-- Table: financialcore.inbox_processed (08_database_design.md §5 & §7.4)
CREATE TABLE IF NOT EXISTS financialcore.inbox_processed (
    task_id TEXT PRIMARY KEY,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    result_summary JSONB
);

-- Table: financialcore.outbox_events (08_database_design.md §4)
CREATE TABLE IF NOT EXISTS financialcore.outbox_events (
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

CREATE INDEX IF NOT EXISTS idx_financialcore_outbox_unrelayed ON financialcore.outbox_events (created_at) WHERE relayed = false;

-- Role provisioning scoped to financialcore schema (06_architecture.md §5.3, 08_database_design.md §3)
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'financialcore_svc') THEN
        CREATE ROLE financialcore_svc;
    END IF;
END $$;
-- +goose StatementEnd

GRANT USAGE ON SCHEMA financialcore TO financialcore_svc;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA financialcore TO financialcore_svc;
ALTER DEFAULT PRIVILEGES IN SCHEMA financialcore GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO financialcore_svc;

-- +goose Down
DROP TABLE IF EXISTS financialcore.outbox_events;
DROP TABLE IF EXISTS financialcore.inbox_processed;
DROP TABLE IF EXISTS financialcore.category_assignments;
DROP TABLE IF EXISTS financialcore.event_overrides;
DROP TABLE IF EXISTS financialcore.financial_events;
