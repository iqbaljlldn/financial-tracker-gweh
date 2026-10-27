-- name: InsertAuditLog :one
INSERT INTO administration.audit_log (
    id, user_id, action, actor, metadata, occurred_at
) VALUES (
    $1, $2, $3, $4, $5, $6
) RETURNING *;

-- name: ListAuditLogs :many
SELECT id, user_id, action, actor, metadata, occurred_at
FROM administration.audit_log
WHERE (sqlc.narg('user_id')::uuid IS NULL OR user_id = sqlc.narg('user_id')::uuid)
  AND (sqlc.narg('action')::text IS NULL OR action = sqlc.narg('action')::text)
  AND (sqlc.narg('from_time')::timestamptz IS NULL OR occurred_at >= sqlc.narg('from_time')::timestamptz)
  AND (sqlc.narg('to_time')::timestamptz IS NULL OR occurred_at <= sqlc.narg('to_time')::timestamptz)
ORDER BY occurred_at DESC
LIMIT $1 OFFSET $2;

-- name: CountAuditLogs :one
SELECT COUNT(*)
FROM administration.audit_log
WHERE (sqlc.narg('user_id')::uuid IS NULL OR user_id = sqlc.narg('user_id')::uuid)
  AND (sqlc.narg('action')::text IS NULL OR action = sqlc.narg('action')::text)
  AND (sqlc.narg('from_time')::timestamptz IS NULL OR occurred_at >= sqlc.narg('from_time')::timestamptz)
  AND (sqlc.narg('to_time')::timestamptz IS NULL OR occurred_at <= sqlc.narg('to_time')::timestamptz);
