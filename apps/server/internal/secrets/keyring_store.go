package secrets

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/99designs/keyring"
)

// allowedBackends restricts the library to real OS-native credential stores.
//
// github.com/99designs/keyring also ships a "pass" backend, which shells out
// to the external pass command, and a "file" backend, a password-encrypted
// file on disk. Neither is permitted here: shelling out to an external
// credential command and falling back to a file-based store are both
// explicitly excluded from this application's secret storage, so only the
// three backends the credential-store foundation was scoped for are listed.
// keyring.Open silently skips any backend not available on the current
// platform, so listing all three unconditionally is safe on every OS.
var allowedBackends = []keyring.BackendType{
	keyring.WinCredBackend,
	keyring.KeychainBackend,
	keyring.SecretServiceBackend,
}

// KeyringStore is the production SecretStore, backed by the operating
// system's credential store via github.com/99designs/keyring.
//
// See docs/progress.md for why this library was chosen: every backend it
// uses for the three platforms above is a native binding (Win32 credential
// APIs, the macOS Security framework, or D-Bus), never a shelled-out command.
type KeyringStore struct {
	mu          sync.Mutex
	kr          keyring.Keyring
	serviceName string
}

// NewKeyringStore constructs a store without touching the operating system.
//
// Opening the OS backend is deferred to first use, so constructing this
// value never triggers an OS prompt and never fails backend startup just
// because no credential store happens to be available yet.
func NewKeyringStore() *KeyringStore {
	return &KeyringStore{serviceName: ServiceName}
}

// newKeyringStoreForTesting builds a store under a caller-chosen service
// name, so the opt-in real-credential-store smoke test (see
// keyring_store_smoketest_test.go) can use a unique, disposable name instead
// of the production ServiceName - it must never write under the same
// namespace a real installation uses.
func newKeyringStoreForTesting(serviceName string) *KeyringStore {
	return &KeyringStore{serviceName: serviceName}
}

// open returns the cached backend, opening it on first use. Held under mu
// for the caller's whole operation, not just this step: the backends this
// package uses are not documented as safe for concurrent use, so every
// operation is serialized rather than auditing each backend's internals.
func (s *KeyringStore) openLocked() (keyring.Keyring, error) {
	if s.kr != nil {
		return s.kr, nil
	}

	kr, err := keyring.Open(keyring.Config{
		ServiceName:     s.serviceName,
		AllowedBackends: allowedBackends,
	})
	if err != nil {
		// keyring.Open's only failure mode is "no allowed backend is usable
		// on this system" (ErrNoAvailImpl): every per-backend open error is
		// swallowed internally and treated as that backend being
		// unavailable. There is nothing more specific to report.
		return nil, fmt.Errorf("%w: %s", ErrUnavailable, err)
	}
	s.kr = kr
	return kr, nil
}

func (s *KeyringStore) Set(ctx context.Context, key string, value []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	kr, err := s.openLocked()
	if err != nil {
		return err
	}

	if err := kr.Set(keyring.Item{Key: key, Data: value}); err != nil {
		return fmt.Errorf("%w: %s", ErrUnavailable, err)
	}
	return nil
}

func (s *KeyringStore) Get(ctx context.Context, key string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	kr, err := s.openLocked()
	if err != nil {
		return nil, err
	}

	item, err := kr.Get(key)
	if err != nil {
		if errors.Is(err, keyring.ErrKeyNotFound) {
			return nil, ErrNotFound
		}
		// A reachable backend that still refuses the read - most often a
		// locked keychain or a denied permission - is treated the same as
		// an unreachable one: from an operator's perspective both need the
		// same fix (unlock, grant access) and both must answer a stable
		// status rather than a generic server error.
		return nil, fmt.Errorf("%w: %s", ErrUnavailable, err)
	}
	return item.Data, nil
}

func (s *KeyringStore) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	kr, err := s.openLocked()
	if err != nil {
		return err
	}

	if err := kr.Remove(key); err != nil {
		if errors.Is(err, keyring.ErrKeyNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("%w: %s", ErrUnavailable, err)
	}
	return nil
}

func (s *KeyringStore) Exists(ctx context.Context, key string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	kr, err := s.openLocked()
	if err != nil {
		return false, err
	}

	if _, err := kr.Get(key); err != nil {
		if errors.Is(err, keyring.ErrKeyNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("%w: %s", ErrUnavailable, err)
	}
	return true, nil
}
