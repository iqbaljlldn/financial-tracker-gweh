-- name: InsertFinancialCoreOutboxEvent :exec
INSERT INTO financialcore.outbox_events (
    id, aggregate_type, aggregate_id, event_type, payload, payload_schema_version, created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: ListUnrelayedFinancialCoreOutbox :many
SELECT id, aggregate_type, aggregate_id, event_type, payload, created_at
FROM financialcore.outbox_events
WHERE relayed = false
ORDER BY created_at ASC
LIMIT $1;

-- name: MarkFinancialCoreOutboxRelayed :exec
UPDATE financialcore.outbox_events
SET relayed = true, relayed_at = now()
WHERE id = ANY(@ids::uuid[]);
