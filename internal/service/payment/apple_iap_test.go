package payment

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/dundunHa/go-serverhttp-template/internal/dao"
	"github.com/dundunHa/go-serverhttp-template/internal/model"
)

// fakeVerifier returns a canned Apple transaction or a canned error.
type fakeVerifier struct {
	tx  *AppleTransaction
	err error
}

func (f *fakeVerifier) FetchTransaction(_ context.Context, _ string, _ Environment) (*AppleTransaction, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.tx, nil
}

// fakeIAPDAO simulates dao.SubscriptionDAO.InTx with an in-memory tx that records every upsert.
type fakeIAPDAO struct {
	mu            sync.Mutex
	upserts       []model.SubscriptionUpsert
	forceConflict bool
	commitFn      func(model.SubscriptionUpsert) (model.Subscription, error)
}

func (f *fakeIAPDAO) InTx(ctx context.Context, fn func(dao.SubscriptionTx) error) error {
	tx := &fakeIAPTx{owner: f}
	if err := fn(tx); err != nil {
		return err
	}
	return nil
}

type fakeIAPTx struct {
	owner *fakeIAPDAO
}

func (t *fakeIAPTx) InsertAppleEventIfNotExists(_ context.Context, _ model.AppleEventInsert) (bool, int64, error) {
	return true, 1, nil
}

func (t *fakeIAPTx) UpsertSubscriptionWithOwnershipCheck(_ context.Context, in model.SubscriptionUpsert) (model.Subscription, error) {
	t.owner.mu.Lock()
	defer t.owner.mu.Unlock()
	t.owner.upserts = append(t.owner.upserts, in)
	if t.owner.forceConflict {
		return model.Subscription{}, dao.ErrSubscriptionOwnershipConflict
	}
	if t.owner.commitFn != nil {
		return t.owner.commitFn(in)
	}
	return model.Subscription{
		ID:                    1,
		UserID:                in.UserID,
		AppAccountToken:       in.AppAccountToken,
		Environment:           in.Environment,
		OriginalTransactionID: in.OriginalTransactionID,
		LastTransactionID:     in.LastTransactionID,
		PlanID:                in.PlanID,
		ProviderProductID:     in.ProviderProductID,
		SubscriptionGroupID:   in.SubscriptionGroupID,
		Level:                 in.Level,
		Status:                in.Status,
		AutoRenewStatus:       in.AutoRenewStatus,
		CurrentPeriodStart:    in.CurrentPeriodStart,
		CurrentPeriodEnd:      in.CurrentPeriodEnd,
		LastEventAt:           in.LastEventAt,
	}, nil
}

func (t *fakeIAPTx) GetSubscriptionByOriginalTx(_ context.Context, _ string, _ model.AppleEnvironment) (model.Subscription, error) {
	return model.Subscription{}, dao.ErrSubscriptionNotFound
}

func newProdCatalog(t testing.TB) *Catalog {
	t.Helper()
	cfg := validProdConfig()
	cfg.Products = `[{"plan_id":"pro_monthly","product_id":"com.app.pro.monthly","level":1,"environment":"Production","subscription_group_id":"21456789"}]`
	c, err := NewCatalog(cfg, "dev")
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	return c
}

func newTokensWithFakeDAO(userID int64, token string) *TokenService {
	d := newFakeTokenDAO()
	d.tokens[userID] = token
	return NewTokenService(d)
}

func TestAppleIAPService_VerifyTransaction_Happy(t *testing.T) {
	catalog := newProdCatalog(t)
	const userID int64 = 42
	tok := "00000000-0000-4000-8000-000000000042"
	tokens := newTokensWithFakeDAO(userID, tok)
	expires := time.Now().Add(30 * 24 * time.Hour).UTC()
	verifier := &fakeVerifier{tx: &AppleTransaction{
		TransactionID:         "tx-1",
		OriginalTransactionID: "ot-1",
		AppAccountToken:       tok,
		BundleID:              "com.app.test",
		Environment:           EnvProduction,
		ProductID:             "com.app.pro.monthly",
		Type:                  "Auto-Renewable Subscription",
		PurchaseDate:          time.Now().UTC(),
		ExpiresDate:           expires,
	}}
	dao := &fakeIAPDAO{}
	svc := NewAppleIAPService(catalog, verifier, tokens, dao)

	info, err := svc.VerifyTransaction(context.Background(), userID, "tx-1")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if info.Status != "ACTIVE" {
		t.Fatalf("status = %q, want ACTIVE", info.Status)
	}
	if info.ProductID != "pro_monthly" {
		t.Fatalf("product_id = %q, want canonical plan id pro_monthly", info.ProductID)
	}
	if info.SubscribeLevel != 1 {
		t.Fatalf("level = %d, want 1", info.SubscribeLevel)
	}
	if len(dao.upserts) != 1 {
		t.Fatalf("expected 1 upsert, got %d", len(dao.upserts))
	}
}

func TestAppleIAPService_VerifyTransaction_Errors(t *testing.T) {
	catalog := newProdCatalog(t)
	const userID int64 = 42
	tok := "00000000-0000-4000-8000-000000000042"
	expires := time.Now().Add(30 * 24 * time.Hour).UTC()

	cases := []struct {
		name      string
		tx        *AppleTransaction
		txErr     error
		userID    int64
		boundUser int64
		boundTok  string
		wantErr   error
		conflict  bool
	}{
		{
			name:    "empty_transaction_id_rejected_before_fetch",
			tx:      nil,
			txErr:   nil,
			userID:  userID,
			wantErr: nil, // tested separately below for empty txID
		},
		{
			name:    "empty_app_account_token_in_apple_tx",
			tx:      &AppleTransaction{TransactionID: "tx", OriginalTransactionID: "ot", BundleID: "com.app.test", Environment: EnvProduction, ProductID: "com.app.pro.monthly", Type: "Auto-Renewable Subscription", PurchaseDate: time.Now(), ExpiresDate: expires, AppAccountToken: ""},
			userID:  userID,
			wantErr: ErrEmptyAppAccountToken,
		},
		{
			name: "non_subscription_type",
			tx: &AppleTransaction{TransactionID: "tx", OriginalTransactionID: "ot", BundleID: "com.app.test", Environment: EnvProduction, ProductID: "com.app.pro.monthly",
				Type:            "Consumable",
				AppAccountToken: tok, PurchaseDate: time.Now(), ExpiresDate: expires},
			userID:  userID,
			wantErr: ErrUnsupportedProductType,
		},
		{
			name: "revoked_transaction",
			tx: &AppleTransaction{TransactionID: "tx", OriginalTransactionID: "ot", BundleID: "com.app.test", Environment: EnvProduction, ProductID: "com.app.pro.monthly",
				Type:            "Auto-Renewable Subscription",
				AppAccountToken: tok,
				PurchaseDate:    time.Now(), ExpiresDate: expires,
				RevocationDate: timePtr(time.Now().Add(-1 * time.Hour))},
			userID:  userID,
			wantErr: ErrTransactionRevoked,
		},
		{
			name: "unknown_product_in_catalog",
			tx: &AppleTransaction{TransactionID: "tx", OriginalTransactionID: "ot", BundleID: "com.app.test", Environment: EnvProduction, ProductID: "com.unknown.sku",
				Type:            "Auto-Renewable Subscription",
				AppAccountToken: tok, PurchaseDate: time.Now(), ExpiresDate: expires},
			userID:  userID,
			wantErr: ErrUnknownProduct,
		},
		{
			name: "token_maps_to_other_user",
			tx: &AppleTransaction{TransactionID: "tx", OriginalTransactionID: "ot", BundleID: "com.app.test", Environment: EnvProduction, ProductID: "com.app.pro.monthly",
				Type:            "Auto-Renewable Subscription",
				AppAccountToken: tok, PurchaseDate: time.Now(), ExpiresDate: expires},
			userID: userID, boundUser: 99, boundTok: tok,
			wantErr: ErrAppAccountTokenMismatch,
		},
		{
			name: "token_not_bound_yet",
			tx: &AppleTransaction{TransactionID: "tx", OriginalTransactionID: "ot", BundleID: "com.app.test", Environment: EnvProduction, ProductID: "com.app.pro.monthly",
				Type:            "Auto-Renewable Subscription",
				AppAccountToken: tok, PurchaseDate: time.Now(), ExpiresDate: expires},
			userID: userID,
			// no token bound
			wantErr: ErrAppAccountTokenMismatch,
		},
		{
			name: "ownership_conflict",
			tx: &AppleTransaction{TransactionID: "tx", OriginalTransactionID: "ot", BundleID: "com.app.test", Environment: EnvProduction, ProductID: "com.app.pro.monthly",
				Type:            "Auto-Renewable Subscription",
				AppAccountToken: tok, PurchaseDate: time.Now(), ExpiresDate: expires},
			userID: userID, boundUser: userID, boundTok: tok,
			conflict: true,
			wantErr:  ErrSubscriptionOwnershipConflict,
		},
		{
			name:    "apple_auth_rejected_no_fallback",
			tx:      nil,
			txErr:   ErrAppleAuthRejected,
			userID:  userID,
			wantErr: ErrAppleAuthRejected,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tokens := NewTokenService(newFakeTokenDAO())
			if tc.boundTok != "" {
				bound := newFakeTokenDAO()
				bound.tokens[tc.boundUser] = tc.boundTok
				tokens = NewTokenService(bound)
			}
			verifier := &fakeVerifier{tx: tc.tx, err: tc.txErr}
			dao := &fakeIAPDAO{forceConflict: tc.conflict}
			svc := NewAppleIAPService(catalog, verifier, tokens, dao)
			_, err := svc.VerifyTransaction(context.Background(), tc.userID, "tx-1")
			if tc.wantErr == nil {
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("got %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestAppleIAPService_EmptyTransactionID(t *testing.T) {
	catalog := newProdCatalog(t)
	tokens := NewTokenService(newFakeTokenDAO())
	verifier := &fakeVerifier{}
	dao := &fakeIAPDAO{}
	svc := NewAppleIAPService(catalog, verifier, tokens, dao)
	_, err := svc.VerifyTransaction(context.Background(), 1, "  ")
	if err == nil {
		t.Fatal("expected error for empty transaction id")
	}
}

func TestAppleIAPService_NotConfigured(t *testing.T) {
	svc := &AppleIAPService{}
	_, err := svc.VerifyTransaction(context.Background(), 1, "tx-1")
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("got %v, want ErrNotConfigured", err)
	}
}

func TestAPIStatusForSubscription(t *testing.T) {
	now := time.Now().UTC()
	future := now.Add(24 * time.Hour)
	past := now.Add(-24 * time.Hour)
	cases := []struct {
		name string
		sub  model.Subscription
		want string
	}{
		{"active_future", model.Subscription{Status: model.SubscriptionStatusActive, CurrentPeriodEnd: future}, "ACTIVE"},
		{"active_past_is_expired", model.Subscription{Status: model.SubscriptionStatusActive, CurrentPeriodEnd: past}, "EXPIRED"},
		{"canceled_future_still_entitled", model.Subscription{Status: model.SubscriptionStatusCanceled, CurrentPeriodEnd: future}, "CANCELED"},
		{"canceled_past_is_expired", model.Subscription{Status: model.SubscriptionStatusCanceled, CurrentPeriodEnd: past}, "EXPIRED"},
		{"expired_db_status", model.Subscription{Status: model.SubscriptionStatusExpired, CurrentPeriodEnd: future}, "EXPIRED"},
		{"revoked_db_status", model.Subscription{Status: model.SubscriptionStatusRevoked, CurrentPeriodEnd: future}, "EXPIRED"},
		{"grace_extends_active", model.Subscription{Status: model.SubscriptionStatusActive, CurrentPeriodEnd: past, GracePeriodExpiresAt: &future}, "ACTIVE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := APIStatusForSubscription(tc.sub, now); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func timePtr(t time.Time) *time.Time { return &t }
