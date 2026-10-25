-- name: InsertExtractionResult :one
INSERT INTO understanding.extraction_results (
    id, ingested_message_id, method, template_version, prompt_version, model_identifier,
    candidate_payload, confidence_aggregate, confidence_per_field, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
) RETURNING *;

-- name: ListRecentExtractionResults :many
SELECT id, ingested_message_id, method, template_version, prompt_version, model_identifier,
       candidate_payload, confidence_aggregate, confidence_per_field, created_at
FROM understanding.extraction_results
ORDER BY created_at DESC
LIMIT $1;

-- name: GetTemplateStatsInWindow :one
SELECT
    COUNT(*)::int AS total_count,
    COUNT(CASE WHEN confidence_aggregate < 0.90 THEN 1 END)::int AS failed_count
FROM understanding.extraction_results
WHERE method = 'template'
  AND template_version = $1
  AND created_at >= $2;
