-- name: GetUser :one
SELECT id, name
FROM users
WHERE id = $1;

-- name: CreateUser :one
INSERT INTO users (name)
VALUES ($1)
RETURNING id, name;

-- name: GetUserInfoByAuthIdentity :one
SELECT
    u.id,
    ai.email,
    ai.provider,
    ai.provider_subject
FROM auth_identities ai
JOIN users u ON u.id = ai.user_id
WHERE ai.provider = $1
  AND ai.provider_subject = $2;

-- name: CreateAuthIdentity :one
INSERT INTO auth_identities (
    provider,
    provider_subject,
    user_id,
    email
) VALUES (
    $1,
    $2,
    $3,
    $4
)
RETURNING provider, provider_subject, user_id, email;
