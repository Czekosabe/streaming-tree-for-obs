package visualpackage

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/streaming-tree/server/internal/domain/visualasset"
	"github.com/streaming-tree/server/internal/domain/visualtemplate"
	"github.com/streaming-tree/server/internal/storage/sqlite"
)

func newTestService(t *testing.T) (*Service, *visualasset.Service, *visualtemplate.Service) {
	t.Helper()
	ctx := context.Background()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := sqlite.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := sqlite.Migrate(ctx, db.DB); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}

	store := visualasset.NewFileStore(filepath.Join(t.TempDir(), "assets"))
	if err := store.EnsureDirs(); err != nil {
		t.Fatalf("ensure asset dirs: %v", err)
	}
	assetRepo := sqlite.NewVisualAssetRepository(db.DB)
	assetSvc := visualasset.NewService(assetRepo, store, nil)

	tmplRepo := sqlite.NewVisualTemplateRepository(db.DB)
	tmplSvc, err := visualtemplate.NewService(tmplRepo, nil, nil)
	if err != nil {
		t.Fatalf("build template service: %v", err)
	}

	pkgSvc := NewService(assetSvc, tmplSvc, nil)
	return pkgSvc, assetSvc, tmplSvc
}

func TestService_Import_CreatesTemplateAndAsset(t *testing.T) {
	pkgSvc, assetSvc, tmplSvc := newTestService(t)
	ctx := context.Background()

	data := validPackageZip(t)
	tmpl, err := pkgSvc.Import(ctx, data)
	if err != nil {
		t.Fatalf("Import() returned an error: %v", err)
	}
	if tmpl.Target != visualtemplate.TargetAlert {
		t.Errorf("target = %q, want alert", tmpl.Target)
	}
	if len(tmpl.Document.Layers) != 1 || tmpl.Document.Layers[0].Image == nil {
		t.Fatalf("expected one image layer, got %+v", tmpl.Document.Layers)
	}
	localAssetID := tmpl.Document.Layers[0].Image.AssetID
	if localAssetID == "pkgasset_0001" {
		t.Fatal("package-local asset id was never rewritten to a local id")
	}

	asset, err := assetSvc.Get(ctx, localAssetID)
	if err != nil {
		t.Fatalf("Get(%q) returned an error: %v", localAssetID, err)
	}
	if asset.Kind != visualasset.KindImage {
		t.Errorf("asset kind = %q, want image", asset.Kind)
	}
	if asset.Source != visualasset.SourcePackage {
		t.Errorf("asset source = %q, want package", asset.Source)
	}

	count, err := assetSvc.ReferenceCount(ctx, localAssetID)
	if err != nil {
		t.Fatalf("ReferenceCount() returned an error: %v", err)
	}
	if count != 1 {
		t.Errorf("reference count = %d, want 1", count)
	}

	// Deleting the template does not delete the asset (docs/visual-
	// template-packages.md §46 - "no template provenance coupling").
	if err := tmplSvc.Delete(ctx, tmpl.ID); err != nil {
		t.Fatalf("Delete(template) returned an error: %v", err)
	}
	if err := assetSvc.ClearTemplateRefs(ctx, tmpl.ID); err != nil {
		t.Fatalf("ClearTemplateRefs() returned an error: %v", err)
	}
	if _, err := assetSvc.Get(ctx, localAssetID); err != nil {
		t.Errorf("asset should still exist after its importing template is deleted, got %v", err)
	}
}

func TestService_Import_NeverTrustsPreview(t *testing.T) {
	pkgSvc, _, tmplSvc := newTestService(t)
	ctx := context.Background()

	data := validPackageZip(t)
	preview, err := pkgSvc.ImportPreview(ctx, data)
	if err != nil {
		t.Fatalf("ImportPreview() returned an error: %v", err)
	}
	defer pkgSvc.CancelPreview(preview.Token)

	list, err := tmplSvc.List(ctx)
	if err != nil {
		t.Fatalf("List() returned an error: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("preview must persist nothing, found %d templates", len(list))
	}
	if len(preview.Assets) != 1 || preview.Assets[0].PackageAssetID != "pkgasset_0001" {
		t.Fatalf("unexpected preview assets: %+v", preview.Assets)
	}

	// A completely independent Import call (fresh bytes, no reference to
	// the preview token) must still succeed on its own.
	if _, err := pkgSvc.Import(ctx, data); err != nil {
		t.Fatalf("Import() after an unrelated preview returned an error: %v", err)
	}
}

func TestService_ExportImportRoundTrip(t *testing.T) {
	pkgSvc, assetSvc, _ := newTestService(t)
	ctx := context.Background()

	imported, err := pkgSvc.Import(ctx, validPackageZip(t))
	if err != nil {
		t.Fatalf("Import() returned an error: %v", err)
	}
	originalAssetID := imported.Document.Layers[0].Image.AssetID
	originalAsset, err := assetSvc.Get(ctx, originalAssetID)
	if err != nil {
		t.Fatalf("Get() returned an error: %v", err)
	}

	exported, err := pkgSvc.ExportTemplate(ctx, imported.ID)
	if err != nil {
		t.Fatalf("ExportTemplate() returned an error: %v", err)
	}

	reimported, err := pkgSvc.Import(ctx, exported)
	if err != nil {
		t.Fatalf("re-Import() of an exported package returned an error: %v", err)
	}

	if reimported.ID == imported.ID {
		t.Error("re-imported template should get a fresh local id")
	}
	if reimported.Name != imported.Name || reimported.Target != imported.Target {
		t.Errorf("re-imported template metadata does not match: %+v vs %+v", reimported, imported)
	}
	newAssetID := reimported.Document.Layers[0].Image.AssetID
	if newAssetID == originalAssetID {
		t.Error("re-imported asset should get a fresh local id")
	}
	newAsset, err := assetSvc.Get(ctx, newAssetID)
	if err != nil {
		t.Fatalf("Get() returned an error: %v", err)
	}
	if newAsset.Blob == nil || originalAsset.Blob == nil || newAsset.Blob.SHA256 != originalAsset.Blob.SHA256 {
		t.Error("re-imported asset bytes should be identical by SHA-256 (and deduplicated to the same blob)")
	}
}

func TestService_ExportTemplate_AssetFreeTemplate(t *testing.T) {
	pkgSvc, _, tmplSvc := newTestService(t)
	ctx := context.Background()

	doc := assetFreeDocument(t)
	tmpl, err := tmplSvc.Create(ctx, visualtemplate.TargetAlert, "Asset-free", "", "", "", doc)
	if err != nil {
		t.Fatalf("Create() returned an error: %v", err)
	}

	data, err := pkgSvc.ExportTemplate(ctx, tmpl.ID)
	if err != nil {
		t.Fatalf("ExportTemplate() for an asset-free template returned an error: %v", err)
	}
	validated, err := ReadArchive(data)
	if err != nil {
		t.Fatalf("ReadArchive() of the exported asset-free package returned an error: %v", err)
	}
	if len(validated.Assets) != 0 {
		t.Errorf("expected zero assets in an asset-free package, got %d", len(validated.Assets))
	}
}
