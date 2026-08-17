// Package buildinfo exposes static identity information about the binary.
//
// This is the single canonical source for the application's product/creator
// identity - nothing here duplicates as a literal string anywhere else in the
// backend, and the frontend's About page fetches it fresh from
// GET /api/about rather than keeping its own copy (see internal/httpapi/about.go).
package buildinfo

import "runtime/debug"

// ServiceName is reported by the health endpoint and used in log lines.
const ServiceName = "streaming-tree-server"

// Version is the application version. It is kept in sync with the web app's
// package.json version by hand for now; a later stage can inject a real
// release version at build time with -ldflags. Until then it is an internal
// identifier only - user-facing surfaces show IsReleaseBuild()'s honest
// "development build" state instead of presenting this as a release number.
const Version = "0.1.0"

// ProductName is the public product name, shown to end users.
const ProductName = "Streaming Tree for OBS"

// CreatorName is the ONLY public creator/author identity this application
// ever displays. It is deliberately not a real legal name, email, OS
// username, or Git identity - see docs/product-identity-legal.md.
const CreatorName = "Czekosabe"

// RepositoryURL is the canonical public source repository.
const RepositoryURL = "https://github.com/Czekosabe/streaming-tree-for-obs"

// CreatorURL is the creator's public GitHub profile.
const CreatorURL = "https://github.com/Czekosabe"

// SupportURL is the current voluntary creator-support destination.
//
// This is the ONE place this URL is defined. Changing support provider later
// (e.g. to a Czekosabe-owned landing page) means changing this constant
// only - no database migration, no saved user configuration, and no other
// component or test should hold a second copy of this literal.
const SupportURL = "https://streamelements.com/czekosabe/tip"

// ApplicationLicenceStatus is a stable status code, not display prose - the
// frontend maps it to localized copy. "unselected" means no application-wide
// licence has been chosen yet for the first public packaged release; this is
// an explicit, deliberately unresolved operator decision (see
// docs/product-identity-legal.md), not an oversight, and nothing in this
// codebase should ever fill in a guessed licence identifier.
const ApplicationLicenceStatus = "unselected"

// IsReleaseBuild reports whether Version above should be presented as a real
// release. It is always false until Stage 20A establishes real
// release-version injection; user-facing surfaces must show an honest
// "development build" identity while this is false rather than inventing a
// release number.
const IsReleaseBuild = false

// CommitInfo reports the VCS revision this binary was built from, using Go's
// own automatic build-info stamping (available for any `go build`/`go
// install` run inside a git checkout - no -ldflags setup required). This is
// the one piece of build identity the frontend cannot reliably determine on
// its own, since the Vite build has no equivalent automatic stamping.
//
// ok is false when no VCS revision is available at all (e.g. `go run`, a
// build outside a git checkout, or VCS stamping explicitly disabled) -
// callers must not fabricate a commit value in that case.
func CommitInfo() (commit string, dirty bool, ok bool) {
	info, available := debug.ReadBuildInfo()
	if !available {
		return "", false, false
	}

	var revision string
	var modified bool
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}

	if revision == "" {
		return "", false, false
	}

	const shortLength = 12
	if len(revision) > shortLength {
		revision = revision[:shortLength]
	}

	return revision, modified, true
}
