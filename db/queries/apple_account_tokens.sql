-- name: GetAppleAccountTokenByUser :one
SELECT id, user_id, token, created_at, updated_at
FROM apple_account_tokens
WHERE user_id = $1;

-- name: GetAppleAccountTokenByToken :one
SELECT id, user_id, token, created_at, updated_at
FROM apple_account_tokens
WHERE token = $1;

-- name: InsertAppleAccountToken :one
INSERT INTO apple_account_tokens (
    user_id,
    token
) VALUES (
    $1,
    $2
)
RETURNING id, user_id, token, created_at, updated_at;
