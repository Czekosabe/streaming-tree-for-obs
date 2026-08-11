package visualasset

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FileStore is the filesystem-backed, content-addressed blob store
// (docs/visual-template-packages.md §14) - conceptually
// <app-data>/assets/visual/blobs/<sha256hex>, plus a sibling preview-
// staging area conceptually at <app-data>/tmp/template-previews/<token>/
// (docs/visual-template-packages.md §19), both rooted under one
// application-owned directory a caller resolves from
// internal/config.Config.DataDir, mirroring internal/runtime/mediamtx/
// resolver.go's own "<DataDir>/runtime" sibling-subdirectory convention.
// Every path this type ever writes to is application-generated - never
// derived from an untrusted archive entry name or a browser-supplied
// upload filename (docs/visual-template-packages.md §9/§14).
type FileStore struct {
	root string
}

// NewFileStore builds a FileStore rooted at root (an already-resolved
// absolute directory, e.g. filepath.Join(dataDir, "assets", "visual")).
// EnsureDirs must be called once before use.
func NewFileStore(root string) *FileStore {
	return &FileStore{root: root}
}

func (s *FileStore) blobsDir() string     { return filepath.Join(s.root, "blobs") }
func (s *FileStore) tmpDir() string       { return filepath.Join(s.root, "tmp") }
func (s *FileStore) previewsRoot() string { return filepath.Join(s.root, "previews") }

// EnsureDirs creates every managed directory this store needs, if
// missing.
func (s *FileStore) EnsureDirs() error {
	for _, dir := range []string{s.root, s.blobsDir(), s.tmpDir(), s.previewsRoot()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("%w: create %s: %v", ErrStorage, dir, err)
		}
	}
	return nil
}

func (s *FileStore) blobPath(sha256Hex string) string {
	return filepath.Join(s.blobsDir(), sha256Hex)
}

// boundedCopy copies from r into dst, reading at most maxBytes+1 bytes
// (the same "io.LimitReader(src, remaining+1) and detect overflow"
// technique internal/runtime/mediamtx/archive.go's own copyBounded and
// internal/domain/visualpackage's own readEntryBounded already use) -
// letting the underlying reader signal its own real io.EOF exactly at
// the maxBytes boundary, rather than a naive "stop after N bytes"
// wrapper that would misreport a legitimately-exact-length stream as
// ErrTooLarge. Declared sizes are never trusted alone (docs/visual-
// template-packages.md §10): if more than maxBytes bytes are actually
// available, this returns ErrTooLarge.
func boundedCopy(dst io.Writer, r io.Reader, maxBytes int64) (int64, error) {
	limited := io.LimitReader(r, maxBytes+1)
	n, err := io.Copy(dst, limited)
	if err != nil {
		return n, err
	}
	if n > maxBytes {
		return n, ErrTooLarge
	}
	return n, nil
}

// WriteBlob streams r into an application-owned temporary file (bounded
// by maxBytes), computing SHA-256 while reading, then atomically installs
// it into the content-addressed blob store (docs/visual-template-
// packages.md §14's "files first, database second" atomic installation,
// steps 1-5). If a blob with the same hash already exists, the freshly
// written temp file is discarded and the existing one is kept - dedup by
// content (docs/visual-template-packages.md §13/§27). Returns the lower-
// case hex SHA-256 and the exact byte count read.
func (s *FileStore) WriteBlob(r io.Reader, maxBytes int64) (sha256Hex string, size int64, err error) {
	tmp, err := os.CreateTemp(s.tmpDir(), "asset-upload-*")
	if err != nil {
		return "", 0, fmt.Errorf("%w: create temp file: %v", ErrStorage, err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()

	h := sha256.New()
	n, copyErr := boundedCopy(io.MultiWriter(tmp, h), r, maxBytes)
	if copyErr != nil {
		if errors.Is(copyErr, ErrTooLarge) {
			return "", 0, ErrTooLarge
		}
		return "", 0, fmt.Errorf("%w: read upload: %v", ErrStorage, copyErr)
	}
	if err := tmp.Sync(); err != nil {
		return "", 0, fmt.Errorf("%w: sync temp file: %v", ErrStorage, err)
	}
	if err := tmp.Close(); err != nil {
		return "", 0, fmt.Errorf("%w: close temp file: %v", ErrStorage, err)
	}

	sum := hex.EncodeToString(h.Sum(nil))
	dest := s.blobPath(sum)
	if _, statErr := os.Stat(dest); statErr == nil {
		// Already present - identical content, nothing more to write.
		return sum, n, nil
	}
	if err := os.Rename(tmpPath, dest); err != nil {
		return "", 0, fmt.Errorf("%w: install blob: %v", ErrStorage, err)
	}
	// Rename succeeded - the deferred cleanup's os.Remove(tmpPath) below
	// will now silently no-op (ENOENT), which is fine.
	return sum, n, nil
}

// Open returns a reader for the blob with the given hash.
func (s *FileStore) Open(sha256Hex string) (*os.File, error) {
	f, err := os.Open(s.blobPath(sha256Hex))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("%w: open blob: %v", ErrStorage, err)
	}
	return f, nil
}

// Exists reports whether a blob with the given hash is present on disk.
func (s *FileStore) Exists(sha256Hex string) bool {
	_, err := os.Stat(s.blobPath(sha256Hex))
	return err == nil
}

// Delete removes the blob file with the given hash, if present -
// idempotent (deleting an already-missing blob is not an error), used
// only by the delayed garbage-collection pass (docs/visual-template-
// packages.md §15/§16), never by an immediate per-request delete.
func (s *FileStore) Delete(sha256Hex string) error {
	if err := os.Remove(s.blobPath(sha256Hex)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("%w: delete blob: %v", ErrStorage, err)
	}
	return nil
}

// ListBlobFiles returns every blob filename (a SHA-256 hex string)
// currently on disk - used by startup reconciliation to find orphan
// files with no matching database row (docs/visual-template-
// packages.md §16).
func (s *FileStore) ListBlobFiles() ([]string, error) {
	entries, err := os.ReadDir(s.blobsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("%w: list blobs: %v", ErrStorage, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.Type().IsRegular() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// --- package-import preview staging (docs/visual-template-packages.md §19) ---

func (s *FileStore) previewDir(token string) string {
	return filepath.Join(s.previewsRoot(), token)
}

// OpenPreviewAsset opens a staged preview asset for reading - logicalName
// must be exactly the same application-generated name WritePreviewAsset
// was given (never an untrusted path); a name that does not equal its
// own filepath.Base, or that contains a directory traversal segment, is
// rejected as not-found rather than ever being joined onto previewDir.
func (s *FileStore) OpenPreviewAsset(token, logicalName string) (*os.File, error) {
	if logicalName == "" || logicalName != filepath.Base(logicalName) || strings.Contains(logicalName, "..") {
		return nil, ErrNotFound
	}
	f, err := os.Open(filepath.Join(s.previewDir(token), logicalName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("%w: open preview asset: %v", ErrStorage, err)
	}
	return f, nil
}

// WritePreviewAsset stages one verified package asset's bytes under a
// preview session, keyed only by an application-generated logical name
// (never the archive's own untrusted entry path).
func (s *FileStore) WritePreviewAsset(token, logicalName string, r io.Reader, maxBytes int64) (path string, sha256Hex string, size int64, err error) {
	dir := s.previewDir(token)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", 0, fmt.Errorf("%w: create preview dir: %v", ErrStorage, err)
	}
	dest := filepath.Join(dir, logicalName)
	f, err := os.Create(dest)
	if err != nil {
		return "", "", 0, fmt.Errorf("%w: create preview file: %v", ErrStorage, err)
	}
	defer f.Close()

	h := sha256.New()
	n, copyErr := boundedCopy(io.MultiWriter(f, h), r, maxBytes)
	if copyErr != nil {
		_ = os.Remove(dest)
		if errors.Is(copyErr, ErrTooLarge) {
			return "", "", 0, ErrTooLarge
		}
		return "", "", 0, fmt.Errorf("%w: write preview file: %v", ErrStorage, copyErr)
	}
	return dest, hex.EncodeToString(h.Sum(nil)), n, nil
}

// RemovePreview deletes an entire preview session's staged directory -
// best-effort, used on confirm/cancel and on TTL expiration.
func (s *FileStore) RemovePreview(token string) error {
	if err := os.RemoveAll(s.previewDir(token)); err != nil {
		return fmt.Errorf("%w: remove preview dir: %v", ErrStorage, err)
	}
	return nil
}

// RemoveAllPreviews wipes the entire preview-staging root - called once
// on every clean startup (docs/visual-template-packages.md §19: "all
// removed on next startup"), since no preview token issued by a previous
// process can still be legitimately in flight.
func (s *FileStore) RemoveAllPreviews() error {
	entries, err := os.ReadDir(s.previewsRoot())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("%w: list previews: %v", ErrStorage, err)
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(s.previewsRoot(), e.Name())); err != nil {
			return fmt.Errorf("%w: remove preview: %v", ErrStorage, err)
		}
	}
	return nil
}

// PreviewExpiry reports whether a preview session directory's own
// modification time is older than ttl relative to now.
func (s *FileStore) PreviewExpired(token string, ttl time.Duration, now time.Time) (bool, error) {
	info, err := os.Stat(s.previewDir(token))
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, fmt.Errorf("%w: stat preview dir: %v", ErrStorage, err)
	}
	return now.Sub(info.ModTime()) > ttl, nil
}
