package audioasset

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/streaming-tree/server/internal/domain/visualasset"
)

// fakeRepository is a minimal in-memory audioasset.Repository, mirroring
// the fakeRepository pattern already used by internal/domain/visualasset's
// own service tests.
type fakeRepository struct {
	blobs        map[string]Blob
	assets       map[string]Asset
	ruleRefs     map[string]map[string]bool
	templateRefs map[string]map[string]bool
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		blobs: map[string]Blob{}, assets: map[string]Asset{},
		ruleRefs: map[string]map[string]bool{}, templateRefs: map[string]map[string]bool{},
	}
}

func (f *fakeRepository) CreateBlob(ctx context.Context, blob Blob) error {
	if _, ok := f.blobs[blob.SHA256]; ok {
		return nil
	}
	f.blobs[blob.SHA256] = blob
	return nil
}

func (f *fakeRepository) GetBlob(ctx context.Context, sha256Hex string) (Blob, bool, error) {
	b, ok := f.blobs[sha256Hex]
	return b, ok, nil
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

func (f *fakeRepository) UpdateAssetMetadata(ctx context.Context, id, displayName string) (Asset, error) {
	a, ok := f.assets[id]
	if !ok {
		return Asset{}, ErrNotFound
	}
	a.DisplayName = displayName
	f.assets[id] = a
	return a, nil
}

func (f *fakeRepository) DeleteAsset(ctx context.Context, id string) error {
	delete(f.assets, id)
	return nil
}

func (f *fakeRepository) ReferenceCount(ctx context.Context, assetID string) (int, error) {
	count := 0
	for _, set := range f.ruleRefs {
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

func (f *fakeRepository) SetRuleAssetRefs(ctx context.Context, ruleID string, assetIDs []string) error {
	set := map[string]bool{}
	for _, id := range assetIDs {
		set[id] = true
	}
	f.ruleRefs[ruleID] = set
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

func (f *fakeRepository) ClearRuleRefs(ctx context.Context, ruleID string) error {
	delete(f.ruleRefs, ruleID)
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
	delete(f.blobs, sha256Hex)
	return nil
}

var _ Repository = (*fakeRepository)(nil)

func newTestServicePair(t *testing.T) (*Service, *fakeRepository) {
	t.Helper()
	repo := newFakeRepository()
	store := visualasset.NewFileStore(filepath.Join(t.TempDir(), "assets"))
	if err := store.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs() returned an error: %v", err)
	}
	return NewService(repo, store, nil), repo
}

func TestService_Upload_CreatesAssetWithLocalID(t *testing.T) {
	svc, _ := newTestServicePair(t)
	ctx := context.Background()

	wav := buildWAV(t, 44100, 2, 16, 4410)
	asset, err := svc.Upload(ctx, wav, "wav", "audio/wav", "Coin chime", SourceUpload)
	if err != nil {
		t.Fatalf("Upload() returned an error: %v", err)
	}
	if len(asset.ID) < len(AssetIDPrefix) || asset.ID[:len(AssetIDPrefix)] != AssetIDPrefix {
		t.Errorf("expected a server-generated %s id, got %q", AssetIDPrefix, asset.ID)
	}
	if asset.Blob == nil {
		t.Fatalf("expected a resolved blob, got nil")
	}
	if asset.Blob.DurationMS <= 0 {
		t.Errorf("expected a positive computed duration, got %d", asset.Blob.DurationMS)
	}
	if asset.Blob.MediaType != MediaWAV {
		t.Errorf("MediaType = %q, want %q", asset.Blob.MediaType, MediaWAV)
	}
}

func TestService_Upload_DuplicateBytesShareBlobButKeepDistinctMetadata(t *testing.T) {
	svc, repo := newTestServicePair(t)
	ctx := context.Background()
	wav := buildWAV(t, 44100, 1, 16, 100)

	a1, err := svc.Upload(ctx, wav, "wav", "audio/wav", "First name", SourceUpload)
	if err != nil {
		t.Fatalf("first Upload() error = %v", err)
	}
	a2, err := svc.Upload(ctx, wav, "wav", "audio/wav", "Second name", SourceUpload)
	if err != nil {
		t.Fatalf("second Upload() error = %v", err)
	}
	if a1.ID == a2.ID {
		t.Fatalf("expected two distinct logical asset IDs, got the same %q twice", a1.ID)
	}
	if a1.BlobSHA256 != a2.BlobSHA256 {
		t.Errorf("expected identical bytes to share one blob, got %q and %q", a1.BlobSHA256, a2.BlobSHA256)
	}
	if len(repo.blobs) != 1 {
		t.Errorf("expected exactly 1 deduplicated blob row, got %d", len(repo.blobs))
	}
	if a1.DisplayName == a2.DisplayName {
		t.Errorf("expected distinct display names to be preserved, both are %q", a1.DisplayName)
	}
}

func TestService_Upload_RejectsUnsupportedFormat(t *testing.T) {
	svc, _ := newTestServicePair(t)
	ctx := context.Background()
	mp3 := []byte{0xFF, 0xFB, 0x90, 0x00, 0, 0, 0, 0}
	if _, err := svc.Upload(ctx, mp3, "mp3", "audio/mpeg", "", SourceUpload); !errors.Is(err, ErrUnsupported) {
		t.Errorf("Upload() error = %v, want ErrUnsupported", err)
	}
}

func TestService_Upload_RejectsOversizedFile(t *testing.T) {
	svc, _ := newTestServicePair(t)
	ctx := context.Background()
	// 44 header bytes + enough data to exceed MaxSoundBytes.
	numSamples := int(MaxSoundBytes/2) + 1000 // mono 16-bit: 2 bytes/sample
	wav := buildWAV(t, 44100, 1, 16, numSamples)
	if _, err := svc.Upload(ctx, wav, "wav", "audio/wav", "", SourceUpload); !errors.Is(err, ErrTooLarge) {
		t.Errorf("Upload() error = %v, want ErrTooLarge", err)
	}
}

func TestService_Upload_RejectsOverlongDisplayName(t *testing.T) {
	svc, _ := newTestServicePair(t)
	ctx := context.Background()
	wav := buildWAV(t, 44100, 1, 16, 100)
	long := make([]rune, MaxDisplayNameCodePoints+1)
	for i := range long {
		long[i] = 'x'
	}
	if _, err := svc.Upload(ctx, wav, "wav", "audio/wav", string(long), SourceUpload); !errors.Is(err, ErrInvalid) {
		t.Errorf("Upload() error = %v, want ErrInvalid", err)
	}
}

func TestService_Delete_RejectsWhenReferenced(t *testing.T) {
	svc, repo := newTestServicePair(t)
	ctx := context.Background()
	wav := buildWAV(t, 44100, 1, 16, 100)
	asset, err := svc.Upload(ctx, wav, "wav", "audio/wav", "", SourceUpload)
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if err := repo.SetRuleAssetRefs(ctx, "rule_1", []string{asset.ID}); err != nil {
		t.Fatalf("SetRuleAssetRefs() error = %v", err)
	}
	if err := svc.Delete(ctx, asset.ID); !errors.Is(err, ErrInUse) {
		t.Errorf("Delete() error = %v, want ErrInUse", err)
	}
}

func TestService_Delete_SucceedsWhenUnreferenced(t *testing.T) {
	svc, _ := newTestServicePair(t)
	ctx := context.Background()
	wav := buildWAV(t, 44100, 1, 16, 100)
	asset, err := svc.Upload(ctx, wav, "wav", "audio/wav", "", SourceUpload)
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if err := svc.Delete(ctx, asset.ID); err != nil {
		t.Errorf("Delete() error = %v, want nil", err)
	}
	if _, err := svc.Get(ctx, asset.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get() after delete error = %v, want ErrNotFound", err)
	}
}

func TestService_Delete_NeverPhysicallyRemovesTheBlob(t *testing.T) {
	svc, repo := newTestServicePair(t)
	ctx := context.Background()
	wav := buildWAV(t, 44100, 1, 16, 100)
	asset, err := svc.Upload(ctx, wav, "wav", "audio/wav", "", SourceUpload)
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if err := svc.Delete(ctx, asset.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, ok := repo.blobs[asset.BlobSHA256]; !ok {
		t.Error("Delete() removed the blob row directly - it must survive until the next Reconcile pass")
	}
	f, err := svc.OpenBlob(asset.BlobSHA256)
	if err != nil {
		t.Errorf("blob file was physically removed by Delete(), OpenBlob() error = %v", err)
	} else {
		f.Close()
	}
}

func TestService_Reconcile_RemovesOrphanBlobAfterDelete(t *testing.T) {
	svc, _ := newTestServicePair(t)
	ctx := context.Background()
	wav := buildWAV(t, 44100, 1, 16, 100)
	asset, err := svc.Upload(ctx, wav, "wav", "audio/wav", "", SourceUpload)
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if err := svc.Delete(ctx, asset.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	result, err := svc.Reconcile(ctx)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if result.OrphanBlobRowsRemoved != 1 {
		t.Errorf("OrphanBlobRowsRemoved = %d, want 1", result.OrphanBlobRowsRemoved)
	}
	if _, err := svc.OpenBlob(asset.BlobSHA256); err == nil {
		t.Error("expected the orphan blob file to be physically removed after Reconcile()")
	}
}

func TestService_Reconcile_NeverRemovesAReferencedBlob(t *testing.T) {
	svc, repo := newTestServicePair(t)
	ctx := context.Background()
	wav := buildWAV(t, 44100, 1, 16, 100)
	asset, err := svc.Upload(ctx, wav, "wav", "audio/wav", "", SourceUpload)
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if err := repo.SetRuleAssetRefs(ctx, "rule_1", []string{asset.ID}); err != nil {
		t.Fatalf("SetRuleAssetRefs() error = %v", err)
	}
	if _, err := svc.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if _, err := svc.Get(ctx, asset.ID); err != nil {
		t.Errorf("referenced asset was removed by Reconcile(): Get() error = %v", err)
	}
	f, err := svc.OpenBlob(asset.BlobSHA256)
	if err != nil {
		t.Errorf("referenced blob file was removed by Reconcile(): OpenBlob() error = %v", err)
	} else {
		f.Close()
	}
}

func TestService_ResolveSoundAsset_ReturnsBytesAndContentType(t *testing.T) {
	svc, _ := newTestServicePair(t)
	ctx := context.Background()
	wav := buildWAV(t, 44100, 1, 16, 100)
	asset, err := svc.Upload(ctx, wav, "wav", "audio/wav", "", SourceUpload)
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	data, contentType, ok := svc.ResolveSoundAsset(ctx, asset.ID)
	if !ok {
		t.Fatal("ResolveSoundAsset() ok = false, want true")
	}
	if contentType != string(MediaWAV) {
		t.Errorf("contentType = %q, want %q", contentType, MediaWAV)
	}
	if len(data) != len(wav) {
		t.Errorf("resolved %d bytes, want %d (byte-for-byte, no re-encoding)", len(data), len(wav))
	}
}

func TestService_ResolveSoundAsset_FalseForUnknownID(t *testing.T) {
	svc, _ := newTestServicePair(t)
	if _, _, ok := svc.ResolveSoundAsset(context.Background(), "audioasset_doesnotexist"); ok {
		t.Error("ResolveSoundAsset() ok = true for an unknown asset id, want false")
	}
}

func TestService_UpdateMetadata_NeverChangesBlobOrKind(t *testing.T) {
	svc, _ := newTestServicePair(t)
	ctx := context.Background()
	wav := buildWAV(t, 44100, 1, 16, 100)
	asset, err := svc.Upload(ctx, wav, "wav", "audio/wav", "Old name", SourceUpload)
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	updated, err := svc.UpdateMetadata(ctx, asset.ID, "New name")
	if err != nil {
		t.Fatalf("UpdateMetadata() error = %v", err)
	}
	if updated.DisplayName != "New name" {
		t.Errorf("DisplayName = %q, want %q", updated.DisplayName, "New name")
	}
	if updated.BlobSHA256 != asset.BlobSHA256 || updated.Kind != asset.Kind {
		t.Error("UpdateMetadata() must never change the blob reference or kind")
	}
}
