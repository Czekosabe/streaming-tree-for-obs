package audioasset

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/streaming-tree/server/internal/domain/visualasset"
)

// Clock returns the current time - injected everywhere in this project so
// tests are deterministic (see internal/domain/visualasset.Clock).
type Clock func() time.Time

// Service is the validated façade over Repository/*visualasset.FileStore -
// never bypassed by an HTTP handler or by internal/audio's own injected
// resolver, exactly like every other domain package's own Service. store
// is a *second*, independent visualasset.FileStore instance rooted at a
// sibling directory (docs/alert-audio.md §5.1) - this package reuses that
// type's own generic content-addressed blob primitive directly rather than
// duplicating it, and translates its sentinel errors into this package's
// own at every call site below.
type Service struct {
	repo  Repository
	store *visualasset.FileStore
	now   Clock
}

func NewService(repo Repository, store *visualasset.FileStore, now Clock) *Service {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{repo: repo, store: store, now: now}
}

func translateStoreErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, visualasset.ErrTooLarge) {
		return ErrTooLarge
	}
	if errors.Is(err, visualasset.ErrNotFound) {
		return ErrNotFound
	}
	return fmt.Errorf("%w: %v", ErrStorage, err)
}

func validateMetadataBounds(displayName string) error {
	if n := codePointLen(displayName); n > MaxDisplayNameCodePoints {
		return fmt.Errorf("%w: display name must be at most %d characters", ErrInvalid, MaxDisplayNameCodePoints)
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
// declared-media-type agreement, size bound, WAV structural validation and
// duration bound, bounded metadata text), installs it into the shared
// content-addressed blob store, and creates a new logical Asset row
// pointing at it (docs/alert-audio.md §5.3/§5.4). source is either
// SourceUpload (a direct manual upload) or SourcePackage (created while
// importing a package v2, docs/alert-audio.md §10.4) - callers never
// invent a third value.
func (s *Service) Upload(ctx context.Context, data []byte, ext, declaredMediaType, displayName, source string) (Asset, error) {
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
	durationMS, err := ValidateWAV(data)
	if err != nil {
		return Asset{}, err
	}
	if err := validateMetadataBounds(displayName); err != nil {
		return Asset{}, err
	}

	sha, size, err := s.store.WriteBlob(bytes.NewReader(data), maxBytes)
	if err != nil {
		return Asset{}, translateStoreErr(err)
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
		blob = Blob{
			SHA256: sha, MediaType: detected, ByteSize: size, DurationMS: durationMS,
			StorageName: sha, PublicToken: token, CreatedAt: now,
		}
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
		DisplayName: displayName, Source: source, CreatedAt: now, UpdatedAt: now,
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

// List returns every managed audio asset, each with its resolved Blob.
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

// UpdateMetadata replaces an asset's own bounded display name - never its
// blob, kind, or id.
func (s *Service) UpdateMetadata(ctx context.Context, id, displayName string) (Asset, error) {
	if err := validateMetadataBounds(displayName); err != nil {
		return Asset{}, err
	}
	asset, err := s.repo.UpdateAssetMetadata(ctx, id, displayName)
	if err != nil {
		return Asset{}, err
	}
	return s.resolveBlob(ctx, asset)
}

// Delete removes a logical asset - rejected with ErrInUse if any alert
// rule or template audio preset still references it (docs/alert-audio.md
// §5.6). Never physically deletes the underlying blob - see Reconcile.
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

// ReferenceCount exposes the same guard Delete uses, for a management DTO
// that wants to show "N rules/templates use this asset" without naming
// which ones.
func (s *Service) ReferenceCount(ctx context.Context, id string) (int, error) {
	return s.repo.ReferenceCount(ctx, id)
}

// SetRuleAssetRefs/SetTemplateAssetRefs/ClearRuleRefs/ClearTemplateRefs
// pass straight through to the repository - exposed on Service (rather
// than making callers hold a raw Repository) so every caller goes through
// the one façade.
func (s *Service) SetRuleAssetRefs(ctx context.Context, ruleID string, assetIDs []string) error {
	return s.repo.SetRuleAssetRefs(ctx, ruleID, assetIDs)
}

func (s *Service) SetTemplateAssetRefs(ctx context.Context, templateID string, assetIDs []string) error {
	return s.repo.SetTemplateAssetRefs(ctx, templateID, assetIDs)
}

func (s *Service) ClearRuleRefs(ctx context.Context, ruleID string) error {
	return s.repo.ClearRuleRefs(ctx, ruleID)
}

func (s *Service) ClearTemplateRefs(ctx context.Context, templateID string) error {
	return s.repo.ClearTemplateRefs(ctx, templateID)
}

// OpenBlob streams one blob's bytes by hash - used by
// internal/audio's own injected AudioAssetResolver (docs/alert-audio.md
// §8.2) to resolve a SourceAlertSound item's bytes at enqueue/promotion
// time. Never used by any public route directly - internal/audio's own
// existing /api/public/audio/{slug}/bytes/{token} route is the only
// public byte-serving surface a rule-owned sound is ever exposed through.
func (s *Service) OpenBlob(sha256Hex string) (*os.File, error) {
	f, err := s.store.Open(sha256Hex)
	if err != nil {
		return nil, translateStoreErr(err)
	}
	return f, nil
}

// ResolveSoundAsset reads a sound asset's full bytes and content type by
// local asset id - the concrete implementation internal/audio's
// AudioAssetResolver interface is satisfied with (docs/alert-audio.md
// §8.2). Returns ok=false for any resolution failure (unknown asset,
// missing blob) rather than an error - the caller (internal/audio) only
// needs a safe "could this be played" signal, and every failure reason is
// already logged/diagnosable at this layer.
func (s *Service) ResolveSoundAsset(ctx context.Context, assetID string) (data []byte, contentType string, ok bool) {
	asset, err := s.Get(ctx, assetID)
	if err != nil || asset.Blob == nil {
		return nil, "", false
	}
	f, err := s.OpenBlob(asset.Blob.SHA256)
	if err != nil {
		return nil, "", false
	}
	defer f.Close()
	bytes, err := io.ReadAll(f)
	if err != nil {
		return nil, "", false
	}
	return bytes, string(asset.Blob.MediaType), true
}

// ReconcileResult summarizes one startup reconciliation pass, mirroring
// visualasset.ReconcileResult's own shape - purely diagnostic; callers log
// it, never fail startup because of it.
type ReconcileResult struct {
	OrphanBlobFilesRemoved int
	OrphanBlobRowsRemoved  int
	MissingBlobFiles       []string
}

// Reconcile runs the startup audio-asset-store safety pass (docs/alert-
// audio.md §5.7): remove a blob row/file pair with zero references in
// either reference table, remove an untracked on-disk blob file with no
// matching row, and report (without failing) a database row whose backing
// file is missing. Startup-only, not periodic, mirroring
// visualasset.Service.Reconcile's own convention exactly.
func (s *Service) Reconcile(ctx context.Context) (ReconcileResult, error) {
	var result ReconcileResult

	if err := s.store.EnsureDirs(); err != nil {
		return result, translateStoreErr(err)
	}

	orphanHashes, err := s.repo.ListOrphanBlobHashes(ctx)
	if err != nil {
		return result, err
	}
	for _, h := range orphanHashes {
		if err := s.store.Delete(h); err != nil {
			return result, translateStoreErr(err)
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
		return result, translateStoreErr(err)
	}
	onDiskSet := make(map[string]bool, len(onDisk))
	for _, h := range onDisk {
		onDiskSet[h] = true
		if !known[h] {
			if err := s.store.Delete(h); err != nil {
				return result, translateStoreErr(err)
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
