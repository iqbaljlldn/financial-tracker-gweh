-- name: InsertPromptMetric :one
INSERT INTO administration.prompt_metrics (
    id, prompt_version, model_identifier, aggregate_confidence,
    schema_validation_outcome, later_overridden, latency_ms, token_usage, recorded_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
) RETURNING *;

-- name: ListRecentPromptMetrics :many
SELECT * FROM administration.prompt_metrics
ORDER BY recorded_at DESC
LIMIT $1;
