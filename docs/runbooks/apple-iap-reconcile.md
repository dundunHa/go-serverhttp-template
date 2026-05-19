# Apple IAP Reconcile Runbook

This runbook covers replaying Apple App Store Server Notifications V2 events
into the local subscription store when webhooks were missed or failed.

## When to use

Run reconciliation when any of the following is true:

- The webhook endpoint was down or returning 5xx during a known window
  (`/webhooks/apple` returned 401/500 spike in logs).
- A user reports a renewal that did not appear in `/users/me`.
- `apple_events.processing_status` shows `PERMANENT_FAILURE` or
  `PENDING_USER_BINDING` for events that should now resolve.

## Pre-requisites

The same Apple IAP environment variables that power production (`APPLE_IAP_*`).
The reconcile binary signs requests to the App Store Server API with the
production-only key, so reconcile must run in an environment with that key
material available (KMS, secret store, or local `.p8` for dev).

```bash
export DB_DSN="postgres://..."
export APPLE_IAP_BUNDLE_ID="..."
export APPLE_IAP_ISSUER_ID="..."
export APPLE_IAP_KEY_ID="..."
export APPLE_IAP_PRIVATE_KEY="$(cat /secure/path/AuthKey_xxx.p8)"
export APPLE_IAP_PRODUCTS='[{"plan_id":"pro_monthly","product_id":"com.app.pro.monthly","level":1,"environment":"Production"}]'
export APPLE_IAP_ENTITLEMENT_ENVIRONMENTS="Production"
```

## Single window replay

```bash
go run ./cmd/apple-iap-reconcile \
  --from "2026-05-19T00:00:00Z" \
  --to   "2026-05-20T00:00:00Z"
```

The CLI prints a structured log line on completion. Capture
`next_page_token` from the JSON output - if it is non-empty, more results
are available for the same window.

## Pagination

Apple returns up to 20 events per call. To drain a window completely,
loop on the token until it is empty:

```bash
TOKEN=""
while true; do
  OUT=$(go run ./cmd/apple-iap-reconcile \
    --from "$FROM" --to "$TO" --page-token "$TOKEN" 2>&1)
  echo "$OUT"
  TOKEN=$(echo "$OUT" | jq -r '.next_page_token // empty')
  [ -z "$TOKEN" ] && break
done
```

## Targeted replay for one user

If a single transaction is in dispute, replay only its history:

```bash
go run ./cmd/apple-iap-reconcile \
  --original-transaction-id "200000123456789"
```

## Idempotency

Every replayed event hits the same reducer as the live webhook. The
`apple_events.notification_uuid` UNIQUE constraint short-circuits
duplicates, so it is safe to over-replay a window.

If you see `replayed=0 failed=N`, inspect `apple_events.processing_error`
for the first few `PERMANENT_FAILURE` rows - the most common causes are
catalog drift (`product not in catalog`) or a missing user binding from
a sandbox account that was not yet provisioned via
`GET /payment/apple/account-token`.

## What this does NOT do

- It does not invoke the legacy `verifyReceipt` API.
- It does not create new user accounts; events that arrive before a
  user binds an `appAccountToken` will remain `PENDING_USER_BINDING`
  until the user signs in and posts to `/payment/apple/verify`.
- It does not call `GetAllSubscriptionStatuses` (gopay v1.5.118 does
  not expose it). For now, history replay covers the gap; a follow-up
  task tracks adding the raw HTTP fallback.

## Verification

After a replay run, confirm the subscriptions you expect are present:

```sql
SELECT user_id, status, plan_id, current_period_end
FROM apple_subscriptions
WHERE original_transaction_id = '<your-tx>';

SELECT processing_status, count(*)
FROM apple_events
WHERE created_at > now() - interval '1 day'
GROUP BY processing_status;
```

`/users/me` should reflect the recovered status within seconds because
the reader queries the subscription table on each call.
