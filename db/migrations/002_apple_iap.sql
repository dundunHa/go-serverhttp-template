-- Migration: 002_apple_iap
-- Purpose: Add Apple In-App Purchase subscription marker tables.
--   * apple_account_tokens: maps user_id <-> server-generated UUID used as Apple `appAccountToken`.
--   * apple_subscriptions:  current state of an auto-renewable subscription per (original_transaction_id, environment).
--   * apple_events:         idempotent audit log of App Store Server Notification V2 events.
-- Idempotent: uses CREATE TABLE / CREATE INDEX IF NOT EXISTS so re-running this migration is safe.

CREATE TABLE IF NOT EXISTS apple_account_tokens (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id),
    UNIQUE (token)
);

CREATE TABLE IF NOT EXISTS apple_subscriptions (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    app_account_token UUID NOT NULL REFERENCES apple_account_tokens(token),
    environment TEXT NOT NULL,
    original_transaction_id TEXT NOT NULL,
    last_transaction_id TEXT NOT NULL DEFAULT '',
    web_order_line_item_id TEXT NOT NULL DEFAULT '',
    plan_id TEXT NOT NULL,
    provider_product_id TEXT NOT NULL,
    subscription_group_id TEXT NOT NULL DEFAULT '',
    level INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'ACTIVE',
    auto_renew_status TEXT NOT NULL DEFAULT 'UNKNOWN',
    current_period_start TIMESTAMPTZ NOT NULL,
    current_period_end TIMESTAMPTZ NOT NULL,
    grace_period_expires_at TIMESTAMPTZ,
    last_event_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_notification_created_at TIMESTAMPTZ,
    last_payload_hash TEXT NOT NULL DEFAULT '',
    last_transaction_snapshot JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (original_transaction_id, environment)
);

CREATE INDEX IF NOT EXISTS apple_subscriptions_user_env_idx
    ON apple_subscriptions(user_id, environment);

CREATE INDEX IF NOT EXISTS apple_subscriptions_user_entitlement_idx
    ON apple_subscriptions(user_id, level DESC, current_period_end DESC)
    WHERE status IN ('ACTIVE', 'CANCELED');

CREATE INDEX IF NOT EXISTS apple_subscriptions_status_period_idx
    ON apple_subscriptions(status, current_period_end);

CREATE TABLE IF NOT EXISTS apple_events (
    id BIGSERIAL PRIMARY KEY,
    notification_uuid TEXT NOT NULL UNIQUE,
    notification_type TEXT NOT NULL,
    subtype TEXT NOT NULL DEFAULT '',
    environment TEXT NOT NULL,
    user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    app_account_token UUID,
    original_transaction_id TEXT NOT NULL DEFAULT '',
    transaction_id TEXT NOT NULL DEFAULT '',
    web_order_line_item_id TEXT NOT NULL DEFAULT '',
    processing_status TEXT NOT NULL DEFAULT 'PROCESSED',
    processing_error TEXT NOT NULL DEFAULT '',
    raw_jws_sha256 TEXT NOT NULL,
    decoded_payload JSONB,
    notification_created_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS apple_events_transaction_id_idx
    ON apple_events(transaction_id);

CREATE INDEX IF NOT EXISTS apple_events_original_tx_idx
    ON apple_events(original_transaction_id, environment);

CREATE INDEX IF NOT EXISTS apple_events_status_idx
    ON apple_events(processing_status, created_at);
