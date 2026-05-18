package storage

import (
	"context"
	"errors"
	"testing"
)

func TestInitDBRequiresDSN(t *testing.T) {
	db, err := InitDB(context.Background(), "")
	if db != nil {
		t.Fatalf("db = %v, want nil", db)
	}
	if !errors.Is(err, ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, ErrMissingDSN)
	}
}
