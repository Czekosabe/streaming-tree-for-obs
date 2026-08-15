package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/audioasset"
)

func testAudioBlob(sha string) audioasset.Blob {
	return audioasset.Blob{
		SHA256: sha, MediaType: audioasset.MediaWAV, ByteSize: 1234, DurationMS: 500,
		StorageName: sha, PublicToken: "token_" + sha, CreatedAt: time.Now().UTC(),
	}
}

func testAudioAsset(id, blobSHA string) audioasset.Asset {
	now := time.Now().UTC()
	return audioasset.Asset{
		ID: id, BlobSHA256: blobSHA, Kind: audioasset.KindSound,
		DisplayName: "Coin chime", Source: audioasset.SourceUpload, CreatedAt: now, UpdatedAt: now,
	}
}

func TestAudioAssetRepository_BlobAndAssetRoundTrip(t *testing.T) {
	db := newTestDB(t)
	repo := NewAudioAssetRepository(db.DB)
	ctx := context.Background()

	blob := testAudioBlob("aaaa1111")
	if err := repo.CreateBlob(ctx, blob); err != nil {
		t.Fatalf("CreateBlob() returned an error: %v", err)
	}
	// Idempotent: creating the same hash again must not error or duplicate.
	if err := repo.CreateBlob(ctx, blob); err != nil {
		t.Fatalf("CreateBlob() (duplicate) returned an error: %v", err)
	}

	got, found, err := repo.GetBlob(ctx, blob.SHA256)
	if err != nil || !found {
		t.Fatalf("GetBlob() = %v, %v, %v", got, found, err)
	}
	if got.DurationMS != blob.DurationMS {
		t.Errorf("DurationMS = %d, want %d", got.DurationMS, blob.DurationMS)
	}

	asset := testAudioAsset("audioasset_1", blob.SHA256)
	if err := repo.CreateAsset(ctx, asset); err != nil {
		t.Fatalf("CreateAsset() returned an error: %v", err)
	}

	gotAsset, found, err := repo.GetAsset(ctx, asset.ID)
	if err != nil || !found {
		t.Fatalf("GetAsset() = %v, %v, %v", gotAsset, found, err)
	}
	if gotAsset.DisplayName != asset.DisplayName {
		t.Errorf("DisplayName = %q, want %q", gotAsset.DisplayName, asset.DisplayName)
	}

	list, err := repo.ListAssets(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListAssets() = %v, %v, want 1 asset", list, err)
	}

	updated, err := repo.UpdateAssetMetadata(ctx, asset.ID, "New name")
	if err != nil || updated.DisplayName != "New name" {
		t.Fatalf("UpdateAssetMetadata() = %v, %v", updated, err)
	}

	if err := repo.DeleteAsset(ctx, asset.ID); err != nil {
		t.Fatalf("DeleteAsset() returned an error: %v", err)
	}
	if _, found, err := repo.GetAsset(ctx, asset.ID); err != nil || found {
		t.Fatalf("GetAsset() after delete = found=%v, err=%v, want not found", found, err)
	}
}

func TestAudioAssetRepository_ReferenceCountAndReplacement(t *testing.T) {
	db := newTestDB(t)
	repo := NewAudioAssetRepository(db.DB)
	ctx := context.Background()
	now := time.Now().UTC()

	blob := testAudioBlob("bbbb2222")
	if err := repo.CreateBlob(ctx, blob); err != nil {
		t.Fatalf("CreateBlob() returned an error: %v", err)
	}
	asset := testAudioAsset("audioasset_2", blob.SHA256)
	if err := repo.CreateAsset(ctx, asset); err != nil {
		t.Fatalf("CreateAsset() returned an error: %v", err)
	}

	// A real alert_profiles + alert_rules row is required for the FK
	// this project's connection enforces (PRAGMA foreign_keys=1).
	if _, err := db.DB.ExecContext(ctx, `
		INSERT INTO alert_profiles (id, public_slug, name, created_at, updated_at)
		VALUES ('profile_1', 'slug_1', 'Profile', ?, ?)`, now, now); err != nil {
		t.Fatalf("seed alert_profiles: %v", err)
	}
	if _, err := db.DB.ExecContext(ctx, `
		INSERT INTO alert_rules (id, profile_id, name, event_type, text_template, created_at, updated_at)
		VALUES ('rule_1', 'profile_1', 'Rule', 'follow', '{username} followed!', ?, ?)`, now, now); err != nil {
		t.Fatalf("seed alert_rules: %v", err)
	}

	if count, err := repo.ReferenceCount(ctx, asset.ID); err != nil || count != 0 {
		t.Fatalf("ReferenceCount() before ref = %v, %v, want 0", count, err)
	}

	if err := repo.SetRuleAssetRefs(ctx, "rule_1", []string{asset.ID}); err != nil {
		t.Fatalf("SetRuleAssetRefs() returned an error: %v", err)
	}
	if count, err := repo.ReferenceCount(ctx, asset.ID); err != nil || count != 1 {
		t.Fatalf("ReferenceCount() after ref = %v, %v, want 1", count, err)
	}

	// Full-replacement semantics: calling again with an empty list clears it.
	if err := repo.SetRuleAssetRefs(ctx, "rule_1", nil); err != nil {
		t.Fatalf("SetRuleAssetRefs() (clear) returned an error: %v", err)
	}
	if count, err := repo.ReferenceCount(ctx, asset.ID); err != nil || count != 0 {
		t.Fatalf("ReferenceCount() after replace-with-empty = %v, %v, want 0", count, err)
	}

	if err := repo.SetRuleAssetRefs(ctx, "rule_1", []string{asset.ID}); err != nil {
		t.Fatalf("SetRuleAssetRefs() (re-add) returned an error: %v", err)
	}
	if err := repo.ClearRuleRefs(ctx, "rule_1"); err != nil {
		t.Fatalf("ClearRuleRefs() returned an error: %v", err)
	}
	if count, err := repo.ReferenceCount(ctx, asset.ID); err != nil || count != 0 {
		t.Fatalf("ReferenceCount() after ClearRuleRefs = %v, %v, want 0", count, err)
	}
}

func TestAudioAssetRepository_TemplateReferenceCascadesOnDelete(t *testing.T) {
	db := newTestDB(t)
	repo := NewAudioAssetRepository(db.DB)
	ctx := context.Background()
	now := time.Now().UTC()

	blob := testAudioBlob("cccc3333")
	if err := repo.CreateBlob(ctx, blob); err != nil {
		t.Fatalf("CreateBlob() returned an error: %v", err)
	}
	asset := testAudioAsset("audioasset_3", blob.SHA256)
	if err := repo.CreateAsset(ctx, asset); err != nil {
		t.Fatalf("CreateAsset() returned an error: %v", err)
	}

	if _, err := db.DB.ExecContext(ctx, `
		INSERT INTO visual_templates (id, target_kind, name, description, author, license, template_schema_version, document_json, created_at, updated_at)
		VALUES ('template_1', 'alert', 'My template', '', '', '', 1, '{}', ?, ?)`, now, now); err != nil {
		t.Fatalf("seed visual_templates: %v", err)
	}

	if err := repo.SetTemplateAssetRefs(ctx, "template_1", []string{asset.ID}); err != nil {
		t.Fatalf("SetTemplateAssetRefs() returned an error: %v", err)
	}
	if count, err := repo.ReferenceCount(ctx, asset.ID); err != nil || count != 1 {
		t.Fatalf("ReferenceCount() = %v, %v, want 1", count, err)
	}

	// ON DELETE CASCADE: deleting the owning template row removes the
	// reference row automatically, without an explicit ClearTemplateRefs
	// call.
	if _, err := db.DB.ExecContext(ctx, `DELETE FROM visual_templates WHERE id = 'template_1'`); err != nil {
		t.Fatalf("delete visual_templates: %v", err)
	}
	if count, err := repo.ReferenceCount(ctx, asset.ID); err != nil || count != 0 {
		t.Fatalf("ReferenceCount() after owner delete = %v, %v, want 0 (cascade)", count, err)
	}
}

func TestAudioAssetRepository_OrphanAndKnownBlobHashes(t *testing.T) {
	db := newTestDB(t)
	repo := NewAudioAssetRepository(db.DB)
	ctx := context.Background()

	blobReferenced := testAudioBlob("dddd4444")
	blobOrphan := testAudioBlob("eeee5555")
	if err := repo.CreateBlob(ctx, blobReferenced); err != nil {
		t.Fatalf("CreateBlob() returned an error: %v", err)
	}
	if err := repo.CreateBlob(ctx, blobOrphan); err != nil {
		t.Fatalf("CreateBlob() returned an error: %v", err)
	}
	asset := testAudioAsset("audioasset_4", blobReferenced.SHA256)
	if err := repo.CreateAsset(ctx, asset); err != nil {
		t.Fatalf("CreateAsset() returned an error: %v", err)
	}

	orphans, err := repo.ListOrphanBlobHashes(ctx)
	if err != nil {
		t.Fatalf("ListOrphanBlobHashes() returned an error: %v", err)
	}
	if len(orphans) != 1 || orphans[0] != blobOrphan.SHA256 {
		t.Fatalf("ListOrphanBlobHashes() = %v, want exactly [%q]", orphans, blobOrphan.SHA256)
	}

	known, err := repo.ListBlobHashes(ctx)
	if err != nil || len(known) != 2 {
		t.Fatalf("ListBlobHashes() = %v, %v, want 2 known hashes", known, err)
	}

	if err := repo.DeleteBlobRow(ctx, blobOrphan.SHA256); err != nil {
		t.Fatalf("DeleteBlobRow() returned an error: %v", err)
	}
	known, err = repo.ListBlobHashes(ctx)
	if err != nil || len(known) != 1 {
		t.Fatalf("ListBlobHashes() after delete = %v, %v, want 1", known, err)
	}
}
