-- name: CreateIngestionOutboxEvent :exec
INSERT INTO ingestion.outbox_events (
    id, aggregate_type, aggregate_id, event_type, payload, payload_schema_version, created_at, relayed
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
);

-- name: FetchUnrelayedOutboxEvents :many
SELECT * FROM ingestion.outbox_events
WHERE relayed = false
ORDER BY created_at ASC
LIMIT $1
FOR UPDATE SKIP LOCKED;

-- name: MarkOutboxEventsRelayed :exec
UPDATE ingestion.outbox_events
SET relayed = true,
    relayed_at = $2
WHERE id = ANY($1::uuid[]);
