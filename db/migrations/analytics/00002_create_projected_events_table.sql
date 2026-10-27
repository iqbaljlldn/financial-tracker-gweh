-- +goose Up
-- Table: analytics.projected_events
-- Internal projection tracking table ensuring idempotent updates, override application,
-- and archive subtractions without any cross-schema reads to financialcore (06_architecture.md §7).
CREATE TABLE IF NOT EXISTS analytics.projected_events (
    event_id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    event_type TEXT NOT NULL,
    amount_value NUMERIC(18,4) NOT NULL,
    currency CHAR(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    occurred_at TIMESTAMPTZ NOT NULL,
    merchant_normalized_name TEXT NOT NULL,
    category TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived')),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_projected_events_user_occurred ON analytics.projected_events (user_id, occurred_at DESC);

GRANT SELECT, INSERT, UPDATE, DELETE ON analytics.projected_events TO analytics_svc;

-- +goose Down
DROP TABLE IF EXISTS analytics.projected_events;
