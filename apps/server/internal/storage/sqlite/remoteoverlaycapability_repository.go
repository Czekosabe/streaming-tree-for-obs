package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/domain/remoteoverlay"
)

// RemoteOverlayCapabilityRepository is the SQLite implementation of
// remoteoverlay.Repository.
type RemoteOverlayCapabilityRepository struct {
	db *sql.DB
}

// NewRemoteOverlayCapabilityRepository builds a repository over an
// open database.
func NewRemoteOverlayCapabilityRepository(db *sql.DB) *RemoteOverlayCapabilityRepository {
	return &RemoteOverlayCapabilityRepository{db: db}
}

var _ remoteoverlay.Repository = (*RemoteOverlayCapabilityRepository)(nil)

func remoteOverlayStorageErr(op string, err error) error {
	return fmt.Errorf("remote overlay capability storage: %s: %w", op, err)
}

// Issue generates a fresh token and atomically replaces any previous
// capability for (domain, localSlug) within one transaction: the old
// row (if any) is deleted, then the new one inserted, so a concurrent
// Resolve of the token being replaced observes either the fully-old
// or fully-new state, never a moment with both.
func (r *RemoteOverlayCapabilityRepository) Issue(ctx context.Context, domain remoteoverlay.Domain, localSlug string) (remoteoverlay.Capability, error) {
	if !domain.IsValid() {
		return remoteoverlay.Capability{}, remoteoverlay.ErrInvalidDomain
	}

	token, err := remoteoverlay.NewToken()
	if err != nil {
		return remoteoverlay.Capability{}, err
	}
	now := time.Now().UTC()

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return remoteoverlay.Capability{}, remoteOverlayStorageErr("begin issue", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM remote_overlay_capabilities WHERE domain = ? AND local_slug = ?`,
		string(domain), localSlug,
	); err != nil {
		return remoteoverlay.Capability{}, remoteOverlayStorageErr("delete previous", err)
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO remote_overlay_capabilities (token, domain, local_slug, created_at) VALUES (?, ?, ?, ?)`,
		token, string(domain), localSlug, platform.FormatTimestamp(now),
	); err != nil {
		return remoteoverlay.Capability{}, remoteOverlayStorageErr("insert", err)
	}

	if err := tx.Commit(); err != nil {
		return remoteoverlay.Capability{}, remoteOverlayStorageErr("commit issue", err)
	}

	return remoteoverlay.Capability{Token: token, Domain: domain, LocalSlug: localSlug, CreatedAt: now}, nil
}

// Revoke removes any capability for (domain, localSlug). Idempotent:
// removing zero rows is not an error.
func (r *RemoteOverlayCapabilityRepository) Revoke(ctx context.Context, domain remoteoverlay.Domain, localSlug string) error {
	if !domain.IsValid() {
		return remoteoverlay.ErrInvalidDomain
	}
	if _, err := r.db.ExecContext(ctx,
		`DELETE FROM remote_overlay_capabilities WHERE domain = ? AND local_slug = ?`,
		string(domain), localSlug,
	); err != nil {
		return remoteOverlayStorageErr("revoke", err)
	}
	return nil
}

// Get returns the current capability for (domain, localSlug), or
// (Capability{}, false, nil) if none is issued.
func (r *RemoteOverlayCapabilityRepository) Get(ctx context.Context, domain remoteoverlay.Domain, localSlug string) (remoteoverlay.Capability, bool, error) {
	if !domain.IsValid() {
		return remoteoverlay.Capability{}, false, remoteoverlay.ErrInvalidDomain
	}
	row := r.db.QueryRowContext(ctx,
		`SELECT token, created_at FROM remote_overlay_capabilities WHERE domain = ? AND local_slug = ?`,
		string(domain), localSlug,
	)
	var token, createdAtText string
	if err := row.Scan(&token, &createdAtText); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return remoteoverlay.Capability{}, false, nil
		}
		return remoteoverlay.Capability{}, false, remoteOverlayStorageErr("get", err)
	}
	createdAt, err := platform.ParseTimestamp(createdAtText)
	if err != nil {
		return remoteoverlay.Capability{}, false, remoteOverlayStorageErr("parse created_at", err)
	}
	return remoteoverlay.Capability{Token: token, Domain: domain, LocalSlug: localSlug, CreatedAt: createdAt}, true, nil
}

// Resolve looks up a presented token and returns the real local slug
// it grants access to for domain, or ("", false, nil) if no capability
// currently matches. The domain must match too, not just the token
// value - a token issued for one domain never resolves under another.
func (r *RemoteOverlayCapabilityRepository) Resolve(ctx context.Context, domain remoteoverlay.Domain, token string) (string, bool, error) {
	if !domain.IsValid() {
		return "", false, remoteoverlay.ErrInvalidDomain
	}
	if token == "" {
		return "", false, nil
	}
	row := r.db.QueryRowContext(ctx,
		`SELECT local_slug FROM remote_overlay_capabilities WHERE domain = ? AND token = ?`,
		string(domain), token,
	)
	var localSlug string
	if err := row.Scan(&localSlug); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, remoteOverlayStorageErr("resolve", err)
	}
	return localSlug, true, nil
}
