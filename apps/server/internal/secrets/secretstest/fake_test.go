package secretstest

import (
	"context"
	"errors"
	"testing"

	"github.com/streaming-tree/server/internal/secrets"
)

func TestStoreSetGetRoundTrip(t *testing.T) {
	s := New()
	ctx := context.Background()

	if err := s.Set(ctx, "k", []byte("value")); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	got, err := s.Get(ctx, "k")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if string(got) != "value" {
		t.Errorf("Get() = %q, want %q", got, "value")
	}
}

func TestStoreGetMissingKeyReturnsErrNotFound(t *testing.T) {
	s := New()
	_, err := s.Get(context.Background(), "missing")
	if !errors.Is(err, secrets.ErrNotFound) {
		t.Errorf("Get() error = %v, want ErrNotFound", err)
	}
}

func TestStoreDeleteMissingKeyReturnsErrNotFound(t *testing.T) {
	s := New()
	err := s.Delete(context.Background(), "missing")
	if !errors.Is(err, secrets.ErrNotFound) {
		t.Errorf("Delete() error = %v, want ErrNotFound", err)
	}
}

func TestStoreExistsReflectsSetAndDelete(t *testing.T) {
	s := New()
	ctx := context.Background()

	exists, err := s.Exists(ctx, "k")
	if err != nil || exists {
		t.Fatalf("Exists() before Set = (%v, %v), want (false, nil)", exists, err)
	}

	if err := s.Set(ctx, "k", []byte("v")); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	exists, err = s.Exists(ctx, "k")
	if err != nil || !exists {
		t.Fatalf("Exists() after Set = (%v, %v), want (true, nil)", exists, err)
	}

	if err := s.Delete(ctx, "k"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	exists, err = s.Exists(ctx, "k")
	if err != nil || exists {
		t.Fatalf("Exists() after Delete = (%v, %v), want (false, nil)", exists, err)
	}
}

func TestStoreUnavailableRejectsEveryOperation(t *testing.T) {
	s := New()
	s.Unavailable = true
	ctx := context.Background()

	if err := s.Set(ctx, "k", []byte("v")); !errors.Is(err, secrets.ErrUnavailable) {
		t.Errorf("Set() error = %v, want ErrUnavailable", err)
	}
	if _, err := s.Get(ctx, "k"); !errors.Is(err, secrets.ErrUnavailable) {
		t.Errorf("Get() error = %v, want ErrUnavailable", err)
	}
	if err := s.Delete(ctx, "k"); !errors.Is(err, secrets.ErrUnavailable) {
		t.Errorf("Delete() error = %v, want ErrUnavailable", err)
	}
	if _, err := s.Exists(ctx, "k"); !errors.Is(err, secrets.ErrUnavailable) {
		t.Errorf("Exists() error = %v, want ErrUnavailable", err)
	}
}

func TestStoreFailNextIsConsumedOnce(t *testing.T) {
	s := New()
	ctx := context.Background()
	boom := errors.New("boom")
	s.FailNext = boom

	if err := s.Set(ctx, "k", []byte("v")); !errors.Is(err, boom) {
		t.Fatalf("first Set() error = %v, want boom", err)
	}
	// The failure was consumed: this call must succeed.
	if err := s.Set(ctx, "k", []byte("v")); err != nil {
		t.Fatalf("second Set() error = %v, want nil", err)
	}
}

func TestStoreMutatingCallerCannotModifyStoredBytes(t *testing.T) {
	s := New()
	ctx := context.Background()

	value := []byte("original")
	if err := s.Set(ctx, "k", value); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	value[0] = 'X'

	got, err := s.Get(ctx, "k")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if string(got) != "original" {
		t.Errorf("Get() = %q, want %q (Set must copy its input)", got, "original")
	}
}
