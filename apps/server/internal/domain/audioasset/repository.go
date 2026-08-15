package audioasset

import "context"

// Repository is the persistence port this domain package depends on -
// implemented by internal/storage/sqlite, mirroring
// visualasset.Repository's own shape exactly (docs/alert-audio.md §5.5).
// Deliberately asset-centric only: SetRuleAssetRefs/SetTemplateAssetRefs
// accept plain owner-id strings, never an alerts.Rule or
// visualtemplate.Template - this package never imports either sibling
// domain (see this package's own doc comment).
type Repository interface {
	// CreateBlob inserts blob's metadata row if no row for its SHA256
	// already exists (content dedup, docs/alert-audio.md §5.3) -
	// idempotent no-op when the hash is already known.
	CreateBlob(ctx context.Context, blob Blob) error
	GetBlob(ctx context.Context, sha256Hex string) (Blob, bool, error)

	CreateAsset(ctx context.Context, asset Asset) error
	GetAsset(ctx context.Context, id string) (Asset, bool, error)
	ListAssets(ctx context.Context) ([]Asset, error)
	UpdateAssetMetadata(ctx context.Context, id, displayName string) (Asset, error)
	// DeleteAsset removes only the logical asset row - never the
	// underlying blob (docs/alert-audio.md §5.6).
	DeleteAsset(ctx context.Context, id string) error

	// ReferenceCount returns how many alert-rule and template rows
	// currently reference assetID (docs/alert-audio.md §5.6's delete
	// guard).
	ReferenceCount(ctx context.Context, assetID string) (int, error)

	// SetRuleAssetRefs/SetTemplateAssetRefs replace the full set of
	// audio-asset references a given owning rule/template row carries -
	// called by the alerts/visualtemplate service bridge on every save.
	SetRuleAssetRefs(ctx context.Context, ruleID string, assetIDs []string) error
	SetTemplateAssetRefs(ctx context.Context, templateID string, assetIDs []string) error
	// ClearRuleRefs/ClearTemplateRefs remove every reference row for one
	// owning rule/template - called when the owner itself is deleted.
	ClearRuleRefs(ctx context.Context, ruleID string) error
	ClearTemplateRefs(ctx context.Context, templateID string) error

	// ListOrphanBlobHashes returns every blob SHA-256 with zero rows in
	// either reference table - startup GC candidates (docs/alert-
	// audio.md §5.7).
	ListOrphanBlobHashes(ctx context.Context) ([]string, error)
	// ListBlobHashes returns every blob SHA-256 currently known to the
	// database, used by startup reconciliation to find an on-disk file
	// with no matching row at all.
	ListBlobHashes(ctx context.Context) ([]string, error)
	// DeleteBlobRow removes only the blob metadata row - the caller is
	// responsible for also removing the backing file via the shared
	// visualasset.FileStore instance.
	DeleteBlobRow(ctx context.Context, sha256Hex string) error
}
