package backup

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Staging holds one uploaded package's raw bytes between
// RestorePreview and Restore, addressed by a fresh, random,
// time-bounded token - so a large upload never has to be re-sent to
// confirm it, and so Restore can re-validate the ORIGINAL bytes from
// scratch rather than trusting whatever RestorePreview already parsed
// in memory (docs/backup-restore.md §7 step 7).
type Staging interface {
	// Put writes data under a fresh token and returns it.
	Put(data []byte) (token string, err error)
	// Get reads back the bytes staged under token, or ErrNotFound if
	// the token does not exist or has expired.
	Get(token string) ([]byte, error)
	// Remove deletes the staged bytes for token - idempotent, never an
	// error for an already-removed/unknown token.
	Remove(token string)
}

// FileStaging is Staging backed by a dedicated directory under the
// application's own data directory - never the same directory a real
// backup file the operator saved lives in, and never anywhere a
// public/overlay route can reach (docs/backup-restore.md §28: "delete
// temporary backup uploads after completion/failure").
type FileStaging struct {
	dir string
	mu  sync.Mutex
	// expiresAt tracks each token's own deadline in memory - the
	// staged file's mtime alone is not trusted, so a fresh process
	// restart naturally invalidates every prior session (no state
	// survives a restart, matching PreviewSession's own "nothing
	// mutates until Restore" contract).
	expiresAt map[string]time.Time
	ttl       time.Duration
}

// NewFileStaging creates (if needed) dir and returns a FileStaging
// rooted there.
func NewFileStaging(dir string, ttl time.Duration) (*FileStaging, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create backup staging directory: %w", err)
	}
	return &FileStaging{dir: dir, expiresAt: map[string]time.Time{}, ttl: ttl}, nil
}

func newStagingToken() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate staging token: %w", err)
	}
	return "rst_" + hex.EncodeToString(buf), nil
}

func (s *FileStaging) path(token string) string {
	return filepath.Join(s.dir, token+".bin")
}

func (s *FileStaging) Put(data []byte) (string, error) {
	token, err := newStagingToken()
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(s.path(token), data, 0o600); err != nil {
		return "", fmt.Errorf("stage backup upload: %w", err)
	}
	s.mu.Lock()
	s.expiresAt[token] = time.Now().Add(s.ttl)
	s.mu.Unlock()
	return token, nil
}

func (s *FileStaging) Get(token string) ([]byte, error) {
	s.mu.Lock()
	deadline, known := s.expiresAt[token]
	s.mu.Unlock()
	if !known || time.Now().After(deadline) {
		s.Remove(token)
		return nil, ErrNotFound
	}
	data, err := os.ReadFile(s.path(token))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("read staged backup: %w", err)
	}
	return data, nil
}

func (s *FileStaging) Remove(token string) {
	s.mu.Lock()
	delete(s.expiresAt, token)
	s.mu.Unlock()
	_ = os.Remove(s.path(token))
}
