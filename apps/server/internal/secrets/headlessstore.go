// Package secrets: Stage 20D2A headless secret storage
// (docs/linux-headless-server.md §8/§9).
//
// HeadlessStore is a second SecretStore implementation, selected only
// when the application is explicitly started with --headless - never
// inferred from GOOS. Desktop Linux keeps using KeyringStore/Secret
// Service unconditionally; a headless deployment never attempts to
// open a desktop D-Bus session at all.
package secrets

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const (
	// headlessStoreFormat/Version identify the on-disk envelope. A file
	// with a different format or a version this build does not
	// understand is rejected outright rather than guessed at, exactly
	// like the release manifest's own strict format/version check.
	headlessStoreFormat  = "streaming-tree-headless-secrets"
	headlessStoreVersion = 1

	// masterKeyLength is exactly 32 bytes - AES-256.
	masterKeyLength = 32

	// gcmNonceLength is the standard, recommended nonce size for
	// AES-GCM (96 bits).
	gcmNonceLength = 12

	// HeadlessMasterKeyCredentialName is the fixed systemd
	// LoadCredential= name the shipped unit exposes the master key
	// under (docs/linux-headless-server.md §9) - never derived from
	// user or frontend input.
	HeadlessMasterKeyCredentialName = "streaming-tree-master-key"
)

// HeadlessStore is a SecretStore backed by a single AES-256-GCM
// encrypted JSON file. Every value is sealed under a fresh random
// nonce with the entry's own key bound in as AEAD associated data, so
// swapping ciphertext between two entries fails authentication even
// with a valid master key.
type HeadlessStore struct {
	mu   sync.Mutex
	path string
	aead cipher.AEAD
}

type headlessStoreFile struct {
	Format  string            `json:"format"`
	Version int               `json:"version"`
	Entries map[string]string `json:"entries"`
}

// NewHeadlessStore constructs a store backed by the encrypted file at
// path (created on first Set if it does not yet exist), using
// masterKey - which must be exactly 32 bytes - as the AES-256-GCM key.
// masterKey's own bytes are not retained beyond constructing the AEAD
// cipher.
func NewHeadlessStore(path string, masterKey []byte) (*HeadlessStore, error) {
	if len(masterKey) != masterKeyLength {
		return nil, fmt.Errorf("%w: headless master key must be exactly %d bytes, got %d",
			ErrUnavailable, masterKeyLength, len(masterKey))
	}
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrUnavailable, err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrUnavailable, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrUnavailable, err)
	}
	return &HeadlessStore{path: path, aead: aead}, nil
}

// LoadHeadlessMasterKey reads the master key from the systemd
// credentials directory (docs/linux-headless-server.md §9) -
// $CREDENTIALS_DIRECTORY/streaming-tree-master-key - never an
// environment variable *value*, a command-line flag, or an
// application-parsed config file. A headless deployment fails closed:
// a missing CREDENTIALS_DIRECTORY, a missing/unreadable file, or a
// wrong-length key are all reported here rather than deferred to the
// first secret operation.
func LoadHeadlessMasterKey() ([]byte, error) {
	dir := os.Getenv("CREDENTIALS_DIRECTORY")
	if dir == "" {
		return nil, fmt.Errorf(
			"%w: CREDENTIALS_DIRECTORY is not set - the headless master key must be provisioned via the systemd unit's LoadCredential= directive, see docs/linux-headless-server.md §9",
			ErrUnavailable)
	}
	path := filepath.Join(dir, HeadlessMasterKeyCredentialName)
	key, err := os.ReadFile(path) // #nosec G304 -- CREDENTIALS_DIRECTORY and the fixed credential name are systemd-controlled, not user/frontend input.
	if err != nil {
		return nil, fmt.Errorf("%w: reading headless master key: %s", ErrUnavailable, err)
	}
	if len(key) != masterKeyLength {
		return nil, fmt.Errorf("%w: headless master key must be exactly %d bytes, got %d",
			ErrUnavailable, masterKeyLength, len(key))
	}
	return key, nil
}

func (s *HeadlessStore) Set(ctx context.Context, key string, value []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.withLock(func() error {
		data, err := s.readFile()
		if err != nil {
			return err
		}

		nonce := make([]byte, gcmNonceLength)
		if _, err := rand.Read(nonce); err != nil {
			return fmt.Errorf("%w: generating nonce: %s", ErrFailure, err)
		}
		sealed := s.aead.Seal(nonce, nonce, value, []byte(key))
		data.Entries[key] = base64.StdEncoding.EncodeToString(sealed)

		return s.writeFile(data)
	})
}

func (s *HeadlessStore) Get(ctx context.Context, key string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	var value []byte
	err := s.withLock(func() error {
		data, err := s.readFile()
		if err != nil {
			return err
		}
		encoded, ok := data.Entries[key]
		if !ok {
			return ErrNotFound
		}
		sealed, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return fmt.Errorf("%w: malformed ciphertext encoding", ErrFailure)
		}
		if len(sealed) < gcmNonceLength {
			return fmt.Errorf("%w: truncated ciphertext", ErrFailure)
		}
		nonce, ciphertext := sealed[:gcmNonceLength], sealed[gcmNonceLength:]
		plaintext, err := s.aead.Open(nil, nonce, ciphertext, []byte(key))
		if err != nil {
			return fmt.Errorf("%w: authentication failed (wrong master key or tampered data)", ErrFailure)
		}
		value = plaintext
		return nil
	})
	return value, err
}

func (s *HeadlessStore) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.withLock(func() error {
		data, err := s.readFile()
		if err != nil {
			return err
		}
		if _, ok := data.Entries[key]; !ok {
			return ErrNotFound
		}
		delete(data.Entries, key)
		return s.writeFile(data)
	})
}

func (s *HeadlessStore) Exists(ctx context.Context, key string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	var exists bool
	err := s.withLock(func() error {
		data, err := s.readFile()
		if err != nil {
			return err
		}
		_, exists = data.Entries[key]
		return nil
	})
	return exists, err
}

// withLock serializes access to the backing file both within this
// process (the caller already holds s.mu) and across processes, via a
// real flock(2) on a dedicated "<path>.lock" file - never the data
// file itself, since os.Rename (writeFile's own atomicity mechanism)
// would otherwise silently detach an already-held lock from the
// file's new inode.
func (s *HeadlessStore) withLock(fn func() error) error {
	lockFile, err := os.OpenFile(s.path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrFailure, err)
	}
	defer lockFile.Close()

	if err := flockFile(lockFile); err != nil {
		return fmt.Errorf("%w: %s", ErrFailure, err)
	}
	defer unflockFile(lockFile)

	return fn()
}

// readFile reads and validates the current envelope. A missing file
// (first use) is not an error - it is treated as an empty store.
func (s *HeadlessStore) readFile() (headlessStoreFile, error) {
	raw, err := os.ReadFile(s.path) // #nosec G304 -- s.path is fixed at construction, never user/frontend input.
	if errors.Is(err, os.ErrNotExist) {
		return headlessStoreFile{Format: headlessStoreFormat, Version: headlessStoreVersion, Entries: map[string]string{}}, nil
	}
	if err != nil {
		return headlessStoreFile{}, fmt.Errorf("%w: %s", ErrUnavailable, err)
	}
	if len(raw) == 0 {
		return headlessStoreFile{Format: headlessStoreFormat, Version: headlessStoreVersion, Entries: map[string]string{}}, nil
	}

	var data headlessStoreFile
	if err := json.Unmarshal(raw, &data); err != nil {
		return headlessStoreFile{}, fmt.Errorf("%w: malformed secret store file: %s", ErrFailure, err)
	}
	if data.Format != headlessStoreFormat {
		return headlessStoreFile{}, fmt.Errorf("%w: unrecognized secret store format %q", ErrFailure, data.Format)
	}
	if data.Version != headlessStoreVersion {
		return headlessStoreFile{}, fmt.Errorf("%w: unsupported secret store version %d", ErrFailure, data.Version)
	}
	if data.Entries == nil {
		data.Entries = map[string]string{}
	}
	return data, nil
}

// writeFile atomically replaces the backing file: write to a fresh
// temp file in the same directory, fsync, close, chmod, then rename -
// a partially-written file is never visible under the real path.
func (s *HeadlessStore) writeFile(data headlessStoreFile) error {
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("%w: %s", ErrFailure, err)
	}

	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, ".secrets-*.tmp")
	if err != nil {
		return fmt.Errorf("%w: %s", ErrFailure, err)
	}
	tmpPath := tmp.Name()
	renamed := false
	defer func() {
		if !renamed {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("%w: %s", ErrFailure, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("%w: %s", ErrFailure, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("%w: %s", ErrFailure, err)
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return fmt.Errorf("%w: %s", ErrFailure, err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("%w: %s", ErrFailure, err)
	}
	renamed = true
	return nil
}
