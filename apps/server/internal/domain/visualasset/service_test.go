package visualasset

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

// fakeRepository is a minimal in-memory visualasset.Repository, mirroring
// the fakeRepository pattern already used by internal/domain/
// visualtemplate and internal/domain/visualdesign's own service tests.
type fakeRepository struct {
	blobs        map[string]Blob
	blobsByToken map[string]string
	assets       map[string]Asset
	designRefs   map[string]map[string]bool
	templateRefs map[string]map[string]bool
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		blobs: map[string]Blob{}, blobsByToken: map[string]string{}, assets: map[string]Asset{},
		designRefs: map[string]map[string]bool{}, templateRefs: map[string]map[string]bool{},
	}
}

func (f *fakeRepository) CreateBlob(ctx context.Context, blob Blob) error {
	if _, ok := f.blobs[blob.SHA256]; ok {
		return nil
	}
	f.blobs[blob.SHA256] = blob
	f.blobsByToken[blob.PublicToken] = blob.SHA256
	return nil
}

func (f *fakeRepository) GetBlob(ctx context.Context, sha256Hex string) (Blob, bool, error) {
	b, ok := f.blobs[sha256Hex]
	return b, ok, nil
}

func (f *fakeRepository) GetBlobByPublicToken(ctx context.Context, token string) (Blob, bool, error) {
	sha, ok := f.blobsByToken[token]
	if !ok {
		return Blob{}, false, nil
	}
	return f.blobs[sha], true, nil
}

func (f *fakeRepository) CreateAsset(ctx context.Context, asset Asset) error {
	f.assets[asset.ID] = asset
	return nil
}

func (f *fakeRepository) GetAsset(ctx context.Context, id string) (Asset, bool, error) {
	a, ok := f.assets[id]
	return a, ok, nil
}

func (f *fakeRepository) ListAssets(ctx context.Context) ([]Asset, error) {
	out := make([]Asset, 0, len(f.assets))
	for _, a := range f.assets {
		out = append(out, a)
	}
	return out, nil
}

func (f *fakeRepository) UpdateAssetMetadata(ctx context.Context, id, displayName, author, license, notice string) (Asset, error) {
	a, ok := f.assets[id]
	if !ok {
		return Asset{}, ErrNotFound
	}
	a.DisplayName, a.Author, a.License, a.Notice = displayName, author, license, notice
	f.assets[id] = a
	return a, nil
}

func (f *fakeRepository) DeleteAsset(ctx context.Context, id string) error {
	delete(f.assets, id)
	return nil
}

func (f *fakeRepository) ReferenceCount(ctx context.Context, assetID string) (int, error) {
	count := 0
	for _, set := range f.designRefs {
		if set[assetID] {
			count++
		}
	}
	for _, set := range f.templateRefs {
		if set[assetID] {
			count++
		}
	}
	return count, nil
}

func (f *fakeRepository) SetDesignAssetRefs(ctx context.Context, designID string, assetIDs []string) error {
	set := map[string]bool{}
	for _, id := range assetIDs {
		set[id] = true
	}
	f.designRefs[designID] = set
	return nil
}

func (f *fakeRepository) SetTemplateAssetRefs(ctx context.Context, templateID string, assetIDs []string) error {
	set := map[string]bool{}
	for _, id := range assetIDs {
		set[id] = true
	}
	f.templateRefs[templateID] = set
	return nil
}

func (f *fakeRepository) ClearDesignRefs(ctx context.Context, designID string) error {
	delete(f.designRefs, designID)
	return nil
}

func (f *fakeRepository) ClearTemplateRefs(ctx context.Context, templateID string) error {
	delete(f.templateRefs, templateID)
	return nil
}

func (f *fakeRepository) ListOrphanBlobHashes(ctx context.Context) ([]string, error) {
	used := map[string]bool{}
	for _, a := range f.assets {
		used[a.BlobSHA256] = true
	}
	var out []string
	for sha := range f.blobs {
		if !used[sha] {
			out = append(out, sha)
		}
	}
	return out, nil
}

func (f *fakeRepository) ListBlobHashes(ctx context.Context) ([]string, error) {
	out := make([]string, 0, len(f.blobs))
	for sha := range f.blobs {
		out = append(out, sha)
	}
	return out, nil
}

func (f *fakeRepository) DeleteBlobRow(ctx context.Context, sha256Hex string) error {
	blob := f.blobs[sha256Hex]
	delete(f.blobs, sha256Hex)
	delete(f.blobsByToken, blob.PublicToken)
	return nil
}

var _ Repository = (*fakeRepository)(nil)

func newTestServicePair(t *testing.T) (*Service, *fakeRepository) {
	t.Helper()
	repo := newFakeRepository()
	store := NewFileStore(filepath.Join(t.TempDir(), "assets"))
	if err := store.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs() returned an error: %v", err)
	}
	return NewService(repo, store, nil), repo
}

func TestService_Upload_CreatesAssetWithLocalID(t *testing.T) {
	svc, _ := newTestServicePair(t)
	ctx := context.Background()

	png := testPNG(t, 4, 4)
	asset, err := svc.Upload(ctx, png, "png", "image/png", "My badge", "Me", "CC0", "", SourceUpload)
	if err != nil {
		t.Fatalf("Upload() returned an error: %v", err)
	}
	if asset.ID == "" || asset.ID[:6] != "asset_" {
		t.Errorf("expected a server-generated asset_ id, got %q", asset.ID)
	}
	if asset.Blob == nil || asset.Blob.PublicToken == "" {
		t.Fatalf("expected a resolved blob with a public token, got %+v", asset.Blob)
	}
}

func TestService_Upload_DuplicateBytesShareBlobButKeepDistinctMetadata(t *testing.T) {
	svc, _ := newTestServicePair(t)
	ctx := context.Background()

	png := testPNG(t, 4, 4)
	a1, err := svc.Upload(ctx, png, "png", "image/png", "First name", "", "", "", SourceUpload)
	if err != nil {
		t.Fatalf("Upload() returned an error: %v", err)
	}
	a2, err := svc.Upload(ctx, png, "png", "image/png", "Second name", "", "", "", SourceUpload)
	if err != nil {
		t.Fatalf("Upload() returned an error: %v", err)
	}
	if a1.ID == a2.ID {
		t.Error("two separate uploads should get distinct logical asset ids")
	}
	if a1.BlobSHA256 != a2.BlobSHA256 {
		t.Error("identical bytes should share one blob")
	}
	if a1.DisplayName == a2.DisplayName {
		t.Error("distinct uploads should keep distinct metadata even with identical bytes")
	}
}

func TestService_Upload_RejectsMismatchedType(t *testing.T) {
	svc, _ := newTestServicePair(t)
	ctx := context.Background()

	if _, err := svc.Upload(ctx, []byte("<svg></svg>"), "png", "image/png", "", "", "", "", SourceUpload); !errors.Is(err, ErrUnsupported) {
		t.Errorf("expected ErrUnsupported for SVG content, got %v", err)
	}
}

func TestService_Delete_RejectsWhenInUse(t *testing.T) {
	svc, _ := newTestServicePair(t)
	ctx := context.Background()

	png := testPNG(t, 4, 4)
	asset, err := svc.Upload(ctx, png, "png", "image/png", "", "", "", "", SourceUpload)
	if err != nil {
		t.Fatalf("Upload() returned an error: %v", err)
	}
	if err := svc.SetDesignAssetRefs(ctx, "design-1", []string{asset.ID}); err != nil {
		t.Fatalf("SetDesignAssetRefs() returned an error: %v", err)
	}
	if err := svc.Delete(ctx, asset.ID); !errors.Is(err, ErrInUse) {
		t.Fatalf("expected ErrInUse, got %v", err)
	}
	if err := svc.ClearDesignRefs(ctx, "design-1"); err != nil {
		t.Fatalf("ClearDesignRefs() returned an error: %v", err)
	}
	if err := svc.Delete(ctx, asset.ID); err != nil {
		t.Fatalf("Delete() after clearing refs returned an error: %v", err)
	}
}

func TestService_PublicURLForAsset(t *testing.T) {
	svc, _ := newTestServicePair(t)
	ctx := context.Background()

	png := testPNG(t, 4, 4)
	asset, err := svc.Upload(ctx, png, "png", "image/png", "", "", "", "", SourceUpload)
	if err != nil {
		t.Fatalf("Upload() returned an error: %v", err)
	}
	path, mediaType, ok := svc.PublicURLForAsset(ctx, asset.ID)
	if !ok {
		t.Fatal("expected PublicURLForAsset to resolve")
	}
	if mediaType != MediaPNG {
		t.Errorf("mediaType = %q, want image/png", mediaType)
	}
	if path == "" || path[:len("/api/public/visual-assets/")] != "/api/public/visual-assets/" {
		t.Errorf("unexpected public path %q", path)
	}
	if _, _, ok := svc.PublicURLForAsset(ctx, "asset_doesnotexist"); ok {
		t.Error("expected PublicURLForAsset to fail for an unknown asset")
	}
}

func TestService_Reconcile_RemovesOrphans(t *testing.T) {
	svc, repo := newTestServicePair(t)
	ctx := context.Background()

	png := testPNG(t, 4, 4)
	asset, err := svc.Upload(ctx, png, "png", "image/png", "", "", "", "", SourceUpload)
	if err != nil {
		t.Fatalf("Upload() returned an error: %v", err)
	}
	if err := repo.DeleteAsset(ctx, asset.ID); err != nil {
		t.Fatalf("DeleteAsset() returned an error: %v", err)
	}

	result, err := svc.Reconcile(ctx)
	if err != nil {
		t.Fatalf("Reconcile() returned an error: %v", err)
	}
	if result.OrphanBlobRowsRemoved != 1 {
		t.Errorf("OrphanBlobRowsRemoved = %d, want 1", result.OrphanBlobRowsRemoved)
	}
	if _, found, _ := svc.repo.GetBlob(ctx, asset.BlobSHA256); found {
		t.Error("orphan blob row should have been removed")
	}
}
