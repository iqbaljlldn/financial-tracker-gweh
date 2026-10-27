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

-- name: GetPromptMetricsSummary :many
SELECT prompt_version,
       model_identifier,
       COUNT(*)::bigint AS total_calls,
       AVG(aggregate_confidence)::numeric(4,3) AS avg_confidence,
       COUNT(CASE WHEN schema_validation_outcome = 'valid' THEN 1 END)::bigint AS valid_count,
       COUNT(CASE WHEN schema_validation_outcome <> 'valid' THEN 1 END)::bigint AS invalid_count,
       COUNT(CASE WHEN later_overridden THEN 1 END)::bigint AS overridden_count,
       AVG(latency_ms)::numeric(10,2) AS avg_latency_ms,
       SUM(token_usage)::bigint AS total_tokens
FROM administration.prompt_metrics
GROUP BY prompt_version, model_identifier
ORDER BY prompt_version DESC;
