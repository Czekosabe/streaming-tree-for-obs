package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/domain/remotetarget"
)

// RemoteTargetRepository is the SQLite implementation of
// remotetarget.Repository.
type RemoteTargetRepository struct {
	db *sql.DB
}

// NewRemoteTargetRepository builds a repository over an open database.
func NewRemoteTargetRepository(db *sql.DB) *RemoteTargetRepository {
	return &RemoteTargetRepository{db: db}
}

var _ remotetarget.Repository = (*RemoteTargetRepository)(nil)

func remoteTargetStorageErr(op string, err error) error {
	return fmt.Errorf("%w: %s: %v", remotetarget.ErrStorage, op, err)
}

const remoteTargetColumns = `platform_id, provider_id, resource_type, resource_id, display_name, created_at, updated_at`

func scanRemoteTarget(scanner interface{ Scan(...any) error }) (remotetarget.Target, error) {
	var (
		t         remotetarget.Target
		createdAt string
		updatedAt string
	)
	if err := scanner.Scan(&t.PlatformID, &t.ProviderID, &t.ResourceType, &t.ResourceID, &t.DisplayName, &createdAt, &updatedAt); err != nil {
		return remotetarget.Target{}, err
	}
	var err error
	if t.CreatedAt, err = platform.ParseTimestamp(createdAt); err != nil {
		return remotetarget.Target{}, fmt.Errorf("parse created_at %q: %w", createdAt, err)
	}
	if t.UpdatedAt, err = platform.ParseTimestamp(updatedAt); err != nil {
		return remotetarget.Target{}, fmt.Errorf("parse updated_at %q: %w", updatedAt, err)
	}
	return t, nil
}

// Get returns the target set for a platform, if any.
func (r *RemoteTargetRepository) Get(ctx context.Context, platformID string) (remotetarget.Target, bool, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+remoteTargetColumns+` FROM platform_remote_targets WHERE platform_id = ?`, platformID)
	t, err := scanRemoteTarget(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return remotetarget.Target{}, false, nil
		}
		return remotetarget.Target{}, false, remoteTargetStorageErr("get remote target", err)
	}
	return t, true, nil
}

// Set creates or replaces a platform's target. "INSERT ... ON CONFLICT" is
// used rather than a delete-then-insert, mirroring AccountRepository.SetLink's
// own reasoning: a replace is one statement, and there is no window where
// the platform briefly has no target.
func (r *RemoteTargetRepository) Set(ctx context.Context, t remotetarget.Target, now time.Time) (remotetarget.Target, error) {
	nowText := platform.FormatTimestamp(now)
	if _, err := r.db.ExecContext(ctx, `
        INSERT INTO platform_remote_targets (platform_id, provider_id, resource_type, resource_id, display_name, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT (platform_id) DO UPDATE SET
            provider_id = excluded.provider_id, resource_type = excluded.resource_type,
            resource_id = excluded.resource_id, display_name = excluded.display_name, updated_at = excluded.updated_at`,
		t.PlatformID, t.ProviderID, t.ResourceType, t.ResourceID, t.DisplayName, nowText, nowText,
	); err != nil {
		if isForeignKeyViolation(err) {
			return remotetarget.Target{}, fmt.Errorf("%w: platform does not exist", remotetarget.ErrStorage)
		}
		return remotetarget.Target{}, remoteTargetStorageErr("set remote target", err)
	}

	saved, found, err := r.Get(ctx, t.PlatformID)
	if err != nil {
		return remotetarget.Target{}, err
	}
	if !found {
		return remotetarget.Target{}, remoteTargetStorageErr("set remote target", errors.New("target missing immediately after write"))
	}
	return saved, nil
}

// Delete removes a platform's target. Deleting an absent target is not an
// error.
func (r *RemoteTargetRepository) Delete(ctx context.Context, platformID string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM platform_remote_targets WHERE platform_id = ?`, platformID); err != nil {
		return remoteTargetStorageErr("delete remote target", err)
	}
	return nil
}
