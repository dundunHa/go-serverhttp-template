-- name: GetSubscriptionByOriginalTx :one
SELECT *
FROM apple_subscriptions
WHERE original_transaction_id = $1
  AND environment = $2;

-- name: ListSubscriptionsForUserEntitlement :many
SELECT *
FROM apple_subscriptions
WHERE user_id = $1
  AND environment = ANY($2::text[])
ORDER BY
    CASE WHEN status IN ('ACTIVE', 'CANCELED')
              AND COALESCE(grace_period_expires_at, current_period_end) > now()
         THEN 0 ELSE 1 END,
    level DESC,
    COALESCE(grace_period_expires_at, current_period_end) DESC,
    last_event_at DESC;

-- name: UpsertSubscription :one
INSERT INTO apple_subscriptions (
    user_id,
    app_account_token,
    environment,
    original_transaction_id,
    last_transaction_id,
    web_order_line_item_id,
    plan_id,
    provider_product_id,
    subscription_group_id,
    level,
    status,
    auto_renew_status,
    current_period_start,
    current_period_end,
    grace_period_expires_at,
    last_event_at,
    last_notification_created_at,
    last_payload_hash,
    last_transaction_snapshot
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
    $11, $12, $13, $14, $15, $16, $17, $18, $19
)
ON CONFLICT (original_transaction_id, environment) DO UPDATE SET
    last_transaction_id          = EXCLUDED.last_transaction_id,
    web_order_line_item_id       = EXCLUDED.web_order_line_item_id,
    plan_id                      = EXCLUDED.plan_id,
    provider_product_id          = EXCLUDED.provider_product_id,
    subscription_group_id        = EXCLUDED.subscription_group_id,
    level                        = EXCLUDED.level,
    status                       = EXCLUDED.status,
    auto_renew_status            = EXCLUDED.auto_renew_status,
    current_period_start         = EXCLUDED.current_period_start,
    current_period_end           = EXCLUDED.current_period_end,
    grace_period_expires_at      = EXCLUDED.grace_period_expires_at,
    last_event_at                = EXCLUDED.last_event_at,
    last_notification_created_at = EXCLUDED.last_notification_created_at,
    last_payload_hash            = EXCLUDED.last_payload_hash,
    last_transaction_snapshot    = EXCLUDED.last_transaction_snapshot,
    updated_at                   = now()
RETURNING *;
