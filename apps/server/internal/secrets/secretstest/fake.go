// Package secretstest provides an in-memory secrets.SecretStore for tests.
//
// It exists so tests never place a real secret in the developer's actual OS
// credential store. It has no security properties whatsoever - values are
// held in a plain Go map - and must never be wired into production code; see
// internal/secrets for the real, OS-backed implementation.
package secretstest

import (
	"context"
	"sync"

	"github.com/streaming-tree/server/internal/secrets"
)

// Store is a concurrency-safe, in-memory secrets.SecretStore.
type Store struct {
	mu     sync.Mutex
	values map[string][]byte

	// Unavailable, when true, makes every call behave as though the OS
	// credential store could not be reached (secrets.ErrUnavailable).
	Unavailable bool

	// FailNext, when non-nil, is returned by the next call and then cleared.
	// It exists to exercise a failure path (secrets.ErrFailure in
	// particular) that the real store cannot currently produce.
	FailNext error
}

// New returns an empty Store.
func New() *Store {
	return &Store{values: make(map[string][]byte)}
}

func (s *Store) takeFailureLocked() error {
	err := s.FailNext
	s.FailNext = nil
	return err
}

func (s *Store) Set(ctx context.Context, key string, value []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Unavailable {
		return secrets.ErrUnavailable
	}
	if err := s.takeFailureLocked(); err != nil {
		return err
	}

	stored := make([]byte, len(value))
	copy(stored, value)
	s.values[key] = stored
	return nil
}

func (s *Store) Get(ctx context.Context, key string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Unavailable {
		return nil, secrets.ErrUnavailable
	}
	if err := s.takeFailureLocked(); err != nil {
		return nil, err
	}

	value, ok := s.values[key]
	if !ok {
		return nil, secrets.ErrNotFound
	}
	out := make([]byte, len(value))
	copy(out, value)
	return out, nil
}

func (s *Store) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Unavailable {
		return secrets.ErrUnavailable
	}
	if err := s.takeFailureLocked(); err != nil {
		return err
	}

	if _, ok := s.values[key]; !ok {
		return secrets.ErrNotFound
	}
	delete(s.values, key)
	return nil
}

func (s *Store) Exists(ctx context.Context, key string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Unavailable {
		return false, secrets.ErrUnavailable
	}
	if err := s.takeFailureLocked(); err != nil {
		return false, err
	}

	_, ok := s.values[key]
	return ok, nil
}

// Len reports how many values are currently stored, mainly so tests can
// assert that deleting one platform's credential left everything else
// untouched without reaching into the map directly.
func (s *Store) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.values)
}

// Has reports whether a value is stored under key, bypassing Unavailable and
// FailNext so tests can assert on real state after exercising a failure path.
func (s *Store) Has(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.values[key]
	return ok
}
