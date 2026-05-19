package payment

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeReconciler struct {
	events []AppleWebhookEvent
	next   string
	err    error
}

func (f *fakeReconciler) GetNotificationHistory(_ context.Context, _ NotificationHistoryRequest) ([]AppleWebhookEvent, string, error) {
	if f.err != nil {
		return nil, "", f.err
	}
	return f.events, f.next, nil
}

func (f *fakeReconciler) GetSubscriptionStatus(_ context.Context, _ string, _ Environment) (*AppleSubscriptionStatus, error) {
	return nil, errors.New("not implemented")
}

func TestAppleReconcileService_NotConfigured(t *testing.T) {
	svc := &AppleReconcileService{}
	_, err := svc.Replay(context.Background(), NotificationHistoryRequest{})
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("got %v, want ErrNotConfigured", err)
	}
}

func TestAppleReconcileService_ReplaysEachEventThroughReducer(t *testing.T) {
	now := time.Now().UTC()
	tok := "00000000-0000-4000-8000-000000000200"
	tokens := newTokensWithFakeDAO(200, tok)

	dao := &fakeIAPDAO{}
	mockVerifier := &fakeWebhookVerifier{}
	wh := NewAppleWebhookService(newProdCatalog(t), mockVerifier, tokens, dao)

	tx := AppleTransaction{
		TransactionID: "tx", OriginalTransactionID: "ot",
		AppAccountToken: tok, BundleID: "com.app.example",
		Environment: EnvProduction, ProductID: "com.app.pro.monthly",
		Type: "Auto-Renewable Subscription",
		PurchaseDate: now, ExpiresDate: now.Add(24 * time.Hour),
	}
	events := []AppleWebhookEvent{
		{NotificationUUID: "n1", NotificationType: "DID_RENEW", Environment: EnvProduction, Transaction: &tx, SignedPayloadSHA256: "h1"},
		{NotificationUUID: "n2", NotificationType: "REFUND", Environment: EnvProduction, Transaction: &tx, SignedPayloadSHA256: "h2"},
	}
	rec := &fakeReconciler{events: events, next: "page-2"}
	svc := NewAppleReconcileService(rec, wh)

	res, err := svc.Replay(context.Background(), NotificationHistoryRequest{})
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if res.Replayed != 2 {
		t.Fatalf("expected 2 replays, got %d", res.Replayed)
	}
	if res.Failed != 0 {
		t.Fatalf("expected 0 failures, got %d (errors=%v)", res.Failed, res.LastErrors)
	}
	if res.NextPageToken != "page-2" {
		t.Fatalf("next token mismatch: %q", res.NextPageToken)
	}
	if len(dao.upserts) != 2 {
		t.Fatalf("expected 2 upserts via reducer, got %d", len(dao.upserts))
	}
}

func TestAppleReconcileService_PropagatesReconcilerError(t *testing.T) {
	wh := NewAppleWebhookService(newProdCatalog(t), &fakeWebhookVerifier{}, NewTokenService(newFakeTokenDAO()), &fakeIAPDAO{})
	rec := &fakeReconciler{err: errors.New("apple unreachable")}
	svc := NewAppleReconcileService(rec, wh)

	_, err := svc.Replay(context.Background(), NotificationHistoryRequest{})
	if err == nil {
		t.Fatal("expected error from reconciler")
	}
}
