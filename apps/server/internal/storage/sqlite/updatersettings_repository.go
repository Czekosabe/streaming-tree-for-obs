package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/domain/updatersettings"
)

// UpdateSettingsRepository is the SQLite implementation of
// updatersettings.Repository.
type UpdateSettingsRepository struct {
	db *sql.DB
}

// NewUpdateSettingsRepository builds a repository over an open database.
func NewUpdateSettingsRepository(db *sql.DB) *UpdateSettingsRepository {
	return &UpdateSettingsRepository{db: db}
}

var _ updatersettings.Repository = (*UpdateSettingsRepository)(nil)

func updateSettingsStorageErr(op string, err error) error {
	return fmt.Errorf("%w: %s: %v", updatersettings.ErrStorage, op, err)
}

func scanUpdatePreferences(scanner interface{ Scan(...any) error }) (updatersettings.Preferences, error) {
	var (
		p                    updatersettings.Preferences
		autoCheck            int
		createdAt, updatedAt string
	)
	if err := scanner.Scan(&autoCheck, &createdAt, &updatedAt); err != nil {
		return updatersettings.Preferences{}, err
	}
	p.AutoCheck = autoCheck != 0
	var err error
	if p.CreatedAt, err = platform.ParseTimestamp(createdAt); err != nil {
		return updatersettings.Preferences{}, fmt.Errorf("parse created_at %q: %w", createdAt, err)
	}
	if p.UpdatedAt, err = platform.ParseTimestamp(updatedAt); err != nil {
		return updatersettings.Preferences{}, fmt.Errorf("parse updated_at %q: %w", updatedAt, err)
	}
	return p, nil
}

// GetPreferences returns the singleton preferences row, if any.
func (r *UpdateSettingsRepository) GetPreferences(ctx context.Context) (updatersettings.Preferences, bool, error) {
	row := r.db.QueryRowContext(ctx, `SELECT auto_check, created_at, updated_at FROM update_preferences WHERE id = 1`)
	p, err := scanUpdatePreferences(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return updatersettings.Preferences{}, false, nil
		}
		return updatersettings.Preferences{}, false, updateSettingsStorageErr("get preferences", err)
	}
	return p, true, nil
}

// SetPreferences replaces the singleton preferences row in full.
func (r *UpdateSettingsRepository) SetPreferences(ctx context.Context, p updatersettings.Preferences, now time.Time) (updatersettings.Preferences, error) {
	nowText := platform.FormatTimestamp(now)
	if _, err := r.db.ExecContext(ctx, `
        INSERT INTO update_preferences (id, auto_check, created_at, updated_at)
        VALUES (1, ?, ?, ?)
        ON CONFLICT (id) DO UPDATE SET
            auto_check = excluded.auto_check,
            updated_at = excluded.updated_at`,
		boolToInt(p.AutoCheck), nowText, nowText,
	); err != nil {
		return updatersettings.Preferences{}, updateSettingsStorageErr("set preferences", err)
	}

	saved, found, err := r.GetPreferences(ctx)
	if err != nil {
		return updatersettings.Preferences{}, err
	}
	if !found {
		return updatersettings.Preferences{}, updateSettingsStorageErr("set preferences", errors.New("preferences missing immediately after write"))
	}
	return saved, nil
}
