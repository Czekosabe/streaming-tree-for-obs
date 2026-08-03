// Package sqlite holds the SQLite persistence layer: connection setup, the
// embedded migration runner and the platform repository.
//
// The driver is modernc.org/sqlite, a pure-Go translation of SQLite. It needs
// no CGO, so the server still builds and cross-compiles as a single static
// binary with `go build`.
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// DB wraps the connection pool together with the path it was opened from.
type DB struct {
	*sql.DB
	path        string
	journalMode string
}

// Path returns the resolved database file path. Safe to log: it contains no
// credentials, and the application stores none.
func (db *DB) Path() string { return db.path }

const (
	// busyTimeout is how long a statement waits for a lock before failing.
	// Generous because this is a local, single-user application.
	busyTimeout = 5 * time.Second

	// SQLite handles one writer at a time. Capping the pool keeps contention
	// predictable instead of letting many connections fight over the write lock.
	maxOpenConns = 4
	maxIdleConns = 4
	connMaxIdle  = 5 * time.Minute
)

// Open creates the parent directory if needed, opens the database and applies
// the connection pragmas.
//
// Pragma failures are returned, never ignored: running without foreign keys
// would silently break the cascading deletes the schema relies on.
func Open(ctx context.Context, path string) (*DB, error) {
	if path == "" {
		return nil, fmt.Errorf("sqlite: database path is empty")
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("sqlite: create data directory %q: %w", dir, err)
	}

	// _pragma parameters are applied by the driver to every pooled connection,
	// which matters because PRAGMA settings are per-connection in SQLite.
	dsn := fmt.Sprintf(
		"file:%s?_pragma=foreign_keys(1)&_pragma=busy_timeout(%d)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)",
		path, busyTimeout.Milliseconds(),
	)

	handle, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open %q: %w", path, err)
	}

	handle.SetMaxOpenConns(maxOpenConns)
	handle.SetMaxIdleConns(maxIdleConns)
	handle.SetConnMaxIdleTime(connMaxIdle)

	if err := handle.PingContext(ctx); err != nil {
		_ = handle.Close()
		return nil, fmt.Errorf("sqlite: connect to %q: %w", path, err)
	}

	db := &DB{DB: handle, path: path}

	if err := db.verifyPragmas(ctx); err != nil {
		_ = handle.Close()
		return nil, err
	}

	return db, nil
}

// verifyPragmas confirms the settings actually took effect rather than trusting
// the DSN. A silently ignored pragma would be a correctness bug, not a warning.
func (db *DB) verifyPragmas(ctx context.Context) error {
	var foreignKeys int
	if err := db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		return fmt.Errorf("sqlite: read foreign_keys pragma: %w", err)
	}
	if foreignKeys != 1 {
		return fmt.Errorf("sqlite: foreign_keys pragma is %d, expected 1", foreignKeys)
	}

	var timeout int
	if err := db.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&timeout); err != nil {
		return fmt.Errorf("sqlite: read busy_timeout pragma: %w", err)
	}
	if timeout <= 0 {
		return fmt.Errorf("sqlite: busy_timeout pragma is %d, expected a positive value", timeout)
	}

	// WAL is unavailable on some network filesystems and for in-memory
	// databases. That is acceptable - the mode is a performance choice, not a
	// correctness one - so it is reported rather than treated as fatal.
	var journalMode string
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journalMode); err != nil {
		return fmt.Errorf("sqlite: read journal_mode pragma: %w", err)
	}
	db.journalMode = journalMode

	return nil
}

// JournalMode reports the journal mode the database actually ended up in.
func (db *DB) JournalMode() string { return db.journalMode }
