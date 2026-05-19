-- name: InsertAppleEventIfNotExists :one
INSERT INTO apple_events (
    notification_uuid,
    notification_type,
    subtype,
    environment,
    user_id,
    app_account_token,
    original_transaction_id,
    transaction_id,
    web_order_line_item_id,
    processing_status,
    processing_error,
    raw_jws_sha256,
    decoded_payload,
    notification_created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
)
ON CONFLICT (notification_uuid) DO NOTHING
RETURNING id;

-- name: GetAppleEventByUUID :one
SELECT *
FROM apple_events
WHERE notification_uuid = $1;

-- name: ListPendingAppleEvents :many
SELECT *
FROM apple_events
WHERE processing_status = $1
ORDER BY created_at ASC
LIMIT $2;
