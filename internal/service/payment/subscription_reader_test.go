package payment

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dundunHa/go-serverhttp-template/internal/dao"
	"github.com/dundunHa/go-serverhttp-template/internal/model"
)

type readerDAO struct {
	rows    []model.Subscription
	listErr error
}

func (d *readerDAO) GetOrCreateAccountToken(_ context.Context, _ int64) (string, error) {
	return "", nil
}
func (d *readerDAO) GetAccountTokenByToken(_ context.Context, _ string) (model.AppleAccountToken, error) {
	return model.AppleAccountToken{}, nil
}
func (d *readerDAO) GetSubscriptionByOriginalTx(_ context.Context, _ string, _ model.AppleEnvironment) (model.Subscription, error) {
	return model.Subscription{}, dao.ErrSubscriptionNotFound
}
func (d *readerDAO) ListSubscriptionsForUserEntitlement(_ context.Context, _ int64, _ []model.AppleEnvironment) ([]model.Subscription, error) {
	if d.listErr != nil {
		return nil, d.listErr
	}
	return d.rows, nil
}
func (d *readerDAO) InTx(_ context.Context, _ func(dao.SubscriptionTx) error) error {
	return errors.New("not used in reader tests")
}

func TestSubscriptionReader_NoneWhenReaderNil(t *testing.T) {
	var r *SubscriptionReader
	info, err := r.LoadSubscriptionInfo(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Status != "NONE" {
		t.Fatalf("status = %q, want NONE", info.Status)
	}
}

func TestSubscriptionReader_NoneWhenDAOEmpty(t *testing.T) {
	c := newProdCatalog(t)
	d := &readerDAO{}
	r := NewSubscriptionReader(d, c)
	info, err := r.LoadSubscriptionInfo(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Status != "NONE" {
		t.Fatalf("status = %q, want NONE", info.Status)
	}
}

func TestSubscriptionReader_StatusActive(t *testing.T) {
	c := newProdCatalog(t)
	now := time.Now().UTC()
	d := &readerDAO{rows: []model.Subscription{{
		Status:           model.SubscriptionStatusActive,
		Environment:      model.AppleEnvProduction,
		PlanID:           "pro_monthly",
		Level:            1,
		CurrentPeriodEnd: now.Add(48 * time.Hour),
		LastEventAt:      now,
	}}}
	r := NewSubscriptionReader(d, c)
	r.now = func() time.Time { return now }
	info, err := r.LoadSubscriptionInfo(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Status != "ACTIVE" || info.ProductID != "pro_monthly" || info.SubscribeLevel != 1 {
		t.Fatalf("unexpected info: %+v", info)
	}
}

func TestSubscriptionReader_StatusCanceledStillEntitled(t *testing.T) {
	c := newProdCatalog(t)
	now := time.Now().UTC()
	d := &readerDAO{rows: []model.Subscription{{
		Status:           model.SubscriptionStatusCanceled,
		Environment:      model.AppleEnvProduction,
		PlanID:           "pro_monthly",
		Level:            1,
		CurrentPeriodEnd: now.Add(48 * time.Hour),
		LastEventAt:      now,
	}}}
	r := NewSubscriptionReader(d, c)
	r.now = func() time.Time { return now }
	info, _ := r.LoadSubscriptionInfo(context.Background(), 1)
	if info.Status != "CANCELED" || info.SubscribeLevel != 1 {
		t.Fatalf("unexpected info: %+v", info)
	}
}

func TestSubscriptionReader_StatusExpiredZeroLevel(t *testing.T) {
	c := newProdCatalog(t)
	now := time.Now().UTC()
	d := &readerDAO{rows: []model.Subscription{{
		Status:           model.SubscriptionStatusActive,
		Environment:      model.AppleEnvProduction,
		PlanID:           "pro_monthly",
		Level:            1,
		CurrentPeriodEnd: now.Add(-48 * time.Hour),
		LastEventAt:      now.Add(-72 * time.Hour),
	}}}
	r := NewSubscriptionReader(d, c)
	r.now = func() time.Time { return now }
	info, _ := r.LoadSubscriptionInfo(context.Background(), 1)
	if info.Status != "EXPIRED" {
		t.Fatalf("status = %q, want EXPIRED", info.Status)
	}
	if info.SubscribeLevel != 0 {
		t.Fatalf("expired level should be 0, got %d", info.SubscribeLevel)
	}
}

func TestSubscriptionReader_StatusRevokedMapsToExpired(t *testing.T) {
	c := newProdCatalog(t)
	now := time.Now().UTC()
	d := &readerDAO{rows: []model.Subscription{{
		Status:           model.SubscriptionStatusRevoked,
		Environment:      model.AppleEnvProduction,
		PlanID:           "pro_monthly",
		Level:            1,
		CurrentPeriodEnd: now.Add(48 * time.Hour),
	}}}
	r := NewSubscriptionReader(d, c)
	r.now = func() time.Time { return now }
	info, _ := r.LoadSubscriptionInfo(context.Background(), 1)
	if info.Status != "EXPIRED" {
		t.Fatalf("revoked must map to EXPIRED, got %s", info.Status)
	}
}

func TestSubscriptionReader_PicksFirstActiveFromOrderedRows(t *testing.T) {
	c := newProdCatalog(t)
	now := time.Now().UTC()
	d := &readerDAO{rows: []model.Subscription{
		{Status: model.SubscriptionStatusActive, Environment: model.AppleEnvProduction, PlanID: "pro_monthly", Level: 5, CurrentPeriodEnd: now.Add(24 * time.Hour)},
		{Status: model.SubscriptionStatusActive, Environment: model.AppleEnvProduction, PlanID: "basic_monthly", Level: 1, CurrentPeriodEnd: now.Add(72 * time.Hour)},
	}}
	r := NewSubscriptionReader(d, c)
	r.now = func() time.Time { return now }
	info, _ := r.LoadSubscriptionInfo(context.Background(), 1)
	if info.SubscribeLevel != 5 || info.ProductID != "pro_monthly" {
		t.Fatalf("expected highest-level row first, got %+v", info)
	}
}
