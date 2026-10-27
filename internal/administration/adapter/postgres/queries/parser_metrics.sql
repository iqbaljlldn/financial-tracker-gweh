-- name: InsertParserMetric :one
INSERT INTO administration.parser_metrics (
    id, sender_pattern, template_version, total_parsed, failed_parsed, recorded_at
) VALUES (
    $1, $2, $3, $4, $5, $6
) RETURNING *;

-- name: ListRecentParserMetrics :many
SELECT id, sender_pattern, template_version, total_parsed, failed_parsed, recorded_at
FROM administration.parser_metrics
ORDER BY recorded_at DESC
LIMIT $1;

-- name: GetAggregatedParserMetrics :many
SELECT sender_pattern,
       template_version,
       SUM(total_parsed)::bigint AS total_parsed,
       SUM(failed_parsed)::bigint AS failed_parsed,
       MAX(recorded_at) AS last_recorded_at
FROM administration.parser_metrics
GROUP BY sender_pattern, template_version
ORDER BY sender_pattern, template_version;
