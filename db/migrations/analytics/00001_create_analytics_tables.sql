-- +goose Up
CREATE SCHEMA IF NOT EXISTS analytics;

-- Table: analytics.cash_flow_view (08_database_design.md §11.1)
CREATE TABLE IF NOT EXISTS analytics.cash_flow_view (
    user_id UUID NOT NULL,
    period_bucket DATE NOT NULL,
    granularity TEXT NOT NULL CHECK (granularity IN ('daily', 'monthly')),
    total_income_value NUMERIC(18,4) NOT NULL DEFAULT 0.0000,
    total_expense_value NUMERIC(18,4) NOT NULL DEFAULT 0.0000,
    currency CHAR(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, period_bucket, granularity, currency)
);

CREATE INDEX IF NOT EXISTS idx_cash_flow_view_user_bucket ON analytics.cash_flow_view (user_id, period_bucket DESC);

-- Table: analytics.category_breakdown_view (08_database_design.md §11.2)
CREATE TABLE IF NOT EXISTS analytics.category_breakdown_view (
    user_id UUID NOT NULL,
    period_bucket DATE NOT NULL,
    category TEXT NOT NULL,
    total_value NUMERIC(18,4) NOT NULL DEFAULT 0.0000,
    currency CHAR(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    percentage_share NUMERIC(5,2) NOT NULL DEFAULT 0.00,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, period_bucket, category, currency)
);

CREATE INDEX IF NOT EXISTS idx_category_breakdown_user_bucket ON analytics.category_breakdown_view (user_id, period_bucket DESC);

-- Table: analytics.merchant_ranking_view (08_database_design.md §11.3)
CREATE TABLE IF NOT EXISTS analytics.merchant_ranking_view (
    user_id UUID NOT NULL,
    period_bucket DATE NOT NULL,
    merchant_normalized_name TEXT NOT NULL,
    total_value NUMERIC(18,4) NOT NULL DEFAULT 0.0000,
    transaction_count INTEGER NOT NULL DEFAULT 0,
    currency CHAR(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, period_bucket, merchant_normalized_name, currency)
);

CREATE INDEX IF NOT EXISTS idx_merchant_ranking_user_bucket ON analytics.merchant_ranking_view (user_id, period_bucket DESC);

-- Table: analytics.trend_view (08_database_design.md §11.4)
CREATE TABLE IF NOT EXISTS analytics.trend_view (
    user_id UUID NOT NULL,
    period_bucket DATE NOT NULL,
    granularity TEXT NOT NULL CHECK (granularity IN ('daily', 'monthly')),
    net_flow_value NUMERIC(18,4) NOT NULL DEFAULT 0.0000,
    currency CHAR(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, period_bucket, granularity, currency)
);

CREATE INDEX IF NOT EXISTS idx_trend_view_user_bucket ON analytics.trend_view (user_id, period_bucket DESC);

-- Table: analytics.projection_checkpoint (08_database_design.md §11.5)
CREATE TABLE IF NOT EXISTS analytics.projection_checkpoint (
    source_module TEXT PRIMARY KEY,
    last_processed_outbox_event_id UUID NOT NULL,
    checkpoint_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Role provisioning scoped to analytics schema (06_architecture.md §5.3, 08_database_design.md §3)
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'analytics_svc') THEN
        CREATE ROLE analytics_svc;
    END IF;
END $$;
-- +goose StatementEnd

GRANT USAGE ON SCHEMA analytics TO analytics_svc;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA analytics TO analytics_svc;
ALTER DEFAULT PRIVILEGES IN SCHEMA analytics GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO analytics_svc;

-- +goose Down
DROP TABLE IF EXISTS analytics.projection_checkpoint;
DROP TABLE IF EXISTS analytics.trend_view;
DROP TABLE IF EXISTS analytics.merchant_ranking_view;
DROP TABLE IF EXISTS analytics.category_breakdown_view;
DROP TABLE IF EXISTS analytics.cash_flow_view;
