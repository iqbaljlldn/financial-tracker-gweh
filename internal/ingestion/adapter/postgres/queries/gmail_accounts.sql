-- name: CreateGmailAccount :exec
INSERT INTO ingestion.gmail_accounts (
    id, user_id, email_address, status, initial_sync_window_days, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
);

-- name: UpdateGmailAccount :exec
UPDATE ingestion.gmail_accounts
SET status = $2,
    last_history_id = $3,
    watch_expiration = $4,
    updated_at = $5
WHERE id = $1;

-- name: FindGmailAccountByID :one
SELECT * FROM ingestion.gmail_accounts
WHERE id = $1;

-- name: FindGmailAccountByEmail :one
SELECT * FROM ingestion.gmail_accounts
WHERE email_address = $1;

-- name: ListGmailAccountsByUserID :many
SELECT * FROM ingestion.gmail_accounts
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: ListActiveGmailAccountsForWatchRenewal :many
SELECT * FROM ingestion.gmail_accounts
WHERE status = 'active'
  AND (watch_expiration IS NULL OR watch_expiration <= $1)
LIMIT $2;
