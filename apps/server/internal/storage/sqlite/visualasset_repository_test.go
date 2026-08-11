package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/visualasset"
)

func testBlob(sha string) visualasset.Blob {
	return visualasset.Blob{
		SHA256: sha, MediaType: visualasset.MediaPNG, ByteSize: 1234,
		StorageName: sha, PublicToken: "token_" + sha, CreatedAt: time.Now().UTC(),
	}
}

func testAsset(id, blobSHA string) visualasset.Asset {
	now := time.Now().UTC()
	return visualasset.Asset{
		ID: id, BlobSHA256: blobSHA, Kind: visualasset.KindImage,
		DisplayName: "Badge", Author: "", License: "", Notice: "",
		Source: visualasset.SourceUpload, CreatedAt: now, UpdatedAt: now,
	}
}

func TestVisualAssetRepository_BlobAndAssetRoundTrip(t *testing.T) {
	db := newTestDB(t)
	repo := NewVisualAssetRepository(db.DB)
	ctx := context.Background()

	blob := testBlob("aaaa1111")
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
	if got.PublicToken != blob.PublicToken {
		t.Errorf("PublicToken = %q, want %q", got.PublicToken, blob.PublicToken)
	}

	byToken, found, err := repo.GetBlobByPublicToken(ctx, blob.PublicToken)
	if err != nil || !found || byToken.SHA256 != blob.SHA256 {
		t.Fatalf("GetBlobByPublicToken() = %v, %v, %v", byToken, found, err)
	}

	asset := testAsset("asset_1", blob.SHA256)
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
		t.Fatalf("ListAssets() = %v, %v", list, err)
	}

	updated, err := repo.UpdateAssetMetadata(ctx, asset.ID, "New name", "Author", "MIT", "notice")
	if err != nil {
		t.Fatalf("UpdateAssetMetadata() returned an error: %v", err)
	}
	if updated.DisplayName != "New name" {
		t.Errorf("DisplayName after update = %q", updated.DisplayName)
	}
}

func TestVisualAssetRepository_ReferenceTrackingAndOrphanGC(t *testing.T) {
	db := newTestDB(t)
	repo := NewVisualAssetRepository(db.DB)
	ctx := context.Background()

	blob := testBlob("bbbb2222")
	if err := repo.CreateBlob(ctx, blob); err != nil {
		t.Fatalf("CreateBlob() returned an error: %v", err)
	}
	asset := testAsset("asset_2", blob.SHA256)
	if err := repo.CreateAsset(ctx, asset); err != nil {
		t.Fatalf("CreateAsset() returned an error: %v", err)
	}

	count, err := repo.ReferenceCount(ctx, asset.ID)
	if err != nil || count != 0 {
		t.Fatalf("ReferenceCount() = %d, %v, want 0", count, err)
	}

	// A real visual_designs row is required for the foreign key on
	// visual_design_asset_refs.design_id.
	if _, err := db.DB.ExecContext(ctx, `
		INSERT INTO visual_designs (id, owner_kind, owner_id, schema_version, document_json, revision, created_at, updated_at)
		VALUES ('design_1', 'alert_rule', 'rule_1', 3, '{}', 1, ?, ?)`,
		time.Now().UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("insert visual_designs fixture: %v", err)
	}

	if err := repo.SetDesignAssetRefs(ctx, "design_1", []string{asset.ID}); err != nil {
		t.Fatalf("SetDesignAssetRefs() returned an error: %v", err)
	}
	count, err = repo.ReferenceCount(ctx, asset.ID)
	if err != nil || count != 1 {
		t.Fatalf("ReferenceCount() after ref = %d, %v, want 1", count, err)
	}

	// Deleting the design cascades to its ref rows (ON DELETE CASCADE).
	if _, err := db.DB.ExecContext(ctx, `DELETE FROM visual_designs WHERE id = 'design_1'`); err != nil {
		t.Fatalf("delete visual_designs fixture: %v", err)
	}
	count, err = repo.ReferenceCount(ctx, asset.ID)
	if err != nil || count != 0 {
		t.Fatalf("ReferenceCount() after cascade delete = %d, %v, want 0", count, err)
	}

	if err := repo.DeleteAsset(ctx, asset.ID); err != nil {
		t.Fatalf("DeleteAsset() returned an error: %v", err)
	}
	orphans, err := repo.ListOrphanBlobHashes(ctx)
	if err != nil {
		t.Fatalf("ListOrphanBlobHashes() returned an error: %v", err)
	}
	found := false
	for _, h := range orphans {
		if h == blob.SHA256 {
			found = true
		}
	}
	if !found {
		t.Errorf("expected blob %q to be listed as orphaned, got %v", blob.SHA256, orphans)
	}
}

func TestVisualAssetRepository_PersistsAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/persist.db"
	ctx := context.Background()

	db1, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open() returned an error: %v", err)
	}
	if _, err := Migrate(ctx, db1.DB); err != nil {
		t.Fatalf("Migrate() returned an error: %v", err)
	}
	repo1 := NewVisualAssetRepository(db1.DB)
	blob := testBlob("cccc3333")
	if err := repo1.CreateBlob(ctx, blob); err != nil {
		t.Fatalf("CreateBlob() returned an error: %v", err)
	}
	if err := db1.Close(); err != nil {
		t.Fatalf("Close() returned an error: %v", err)
	}

	db2, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("re-Open() returned an error: %v", err)
	}
	t.Cleanup(func() { _ = db2.Close() })
	repo2 := NewVisualAssetRepository(db2.DB)
	got, found, err := repo2.GetBlob(ctx, blob.SHA256)
	if err != nil || !found {
		t.Fatalf("GetBlob() after reopen = %v, %v, %v", got, found, err)
	}
}
