-- name: InsertFinancialCoreOutboxEvent :exec
INSERT INTO financialcore.outbox_events (
    id, aggregate_type, aggregate_id, event_type, payload, payload_schema_version, created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7);
