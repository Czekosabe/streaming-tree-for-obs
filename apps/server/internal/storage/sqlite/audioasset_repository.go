package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/streaming-tree/server/internal/domain/audioasset"
	"github.com/streaming-tree/server/internal/domain/platform"
)

// AudioAssetRepository is the SQLite implementation of
// audioasset.Repository (migration 0022_audio_assets.sql).
type AudioAssetRepository struct {
	db *sql.DB
}

func NewAudioAssetRepository(db *sql.DB) *AudioAssetRepository {
	return &AudioAssetRepository{db: db}
}

var _ audioasset.Repository = (*AudioAssetRepository)(nil)

func audioAssetStorageErr(op string, err error) error {
	return fmt.Errorf("%w: %s: %v", audioasset.ErrStorage, op, err)
}

const audioAssetBlobColumns = `sha256, media_type, byte_size, duration_ms, storage_name, public_token, created_at`

func scanAudioBlob(scanner interface{ Scan(...any) error }) (audioasset.Blob, error) {
	var (
		b         audioasset.Blob
		mediaType string
		createdAt string
	)
	if err := scanner.Scan(&b.SHA256, &mediaType, &b.ByteSize, &b.DurationMS, &b.StorageName, &b.PublicToken, &createdAt); err != nil {
		return audioasset.Blob{}, err
	}
	b.MediaType = audioasset.MediaType(mediaType)
	t, err := platform.ParseTimestamp(createdAt)
	if err != nil {
		return audioasset.Blob{}, fmt.Errorf("parse created_at %q: %w", createdAt, err)
	}
	b.CreatedAt = t
	return b, nil
}

func (r *AudioAssetRepository) CreateBlob(ctx context.Context, blob audioasset.Blob) error {
	_, found, err := r.GetBlob(ctx, blob.SHA256)
	if err != nil {
		return err
	}
	if found {
		return nil
	}
	if _, err := r.db.ExecContext(ctx, `
		INSERT INTO audioasset_blobs (`+audioAssetBlobColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		blob.SHA256, string(blob.MediaType), blob.ByteSize, blob.DurationMS, blob.StorageName, blob.PublicToken, platform.FormatTimestamp(blob.CreatedAt),
	); err != nil {
		return audioAssetStorageErr("create blob", err)
	}
	return nil
}

func (r *AudioAssetRepository) GetBlob(ctx context.Context, sha256Hex string) (audioasset.Blob, bool, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+audioAssetBlobColumns+` FROM audioasset_blobs WHERE sha256 = ?`, sha256Hex)
	b, err := scanAudioBlob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return audioasset.Blob{}, false, nil
	}
	if err != nil {
		return audioasset.Blob{}, false, audioAssetStorageErr("get blob", err)
	}
	return b, true, nil
}

const audioAssetColumns = `id, blob_sha256, kind, display_name, source, created_at, updated_at`

func scanAudioAsset(scanner interface{ Scan(...any) error }) (audioasset.Asset, error) {
	var (
		a                    audioasset.Asset
		kind, source         string
		createdAt, updatedAt string
	)
	if err := scanner.Scan(&a.ID, &a.BlobSHA256, &kind, &a.DisplayName, &source, &createdAt, &updatedAt); err != nil {
		return audioasset.Asset{}, err
	}
	a.Kind = audioasset.Kind(kind)
	a.Source = source
	var err error
	if a.CreatedAt, err = platform.ParseTimestamp(createdAt); err != nil {
		return audioasset.Asset{}, fmt.Errorf("parse created_at %q: %w", createdAt, err)
	}
	if a.UpdatedAt, err = platform.ParseTimestamp(updatedAt); err != nil {
		return audioasset.Asset{}, fmt.Errorf("parse updated_at %q: %w", updatedAt, err)
	}
	return a, nil
}

func (r *AudioAssetRepository) CreateAsset(ctx context.Context, asset audioasset.Asset) error {
	if _, err := r.db.ExecContext(ctx, `
		INSERT INTO audioasset_assets (`+audioAssetColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		asset.ID, asset.BlobSHA256, string(asset.Kind), asset.DisplayName, asset.Source,
		platform.FormatTimestamp(asset.CreatedAt), platform.FormatTimestamp(asset.UpdatedAt),
	); err != nil {
		return audioAssetStorageErr("create asset", err)
	}
	return nil
}

func (r *AudioAssetRepository) GetAsset(ctx context.Context, id string) (audioasset.Asset, bool, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+audioAssetColumns+` FROM audioasset_assets WHERE id = ?`, id)
	a, err := scanAudioAsset(row)
	if errors.Is(err, sql.ErrNoRows) {
		return audioasset.Asset{}, false, nil
	}
	if err != nil {
		return audioasset.Asset{}, false, audioAssetStorageErr("get asset", err)
	}
	return a, true, nil
}

func (r *AudioAssetRepository) ListAssets(ctx context.Context) ([]audioasset.Asset, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+audioAssetColumns+` FROM audioasset_assets ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, audioAssetStorageErr("list assets", err)
	}
	defer rows.Close()

	var out []audioasset.Asset
	for rows.Next() {
		a, err := scanAudioAsset(rows)
		if err != nil {
			return nil, audioAssetStorageErr("list assets", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, audioAssetStorageErr("list assets", err)
	}
	return out, nil
}

func (r *AudioAssetRepository) UpdateAssetMetadata(ctx context.Context, id, displayName string) (audioasset.Asset, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE audioasset_assets SET display_name = ?, updated_at = ?
		WHERE id = ?`,
		displayName, platform.FormatTimestamp(time.Now().UTC()), id,
	)
	if err != nil {
		return audioasset.Asset{}, audioAssetStorageErr("update asset metadata", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return audioasset.Asset{}, audioAssetStorageErr("update asset metadata", err)
	}
	if affected == 0 {
		return audioasset.Asset{}, audioasset.ErrNotFound
	}
	a, found, err := r.GetAsset(ctx, id)
	if err != nil {
		return audioasset.Asset{}, err
	}
	if !found {
		return audioasset.Asset{}, audioasset.ErrNotFound
	}
	return a, nil
}

func (r *AudioAssetRepository) DeleteAsset(ctx context.Context, id string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM audioasset_assets WHERE id = ?`, id); err != nil {
		return audioAssetStorageErr("delete asset", err)
	}
	return nil
}

func (r *AudioAssetRepository) ReferenceCount(ctx context.Context, assetID string) (int, error) {
	var count int
	if err := r.db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM alert_rule_audio_asset_refs WHERE asset_id = ?) +
			(SELECT COUNT(*) FROM alert_template_audio_asset_refs WHERE asset_id = ?)`,
		assetID, assetID,
	).Scan(&count); err != nil {
		return 0, audioAssetStorageErr("reference count", err)
	}
	return count, nil
}

func replaceAudioRefs(ctx context.Context, db *sql.DB, table, ownerColumn, ownerID string, assetIDs []string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return audioAssetStorageErr("replace refs: begin tx", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE `+ownerColumn+` = ?`, ownerID); err != nil {
		return audioAssetStorageErr("replace refs: clear", err)
	}
	seen := make(map[string]bool, len(assetIDs))
	for _, assetID := range assetIDs {
		if assetID == "" || seen[assetID] {
			continue
		}
		seen[assetID] = true
		if _, err := tx.ExecContext(ctx, `INSERT INTO `+table+` (`+ownerColumn+`, asset_id) VALUES (?, ?)`, ownerID, assetID); err != nil {
			return audioAssetStorageErr("replace refs: insert", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return audioAssetStorageErr("replace refs: commit", err)
	}
	return nil
}

func (r *AudioAssetRepository) SetRuleAssetRefs(ctx context.Context, ruleID string, assetIDs []string) error {
	return replaceAudioRefs(ctx, r.db, "alert_rule_audio_asset_refs", "rule_id", ruleID, assetIDs)
}

func (r *AudioAssetRepository) SetTemplateAssetRefs(ctx context.Context, templateID string, assetIDs []string) error {
	return replaceAudioRefs(ctx, r.db, "alert_template_audio_asset_refs", "template_id", templateID, assetIDs)
}

func (r *AudioAssetRepository) ClearRuleRefs(ctx context.Context, ruleID string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM alert_rule_audio_asset_refs WHERE rule_id = ?`, ruleID); err != nil {
		return audioAssetStorageErr("clear rule refs", err)
	}
	return nil
}

func (r *AudioAssetRepository) ClearTemplateRefs(ctx context.Context, templateID string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM alert_template_audio_asset_refs WHERE template_id = ?`, templateID); err != nil {
		return audioAssetStorageErr("clear template refs", err)
	}
	return nil
}

func (r *AudioAssetRepository) ListOrphanBlobHashes(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT b.sha256 FROM audioasset_blobs b
		WHERE NOT EXISTS (SELECT 1 FROM audioasset_assets a WHERE a.blob_sha256 = b.sha256)`)
	if err != nil {
		return nil, audioAssetStorageErr("list orphan blobs", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			return nil, audioAssetStorageErr("list orphan blobs", err)
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (r *AudioAssetRepository) ListBlobHashes(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT sha256 FROM audioasset_blobs`)
	if err != nil {
		return nil, audioAssetStorageErr("list blob hashes", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			return nil, audioAssetStorageErr("list blob hashes", err)
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (r *AudioAssetRepository) DeleteBlobRow(ctx context.Context, sha256Hex string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM audioasset_blobs WHERE sha256 = ?`, sha256Hex); err != nil {
		return audioAssetStorageErr("delete blob row", err)
	}
	return nil
}
