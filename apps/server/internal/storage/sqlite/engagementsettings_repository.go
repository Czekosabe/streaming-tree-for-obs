package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/streaming-tree/server/internal/domain/engagementsettings"
	"github.com/streaming-tree/server/internal/domain/platform"
)

// EngagementSettingsRepository is the SQLite implementation of
// engagementsettings.Repository.
type EngagementSettingsRepository struct {
	db *sql.DB
}

// NewEngagementSettingsRepository builds a repository over an open database.
func NewEngagementSettingsRepository(db *sql.DB) *EngagementSettingsRepository {
	return &EngagementSettingsRepository{db: db}
}

var _ engagementsettings.Repository = (*EngagementSettingsRepository)(nil)

func engagementSettingsStorageErr(op string, err error) error {
	return fmt.Errorf("%w: %s: %v", engagementsettings.ErrStorage, op, err)
}

const engagementSettingsColumns = `account_id, enabled, created_at, updated_at`

func scanEngagementSettings(scanner interface{ Scan(...any) error }) (engagementsettings.Settings, error) {
	var (
		s         engagementsettings.Settings
		enabled   int
		createdAt string
		updatedAt string
	)
	if err := scanner.Scan(&s.AccountID, &enabled, &createdAt, &updatedAt); err != nil {
		return engagementsettings.Settings{}, err
	}
	s.Enabled = enabled != 0
	var err error
	if s.CreatedAt, err = platform.ParseTimestamp(createdAt); err != nil {
		return engagementsettings.Settings{}, fmt.Errorf("parse created_at %q: %w", createdAt, err)
	}
	if s.UpdatedAt, err = platform.ParseTimestamp(updatedAt); err != nil {
		return engagementsettings.Settings{}, fmt.Errorf("parse updated_at %q: %w", updatedAt, err)
	}
	return s, nil
}

// Get returns the settings for one account, if any.
func (r *EngagementSettingsRepository) Get(ctx context.Context, accountID string) (engagementsettings.Settings, bool, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+engagementSettingsColumns+` FROM connected_account_engagement_settings WHERE account_id = ?`, accountID)
	s, err := scanEngagementSettings(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return engagementsettings.Settings{}, false, nil
		}
		return engagementsettings.Settings{}, false, engagementSettingsStorageErr("get engagement settings", err)
	}
	return s, true, nil
}

// Set creates or replaces one account's settings.
func (r *EngagementSettingsRepository) Set(ctx context.Context, s engagementsettings.Settings, now time.Time) (engagementsettings.Settings, error) {
	nowText := platform.FormatTimestamp(now)
	enabled := 0
	if s.Enabled {
		enabled = 1
	}
	if _, err := r.db.ExecContext(ctx, `
        INSERT INTO connected_account_engagement_settings (account_id, enabled, created_at, updated_at)
        VALUES (?, ?, ?, ?)
        ON CONFLICT (account_id) DO UPDATE SET
            enabled = excluded.enabled, updated_at = excluded.updated_at`,
		s.AccountID, enabled, nowText, nowText,
	); err != nil {
		if isForeignKeyViolation(err) {
			return engagementsettings.Settings{}, fmt.Errorf("%w: connected account does not exist", engagementsettings.ErrStorage)
		}
		return engagementsettings.Settings{}, engagementSettingsStorageErr("set engagement settings", err)
	}

	saved, found, err := r.Get(ctx, s.AccountID)
	if err != nil {
		return engagementsettings.Settings{}, err
	}
	if !found {
		return engagementsettings.Settings{}, engagementSettingsStorageErr("set engagement settings", errors.New("settings missing immediately after write"))
	}
	return saved, nil
}

// Delete removes one account's settings. Deleting an absent row is not an
// error.
func (r *EngagementSettingsRepository) Delete(ctx context.Context, accountID string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM connected_account_engagement_settings WHERE account_id = ?`, accountID); err != nil {
		return engagementSettingsStorageErr("delete engagement settings", err)
	}
	return nil
}

// ListEnabled returns every account currently configured enabled, ordered by
// account_id for a stable, test-friendly order.
func (r *EngagementSettingsRepository) ListEnabled(ctx context.Context) ([]engagementsettings.Settings, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+engagementSettingsColumns+` FROM connected_account_engagement_settings WHERE enabled = 1 ORDER BY account_id`)
	if err != nil {
		return nil, engagementSettingsStorageErr("list enabled engagement settings", err)
	}
	defer rows.Close()

	var out []engagementsettings.Settings
	for rows.Next() {
		s, err := scanEngagementSettings(rows)
		if err != nil {
			return nil, engagementSettingsStorageErr("list enabled engagement settings", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, engagementSettingsStorageErr("list enabled engagement settings", err)
	}
	return out, nil
}
