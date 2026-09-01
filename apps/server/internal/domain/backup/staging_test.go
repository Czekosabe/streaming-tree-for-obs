package backup

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestFileStagingPutThenGetRoundTrips(t *testing.T) {
	staging, err := NewFileStaging(filepath.Join(t.TempDir(), "staging"), time.Minute)
	if err != nil {
		t.Fatalf("NewFileStaging() error = %v", err)
	}

	token, err := staging.Put([]byte("hello"))
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if token == "" {
		t.Fatal("Put() returned an empty token")
	}

	data, err := staging.Get(token)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("Get() = %q, want %q", data, "hello")
	}
}

func TestFileStagingGetUnknownTokenReturnsNotFound(t *testing.T) {
	staging, err := NewFileStaging(filepath.Join(t.TempDir(), "staging"), time.Minute)
	if err != nil {
		t.Fatalf("NewFileStaging() error = %v", err)
	}
	if _, err := staging.Get("rst_does_not_exist"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() error = %v, want ErrNotFound", err)
	}
}

func TestFileStagingExpiresAfterTTL(t *testing.T) {
	staging, err := NewFileStaging(filepath.Join(t.TempDir(), "staging"), -time.Second)
	if err != nil {
		t.Fatalf("NewFileStaging() error = %v", err)
	}
	token, err := staging.Put([]byte("stale"))
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if _, err := staging.Get(token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() error = %v, want ErrNotFound for an already-expired session", err)
	}
}

func TestFileStagingRemoveIsIdempotent(t *testing.T) {
	staging, err := NewFileStaging(filepath.Join(t.TempDir(), "staging"), time.Minute)
	if err != nil {
		t.Fatalf("NewFileStaging() error = %v", err)
	}
	token, err := staging.Put([]byte("x"))
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	staging.Remove(token)
	staging.Remove(token) // must not panic or error

	if _, err := staging.Get(token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() error = %v, want ErrNotFound after Remove", err)
	}
}
