-- name: ListGmailAccountsScaffold :many
SELECT * FROM ingestion.gmail_accounts
WHERE user_id = $1;
