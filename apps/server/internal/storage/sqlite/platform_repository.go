package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/streaming-tree/server/internal/domain/platform"
)

// PlatformRepository is the SQLite implementation of platform.Repository.
//
// Every exported method converts driver errors into the domain sentinels, so no
// SQLite text ever reaches an HTTP response.
type PlatformRepository struct {
	db *sql.DB
}

// NewPlatformRepository builds a repository over an open database.
func NewPlatformRepository(db *sql.DB) *PlatformRepository {
	return &PlatformRepository{db: db}
}

var _ platform.Repository = (*PlatformRepository)(nil)

// storageErr wraps an unexpected driver failure, keeping the detail for logs
// while giving callers a sentinel to match on.
func storageErr(op string, err error) error {
	return fmt.Errorf("%w: %s: %v", platform.ErrStorage, op, err)
}

const platformColumns = `
    p.id, p.provider_id, p.display_name, p.enabled, p.sort_order, p.created_at, p.updated_at,
    m.title, m.description, m.category, m.language, m.visibility,
    m.mature_content, m.dvr, m.latency_mode, m.updated_at`

// scanPlatform reads one joined platforms + platform_metadata row.
//
// The metadata columns are nullable both because a provider may not support a
// field and because the LEFT JOIN can miss the row entirely; both collapse to
// the zero value.
func scanPlatform(scanner interface{ Scan(...any) error }) (platform.Platform, error) {
	var (
		p                 platform.Platform
		providerID        string
		enabled           int
		createdAt         string
		updatedAt         string
		title             sql.NullString
		description       sql.NullString
		category          sql.NullString
		language          sql.NullString
		visibility        sql.NullString
		matureContent     sql.NullInt64
		dvr               sql.NullInt64
		latencyMode       sql.NullString
		metadataUpdatedAt sql.NullString
	)

	if err := scanner.Scan(
		&p.ID, &providerID, &p.DisplayName, &enabled, &p.SortOrder, &createdAt, &updatedAt,
		&title, &description, &category, &language, &visibility,
		&matureContent, &dvr, &latencyMode, &metadataUpdatedAt,
	); err != nil {
		return platform.Platform{}, err
	}

	p.ProviderID = platform.ProviderID(providerID)
	p.Enabled = enabled != 0

	var err error
	if p.CreatedAt, err = platform.ParseTimestamp(createdAt); err != nil {
		return platform.Platform{}, fmt.Errorf("parse created_at %q: %w", createdAt, err)
	}
	if p.UpdatedAt, err = platform.ParseTimestamp(updatedAt); err != nil {
		return platform.Platform{}, fmt.Errorf("parse updated_at %q: %w", updatedAt, err)
	}

	p.Metadata = platform.Metadata{
		Title:         title.String,
		Description:   description.String,
		Category:      category.String,
		Language:      language.String,
		Visibility:    visibility.String,
		MatureContent: matureContent.Valid && matureContent.Int64 != 0,
		DVR:           dvr.Valid && dvr.Int64 != 0,
		LatencyMode:   latencyMode.String,
		Tags:          []string{},
	}

	if metadataUpdatedAt.Valid {
		if parsed, err := platform.ParseTimestamp(metadataUpdatedAt.String); err == nil {
			p.Metadata.UpdatedAt = parsed
		}
	}

	return p, nil
}

// List returns every configured platform with its tags.
//
// Tags are fetched with one extra query for all platforms rather than one per
// platform, so the dashboard costs two queries regardless of how many
// destinations exist.
func (r *PlatformRepository) List(ctx context.Context) ([]platform.Platform, error) {
	rows, err := r.db.QueryContext(ctx, `
        SELECT `+platformColumns+`
        FROM platforms p
        LEFT JOIN platform_metadata m ON m.platform_id = p.id
        ORDER BY p.sort_order, p.created_at, p.id`)
	if err != nil {
		return nil, storageErr("list platforms", err)
	}
	defer rows.Close()

	platforms := make([]platform.Platform, 0, 8)
	index := make(map[string]int, 8)

	for rows.Next() {
		p, err := scanPlatform(rows)
		if err != nil {
			return nil, storageErr("scan platform", err)
		}
		index[p.ID] = len(platforms)
		platforms = append(platforms, p)
	}
	if err := rows.Err(); err != nil {
		return nil, storageErr("iterate platforms", err)
	}

	if len(platforms) == 0 {
		return platforms, nil
	}

	tagRows, err := r.db.QueryContext(ctx, `
        SELECT platform_id, value
        FROM platform_metadata_tags
        ORDER BY platform_id, position`)
	if err != nil {
		return nil, storageErr("list tags", err)
	}
	defer tagRows.Close()

	for tagRows.Next() {
		var platformID, value string
		if err := tagRows.Scan(&platformID, &value); err != nil {
			return nil, storageErr("scan tag", err)
		}
		if position, ok := index[platformID]; ok {
			platforms[position].Metadata.Tags = append(platforms[position].Metadata.Tags, value)
		}
	}
	if err := tagRows.Err(); err != nil {
		return nil, storageErr("iterate tags", err)
	}

	return platforms, nil
}

// Get returns one configured platform with its ordered tags.
func (r *PlatformRepository) Get(ctx context.Context, id string) (platform.Platform, error) {
	row := r.db.QueryRowContext(ctx, `
        SELECT `+platformColumns+`
        FROM platforms p
        LEFT JOIN platform_metadata m ON m.platform_id = p.id
        WHERE p.id = ?`, id)

	p, err := scanPlatform(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return platform.Platform{}, platform.ErrNotFound
		}
		return platform.Platform{}, storageErr("get platform", err)
	}

	tags, err := r.tags(ctx, r.db, id)
	if err != nil {
		return platform.Platform{}, err
	}
	p.Metadata.Tags = tags

	return p, nil
}

type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func (r *PlatformRepository) tags(ctx context.Context, q queryer, platformID string) ([]string, error) {
	rows, err := q.QueryContext(ctx, `
        SELECT value FROM platform_metadata_tags
        WHERE platform_id = ?
        ORDER BY position`, platformID)
	if err != nil {
		return nil, storageErr("read tags", err)
	}
	defer rows.Close()

	tags := []string{}
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, storageErr("scan tag", err)
		}
		tags = append(tags, value)
	}
	if err := rows.Err(); err != nil {
		return nil, storageErr("iterate tags", err)
	}

	return tags, nil
}

// Create inserts the platform and its initial metadata row in one transaction,
// so a platform can never exist without exactly one metadata record.
func (r *PlatformRepository) Create(ctx context.Context, p platform.Platform) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return storageErr("begin create", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
        INSERT INTO platforms (id, provider_id, display_name, enabled, sort_order, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?)`,
		p.ID, string(p.ProviderID), p.DisplayName, boolToInt(p.Enabled), p.SortOrder,
		platform.FormatTimestamp(p.CreatedAt), platform.FormatTimestamp(p.UpdatedAt),
	); err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: platform %s already exists", platform.ErrConflict, p.ID)
		}
		return storageErr("insert platform", err)
	}

	def, ok := platform.Definition(p.ProviderID)
	if !ok {
		return fmt.Errorf("%w: unknown provider %q", platform.ErrUnknownProvider, p.ProviderID)
	}

	if err := insertMetadataRow(ctx, tx, p.ID, def, p.Metadata); err != nil {
		return err
	}

	// Every platform also gets a default (unconfigured) output-settings row in
	// the same transaction, so it is never possible to observe a platform that
	// exists without one - see internal/domain/output.
	if err := insertDefaultOutputSettingsRow(ctx, tx, p.ID, platform.FormatTimestamp(p.CreatedAt)); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return storageErr("commit create", err)
	}
	return nil
}

// Update replaces the mutable configuration fields.
func (r *PlatformRepository) Update(
	ctx context.Context, id string, input platform.UpdateInput, updatedAt string,
) error {
	result, err := r.db.ExecContext(ctx, `
        UPDATE platforms
        SET display_name = ?, enabled = ?, sort_order = ?, updated_at = ?
        WHERE id = ?`,
		input.DisplayName, boolToInt(input.Enabled), input.SortOrder, updatedAt, id)
	if err != nil {
		return storageErr("update platform", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return storageErr("update platform rows", err)
	}
	if affected == 0 {
		return platform.ErrNotFound
	}
	return nil
}

// Delete removes the platform; metadata and tags cascade via foreign keys.
func (r *PlatformRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM platforms WHERE id = ?`, id)
	if err != nil {
		return storageErr("delete platform", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return storageErr("delete platform rows", err)
	}
	if affected == 0 {
		return platform.ErrNotFound
	}
	return nil
}

// SaveMetadata replaces the metadata row and the entire tag list atomically:
// either the new metadata and the new tag order both land, or neither does.
func (r *PlatformRepository) SaveMetadata(
	ctx context.Context, platformID string, m platform.Metadata,
) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return storageErr("begin save metadata", err)
	}
	defer func() { _ = tx.Rollback() }()

	var providerID string
	if err := tx.QueryRowContext(ctx,
		`SELECT provider_id FROM platforms WHERE id = ?`, platformID,
	).Scan(&providerID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return platform.ErrNotFound
		}
		return storageErr("read provider for metadata", err)
	}

	def, ok := platform.Definition(platform.ProviderID(providerID))
	if !ok {
		return fmt.Errorf("%w: unknown provider %q", platform.ErrUnknownProvider, providerID)
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM platform_metadata WHERE platform_id = ?`, platformID); err != nil {
		return storageErr("clear metadata", err)
	}
	if err := insertMetadataRow(ctx, tx, platformID, def, m); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM platform_metadata_tags WHERE platform_id = ?`, platformID); err != nil {
		return storageErr("clear tags", err)
	}

	for position, value := range m.Tags {
		if _, err := tx.ExecContext(ctx, `
            INSERT INTO platform_metadata_tags (platform_id, position, value)
            VALUES (?, ?, ?)`, platformID, position, value); err != nil {
			if isUniqueViolation(err) {
				return fmt.Errorf("%w: duplicate tag %q", platform.ErrConflict, value)
			}
			return storageErr("insert tag", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return storageErr("commit save metadata", err)
	}
	return nil
}

// NextSortOrder returns one past the current maximum, so a new platform is
// appended to the end of the dashboard.
func (r *PlatformRepository) NextSortOrder(ctx context.Context) (int, error) {
	var next sql.NullInt64
	if err := r.db.QueryRowContext(ctx,
		`SELECT MAX(sort_order) FROM platforms`).Scan(&next); err != nil {
		return 0, storageErr("read max sort order", err)
	}
	if !next.Valid {
		return 0, nil
	}
	return int(next.Int64) + 1, nil
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// insertMetadataRow writes one metadata row, storing SQL NULL for every field
// the provider does not support.
func insertMetadataRow(
	ctx context.Context, ex execer, platformID string,
	def platform.ProviderDefinition, m platform.Metadata,
) error {
	caps := def.Capabilities

	updatedAt := m.UpdatedAt
	if updatedAt.IsZero() {
		return fmt.Errorf("%w: metadata updated_at must be set", platform.ErrStorage)
	}

	if _, err := ex.ExecContext(ctx, `
        INSERT INTO platform_metadata
            (platform_id, title, description, category, language, visibility,
             mature_content, dvr, latency_mode, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		platformID,
		nullableText(caps.Title, m.Title),
		nullableText(caps.Description, m.Description),
		nullableText(caps.Category, m.Category),
		nullableText(caps.Language, m.Language),
		nullableText(caps.Visibility, m.Visibility),
		nullableBool(caps.MatureContent, m.MatureContent),
		nullableBool(caps.DVR, m.DVR),
		nullableText(caps.LatencyMode, m.LatencyMode),
		platform.FormatTimestamp(updatedAt),
	); err != nil {
		return storageErr("insert metadata", err)
	}

	return nil
}

func nullableText(supported bool, value string) any {
	if !supported {
		return nil
	}
	return value
}

func nullableBool(supported bool, value bool) any {
	if !supported {
		return nil
	}
	return boolToInt(value)
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
