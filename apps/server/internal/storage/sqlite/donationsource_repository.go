package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/streaming-tree/server/internal/domain/donationsource"
	"github.com/streaming-tree/server/internal/domain/platform"
)

// DonationSourceRepository is the SQLite implementation of
// donationsource.Repository.
type DonationSourceRepository struct {
	db *sql.DB
}

// NewDonationSourceRepository builds a repository over an open database.
func NewDonationSourceRepository(db *sql.DB) *DonationSourceRepository {
	return &DonationSourceRepository{db: db}
}

var _ donationsource.Repository = (*DonationSourceRepository)(nil)

func donationSourceStorageErr(op string, err error) error {
	return fmt.Errorf("%w: %s: %v", donationsource.ErrStorage, op, err)
}

const donationSourceColumns = `id, provider_id, label, enabled, remote_channel_id, created_at, updated_at`

func scanDonationSource(scanner interface{ Scan(...any) error }) (donationsource.Source, error) {
	var (
		src             donationsource.Source
		providerID      string
		enabled         int
		remoteChannelID sql.NullString
		createdAt       string
		updatedAt       string
	)
	if err := scanner.Scan(&src.ID, &providerID, &src.Label, &enabled, &remoteChannelID, &createdAt, &updatedAt); err != nil {
		return donationsource.Source{}, err
	}
	src.ProviderID = donationsource.ProviderID(providerID)
	src.Enabled = enabled != 0
	src.RemoteChannelID = remoteChannelID.String
	var err error
	if src.CreatedAt, err = platform.ParseTimestamp(createdAt); err != nil {
		return donationsource.Source{}, fmt.Errorf("parse created_at %q: %w", createdAt, err)
	}
	if src.UpdatedAt, err = platform.ParseTimestamp(updatedAt); err != nil {
		return donationsource.Source{}, fmt.Errorf("parse updated_at %q: %w", updatedAt, err)
	}
	return src, nil
}

// GetSource returns one source, or ErrNotFound.
func (r *DonationSourceRepository) GetSource(ctx context.Context, id string) (donationsource.Source, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+donationSourceColumns+` FROM donation_sources WHERE id = ?`, id)
	src, err := scanDonationSource(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return donationsource.Source{}, donationsource.ErrNotFound
		}
		return donationsource.Source{}, donationSourceStorageErr("get donation source", err)
	}
	return src, nil
}

// ListSources returns every donation source, ordered by creation time.
func (r *DonationSourceRepository) ListSources(ctx context.Context) ([]donationsource.Source, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+donationSourceColumns+` FROM donation_sources ORDER BY created_at, id`)
	if err != nil {
		return nil, donationSourceStorageErr("list donation sources", err)
	}
	defer rows.Close()

	var out []donationsource.Source
	for rows.Next() {
		src, err := scanDonationSource(rows)
		if err != nil {
			return nil, donationSourceStorageErr("list donation sources", err)
		}
		out = append(out, src)
	}
	if err := rows.Err(); err != nil {
		return nil, donationSourceStorageErr("list donation sources", err)
	}
	return out, nil
}

// CreateSource inserts a new source.
func (r *DonationSourceRepository) CreateSource(ctx context.Context, src donationsource.Source) error {
	enabled := 0
	if src.Enabled {
		enabled = 1
	}
	_, err := r.db.ExecContext(ctx, `
        INSERT INTO donation_sources (id, provider_id, label, enabled, remote_channel_id, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?)`,
		src.ID, string(src.ProviderID), src.Label, enabled, src.RemoteChannelID,
		platform.FormatTimestamp(src.CreatedAt), platform.FormatTimestamp(src.UpdatedAt),
	)
	if err != nil {
		return donationSourceStorageErr("create donation source", err)
	}
	return nil
}

// UpdateSource replaces an existing source's mutable fields.
func (r *DonationSourceRepository) UpdateSource(ctx context.Context, src donationsource.Source) error {
	enabled := 0
	if src.Enabled {
		enabled = 1
	}
	result, err := r.db.ExecContext(ctx, `
        UPDATE donation_sources
        SET label = ?, enabled = ?, remote_channel_id = ?, updated_at = ?
        WHERE id = ?`,
		src.Label, enabled, src.RemoteChannelID, platform.FormatTimestamp(src.UpdatedAt), src.ID,
	)
	if err != nil {
		return donationSourceStorageErr("update donation source", err)
	}
	if rows, err := result.RowsAffected(); err == nil && rows == 0 {
		return donationsource.ErrNotFound
	}
	return nil
}

// DeleteSource removes the source row.
func (r *DonationSourceRepository) DeleteSource(ctx context.Context, id string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM donation_sources WHERE id = ?`, id); err != nil {
		return donationSourceStorageErr("delete donation source", err)
	}
	return nil
}
