package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/streaming-tree/server/internal/domain/output"
	"github.com/streaming-tree/server/internal/domain/platform"
)

// OutputRepository is the SQLite implementation of output.Repository.
type OutputRepository struct {
	db *sql.DB
}

// NewOutputRepository builds a repository over an open database.
func NewOutputRepository(db *sql.DB) *OutputRepository {
	return &OutputRepository{db: db}
}

var _ output.Repository = (*OutputRepository)(nil)

func outputStorageErr(op string, err error) error {
	return fmt.Errorf("%w: %s: %v", output.ErrStorage, op, err)
}

func scanOutputSettings(scanner interface{ Scan(...any) error }) (output.Settings, error) {
	var (
		serverURL   string
		autoRestart int
		updatedAt   string
	)

	if err := scanner.Scan(&serverURL, &autoRestart, &updatedAt); err != nil {
		return output.Settings{}, err
	}

	parsed, err := platform.ParseTimestamp(updatedAt)
	if err != nil {
		return output.Settings{}, fmt.Errorf("parse updated_at %q: %w", updatedAt, err)
	}

	return output.Settings{
		ServerURL:   serverURL,
		AutoRestart: autoRestart != 0,
		UpdatedAt:   parsed,
	}, nil
}

// Get returns one platform's output settings.
func (r *OutputRepository) Get(ctx context.Context, platformID string) (output.Settings, error) {
	row := r.db.QueryRowContext(ctx, `
        SELECT server_url, auto_restart, updated_at
        FROM platform_output_settings
        WHERE platform_id = ?`, platformID)

	settings, err := scanOutputSettings(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return output.Settings{}, output.ErrNotFound
		}
		return output.Settings{}, outputStorageErr("get output settings", err)
	}
	return settings, nil
}

// Update replaces the mutable fields of one platform's output settings.
func (r *OutputRepository) Update(
	ctx context.Context, platformID string, input output.UpdateInput, updatedAt string,
) (output.Settings, error) {
	result, err := r.db.ExecContext(ctx, `
        UPDATE platform_output_settings
        SET server_url = ?, auto_restart = ?, updated_at = ?
        WHERE platform_id = ?`,
		input.ServerURL, boolToInt(input.AutoRestart), updatedAt, platformID)
	if err != nil {
		return output.Settings{}, outputStorageErr("update output settings", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return output.Settings{}, outputStorageErr("update output settings rows", err)
	}
	if affected == 0 {
		return output.Settings{}, output.ErrNotFound
	}

	return r.Get(ctx, platformID)
}

// insertDefaultOutputSettingsRow seeds an unconfigured settings row for a
// newly created platform, in the same transaction as the platform and its
// metadata row - see PlatformRepository.Create. This is the SQLite-layer
// detail that makes "every new platform gets output settings transactionally"
// true without the platform domain package needing to know that output
// settings, a concept from a different domain package, exist at all.
func insertDefaultOutputSettingsRow(ctx context.Context, ex execer, platformID, createdAt string) error {
	if _, err := ex.ExecContext(ctx, `
        INSERT INTO platform_output_settings (platform_id, server_url, auto_restart, created_at, updated_at)
        VALUES (?, '', 1, ?, ?)`,
		platformID, createdAt, createdAt,
	); err != nil {
		return outputStorageErr("insert default output settings", err)
	}
	return nil
}
