package payment

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/dundunHa/go-serverhttp-template/internal/dao"
	"github.com/dundunHa/go-serverhttp-template/internal/model"
)

type fakeTokenDAO struct {
	mu               sync.Mutex
	tokens           map[int64]string
	errOnGet         error
	errOnLookup      error
	callsGetOrCreate int
	callsLookup      int
	nextSeed         int64
}

func newFakeTokenDAO() *fakeTokenDAO {
	return &fakeTokenDAO{tokens: map[int64]string{}}
}

func (f *fakeTokenDAO) GetOrCreateAccountToken(_ context.Context, userID int64) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.callsGetOrCreate++
	if f.errOnGet != nil {
		return "", f.errOnGet
	}
	if t, ok := f.tokens[userID]; ok {
		return t, nil
	}
	f.nextSeed++
	t := fmt.Sprintf("00000000-0000-4000-8000-%012d", f.nextSeed)
	f.tokens[userID] = t
	return t, nil
}

func (f *fakeTokenDAO) GetAccountTokenByToken(_ context.Context, token string) (model.AppleAccountToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.callsLookup++
	if f.errOnLookup != nil {
		return model.AppleAccountToken{}, f.errOnLookup
	}
	for uid, t := range f.tokens {
		if t == token {
			return model.AppleAccountToken{ID: 1, UserID: uid, Token: t}, nil
		}
	}
	return model.AppleAccountToken{}, dao.ErrAccountTokenNotFound
}

func TestTokenService_EnsureAccountToken_Stable(t *testing.T) {
	d := newFakeTokenDAO()
	svc := NewTokenService(d)

	first, err := svc.EnsureAccountToken(context.Background(), 42)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	second, err := svc.EnsureAccountToken(context.Background(), 42)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if first != second {
		t.Fatalf("token not stable: %s vs %s", first, second)
	}
	if d.callsGetOrCreate != 2 {
		t.Fatalf("expected 2 dao calls, got %d", d.callsGetOrCreate)
	}
}

func TestTokenService_EnsureAccountToken_PerUser(t *testing.T) {
	d := newFakeTokenDAO()
	svc := NewTokenService(d)

	a, err := svc.EnsureAccountToken(context.Background(), 1)
	if err != nil {
		t.Fatalf("user1: %v", err)
	}
	b, err := svc.EnsureAccountToken(context.Background(), 2)
	if err != nil {
		t.Fatalf("user2: %v", err)
	}
	if a == b {
		t.Fatalf("two users got same token: %s", a)
	}
}

func TestTokenService_EnsureAccountToken_NilDAO(t *testing.T) {
	svc := NewTokenService(nil)
	_, err := svc.EnsureAccountToken(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error when dao is nil")
	}
}

func TestTokenService_EnsureAccountToken_PropagatesDAOError(t *testing.T) {
	d := newFakeTokenDAO()
	d.errOnGet = errors.New("db down")
	svc := NewTokenService(d)

	_, err := svc.EnsureAccountToken(context.Background(), 1)
	if err == nil {
		t.Fatal("expected dao error to propagate")
	}
}

func TestTokenService_ResolveUserByToken_NotFound(t *testing.T) {
	svc := NewTokenService(newFakeTokenDAO())
	_, err := svc.ResolveUserByToken(context.Background(), "00000000-0000-4000-8000-000000000001")
	if !errors.Is(err, ErrAccountTokenNotFound) {
		t.Fatalf("expected ErrAccountTokenNotFound, got %v", err)
	}
}

func TestTokenService_ResolveUserByToken_EmptyToken(t *testing.T) {
	svc := NewTokenService(newFakeTokenDAO())
	_, err := svc.ResolveUserByToken(context.Background(), "")
	if !errors.Is(err, ErrAccountTokenNotFound) {
		t.Fatalf("expected ErrAccountTokenNotFound for empty token, got %v", err)
	}
}

func TestTokenService_ResolveUserByToken_Found(t *testing.T) {
	d := newFakeTokenDAO()
	d.tokens[7] = "00000000-0000-4000-8000-000000000007"
	svc := NewTokenService(d)

	info, err := svc.ResolveUserByToken(context.Background(), "00000000-0000-4000-8000-000000000007")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.UserID != 7 {
		t.Fatalf("user id = %d, want 7", info.UserID)
	}
	if info.Token != "00000000-0000-4000-8000-000000000007" {
		t.Fatalf("token mismatch: %s", info.Token)
	}
}

func TestTokenService_ResolveUserByToken_NilDAO(t *testing.T) {
	svc := NewTokenService(nil)
	_, err := svc.ResolveUserByToken(context.Background(), "abc")
	if err == nil {
		t.Fatal("expected error when dao is nil")
	}
}
