-- name: CreateOAuthToken :one
INSERT INTO identity.oauth_tokens (
    id,
    gmail_account_id,
    encrypted_access_token,
    encrypted_refresh_token,
    access_token_expires_at,
    scope,
    created_at,
    updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
) RETURNING *;

-- name: GetOAuthTokenByGmailAccountID :one
SELECT * FROM identity.oauth_tokens
WHERE gmail_account_id = $1;

-- name: UpdateOAuthToken :one
UPDATE identity.oauth_tokens
SET encrypted_access_token = $2,
    encrypted_refresh_token = $3,
    access_token_expires_at = $4,
    updated_at = $5
WHERE id = $1
RETURNING *;
