package visualdesign

import "errors"

var (
	// ErrStorage wraps any unexpected persistence failure.
	ErrStorage = errors.New("visual design storage failure")
	// ErrNotFound means no design is currently saved for the given
	// owner - a normal, expected state (Stage 13A task Part 19: every
	// existing Stage 12 alert rule currently has no design), never
	// itself an error condition the caller should treat as failure.
	ErrNotFound = errors.New("visual design not found")
	// ErrValidation wraps any semantic document validation failure -
	// see validation.go for the exact rules.
	ErrValidation = errors.New("visual design validation failed")
	// ErrRevisionConflict means a PUT's expected revision did not match
	// the currently persisted revision - another writer saved first
	// (Stage 13A task Part 7/41). The caller must reload and never
	// silently overwrite.
	ErrRevisionConflict = errors.New("visual design revision conflict")
	// ErrTooLarge means the serialized document exceeds
	// MaxDocumentBytes.
	ErrTooLarge = errors.New("visual design document is too large")
	// ErrAssetMissing means a document references a managed asset id
	// (Stage 14B - AssetReferences) that does not exist in the managed
	// asset store. Existence itself can only be checked by the owning
	// service (internal/domain/alerts, internal/domain/chatoverlay),
	// since this package never imports internal/domain/visualasset -
	// this sentinel exists so every caller reports the same stable
	// condition the same way (docs/visual-template-packages.md §57's
	// visual_asset_missing).
	ErrAssetMissing = errors.New("visual design references a managed asset that does not exist")
	// ErrAssetKindMismatch means a document references a managed asset
	// that exists but is the wrong kind for the layer/field referencing
	// it (an image layer pointing at a font asset, for example) - docs/
	// visual-template-packages.md §57's visual_asset_kind_mismatch.
	ErrAssetKindMismatch = errors.New("visual design references a managed asset of the wrong kind")
)
