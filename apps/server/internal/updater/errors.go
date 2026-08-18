// Package updater implements the Stage 20B application updater: a
// GitHub-Releases client, the project-controlled release manifest
// (internal/updater/manifest), the update-manager state machine, and
// (on Windows) the external update-installer handoff. See
// docs/updater.md for the full contract.
package updater

import "errors"

// Sentinel errors every layer of this package returns instead of a
// package-specific error type, mirroring every other domain in this
// codebase (see internal/domain/audio/errors.go).
var (
	// ErrRateLimited means the GitHub API responded with a rate-limit
	// signal. Always nonfatal - the manager retries on its normal
	// schedule (docs/updater.md §9), never in a tight loop.
	ErrRateLimited = errors.New("github api rate limited")

	// ErrRequestFailed wraps any other unexpected network/HTTP failure
	// talking to the GitHub API.
	ErrRequestFailed = errors.New("github api request failed")

	// ErrAssetNotFound means the expected release asset (by exact name)
	// was not present in the release's own assets array.
	ErrAssetNotFound = errors.New("release asset not found")

	// ErrAssetAmbiguous means more than one asset with the expected name
	// was present - never picked between, always rejected outright.
	ErrAssetAmbiguous = errors.New("release asset name is ambiguous")

	// ErrDigestMismatch means a GitHub-reported asset digest (when
	// present) disagreed with the manifest's own SHA-256 - never
	// silently ignored (docs/updater.md §14).
	ErrDigestMismatch = errors.New("release asset digest mismatch")

	// ErrResponseTooLarge means a response body exceeded its bound
	// before finishing - metadata responses and the manifest asset both
	// have small, fixed bounds distinct from the installer's own
	// download-size bound (manifest.MaxArtifactSizeBytes).
	ErrResponseTooLarge = errors.New("github api response too large")
)
