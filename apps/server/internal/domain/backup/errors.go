package backup

import "errors"

// Sentinel errors. Handlers map these to HTTP status codes; a raw
// archive/zip/filesystem error is never surfaced to a caller directly
// - mirrors internal/domain/visualpackage's own errors.go exactly.
var (
	ErrInvalidArchive     = errors.New("backup archive is invalid")
	ErrTooLarge           = errors.New("backup exceeds a size limit")
	ErrTooManyEntries     = errors.New("backup has too many archive entries")
	ErrDecompressionLimit = errors.New("backup exceeds its decompression bound")
	ErrEntryInvalid       = errors.New("backup contains an invalid archive entry")
	ErrManifestInvalid    = errors.New("backup manifest is invalid")
	ErrProductMismatch    = errors.New("backup was not produced by this application")
	ErrVersionUnsupported = errors.New("backup format version is not supported")
	ErrConfigInvalid      = errors.New("backup configuration payload is invalid")
	ErrAssetMissing       = errors.New("backup references an asset that is not present in the archive")
	ErrAssetUnreferenced  = errors.New("backup archive contains an asset not referenced by the manifest")
	ErrAssetHashMismatch  = errors.New("backup asset content does not match its declared hash")

	// ErrNotFound is returned when a preview/restore session token does
	// not exist or has expired.
	ErrNotFound = errors.New("backup session not found")
	// ErrStreamingActive is returned when a restore is attempted while
	// this application considers a broadcast active - see
	// docs/backup-restore.md §7 step 6.
	ErrStreamingActive = errors.New("restore cannot proceed while streaming is active")
)
