package visualpackage

import "errors"

// Stable, internal sentinel errors this package returns - the httpapi
// bridge layer maps each to one of the stable wire error codes named in
// docs/visual-template-packages.md §24 (visual_template_package_*).
// Never surfaced to a caller as a raw archive/zip/filesystem error
// (docs/visual-template-packages.md §24: "a raw ZIP/parser/filesystem
// error is never surfaced to a caller directly").
var (
	ErrInvalidArchive     = errors.New("visual template package archive is invalid")
	ErrTooLarge           = errors.New("visual template package exceeds a size limit")
	ErrTooManyEntries     = errors.New("visual template package has too many archive entries")
	ErrTooManyAssets      = errors.New("visual template package has too many assets")
	ErrDecompressionLimit = errors.New("visual template package exceeds its decompression bound")
	ErrEntryInvalid       = errors.New("visual template package contains an invalid archive entry")
	ErrManifestInvalid    = errors.New("visual template package manifest is invalid")
	ErrVersionUnsupported = errors.New("visual template package schema version is not supported")
	ErrAssetMissing       = errors.New("visual template package references an asset that is not present in the archive")
	ErrAssetUnreferenced  = errors.New("visual template package archive contains an asset not referenced by the manifest")
	ErrAssetHashMismatch  = errors.New("visual template package asset content does not match its declared hash")
	ErrAssetTypeMismatch  = errors.New("visual template package asset type does not match its declared kind or media type")
	ErrAssetUnsupported   = errors.New("visual template package asset type is not supported")
	ErrPreviewExpired     = errors.New("visual template package preview session has expired")
	ErrPreviewNotFound    = errors.New("visual template package preview session was not found")
	// ErrAudioTargetInvalid means a package's manifest carries an
	// alertAudio object while its own template.json target is not
	// "alert" (docs/alert-audio.md §10.2) - rejected outright before any
	// asset (visual or audio) is staged.
	ErrAudioTargetInvalid = errors.New("visual template package audio is only valid for an alert-target template")
)
