package backup

import (
	"fmt"
	"os"
	"path/filepath"
)

// SafetySnapshotStore holds exactly one pre-restore rollback snapshot
// (docs/backup-restore.md §7 step 5/§19) - never a portable, user-
// facing backup in its own right, and never more than the single most
// recent one: writing a new snapshot always replaces the previous
// file, so disk use never grows across repeated restores.
type SafetySnapshotStore interface {
	Save(data []byte) error
	Load() ([]byte, bool, error)
}

// FileSafetySnapshotStore is SafetySnapshotStore backed by one fixed
// path under the application's own data directory.
type FileSafetySnapshotStore struct {
	path string
}

// NewFileSafetySnapshotStore creates (if needed) dir and returns a
// store rooted at dir/pre-restore-safety-snapshot<Extension>.
func NewFileSafetySnapshotStore(dir string) (*FileSafetySnapshotStore, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create backup safety-snapshot directory: %w", err)
	}
	return &FileSafetySnapshotStore{path: filepath.Join(dir, "pre-restore-safety-snapshot"+Extension)}, nil
}

func (s *FileSafetySnapshotStore) Save(data []byte) error {
	// Write-then-rename, matching every other "never leave a truncated
	// file that looks valid" write in this codebase (docs/backup-
	// restore.md §16).
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write safety snapshot: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("install safety snapshot: %w", err)
	}
	return nil
}

func (s *FileSafetySnapshotStore) Load() ([]byte, bool, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read safety snapshot: %w", err)
	}
	return data, true, nil
}
