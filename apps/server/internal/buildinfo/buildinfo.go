// Package buildinfo exposes static identity information about the binary.
//
// This is the single canonical source for the application's product/creator
// identity - nothing here duplicates as a literal string anywhere else in the
// backend, and the frontend's About page fetches it fresh from
// GET /api/about rather than keeping its own copy (see internal/httpapi/about.go).
package buildinfo

import (
	"regexp"
	"runtime/debug"
)

// ServiceName is reported by the health endpoint and used in log lines.
const ServiceName = "streaming-tree-server"

// Version is the hand-maintained internal identifier, kept in sync with the
// web app's package.json version by hand. It is never shown to a user
// directly - EffectiveVersion() is, since a release build overrides it via
// -ldflags (see releaseVersion below).
const Version = "0.1.0"

// releaseVersion, releaseCommit and packagedFlag are set only by the
// Stage 20A release build script (scripts/build-release.ps1), via
// `-ldflags "-X .../buildinfo.releaseVersion=... -X
// .../buildinfo.releaseCommit=... -X .../buildinfo.packagedFlag=true"`.
// They are empty/false in every ordinary `go build`/`go run`/`go test` -
// packaged mode is never inferred from the operating system alone, since
// Windows developers also use the ordinary source/dev workflow (see
// docs/windows-packaging.md §30).
var (
	releaseVersion string
	releaseCommit  string
	packagedFlag   string
)

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

// ApplicationLicenseSPDX is the canonical SPDX expression for Streaming
// Tree for OBS's own first-party application licence, selected by the
// project operator (see docs/product-identity-legal.md). This governs
// first-party source only - third-party dependencies keep their own
// licences, documented in THIRD_PARTY_NOTICES.md.
const ApplicationLicenseSPDX = "GPL-3.0-or-later"

// ApplicationLicenseName is a short, stable display name for
// ApplicationLicenseSPDX. It is not display prose to be translated line by
// line - the frontend uses it directly, since a licence name is not the
// kind of string that varies by UI language. The authoritative full licence
// text is the repository-root LICENSE file.
const ApplicationLicenseName = "GNU General Public License v3 or later"

// Packaged reports whether this binary was produced by the Stage 20A
// release build script, as opposed to an ordinary developer/test build.
func Packaged() bool {
	return packagedFlag == "true"
}

// EffectiveVersion returns the release build script's injected version
// when present, otherwise the hand-maintained internal Version constant.
// The frontend obtains this exclusively through GET /api/about - it never
// invents its own version string.
func EffectiveVersion() string {
	if releaseVersion != "" {
		return releaseVersion
	}
	return Version
}

// IsReleaseBuild reports whether EffectiveVersion() above should be
// presented as a real release rather than an honest "development build"
// identity. True only when the release build script actually injected a
// version - never inferred from Packaged() alone, since a development
// package build (see docs/windows-packaging.md §10) is packaged but still
// not a public release.
func IsReleaseBuild() bool {
	return releaseVersion != ""
}

// strictProductionVersionPattern mirrors scripts/build-release.ps1's own
// `-Version` gate (`^\d+\.\d+\.\d+$`) exactly - the script already uses
// this same test to decide whether a build gets real release-manifest
// metadata at all (see docs/updater.md §5). Keeping this pattern
// identical to the script's is deliberate: the two must never drift, or
// a build the script considers "not a real release version" could still
// end up eligible for production update checking here, or vice versa.
var strictProductionVersionPattern = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// IsStrictProductionVersion reports whether the release build script's
// injected version (see EffectiveVersion) is a strict major.minor.patch
// production release version, with no manual/test label, pre-release
// suffix, or build-metadata suffix.
//
// This is deliberately narrower than IsReleaseBuild(): a manual/test
// packaged build (e.g. version "0.1.0-manualtest+<commit>", built and
// verified locally per docs/windows-packaging.md) really was produced
// by the release script, so IsReleaseBuild() stays true for it (About,
// CommitInfo, and packaging-identity checks all need that to stay
// honest) - but such a build must never participate in production
// update checking, since the release pipeline itself refuses to
// generate real release-manifest metadata for a version shaped like
// that (see build-release.ps1's own identical gate above), so there is
// nothing a manual/test build could ever successfully check against.
// See docs/updater.md's manual/test-build eligibility section for the
// full contract; callers needing "is this build eligible for the
// production updater" should use this, not IsReleaseBuild() alone.
func IsStrictProductionVersion() bool {
	return strictProductionVersionPattern.MatchString(releaseVersion)
}

// CommitInfo reports the VCS revision this binary was built from. A release
// build's injected commit (set by the build script from the exact commit it
// built - reliable even when the release script builds from a staged copy
// of the source rather than a live git checkout) takes precedence; every
// other build falls back to Go's own automatic build-info stamping
// (available for any `go build`/`go install` run inside a git checkout - no
// -ldflags setup required). This is the one piece of build identity the
// frontend cannot reliably determine on its own, since the Vite build has
// no equivalent automatic stamping.
//
// ok is false when no VCS revision is available at all (e.g. `go run`, a
// build outside a git checkout, or VCS stamping explicitly disabled) -
// callers must not fabricate a commit value in that case.
func CommitInfo() (commit string, dirty bool, ok bool) {
	if releaseCommit != "" {
		return releaseCommit, false, true
	}
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
