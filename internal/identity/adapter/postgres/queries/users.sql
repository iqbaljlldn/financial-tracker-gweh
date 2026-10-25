-- name: CreateUser :one
INSERT INTO identity.users (
    id,
    email,
    display_name,
    timezone,
    notification_daily_time,
    created_at,
    updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
) RETURNING *;

-- name: GetUserByID :one
SELECT * FROM identity.users
WHERE id = $1;

-- name: GetUserByEmail :one
SELECT * FROM identity.users
WHERE email = $1;

-- name: UpdateUserSettings :one
UPDATE identity.users
SET display_name = $2,
    timezone = $3,
    notification_daily_time = $4,
    updated_at = $5
WHERE id = $1
RETURNING *;
