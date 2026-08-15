package visualpackage

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/streaming-tree/server/internal/domain/audioasset"
	"github.com/streaming-tree/server/internal/domain/visualasset"
	"github.com/streaming-tree/server/internal/domain/visualtemplate"
	"github.com/streaming-tree/server/internal/storage/sqlite"
)

func newTestService(t *testing.T) (*Service, *visualasset.Service, *visualtemplate.Service) {
	pkgSvc, assetSvc, _, tmplSvc := newTestServiceWithAudio(t)
	return pkgSvc, assetSvc, tmplSvc
}

func newTestServiceWithAudio(t *testing.T) (*Service, *visualasset.Service, *audioasset.Service, *visualtemplate.Service) {
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

	audioStore := visualasset.NewFileStore(filepath.Join(t.TempDir(), "assets-audio"))
	if err := audioStore.EnsureDirs(); err != nil {
		t.Fatalf("ensure audio asset dirs: %v", err)
	}
	audioRepo := sqlite.NewAudioAssetRepository(db.DB)
	audioSvc := audioasset.NewService(audioRepo, audioStore, nil)

	tmplRepo := sqlite.NewVisualTemplateRepository(db.DB)
	tmplSvc, err := visualtemplate.NewService(tmplRepo, nil, nil)
	if err != nil {
		t.Fatalf("build template service: %v", err)
	}
	tmplSvc.SetAssetService(assetSvc)
	tmplSvc.SetAudioAssetService(audioSvc)

	pkgSvc := NewService(assetSvc, audioSvc, tmplSvc, nil)
	return pkgSvc, assetSvc, audioSvc, tmplSvc
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

func TestService_Import_AudioPackage_CreatesTemplateWithAlertAudio(t *testing.T) {
	pkgSvc, _, audioSvc, _ := newTestServiceWithAudio(t)
	ctx := context.Background()

	tmpl, err := pkgSvc.Import(ctx, validPackageWithAudioZip(t))
	if err != nil {
		t.Fatalf("Import() returned an error: %v", err)
	}
	if tmpl.AlertAudio == nil {
		t.Fatal("AlertAudio = nil, want the imported preset")
	}
	if !tmpl.AlertAudio.SoundEnabled || tmpl.AlertAudio.SoundVolume != 0.8 {
		t.Errorf("sound fields = %+v, want soundEnabled=true, soundVolume=0.8", tmpl.AlertAudio)
	}
	if !tmpl.AlertAudio.TTSEnabled || tmpl.AlertAudio.TTSTemplate != "{username} says hi" || tmpl.AlertAudio.TTSVolume != 0.5 {
		t.Errorf("TTS fields = %+v, want the imported TTS preset", tmpl.AlertAudio)
	}
	if tmpl.AlertAudio.SoundAssetID == "pkgaudio_0001" {
		t.Fatal("package-local audio asset id was never rewritten to a local id")
	}

	asset, err := audioSvc.Get(ctx, tmpl.AlertAudio.SoundAssetID)
	if err != nil {
		t.Fatalf("audioSvc.Get(%q) returned an error: %v", tmpl.AlertAudio.SoundAssetID, err)
	}
	if asset.Source != audioasset.SourcePackage {
		t.Errorf("asset source = %q, want package", asset.Source)
	}

	count, err := audioSvc.ReferenceCount(ctx, tmpl.AlertAudio.SoundAssetID)
	if err != nil {
		t.Fatalf("ReferenceCount() returned an error: %v", err)
	}
	if count != 1 {
		t.Errorf("reference count = %d, want 1", count)
	}
}

func TestService_ImportPreview_AudioPackage_NeverPersistsButDescribesAudio(t *testing.T) {
	pkgSvc, _, audioSvc, tmplSvc := newTestServiceWithAudio(t)
	ctx := context.Background()

	preview, err := pkgSvc.ImportPreview(ctx, validPackageWithAudioZip(t))
	if err != nil {
		t.Fatalf("ImportPreview() returned an error: %v", err)
	}
	defer pkgSvc.CancelPreview(preview.Token)

	if preview.AlertAudio == nil {
		t.Fatal("AlertAudio = nil, want a populated preview description")
	}
	if !preview.AlertAudio.SoundEnabled || preview.AlertAudio.SoundDisplayName != "Coin chime" || preview.AlertAudio.SoundDurationMS != 100 {
		t.Errorf("preview.AlertAudio = %+v, want the manifest's own sound metadata", preview.AlertAudio)
	}
	if !preview.AlertAudio.TTSEnabled || preview.AlertAudio.TTSTemplate != "{username} says hi" {
		t.Errorf("preview.AlertAudio TTS fields = %+v, want the manifest's own TTS preset", preview.AlertAudio)
	}

	list, err := tmplSvc.List(ctx)
	if err != nil {
		t.Fatalf("List() returned an error: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("preview must persist nothing, found %d templates", len(list))
	}
	assets, err := audioSvc.List(ctx)
	if err != nil {
		t.Fatalf("audioSvc.List() returned an error: %v", err)
	}
	if len(assets) != 0 {
		t.Fatalf("preview must never create a real managed audio asset, found %d", len(assets))
	}
}

func TestService_Import_ChatTargetWithAlertAudio_Rejected(t *testing.T) {
	pkgSvc, _, audioSvc, _ := newTestServiceWithAudio(t)
	ctx := context.Background()

	manifest, template, wav := validPackageWithAudioParts(t)
	// Flip the embedded template.json's own target to "chat" - the
	// manifest's alertAudio object is otherwise untouched.
	chatTemplate := bytes.Replace(template, []byte(`"target": "alert"`), []byte(`"target": "chat"`), 1)
	data := buildZipCustom(t, []zipEntry{
		{name: "manifest.json", data: manifest},
		{name: TemplatePath, data: chatTemplate},
		{name: "audio/pkgaudio_0001.wav", data: wav},
	})

	if _, err := pkgSvc.Import(ctx, data); !errors.Is(err, ErrAudioTargetInvalid) {
		t.Fatalf("Import() error = %v, want ErrAudioTargetInvalid", err)
	}
	// "before any asset is even staged" - no audio asset must have been
	// uploaded despite the rejection.
	assets, err := audioSvc.List(ctx)
	if err != nil {
		t.Fatalf("audioSvc.List() returned an error: %v", err)
	}
	if len(assets) != 0 {
		t.Errorf("expected zero audio assets after a rejected chat-target audio import, got %d", len(assets))
	}
}

func TestService_ExportTemplate_AudioPreset_WritesV2Manifest(t *testing.T) {
	pkgSvc, _, _, _ := newTestServiceWithAudio(t)
	ctx := context.Background()

	imported, err := pkgSvc.Import(ctx, validPackageWithAudioZip(t))
	if err != nil {
		t.Fatalf("Import() returned an error: %v", err)
	}

	exported, err := pkgSvc.ExportTemplate(ctx, imported.ID)
	if err != nil {
		t.Fatalf("ExportTemplate() returned an error: %v", err)
	}
	validated, err := ReadArchive(exported)
	if err != nil {
		t.Fatalf("ReadArchive() of the exported audio package returned an error: %v", err)
	}
	if validated.Manifest.SchemaVersion != ManifestSchemaVersionV2 {
		t.Errorf("SchemaVersion = %d, want %d for an audio-bearing template", validated.Manifest.SchemaVersion, ManifestSchemaVersionV2)
	}
	if validated.Manifest.AlertAudio == nil || !validated.Manifest.AlertAudio.SoundEnabled {
		t.Fatalf("exported manifest.AlertAudio = %+v, want the sound preset", validated.Manifest.AlertAudio)
	}
	if len(validated.AudioAssets) != 1 {
		t.Fatalf("exported manifest has %d audio assets, want 1", len(validated.AudioAssets))
	}

	reimported, err := pkgSvc.Import(ctx, exported)
	if err != nil {
		t.Fatalf("re-Import() of an exported audio package returned an error: %v", err)
	}
	if reimported.AlertAudio == nil || !reimported.AlertAudio.SoundEnabled || reimported.AlertAudio.SoundAssetID == imported.AlertAudio.SoundAssetID {
		t.Errorf("reimported.AlertAudio = %+v, want a fresh local sound asset id", reimported.AlertAudio)
	}
}

func TestService_ExportTemplate_NoAudioPreset_StaysV1(t *testing.T) {
	pkgSvc, _, tmplSvc := newTestService(t)
	ctx := context.Background()

	doc := assetFreeDocument(t)
	tmpl, err := tmplSvc.Create(ctx, visualtemplate.TargetAlert, "No audio", "", "", "", doc)
	if err != nil {
		t.Fatalf("Create() returned an error: %v", err)
	}
	data, err := pkgSvc.ExportTemplate(ctx, tmpl.ID)
	if err != nil {
		t.Fatalf("ExportTemplate() returned an error: %v", err)
	}
	validated, err := ReadArchive(data)
	if err != nil {
		t.Fatalf("ReadArchive() returned an error: %v", err)
	}
	if validated.Manifest.SchemaVersion != ManifestSchemaVersionV1 {
		t.Errorf("SchemaVersion = %d, want %d for a template with no audio preset", validated.Manifest.SchemaVersion, ManifestSchemaVersionV1)
	}
	if validated.Manifest.AlertAudio != nil || len(validated.Manifest.AudioAssets) != 0 {
		t.Errorf("expected no audio manifest objects, got AlertAudio=%+v AudioAssets=%+v", validated.Manifest.AlertAudio, validated.Manifest.AudioAssets)
	}
}
