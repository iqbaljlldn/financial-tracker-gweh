-- name: ListAuditLogScaffold :many
SELECT * FROM administration.audit_log
ORDER BY occurred_at DESC
LIMIT $1;
