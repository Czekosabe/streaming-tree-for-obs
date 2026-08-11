package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/domain/visualasset"
)

// VisualAssetRepository is the SQLite implementation of
// visualasset.Repository (migration 0018_visual_assets.sql).
type VisualAssetRepository struct {
	db *sql.DB
}

func NewVisualAssetRepository(db *sql.DB) *VisualAssetRepository {
	return &VisualAssetRepository{db: db}
}

var _ visualasset.Repository = (*VisualAssetRepository)(nil)

func visualAssetStorageErr(op string, err error) error {
	return fmt.Errorf("%w: %s: %v", visualasset.ErrStorage, op, err)
}

const visualAssetBlobColumns = `sha256, media_type, byte_size, storage_name, public_token, created_at`

func scanBlob(scanner interface{ Scan(...any) error }) (visualasset.Blob, error) {
	var (
		b         visualasset.Blob
		mediaType string
		createdAt string
	)
	if err := scanner.Scan(&b.SHA256, &mediaType, &b.ByteSize, &b.StorageName, &b.PublicToken, &createdAt); err != nil {
		return visualasset.Blob{}, err
	}
	b.MediaType = visualasset.MediaType(mediaType)
	t, err := platform.ParseTimestamp(createdAt)
	if err != nil {
		return visualasset.Blob{}, fmt.Errorf("parse created_at %q: %w", createdAt, err)
	}
	b.CreatedAt = t
	return b, nil
}

func (r *VisualAssetRepository) CreateBlob(ctx context.Context, blob visualasset.Blob) error {
	_, found, err := r.GetBlob(ctx, blob.SHA256)
	if err != nil {
		return err
	}
	if found {
		return nil
	}
	if _, err := r.db.ExecContext(ctx, `
		INSERT INTO visual_asset_blobs (`+visualAssetBlobColumns+`) VALUES (?, ?, ?, ?, ?, ?)`,
		blob.SHA256, string(blob.MediaType), blob.ByteSize, blob.StorageName, blob.PublicToken, platform.FormatTimestamp(blob.CreatedAt),
	); err != nil {
		return visualAssetStorageErr("create blob", err)
	}
	return nil
}

func (r *VisualAssetRepository) GetBlob(ctx context.Context, sha256Hex string) (visualasset.Blob, bool, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+visualAssetBlobColumns+` FROM visual_asset_blobs WHERE sha256 = ?`, sha256Hex)
	b, err := scanBlob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return visualasset.Blob{}, false, nil
	}
	if err != nil {
		return visualasset.Blob{}, false, visualAssetStorageErr("get blob", err)
	}
	return b, true, nil
}

func (r *VisualAssetRepository) GetBlobByPublicToken(ctx context.Context, token string) (visualasset.Blob, bool, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+visualAssetBlobColumns+` FROM visual_asset_blobs WHERE public_token = ?`, token)
	b, err := scanBlob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return visualasset.Blob{}, false, nil
	}
	if err != nil {
		return visualasset.Blob{}, false, visualAssetStorageErr("get blob by public token", err)
	}
	return b, true, nil
}

const visualAssetColumns = `id, blob_sha256, kind, display_name, author, license, notice, source, created_at, updated_at`

func scanAsset(scanner interface{ Scan(...any) error }) (visualasset.Asset, error) {
	var (
		a                    visualasset.Asset
		kind, source         string
		createdAt, updatedAt string
	)
	if err := scanner.Scan(&a.ID, &a.BlobSHA256, &kind, &a.DisplayName, &a.Author, &a.License, &a.Notice, &source, &createdAt, &updatedAt); err != nil {
		return visualasset.Asset{}, err
	}
	a.Kind = visualasset.Kind(kind)
	a.Source = source
	var err error
	if a.CreatedAt, err = platform.ParseTimestamp(createdAt); err != nil {
		return visualasset.Asset{}, fmt.Errorf("parse created_at %q: %w", createdAt, err)
	}
	if a.UpdatedAt, err = platform.ParseTimestamp(updatedAt); err != nil {
		return visualasset.Asset{}, fmt.Errorf("parse updated_at %q: %w", updatedAt, err)
	}
	return a, nil
}

func (r *VisualAssetRepository) CreateAsset(ctx context.Context, asset visualasset.Asset) error {
	if _, err := r.db.ExecContext(ctx, `
		INSERT INTO visual_assets (`+visualAssetColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		asset.ID, asset.BlobSHA256, string(asset.Kind), asset.DisplayName, asset.Author, asset.License, asset.Notice, asset.Source,
		platform.FormatTimestamp(asset.CreatedAt), platform.FormatTimestamp(asset.UpdatedAt),
	); err != nil {
		return visualAssetStorageErr("create asset", err)
	}
	return nil
}

func (r *VisualAssetRepository) GetAsset(ctx context.Context, id string) (visualasset.Asset, bool, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+visualAssetColumns+` FROM visual_assets WHERE id = ?`, id)
	a, err := scanAsset(row)
	if errors.Is(err, sql.ErrNoRows) {
		return visualasset.Asset{}, false, nil
	}
	if err != nil {
		return visualasset.Asset{}, false, visualAssetStorageErr("get asset", err)
	}
	return a, true, nil
}

func (r *VisualAssetRepository) ListAssets(ctx context.Context) ([]visualasset.Asset, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+visualAssetColumns+` FROM visual_assets ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, visualAssetStorageErr("list assets", err)
	}
	defer rows.Close()

	var out []visualasset.Asset
	for rows.Next() {
		a, err := scanAsset(rows)
		if err != nil {
			return nil, visualAssetStorageErr("list assets", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, visualAssetStorageErr("list assets", err)
	}
	return out, nil
}

func (r *VisualAssetRepository) UpdateAssetMetadata(ctx context.Context, id, displayName, author, license, notice string) (visualasset.Asset, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE visual_assets SET display_name = ?, author = ?, license = ?, notice = ?, updated_at = ?
		WHERE id = ?`,
		displayName, author, license, notice, platform.FormatTimestamp(time.Now().UTC()), id,
	)
	if err != nil {
		return visualasset.Asset{}, visualAssetStorageErr("update asset metadata", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return visualasset.Asset{}, visualAssetStorageErr("update asset metadata", err)
	}
	if affected == 0 {
		return visualasset.Asset{}, visualasset.ErrNotFound
	}
	a, found, err := r.GetAsset(ctx, id)
	if err != nil {
		return visualasset.Asset{}, err
	}
	if !found {
		return visualasset.Asset{}, visualasset.ErrNotFound
	}
	return a, nil
}

func (r *VisualAssetRepository) DeleteAsset(ctx context.Context, id string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM visual_assets WHERE id = ?`, id); err != nil {
		return visualAssetStorageErr("delete asset", err)
	}
	return nil
}

func (r *VisualAssetRepository) ReferenceCount(ctx context.Context, assetID string) (int, error) {
	var count int
	if err := r.db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM visual_design_asset_refs WHERE asset_id = ?) +
			(SELECT COUNT(*) FROM visual_template_asset_refs WHERE asset_id = ?)`,
		assetID, assetID,
	).Scan(&count); err != nil {
		return 0, visualAssetStorageErr("reference count", err)
	}
	return count, nil
}

func replaceRefs(ctx context.Context, db *sql.DB, table, ownerColumn, ownerID string, assetIDs []string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return visualAssetStorageErr("replace refs: begin tx", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE `+ownerColumn+` = ?`, ownerID); err != nil {
		return visualAssetStorageErr("replace refs: clear", err)
	}
	seen := make(map[string]bool, len(assetIDs))
	for _, assetID := range assetIDs {
		if assetID == "" || seen[assetID] {
			continue
		}
		seen[assetID] = true
		if _, err := tx.ExecContext(ctx, `INSERT INTO `+table+` (`+ownerColumn+`, asset_id) VALUES (?, ?)`, ownerID, assetID); err != nil {
			return visualAssetStorageErr("replace refs: insert", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return visualAssetStorageErr("replace refs: commit", err)
	}
	return nil
}

func (r *VisualAssetRepository) SetDesignAssetRefs(ctx context.Context, designID string, assetIDs []string) error {
	return replaceRefs(ctx, r.db, "visual_design_asset_refs", "design_id", designID, assetIDs)
}

func (r *VisualAssetRepository) SetTemplateAssetRefs(ctx context.Context, templateID string, assetIDs []string) error {
	return replaceRefs(ctx, r.db, "visual_template_asset_refs", "template_id", templateID, assetIDs)
}

func (r *VisualAssetRepository) ClearDesignRefs(ctx context.Context, designID string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM visual_design_asset_refs WHERE design_id = ?`, designID); err != nil {
		return visualAssetStorageErr("clear design refs", err)
	}
	return nil
}

func (r *VisualAssetRepository) ClearTemplateRefs(ctx context.Context, templateID string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM visual_template_asset_refs WHERE template_id = ?`, templateID); err != nil {
		return visualAssetStorageErr("clear template refs", err)
	}
	return nil
}

func (r *VisualAssetRepository) ListOrphanBlobHashes(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT b.sha256 FROM visual_asset_blobs b
		WHERE NOT EXISTS (SELECT 1 FROM visual_assets a WHERE a.blob_sha256 = b.sha256)`)
	if err != nil {
		return nil, visualAssetStorageErr("list orphan blobs", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			return nil, visualAssetStorageErr("list orphan blobs", err)
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (r *VisualAssetRepository) ListBlobHashes(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT sha256 FROM visual_asset_blobs`)
	if err != nil {
		return nil, visualAssetStorageErr("list blob hashes", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			return nil, visualAssetStorageErr("list blob hashes", err)
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (r *VisualAssetRepository) DeleteBlobRow(ctx context.Context, sha256Hex string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM visual_asset_blobs WHERE sha256 = ?`, sha256Hex); err != nil {
		return visualAssetStorageErr("delete blob row", err)
	}
	return nil
}
