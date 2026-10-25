-- name: InsertUnderstandingOutboxEvent :one
INSERT INTO understanding.outbox_events (
    id, aggregate_type, aggregate_id, event_type, payload, payload_schema_version, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
) RETURNING *;

-- name: ListUnrelayedUnderstandingOutboxEvents :many
SELECT id, aggregate_type, aggregate_id, event_type, payload, payload_schema_version, created_at
FROM understanding.outbox_events
WHERE relayed = false
ORDER BY created_at ASC
LIMIT $1;

-- name: MarkUnderstandingOutboxRelayed :exec
UPDATE understanding.outbox_events
SET relayed = true, relayed_at = $2
WHERE id = $1;
