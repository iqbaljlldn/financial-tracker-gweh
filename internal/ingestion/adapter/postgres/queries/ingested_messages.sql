-- name: CreateIngestedMessage :exec
INSERT INTO ingestion.ingested_messages (
    id, gmail_account_id, gmail_message_id, received_at, raw_content_ref, routing_decision, processing_status, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
)
ON CONFLICT (gmail_account_id, gmail_message_id) DO NOTHING;

-- name: CreateIngestedMessageReturning :one
INSERT INTO ingestion.ingested_messages (
    id, gmail_account_id, gmail_message_id, received_at, raw_content_ref, routing_decision, processing_status, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
)
ON CONFLICT (gmail_account_id, gmail_message_id) DO NOTHING
RETURNING id;

-- name: FindIngestedMessageByAccountAndMessageID :one
SELECT * FROM ingestion.ingested_messages
WHERE gmail_account_id = $1 AND gmail_message_id = $2;

-- name: CheckIngestedMessageExists :one
SELECT EXISTS(
    SELECT 1 FROM ingestion.ingested_messages
    WHERE gmail_account_id = $1 AND gmail_message_id = $2
);

-- name: CountIngestedMessagesByAccount :one
SELECT count(*) FROM ingestion.ingested_messages
WHERE gmail_account_id = $1;
