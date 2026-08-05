package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/streaming-tree/server/internal/domain/platform"
)

// YouTubeRegionRepository persists a connected YouTube account's explicit
// category-region override (youtube_channel_settings) - see
// docs/provider-integrations/youtube.md's "Category region" section. Not a
// secret, not part of the generic account.Repository: this setting exists
// only for YouTube.
type YouTubeRegionRepository struct {
	db *sql.DB
}

// NewYouTubeRegionRepository builds a repository over an open database.
func NewYouTubeRegionRepository(db *sql.DB) *YouTubeRegionRepository {
	return &YouTubeRegionRepository{db: db}
}

// GetRegion returns the account's saved region override, if any.
func (r *YouTubeRegionRepository) GetRegion(ctx context.Context, accountID string) (string, bool, error) {
	var region string
	err := r.db.QueryRowContext(ctx, `SELECT region FROM youtube_channel_settings WHERE account_id = ?`, accountID).Scan(&region)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("get youtube region: %w", err)
	}
	return region, true, nil
}

// SetRegion creates or replaces the account's region override.
func (r *YouTubeRegionRepository) SetRegion(ctx context.Context, accountID, region string, now time.Time) error {
	nowText := platform.FormatTimestamp(now)
	if _, err := r.db.ExecContext(ctx, `
        INSERT INTO youtube_channel_settings (account_id, region, updated_at)
        VALUES (?, ?, ?)
        ON CONFLICT (account_id) DO UPDATE SET region = excluded.region, updated_at = excluded.updated_at`,
		accountID, region, nowText,
	); err != nil {
		if isForeignKeyViolation(err) {
			return fmt.Errorf("set youtube region: connected account does not exist")
		}
		return fmt.Errorf("set youtube region: %w", err)
	}
	return nil
}
