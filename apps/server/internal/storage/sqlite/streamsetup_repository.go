package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/domain/streamsetup"
)

// StreamSetupProfileRepository is the SQLite implementation of
// streamsetup.Repository.
type StreamSetupProfileRepository struct {
	db *sql.DB
}

// NewStreamSetupProfileRepository builds a repository over an open
// database.
func NewStreamSetupProfileRepository(db *sql.DB) *StreamSetupProfileRepository {
	return &StreamSetupProfileRepository{db: db}
}

var _ streamsetup.Repository = (*StreamSetupProfileRepository)(nil)

func streamSetupStorageErr(op string, err error) error {
	return fmt.Errorf("stream setup profile storage failure: %s: %w", op, err)
}

const streamSetupProfileColumns = `id, name, note, metadata_preset_id, metadata_preset_name, created_at, updated_at`

func scanStreamSetupProfile(scanner interface{ Scan(...any) error }) (streamsetup.Profile, error) {
	var (
		p                    streamsetup.Profile
		metadataPresetID     sql.NullString
		createdAt, updatedAt string
	)
	if err := scanner.Scan(&p.ID, &p.Name, &p.Note, &metadataPresetID, &p.MetadataPresetName, &createdAt, &updatedAt); err != nil {
		return streamsetup.Profile{}, err
	}
	if metadataPresetID.Valid {
		v := metadataPresetID.String
		p.MetadataPresetID = &v
	}

	var err error
	if p.CreatedAt, err = platform.ParseTimestamp(createdAt); err != nil {
		return streamsetup.Profile{}, fmt.Errorf("parse created_at %q: %w", createdAt, err)
	}
	if p.UpdatedAt, err = platform.ParseTimestamp(updatedAt); err != nil {
		return streamsetup.Profile{}, fmt.Errorf("parse updated_at %q: %w", updatedAt, err)
	}
	return p, nil
}

func (r *StreamSetupProfileRepository) hydrateDestinations(ctx context.Context, p *streamsetup.Profile) error {
	rows, err := r.db.QueryContext(ctx,
		`SELECT platform_id, provider_id, display_name FROM stream_setup_profile_destinations WHERE profile_id = ? ORDER BY position`, p.ID)
	if err != nil {
		return streamSetupStorageErr("query destinations", err)
	}
	defer rows.Close()

	for rows.Next() {
		var platformID sql.NullString
		var d streamsetup.Destination
		if err := rows.Scan(&platformID, &d.ProviderID, &d.DisplayName); err != nil {
			return streamSetupStorageErr("scan destination", err)
		}
		if platformID.Valid {
			v := platformID.String
			d.PlatformID = &v
		}
		p.Destinations = append(p.Destinations, d)
	}
	return rows.Err()
}

func (r *StreamSetupProfileRepository) List(ctx context.Context) ([]streamsetup.Profile, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+streamSetupProfileColumns+` FROM stream_setup_profiles ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, streamSetupStorageErr("list", err)
	}
	defer rows.Close()

	profiles := make([]streamsetup.Profile, 0)
	for rows.Next() {
		p, err := scanStreamSetupProfile(rows)
		if err != nil {
			return nil, streamSetupStorageErr("scan", err)
		}
		profiles = append(profiles, p)
	}
	if err := rows.Err(); err != nil {
		return nil, streamSetupStorageErr("iterate", err)
	}

	for i := range profiles {
		if err := r.hydrateDestinations(ctx, &profiles[i]); err != nil {
			return nil, err
		}
	}
	return profiles, nil
}

func (r *StreamSetupProfileRepository) Get(ctx context.Context, id string) (streamsetup.Profile, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+streamSetupProfileColumns+` FROM stream_setup_profiles WHERE id = ?`, id)
	p, err := scanStreamSetupProfile(row)
	if errors.Is(err, sql.ErrNoRows) {
		return streamsetup.Profile{}, streamsetup.ErrNotFound
	}
	if err != nil {
		return streamsetup.Profile{}, streamSetupStorageErr("get", err)
	}
	if err := r.hydrateDestinations(ctx, &p); err != nil {
		return streamsetup.Profile{}, err
	}
	return p, nil
}

func insertStreamSetupDestinations(ctx context.Context, tx *sql.Tx, profileID string, dests []streamsetup.Destination) error {
	for position, d := range dests {
		var platformID sql.NullString
		if d.PlatformID != nil {
			platformID = sql.NullString{String: *d.PlatformID, Valid: true}
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO stream_setup_profile_destinations (profile_id, position, platform_id, provider_id, display_name)
			VALUES (?, ?, ?, ?, ?)`,
			profileID, position, platformID, d.ProviderID, d.DisplayName,
		); err != nil {
			return streamSetupStorageErr("insert destination", err)
		}
	}
	return nil
}

func (r *StreamSetupProfileRepository) Create(ctx context.Context, p streamsetup.Profile) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return streamSetupStorageErr("begin create", err)
	}
	defer func() { _ = tx.Rollback() }()

	var metadataPresetID sql.NullString
	if p.MetadataPresetID != nil {
		metadataPresetID = sql.NullString{String: *p.MetadataPresetID, Valid: true}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO stream_setup_profiles (`+streamSetupProfileColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.Note, metadataPresetID, p.MetadataPresetName,
		platform.FormatTimestamp(p.CreatedAt), platform.FormatTimestamp(p.UpdatedAt),
	); err != nil {
		if isUniqueViolation(err) {
			return streamsetup.ErrDuplicateName
		}
		return streamSetupStorageErr("insert profile", err)
	}

	if err := insertStreamSetupDestinations(ctx, tx, p.ID, p.Destinations); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return streamSetupStorageErr("commit create", err)
	}
	return nil
}

func (r *StreamSetupProfileRepository) Update(ctx context.Context, p streamsetup.Profile) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return streamSetupStorageErr("begin update", err)
	}
	defer func() { _ = tx.Rollback() }()

	var metadataPresetID sql.NullString
	if p.MetadataPresetID != nil {
		metadataPresetID = sql.NullString{String: *p.MetadataPresetID, Valid: true}
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE stream_setup_profiles
		SET name = ?, note = ?, metadata_preset_id = ?, metadata_preset_name = ?, updated_at = ?
		WHERE id = ?`,
		p.Name, p.Note, metadataPresetID, p.MetadataPresetName, platform.FormatTimestamp(p.UpdatedAt), p.ID,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return streamsetup.ErrDuplicateName
		}
		return streamSetupStorageErr("update profile", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return streamSetupStorageErr("update profile rows", err)
	}
	if affected == 0 {
		return streamsetup.ErrNotFound
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM stream_setup_profile_destinations WHERE profile_id = ?`, p.ID); err != nil {
		return streamSetupStorageErr("clear destinations", err)
	}
	if err := insertStreamSetupDestinations(ctx, tx, p.ID, p.Destinations); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return streamSetupStorageErr("commit update", err)
	}
	return nil
}

func (r *StreamSetupProfileRepository) Count(ctx context.Context) (int, error) {
	var count int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM stream_setup_profiles`).Scan(&count); err != nil {
		return 0, streamSetupStorageErr("count", err)
	}
	return count, nil
}

func (r *StreamSetupProfileRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM stream_setup_profiles WHERE id = ?`, id)
	if err != nil {
		return streamSetupStorageErr("delete", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return streamSetupStorageErr("delete rows", err)
	}
	if affected == 0 {
		return streamsetup.ErrNotFound
	}
	return nil
}
