-- name: InsertConnectorHealth :one
INSERT INTO administration.connector_health_metrics (
    id, connector_type, connector_account_id, status, error_count, latency_ms, recorded_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
) RETURNING *;

-- name: ListLatestConnectorHealth :many
SELECT DISTINCT ON (connector_account_id)
    id, connector_type, connector_account_id, status, error_count, latency_ms, recorded_at
FROM administration.connector_health_metrics
ORDER BY connector_account_id, recorded_at DESC;

-- name: ListRecentConnectorHealth :many
SELECT id, connector_type, connector_account_id, status, error_count, latency_ms, recorded_at
FROM administration.connector_health_metrics
ORDER BY recorded_at DESC
LIMIT $1;
