package visualasset

import "context"

// Repository is the persistence port this domain package depends on -
// implemented by internal/storage/sqlite (docs/visual-template-
// packages.md §13's four-table model). Deliberately asset-centric only:
// SetDesignAssetRefs/SetTemplateAssetRefs accept plain owner-id strings,
// never a visualdesign.Document or visualtemplate.Template - this
// package never imports either sibling domain (see this package's own
// doc comment).
type Repository interface {
	// CreateBlob inserts blob's metadata row if no row for its SHA256
	// already exists (content dedup, docs/visual-template-packages.md
	// §13/§27) - idempotent no-op when the hash is already known.
	CreateBlob(ctx context.Context, blob Blob) error
	GetBlob(ctx context.Context, sha256Hex string) (Blob, bool, error)
	GetBlobByPublicToken(ctx context.Context, token string) (Blob, bool, error)

	CreateAsset(ctx context.Context, asset Asset) error
	GetAsset(ctx context.Context, id string) (Asset, bool, error)
	ListAssets(ctx context.Context) ([]Asset, error)
	UpdateAssetMetadata(ctx context.Context, id, displayName, author, license, notice string) (Asset, error)
	// DeleteAsset removes only the logical asset row - never the
	// underlying blob (docs/visual-template-packages.md §15).
	DeleteAsset(ctx context.Context, id string) error

	// ReferenceCount returns how many design+template rows currently
	// reference assetID (docs/visual-template-packages.md §15's delete
	// guard).
	ReferenceCount(ctx context.Context, assetID string) (int, error)

	// SetDesignAssetRefs/SetTemplateAssetRefs replace the full set of
	// asset references a given owning design/template row carries -
	// called by the visualdesign/visualtemplate service bridge on every
	// save, exactly like visualdesign.Service.Save's own full-replacement
	// semantics.
	SetDesignAssetRefs(ctx context.Context, designID string, assetIDs []string) error
	SetTemplateAssetRefs(ctx context.Context, templateID string, assetIDs []string) error
	// ClearOwnerRefs removes every reference row for one owning
	// design/template - called when the owner itself is deleted.
	ClearDesignRefs(ctx context.Context, designID string) error
	ClearTemplateRefs(ctx context.Context, templateID string) error

	// ListOrphanBlobHashes returns every blob SHA-256 with zero rows in
	// either reference table - startup GC candidates (docs/visual-
	// template-packages.md §16).
	ListOrphanBlobHashes(ctx context.Context) ([]string, error)
	// ListBlobHashes returns every blob SHA-256 currently known to the
	// database, used by startup reconciliation to find an on-disk file
	// with no matching row at all (docs/visual-template-packages.md
	// §16's "detect an untracked orphan blob file").
	ListBlobHashes(ctx context.Context) ([]string, error)
	// DeleteBlobRow removes only the blob metadata row - the caller is
	// responsible for also removing the backing file via FileStore.
	DeleteBlobRow(ctx context.Context, sha256Hex string) error
}
