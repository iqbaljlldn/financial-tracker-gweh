-- name: GetActiveTemplateByPattern :one
SELECT id, sender_pattern, version, extraction_rules, active, created_at
FROM understanding.sender_templates
WHERE sender_pattern = $1 AND active = true
LIMIT 1;

-- name: ListActiveTemplates :many
SELECT id, sender_pattern, version, extraction_rules, active, created_at
FROM understanding.sender_templates
WHERE active = true
ORDER BY sender_pattern ASC;

-- name: GetTemplateByID :one
SELECT id, sender_pattern, version, extraction_rules, active, created_at
FROM understanding.sender_templates
WHERE id = $1 LIMIT 1;

-- name: GetLatestVersionForPattern :one
SELECT COALESCE(MAX(version), 0)::int as max_version
FROM understanding.sender_templates
WHERE sender_pattern = $1;

-- name: DeactivateTemplatesForPattern :exec
UPDATE understanding.sender_templates
SET active = false
WHERE sender_pattern = $1 AND active = true;

-- name: InsertTemplate :one
INSERT INTO understanding.sender_templates (
    id, sender_pattern, version, extraction_rules, active, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6
) RETURNING *;
