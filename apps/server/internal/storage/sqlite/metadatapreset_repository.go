package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"

	"github.com/streaming-tree/server/internal/domain/metadatapreset"
	"github.com/streaming-tree/server/internal/domain/platform"
)

// MetadataPresetRepository is the SQLite implementation of
// metadatapreset.Repository.
type MetadataPresetRepository struct {
	db *sql.DB
}

// NewMetadataPresetRepository builds a repository over an open database.
func NewMetadataPresetRepository(db *sql.DB) *MetadataPresetRepository {
	return &MetadataPresetRepository{db: db}
}

var _ metadatapreset.Repository = (*MetadataPresetRepository)(nil)

func presetStorageErr(op string, err error) error {
	return fmt.Errorf("%w: %s: %v", metadatapreset.ErrStorage, op, err)
}

const presetColumns = `
    id, name, note, title, description, language, visibility,
    mature_content, dvr, latency_mode, created_at, updated_at`

func scanPreset(scanner interface{ Scan(...any) error }) (metadatapreset.Preset, error) {
	var (
		p                    metadatapreset.Preset
		matureContent, dvr   int
		createdAt, updatedAt string
	)
	if err := scanner.Scan(
		&p.ID, &p.Name, &p.Note, &p.Common.Title, &p.Common.Description,
		&p.Common.Language, &p.Common.Visibility, &matureContent, &dvr,
		&p.Common.LatencyMode, &createdAt, &updatedAt,
	); err != nil {
		return metadatapreset.Preset{}, err
	}
	p.Common.MatureContent = matureContent != 0
	p.Common.DVR = dvr != 0
	p.Common.Tags = []string{}
	p.Providers = map[platform.ProviderID]metadatapreset.ProviderMetadata{}

	var err error
	if p.CreatedAt, err = platform.ParseTimestamp(createdAt); err != nil {
		return metadatapreset.Preset{}, fmt.Errorf("parse created_at %q: %w", createdAt, err)
	}
	if p.UpdatedAt, err = platform.ParseTimestamp(updatedAt); err != nil {
		return metadatapreset.Preset{}, fmt.Errorf("parse updated_at %q: %w", updatedAt, err)
	}
	return p, nil
}

// hydrate fills in a preset's Tags and Providers from their own tables -
// split out from the main row scan exactly like PlatformRepository
// hydrates its own tags separately.
func (r *MetadataPresetRepository) hydrate(ctx context.Context, p *metadatapreset.Preset) error {
	tagRows, err := r.db.QueryContext(ctx,
		`SELECT value FROM metadata_preset_tags WHERE preset_id = ? ORDER BY position`, p.ID)
	if err != nil {
		return presetStorageErr("query tags", err)
	}
	defer tagRows.Close()
	for tagRows.Next() {
		var value string
		if err := tagRows.Scan(&value); err != nil {
			return presetStorageErr("scan tag", err)
		}
		p.Common.Tags = append(p.Common.Tags, value)
	}
	if err := tagRows.Err(); err != nil {
		return presetStorageErr("iterate tags", err)
	}

	providerRows, err := r.db.QueryContext(ctx,
		`SELECT provider_id, category, category_id FROM metadata_preset_provider_overrides WHERE preset_id = ?`, p.ID)
	if err != nil {
		return presetStorageErr("query provider overrides", err)
	}
	defer providerRows.Close()
	for providerRows.Next() {
		var providerID, category, categoryID string
		if err := providerRows.Scan(&providerID, &category, &categoryID); err != nil {
			return presetStorageErr("scan provider override", err)
		}
		p.Providers[platform.ProviderID(providerID)] = metadatapreset.ProviderMetadata{
			Category: category, CategoryID: categoryID,
		}
	}
	return providerRows.Err()
}

// List returns every preset, most-recently-updated first.
func (r *MetadataPresetRepository) List(ctx context.Context) ([]metadatapreset.Preset, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+presetColumns+` FROM metadata_presets ORDER BY updated_at DESC, id`)
	if err != nil {
		return nil, presetStorageErr("list", err)
	}
	defer rows.Close()

	presets := make([]metadatapreset.Preset, 0)
	for rows.Next() {
		p, err := scanPreset(rows)
		if err != nil {
			return nil, presetStorageErr("scan", err)
		}
		presets = append(presets, p)
	}
	if err := rows.Err(); err != nil {
		return nil, presetStorageErr("iterate", err)
	}

	for i := range presets {
		if err := r.hydrate(ctx, &presets[i]); err != nil {
			return nil, err
		}
	}
	return presets, nil
}

// Get returns one preset.
func (r *MetadataPresetRepository) Get(ctx context.Context, id string) (metadatapreset.Preset, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+presetColumns+` FROM metadata_presets WHERE id = ?`, id)
	p, err := scanPreset(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return metadatapreset.Preset{}, metadatapreset.ErrNotFound
		}
		return metadatapreset.Preset{}, presetStorageErr("get", err)
	}
	if err := r.hydrate(ctx, &p); err != nil {
		return metadatapreset.Preset{}, err
	}
	return p, nil
}

// Count returns the total number of presets.
func (r *MetadataPresetRepository) Count(ctx context.Context) (int, error) {
	var count int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM metadata_presets`).Scan(&count); err != nil {
		return 0, presetStorageErr("count", err)
	}
	return count, nil
}

// Create inserts a new preset, its tags, and its provider overrides
// atomically.
func (r *MetadataPresetRepository) Create(ctx context.Context, p metadatapreset.Preset) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return presetStorageErr("begin create", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
        INSERT INTO metadata_presets
            (id, name, note, title, description, language, visibility, mature_content, dvr, latency_mode, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.Note, p.Common.Title, p.Common.Description, p.Common.Language, p.Common.Visibility,
		boolToInt(p.Common.MatureContent), boolToInt(p.Common.DVR), p.Common.LatencyMode,
		platform.FormatTimestamp(p.CreatedAt), platform.FormatTimestamp(p.UpdatedAt),
	); err != nil {
		if isUniqueViolation(err) {
			return metadatapreset.ErrDuplicateName
		}
		return presetStorageErr("insert preset", err)
	}

	if err := insertPresetTags(ctx, tx, p.ID, p.Common.Tags); err != nil {
		return err
	}
	if err := insertPresetProviderOverrides(ctx, tx, p.ID, p.Providers); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return presetStorageErr("commit create", err)
	}
	return nil
}

// Update replaces a preset's mutable fields, tags, and provider
// overrides in full, atomically.
func (r *MetadataPresetRepository) Update(ctx context.Context, id string, input metadatapreset.UpdateInput, updatedAt string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return presetStorageErr("begin update", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, `
        UPDATE metadata_presets SET
            name = ?, note = ?, title = ?, description = ?, language = ?, visibility = ?,
            mature_content = ?, dvr = ?, latency_mode = ?, updated_at = ?
        WHERE id = ?`,
		input.Name, input.Note, input.Common.Title, input.Common.Description, input.Common.Language,
		input.Common.Visibility, boolToInt(input.Common.MatureContent), boolToInt(input.Common.DVR),
		input.Common.LatencyMode, updatedAt, id,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return metadatapreset.ErrDuplicateName
		}
		return presetStorageErr("update preset", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return presetStorageErr("update preset rows affected", err)
	}
	if affected == 0 {
		return metadatapreset.ErrNotFound
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM metadata_preset_tags WHERE preset_id = ?`, id); err != nil {
		return presetStorageErr("clear tags", err)
	}
	if err := insertPresetTags(ctx, tx, id, input.Common.Tags); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM metadata_preset_provider_overrides WHERE preset_id = ?`, id); err != nil {
		return presetStorageErr("clear provider overrides", err)
	}
	if err := insertPresetProviderOverrides(ctx, tx, id, input.Providers); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return presetStorageErr("commit update", err)
	}
	return nil
}

// Delete removes a preset together with its tags and provider
// overrides (ON DELETE CASCADE). Never touches any destination's own
// metadata.
func (r *MetadataPresetRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM metadata_presets WHERE id = ?`, id)
	if err != nil {
		return presetStorageErr("delete", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return presetStorageErr("delete rows affected", err)
	}
	if affected == 0 {
		return metadatapreset.ErrNotFound
	}
	return nil
}

func insertPresetTags(ctx context.Context, tx *sql.Tx, presetID string, tags []string) error {
	for position, value := range tags {
		if _, err := tx.ExecContext(ctx, `
            INSERT INTO metadata_preset_tags (preset_id, position, value)
            VALUES (?, ?, ?)`, presetID, position, value,
		); err != nil {
			return presetStorageErr("insert tag", err)
		}
	}
	return nil
}

func insertPresetProviderOverrides(ctx context.Context, tx *sql.Tx, presetID string, providers map[platform.ProviderID]metadatapreset.ProviderMetadata) error {
	// Deterministic order, purely so repeated writes of the same
	// logical content produce identical statement sequences - not
	// required for correctness, but makes captured /LOG-style
	// diagnosis easier if it is ever needed.
	ids := make([]string, 0, len(providers))
	for id := range providers {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)

	for _, id := range ids {
		pm := providers[platform.ProviderID(id)]
		if _, err := tx.ExecContext(ctx, `
            INSERT INTO metadata_preset_provider_overrides (preset_id, provider_id, category, category_id)
            VALUES (?, ?, ?, ?)`, presetID, id, pm.Category, pm.CategoryID,
		); err != nil {
			return presetStorageErr("insert provider override", err)
		}
	}
	return nil
}
