package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"
)

// migrationFiles embeds the SQL files into the binary, so deployment is a
// single executable and no external migration CLI is ever required.
//
//go:embed migrations/*.sql
var migrationFiles embed.FS

// Migration is one ordered schema change.
type Migration struct {
	Version int
	Name    string
	SQL     string
}

// AppliedMigration is a row of the schema_migrations bookkeeping table.
type AppliedMigration struct {
	Version   int
	Name      string
	AppliedAt time.Time
}

const createMigrationsTable = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    INTEGER PRIMARY KEY,
    name       TEXT NOT NULL,
    applied_at TEXT NOT NULL
);`

// LoadMigrations reads and orders the embedded migrations.
//
// Filenames must be "<version>_<name>.sql" with a numeric version, which makes
// the execution order deterministic and independent of filesystem iteration
// order.
func LoadMigrations() ([]Migration, error) {
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return nil, fmt.Errorf("sqlite: read embedded migrations: %w", err)
	}

	migrations := make([]Migration, 0, len(entries))
	seen := make(map[int]string, len(entries))

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		base := strings.TrimSuffix(entry.Name(), ".sql")
		parts := strings.SplitN(base, "_", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("sqlite: migration %q must be named <version>_<name>.sql", entry.Name())
		}

		version, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, fmt.Errorf("sqlite: migration %q has a non-numeric version: %w", entry.Name(), err)
		}
		if previous, duplicate := seen[version]; duplicate {
			return nil, fmt.Errorf("sqlite: migrations %q and %q share version %d", previous, entry.Name(), version)
		}
		seen[version] = entry.Name()

		content, err := migrationFiles.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("sqlite: read migration %q: %w", entry.Name(), err)
		}

		migrations = append(migrations, Migration{
			Version: version,
			Name:    parts[1],
			SQL:     string(content),
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

// AppliedMigrations lists what has already run, oldest first.
func AppliedMigrations(ctx context.Context, db *sql.DB) ([]AppliedMigration, error) {
	if _, err := db.ExecContext(ctx, createMigrationsTable); err != nil {
		return nil, fmt.Errorf("sqlite: ensure schema_migrations: %w", err)
	}

	rows, err := db.QueryContext(ctx,
		`SELECT version, name, applied_at FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: read schema_migrations: %w", err)
	}
	defer rows.Close()

	var applied []AppliedMigration
	for rows.Next() {
		var (
			record    AppliedMigration
			appliedAt string
		)
		if err := rows.Scan(&record.Version, &record.Name, &appliedAt); err != nil {
			return nil, fmt.Errorf("sqlite: scan schema_migrations row: %w", err)
		}
		if parsed, err := time.Parse(time.RFC3339Nano, appliedAt); err == nil {
			record.AppliedAt = parsed
		}
		applied = append(applied, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterate schema_migrations: %w", err)
	}

	return applied, nil
}

// Migrate applies every pending migration in version order and returns the
// versions it applied during this call.
//
// Each migration runs inside its own transaction together with the
// schema_migrations insert, so a failure rolls back the schema change and the
// bookkeeping row alike: a failed migration is never recorded as applied and is
// retried on the next start.
func Migrate(ctx context.Context, db *sql.DB) ([]int, error) {
	migrations, err := LoadMigrations()
	if err != nil {
		return nil, err
	}

	applied, err := AppliedMigrations(ctx, db)
	if err != nil {
		return nil, err
	}

	done := make(map[int]struct{}, len(applied))
	for _, record := range applied {
		done[record.Version] = struct{}{}
	}

	var appliedNow []int

	for _, migration := range migrations {
		if _, already := done[migration.Version]; already {
			continue
		}

		if err := applyMigration(ctx, db, migration); err != nil {
			return appliedNow, err
		}
		appliedNow = append(appliedNow, migration.Version)
	}

	return appliedNow, nil
}

func applyMigration(ctx context.Context, db *sql.DB, migration Migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: begin migration %d: %w", migration.Version, err)
	}
	defer func() {
		// No-op once the transaction has been committed.
		_ = tx.Rollback()
	}()

	if _, err := tx.ExecContext(ctx, migration.SQL); err != nil {
		return fmt.Errorf("sqlite: apply migration %d (%s): %w", migration.Version, migration.Name, err)
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`,
		migration.Version, migration.Name, time.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		return fmt.Errorf("sqlite: record migration %d: %w", migration.Version, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: commit migration %d: %w", migration.Version, err)
	}

	return nil
}
