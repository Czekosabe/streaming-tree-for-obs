package sqlite

import (
	"context"
	"path/filepath"
	"testing"
)

// newTestDB opens a migrated database inside the test's own temporary
// directory.
//
// Every test gets its own file, so tests never share state and never touch the
// real user database.
func newTestDB(t *testing.T) *DB {
	t.Helper()

	db := newEmptyTestDB(t)
	if _, err := Migrate(context.Background(), db.DB); err != nil {
		t.Fatalf("Migrate() returned an error: %v", err)
	}
	return db
}

// newEmptyTestDB opens an unmigrated database in a temporary directory.
func newEmptyTestDB(t *testing.T) *DB {
	t.Helper()

	path := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open(%q) returned an error: %v", path, err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("closing the test database failed: %v", err)
		}
	})

	return db
}
