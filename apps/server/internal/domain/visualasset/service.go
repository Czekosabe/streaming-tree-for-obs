package visualasset

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"time"
)

// Clock returns the current time - injected everywhere in this project
// so tests are deterministic (see internal/domain/visualdesign.Clock).
type Clock func() time.Time

// Service is the validated façade over Repository/FileStore - never
// bypassed by an HTTP handler, exactly like every other domain package's
// own Service.
type Service struct {
	repo  Repository
	store *FileStore
	now   Clock
}

func NewService(repo Repository, store *FileStore, now Clock) *Service {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{repo: repo, store: store, now: now}
}

func validateMetadataBounds(displayName, author, license, notice string) error {
	if n := codePointLen(displayName); n > MaxDisplayNameCodePoints {
		return fmt.Errorf("%w: display name must be at most %d characters", ErrInvalid, MaxDisplayNameCodePoints)
	}
	if n := codePointLen(author); n > MaxAuthorCodePoints {
		return fmt.Errorf("%w: author must be at most %d characters", ErrInvalid, MaxAuthorCodePoints)
	}
	if n := codePointLen(license); n > MaxLicenseCodePoints {
		return fmt.Errorf("%w: license must be at most %d characters", ErrInvalid, MaxLicenseCodePoints)
	}
	if n := codePointLen(notice); n > MaxNoticeCodePoints {
		return fmt.Errorf("%w: notice must be at most %d characters", ErrInvalid, MaxNoticeCodePoints)
	}
	return nil
}

func codePointLen(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}

// Upload validates data as a whole (independent signature/extension/
// declared-media-type agreement, size bound, image-dimension bound where
// applicable, bounded metadata text), installs it into the content-
// addressed blob store, and creates a new logical Asset row pointing at
// it (docs/visual-template-packages.md §14/§17). source is either
// SourceUpload (a direct manual upload) or SourcePackage (created while
// importing a package, docs/visual-template-packages.md §20) - callers
// never invent a third value.
func (s *Service) Upload(ctx context.Context, data []byte, ext, declaredMediaType string, displayName, author, license, notice, source string) (Asset, error) {
	detected, err := VerifyTypeAgreement(data, ext, declaredMediaType)
	if err != nil {
		return Asset{}, err
	}
	kind, ok := detected.KindOf()
	if !ok {
		return Asset{}, fmt.Errorf("%w: %q has no known asset kind", ErrUnsupported, detected)
	}
	maxBytes := MaxBytesFor(kind)
	if int64(len(data)) > maxBytes {
		return Asset{}, fmt.Errorf("%w: asset is %d bytes, exceeding the %d byte limit for %s assets", ErrTooLarge, len(data), maxBytes, kind)
	}
	if kind == KindImage {
		if err := ValidateImageDimensions(data, detected); err != nil {
			return Asset{}, err
		}
	}
	if err := validateMetadataBounds(displayName, author, license, notice); err != nil {
		return Asset{}, err
	}

	sha, size, err := s.store.WriteBlob(bytes.NewReader(data), maxBytes)
	if err != nil {
		return Asset{}, err
	}

	now := s.now()
	blob, found, err := s.repo.GetBlob(ctx, sha)
	if err != nil {
		return Asset{}, err
	}
	if !found {
		token, tokErr := NewPublicToken()
		if tokErr != nil {
			return Asset{}, fmt.Errorf("%w: %v", ErrStorage, tokErr)
		}
		blob = Blob{SHA256: sha, MediaType: detected, ByteSize: size, StorageName: sha, PublicToken: token, CreatedAt: now}
		if err := s.repo.CreateBlob(ctx, blob); err != nil {
			return Asset{}, err
		}
	}

	id, err := NewAssetID()
	if err != nil {
		return Asset{}, fmt.Errorf("%w: %v", ErrStorage, err)
	}
	asset := Asset{
		ID: id, BlobSHA256: sha, Kind: kind,
		DisplayName: displayName, Author: author, License: license, Notice: notice,
		Source: source, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.repo.CreateAsset(ctx, asset); err != nil {
		return Asset{}, err
	}
	blobCopy := blob
	asset.Blob = &blobCopy
	return asset, nil
}

// Get returns one asset (with its resolved Blob) by local id.
func (s *Service) Get(ctx context.Context, id string) (Asset, error) {
	asset, found, err := s.repo.GetAsset(ctx, id)
	if err != nil {
		return Asset{}, err
	}
	if !found {
		return Asset{}, ErrNotFound
	}
	return s.resolveBlob(ctx, asset)
}

func (s *Service) resolveBlob(ctx context.Context, asset Asset) (Asset, error) {
	blob, found, err := s.repo.GetBlob(ctx, asset.BlobSHA256)
	if err != nil {
		return Asset{}, err
	}
	if found {
		asset.Blob = &blob
	}
	return asset, nil
}

// List returns every managed asset, each with its resolved Blob.
func (s *Service) List(ctx context.Context) ([]Asset, error) {
	assets, err := s.repo.ListAssets(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Asset, 0, len(assets))
	for _, a := range assets {
		resolved, err := s.resolveBlob(ctx, a)
		if err != nil {
			return nil, err
		}
		out = append(out, resolved)
	}
	return out, nil
}

// UpdateMetadata replaces an asset's own bounded metadata text - never
// its blob, kind, or id.
func (s *Service) UpdateMetadata(ctx context.Context, id, displayName, author, license, notice string) (Asset, error) {
	if err := validateMetadataBounds(displayName, author, license, notice); err != nil {
		return Asset{}, err
	}
	asset, err := s.repo.UpdateAssetMetadata(ctx, id, displayName, author, license, notice)
	if err != nil {
		return Asset{}, err
	}
	return s.resolveBlob(ctx, asset)
}

// Delete removes a logical asset - rejected with ErrInUse if any design
// or template still references it (docs/visual-template-packages.md
// §15). Never physically deletes the underlying blob - see Reconcile.
func (s *Service) Delete(ctx context.Context, id string) error {
	if _, found, err := s.repo.GetAsset(ctx, id); err != nil {
		return err
	} else if !found {
		return ErrNotFound
	}
	count, err := s.repo.ReferenceCount(ctx, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrInUse
	}
	return s.repo.DeleteAsset(ctx, id)
}

// ReferenceCount exposes the same guard Delete uses, for a management
// DTO that wants to show "N designs/templates use this asset" without
// naming which ones (docs/visual-template-packages.md §15).
func (s *Service) ReferenceCount(ctx context.Context, id string) (int, error) {
	return s.repo.ReferenceCount(ctx, id)
}

// SetDesignAssetRefs/SetTemplateAssetRefs/ClearDesignRefs/
// ClearTemplateRefs pass straight through to the repository - exposed on
// Service (rather than making callers hold a raw Repository) so every
// caller goes through the one façade.
func (s *Service) SetDesignAssetRefs(ctx context.Context, designID string, assetIDs []string) error {
	return s.repo.SetDesignAssetRefs(ctx, designID, assetIDs)
}

func (s *Service) SetTemplateAssetRefs(ctx context.Context, templateID string, assetIDs []string) error {
	return s.repo.SetTemplateAssetRefs(ctx, templateID, assetIDs)
}

func (s *Service) ClearDesignRefs(ctx context.Context, designID string) error {
	return s.repo.ClearDesignRefs(ctx, designID)
}

func (s *Service) ClearTemplateRefs(ctx context.Context, templateID string) error {
	return s.repo.ClearTemplateRefs(ctx, templateID)
}

// PublicBlobByToken resolves a public token (docs/visual-template-
// packages.md §18) to its Blob - the only lookup the public, unauth'd
// asset-serving route ever performs; it never accepts a local asset id.
func (s *Service) PublicBlobByToken(ctx context.Context, token string) (Blob, error) {
	blob, found, err := s.repo.GetBlobByPublicToken(ctx, token)
	if err != nil {
		return Blob{}, err
	}
	if !found {
		return Blob{}, ErrNotFound
	}
	return blob, nil
}

// PublicURLForAsset resolves a local asset id to the safe, app-owned
// public URL path a public visual-design presentation embeds (docs/
// visual-template-packages.md §18/§38) - the one place a local asset id
// is exchanged for its blob's own public token. path is relative
// (`/api/public/visual-assets/{token}`); the caller prefixes any scheme/
// host itself, exactly like every other public URL this project emits.
func (s *Service) PublicURLForAsset(ctx context.Context, assetID string) (path string, mediaType MediaType, ok bool) {
	asset, err := s.Get(ctx, assetID)
	if err != nil || asset.Blob == nil {
		return "", "", false
	}
	return "/api/public/visual-assets/" + asset.Blob.PublicToken, asset.Blob.MediaType, true
}

// OpenBlob streams one blob's bytes by hash - used by the public
// asset-serving handler after PublicBlobByToken resolves the token.
func (s *Service) OpenBlob(sha256Hex string) (*os.File, error) {
	return s.store.Open(sha256Hex)
}

// --- preview-staging passthroughs (docs/visual-template-packages.md
// §19) - thin wrappers so internal/domain/visualpackage never needs its
// own direct *FileStore handle, only this one Service façade. ---

func (s *Service) WritePreviewAsset(token, logicalName string, r io.Reader, maxBytes int64) (path string, sha256Hex string, size int64, err error) {
	return s.store.WritePreviewAsset(token, logicalName, r, maxBytes)
}

func (s *Service) RemovePreview(token string) error {
	return s.store.RemovePreview(token)
}

func (s *Service) PreviewExpired(token string, ttl time.Duration, now time.Time) (bool, error) {
	return s.store.PreviewExpired(token, ttl, now)
}

// ReconcileResult summarizes one startup reconciliation pass (docs/
// visual-template-packages.md §16) - purely diagnostic; callers log it,
// never fail startup because of it (a broken individual asset must never
// prevent the rest of the database from being read).
type ReconcileResult struct {
	OrphanBlobFilesRemoved int
	OrphanBlobRowsRemoved  int
	MissingBlobFiles       []string
}

// Reconcile runs the startup asset-store safety pass (docs/visual-
// template-packages.md §16): remove every leftover preview-staging
// session (no token from a previous process can still be legitimate),
// remove a blob row/file pair with zero references in either reference
// table, remove an untracked on-disk blob file with no matching row, and
// report (without failing) a database row whose backing file is missing.
func (s *Service) Reconcile(ctx context.Context) (ReconcileResult, error) {
	var result ReconcileResult

	if err := s.store.EnsureDirs(); err != nil {
		return result, err
	}
	if err := s.store.RemoveAllPreviews(); err != nil {
		return result, err
	}

	orphanHashes, err := s.repo.ListOrphanBlobHashes(ctx)
	if err != nil {
		return result, err
	}
	for _, h := range orphanHashes {
		if err := s.store.Delete(h); err != nil {
			return result, err
		}
		if err := s.repo.DeleteBlobRow(ctx, h); err != nil {
			return result, err
		}
		result.OrphanBlobRowsRemoved++
	}

	knownHashes, err := s.repo.ListBlobHashes(ctx)
	if err != nil {
		return result, err
	}
	known := make(map[string]bool, len(knownHashes))
	for _, h := range knownHashes {
		known[h] = true
	}
	onDisk, err := s.store.ListBlobFiles()
	if err != nil {
		return result, err
	}
	onDiskSet := make(map[string]bool, len(onDisk))
	for _, h := range onDisk {
		onDiskSet[h] = true
		if !known[h] {
			if err := s.store.Delete(h); err != nil {
				return result, err
			}
			result.OrphanBlobFilesRemoved++
		}
	}
	for h := range known {
		if !onDiskSet[h] {
			result.MissingBlobFiles = append(result.MissingBlobFiles, h)
		}
	}

	return result, nil
}
