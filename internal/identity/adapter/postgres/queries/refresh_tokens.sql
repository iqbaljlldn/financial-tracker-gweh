-- name: CreateRefreshToken :one
INSERT INTO identity.refresh_tokens (
    id,
    user_id,
    token_hash,
    revoked,
    expires_at,
    created_at
) VALUES (
    $1, $2, $3, false, $4, $5
) RETURNING *;

-- name: GetValidRefreshTokenByHash :one
SELECT * FROM identity.refresh_tokens
WHERE token_hash = $1
  AND revoked = false
  AND expires_at > now();

-- name: RevokeRefreshToken :one
UPDATE identity.refresh_tokens
SET revoked = true
WHERE user_id = $1 AND token_hash = $2
RETURNING *;

-- name: RevokeAllUserRefreshTokens :exec
UPDATE identity.refresh_tokens
SET revoked = true
WHERE user_id = $1;
