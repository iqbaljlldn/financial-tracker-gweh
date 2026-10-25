-- +goose Up
CREATE SCHEMA IF NOT EXISTS budget;

-- Table: budget.budgets (08_database_design.md §10.1)
CREATE TABLE IF NOT EXISTS budget.budgets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    scope TEXT NOT NULL,
    period_type TEXT NOT NULL DEFAULT 'monthly',
    threshold_amount_value NUMERIC(18,4) NOT NULL,
    threshold_amount_currency CHAR(3) NOT NULL CHECK (threshold_amount_currency ~ '^[A-Z]{3}$'),
    alert_thresholds NUMERIC(3,2)[] NOT NULL DEFAULT '{0.80,1.00}',
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_budgets_user_id ON budget.budgets (user_id);

-- Table: budget_fired_alerts (08_database_design.md §10.2)
CREATE TABLE IF NOT EXISTS budget.budget_fired_alerts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    budget_id UUID NOT NULL REFERENCES budget.budgets(id) ON DELETE CASCADE,
    period_start DATE NOT NULL,
    threshold NUMERIC(3,2) NOT NULL,
    fired_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_budget_fired_alerts UNIQUE (budget_id, period_start, threshold)
);

CREATE INDEX IF NOT EXISTS idx_budget_fired_alerts_budget_id ON budget.budget_fired_alerts (budget_id);

-- Table: savings_goals (08_database_design.md §10.3)
CREATE TABLE IF NOT EXISTS budget.savings_goals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    target_amount_value NUMERIC(18,4) NOT NULL,
    target_amount_currency CHAR(3) NOT NULL CHECK (target_amount_currency ~ '^[A-Z]{3}$'),
    target_date DATE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_savings_goals_user_id ON budget.savings_goals (user_id);

-- Table: budget.outbox_events (08_database_design.md §4)
CREATE TABLE IF NOT EXISTS budget.outbox_events (
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

CREATE INDEX IF NOT EXISTS idx_budget_outbox_unrelayed ON budget.outbox_events (created_at) WHERE relayed = false;

-- Role provisioning scoped to budget schema (06_architecture.md §5.3, 08_database_design.md §3)
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'budget_svc') THEN
        CREATE ROLE budget_svc;
    END IF;
END $$;
-- +goose StatementEnd

GRANT USAGE ON SCHEMA budget TO budget_svc;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA budget TO budget_svc;
ALTER DEFAULT PRIVILEGES IN SCHEMA budget GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO budget_svc;

-- +goose Down
DROP TABLE IF EXISTS budget.outbox_events;
DROP TABLE IF EXISTS budget.savings_goals;
DROP TABLE IF EXISTS budget.budget_fired_alerts;
DROP TABLE IF EXISTS budget.budgets;
