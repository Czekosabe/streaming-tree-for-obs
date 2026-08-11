package visualasset

import "errors"

var (
	// ErrStorage wraps any unexpected persistence/filesystem failure.
	ErrStorage = errors.New("visual asset storage failure")
	// ErrNotFound means no asset (or, for a public lookup, no blob)
	// exists for the given id/token.
	ErrNotFound = errors.New("visual asset not found")
	// ErrInvalid means the supplied metadata (display name/author/
	// license/notice length, kind) failed a structural bound - never
	// used for a content/signature failure, see ErrUnsupported.
	ErrInvalid = errors.New("visual asset is invalid")
	// ErrUnsupported means the asset's own bytes did not match any
	// accepted signature, or its extension/declared media type/detected
	// signature disagreed (docs/visual-template-packages.md §11).
	ErrUnsupported = errors.New("visual asset type is not supported")
	// ErrTooLarge means the asset exceeded its own kind's size bound
	// (docs/visual-template-packages.md §10).
	ErrTooLarge = errors.New("visual asset is too large")
	// ErrInUse means a delete was rejected because at least one
	// persisted design or template still references the asset
	// (docs/visual-template-packages.md §15) - stable API error
	// visual_asset_in_use.
	ErrInUse = errors.New("visual asset is in use")
)
