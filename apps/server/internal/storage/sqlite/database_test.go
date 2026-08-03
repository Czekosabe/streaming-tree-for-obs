package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenCreatesParentDirectory(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "nested", "deeper", "streaming-tree.db")

	db, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open() returned an error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Fatalf("parent directory was not created: %v", err)
	}
	if db.Path() != path {
		t.Errorf("Path() = %q, want %q", db.Path(), path)
	}
}

func TestOpenEnablesForeignKeys(t *testing.T) {
	db := newEmptyTestDB(t)

	var enabled int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&enabled); err != nil {
		t.Fatalf("reading the foreign_keys pragma failed: %v", err)
	}
	if enabled != 1 {
		t.Errorf("foreign_keys = %d, want 1 - cascading deletes depend on it", enabled)
	}
}

func TestOpenSetsBusyTimeout(t *testing.T) {
	db := newEmptyTestDB(t)

	var timeout int
	if err := db.QueryRow("PRAGMA busy_timeout").Scan(&timeout); err != nil {
		t.Fatalf("reading the busy_timeout pragma failed: %v", err)
	}
	if timeout <= 0 {
		t.Errorf("busy_timeout = %d, want a positive value", timeout)
	}
}

func TestOpenReportsJournalMode(t *testing.T) {
	db := newEmptyTestDB(t)

	if db.JournalMode() == "" {
		t.Error("JournalMode() is empty, want the mode the database ended up in")
	}
}

func TestOpenRejectsEmptyPath(t *testing.T) {
	if _, err := Open(context.Background(), ""); err == nil {
		t.Fatal("Open(\"\") succeeded, want an error")
	}
}

func TestOpenFailsWhenParentIsAFile(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("preparing the fixture failed: %v", err)
	}

	// The parent of the requested path is a regular file, so the directory
	// cannot be created.
	_, err := Open(context.Background(), filepath.Join(blocker, "streaming-tree.db"))
	if err == nil {
		t.Fatal("Open() succeeded for an unusable path, want an error")
	}
}
