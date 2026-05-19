//go:build integration

package dao

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dundunHa/go-serverhttp-template/internal/model"
)

const integrationDSNEnv = "INTEGRATION_DB_DSN"

func mustOpenIntegrationPool(t testing.TB) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv(integrationDSNEnv)
	if dsn == "" {
		t.Skipf("%s not set; skipping integration test", integrationDSNEnv)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect pool: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping db: %v", err)
	}
	return pool
}

// withTestUser inserts a temporary users row for FK-bound subscription / token tests.
// All subscription / event rows reference this user; deferred cleanup truncates
// dependent tables in dependency order to avoid touching unrelated data.
func withTestUser(t testing.TB, pool *pgxpool.Pool) (userID int64, cleanup func()) {
	t.Helper()
	ctx := context.Background()
	row := pool.QueryRow(ctx, "INSERT INTO users (name) VALUES ($1) RETURNING id", "iap-integration-"+t.Name())
	if err := row.Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	cleanup = func() {
		_, _ = pool.Exec(ctx, "DELETE FROM apple_events WHERE user_id = $1", userID)
		_, _ = pool.Exec(ctx, "DELETE FROM apple_subscriptions WHERE user_id = $1", userID)
		_, _ = pool.Exec(ctx, "DELETE FROM apple_account_tokens WHERE user_id = $1", userID)
		_, _ = pool.Exec(ctx, "DELETE FROM users WHERE id = $1", userID)
	}
	return userID, cleanup
}

func TestIntegration_SubscriptionDAO_AccountTokenUniqueAndStable(t *testing.T) {
	pool := mustOpenIntegrationPool(t)
	defer pool.Close()
	userID, cleanup := withTestUser(t, pool)
	defer cleanup()

	dao := NewSubscriptionDAO(pool)
	ctx := context.Background()

	first, err := dao.GetOrCreateAccountToken(ctx, userID)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	second, err := dao.GetOrCreateAccountToken(ctx, userID)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if first != second {
		t.Fatalf("token not stable: %s vs %s", first, second)
	}
	if !strings.Contains(first, "-") {
		t.Fatalf("malformed uuid: %s", first)
	}
}

func TestIntegration_SubscriptionDAO_AccountTokenConcurrentFirstCall(t *testing.T) {
	pool := mustOpenIntegrationPool(t)
	defer pool.Close()
	userID, cleanup := withTestUser(t, pool)
	defer cleanup()

	dao := NewSubscriptionDAO(pool)
	ctx := context.Background()

	const workers = 8
	var wg sync.WaitGroup
	results := make([]string, workers)
	errs := make([]error, workers)
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(idx int) {
			defer wg.Done()
			tok, err := dao.GetOrCreateAccountToken(ctx, userID)
			results[idx] = tok
			errs[idx] = err
		}(i)
	}
	wg.Wait()

	canonical := results[0]
	for i, r := range results {
		if errs[i] != nil {
			t.Fatalf("worker %d: %v", i, errs[i])
		}
		if r != canonical {
			t.Fatalf("worker %d returned different token %s vs %s", i, r, canonical)
		}
	}
}

func TestIntegration_SubscriptionDAO_GetByTokenNotFound(t *testing.T) {
	pool := mustOpenIntegrationPool(t)
	defer pool.Close()

	dao := NewSubscriptionDAO(pool)
	_, err := dao.GetAccountTokenByToken(context.Background(), "00000000-0000-4000-8000-000000000999")
	if !errors.Is(err, ErrAccountTokenNotFound) {
		t.Fatalf("expected ErrAccountTokenNotFound, got %v", err)
	}
}

func TestIntegration_SubscriptionDAO_OwnershipConflict(t *testing.T) {
	pool := mustOpenIntegrationPool(t)
	defer pool.Close()
	userA, cleanA := withTestUser(t, pool)
	defer cleanA()
	userB, cleanB := withTestUser(t, pool)
	defer cleanB()

	dao := NewSubscriptionDAO(pool)
	ctx := context.Background()

	tokenA, err := dao.GetOrCreateAccountToken(ctx, userA)
	if err != nil {
		t.Fatalf("token A: %v", err)
	}
	tokenB, err := dao.GetOrCreateAccountToken(ctx, userB)
	if err != nil {
		t.Fatalf("token B: %v", err)
	}

	now := time.Now().UTC()
	end := now.Add(30 * 24 * time.Hour)
	upsert := func(uid int64, tok string) error {
		return dao.InTx(ctx, func(qtx SubscriptionTx) error {
			_, err := qtx.UpsertSubscriptionWithOwnershipCheck(ctx, model.SubscriptionUpsert{
				UserID:                uid,
				AppAccountToken:       tok,
				Environment:           model.AppleEnvProduction,
				OriginalTransactionID: "shared-original-tx",
				LastTransactionID:     "txn-1",
				PlanID:                "pro_monthly",
				ProviderProductID:     "com.app.pro.monthly",
				Level:                 1,
				Status:                model.SubscriptionStatusActive,
				AutoRenewStatus:       model.AutoRenewStatusOn,
				CurrentPeriodStart:    now,
				CurrentPeriodEnd:      end,
				LastEventAt:           now,
			})
			return err
		})
	}

	if err := upsert(userA, tokenA); err != nil {
		t.Fatalf("user A initial: %v", err)
	}
	err = upsert(userB, tokenB)
	if !errors.Is(err, ErrSubscriptionOwnershipConflict) {
		t.Fatalf("user B should conflict, got %v", err)
	}
}

func TestIntegration_SubscriptionDAO_EventIdempotencyAndAtomicRollback(t *testing.T) {
	pool := mustOpenIntegrationPool(t)
	defer pool.Close()
	userID, cleanup := withTestUser(t, pool)
	defer cleanup()

	dao := NewSubscriptionDAO(pool)
	ctx := context.Background()

	tok, err := dao.GetOrCreateAccountToken(ctx, userID)
	if err != nil {
		t.Fatalf("seed token: %v", err)
	}

	notifUUID := "notif-" + t.Name()
	insert := func() (created bool, e error) {
		e = dao.InTx(ctx, func(qtx SubscriptionTx) error {
			c, _, err := qtx.InsertAppleEventIfNotExists(ctx, model.AppleEventInsert{
				NotificationUUID:      notifUUID,
				NotificationType:      "DID_RENEW",
				Environment:           model.AppleEnvProduction,
				UserID:                userID,
				AppAccountToken:       tok,
				OriginalTransactionID: "ot-1",
				TransactionID:         "tx-1",
				ProcessingStatus:      model.EventStatusProcessed,
				RawJWSSHA256:          "deadbeefcafebabe",
			})
			if err != nil {
				return err
			}
			created = c
			return nil
		})
		return
	}

	c, err := insert()
	if err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if !c {
		t.Fatalf("first insert should be created=true")
	}

	c, err = insert()
	if err != nil {
		t.Fatalf("dup insert: %v", err)
	}
	if c {
		t.Fatalf("duplicate notification_uuid must not create")
	}

	rollbackUUID := "rollback-" + t.Name()
	err = dao.InTx(ctx, func(qtx SubscriptionTx) error {
		if _, _, err := qtx.InsertAppleEventIfNotExists(ctx, model.AppleEventInsert{
			NotificationUUID:      rollbackUUID,
			NotificationType:      "DID_RENEW",
			Environment:           model.AppleEnvProduction,
			UserID:                userID,
			AppAccountToken:       tok,
			OriginalTransactionID: "rollback-ot",
			TransactionID:         "rollback-tx",
			ProcessingStatus:      model.EventStatusProcessed,
			RawJWSSHA256:          "deadbeefcafebabe2",
		}); err != nil {
			return err
		}
		return errors.New("forced rollback")
	})
	if err == nil || err.Error() != "forced rollback" {
		t.Fatalf("expected forced rollback error, got %v", err)
	}

	row := pool.QueryRow(ctx, "SELECT count(*) FROM apple_events WHERE notification_uuid = $1", rollbackUUID)
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count rollback: %v", err)
	}
	if count != 0 {
		t.Fatalf("rollback failed: %d rows persisted", count)
	}
}

func TestIntegration_SubscriptionDAO_ListEntitlementOrderingAndEnvFilter(t *testing.T) {
	pool := mustOpenIntegrationPool(t)
	defer pool.Close()
	userID, cleanup := withTestUser(t, pool)
	defer cleanup()

	dao := NewSubscriptionDAO(pool)
	ctx := context.Background()

	tok, err := dao.GetOrCreateAccountToken(ctx, userID)
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	now := time.Now().UTC()

	insert := func(env model.AppleEnvironment, ot string, level int, status string, end time.Time) {
		err := dao.InTx(ctx, func(qtx SubscriptionTx) error {
			_, e := qtx.UpsertSubscriptionWithOwnershipCheck(ctx, model.SubscriptionUpsert{
				UserID:                userID,
				AppAccountToken:       tok,
				Environment:           env,
				OriginalTransactionID: ot,
				PlanID:                "p" + ot,
				ProviderProductID:     "com.app." + ot,
				Level:                 level,
				Status:                status,
				AutoRenewStatus:       model.AutoRenewStatusOn,
				CurrentPeriodStart:    now,
				CurrentPeriodEnd:      end,
				LastEventAt:           now,
			})
			return e
		})
		if err != nil {
			t.Fatalf("seed %s: %v", ot, err)
		}
	}

	insert(model.AppleEnvProduction, "ot-low-active", 1, model.SubscriptionStatusActive, now.Add(48*time.Hour))
	insert(model.AppleEnvProduction, "ot-high-active", 5, model.SubscriptionStatusActive, now.Add(24*time.Hour))
	insert(model.AppleEnvSandbox, "ot-sandbox", 9, model.SubscriptionStatusActive, now.Add(48*time.Hour))
	insert(model.AppleEnvProduction, "ot-expired", 3, model.SubscriptionStatusExpired, now.Add(-24*time.Hour))

	rows, err := dao.ListSubscriptionsForUserEntitlement(ctx, userID, []model.AppleEnvironment{model.AppleEnvProduction})
	if err != nil {
		t.Fatalf("list entitlement: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows (sandbox filtered), got %d", len(rows))
	}
	if rows[0].OriginalTransactionID != "ot-high-active" {
		t.Fatalf("expected high-level active first, got %s", rows[0].OriginalTransactionID)
	}
	if rows[1].OriginalTransactionID != "ot-low-active" {
		t.Fatalf("expected low-active second, got %s", rows[1].OriginalTransactionID)
	}
	if rows[2].OriginalTransactionID != "ot-expired" {
		t.Fatalf("expected expired last, got %s", rows[2].OriginalTransactionID)
	}
}
