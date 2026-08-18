package updater

import (
	"context"
	"errors"
	"unicode/utf8"

	"github.com/streaming-tree/server/internal/updater/manifest"
)

// CheckNow performs one metadata check against GitHub (docs/updater.md
// §9-§11): fetches the latest release (honoring a held ETag), defends
// against draft/prerelease releases client-side, fetches and validates
// the release manifest, and updates the manager's state to
// up_to_date/available/error accordingly.
//
// This is metadata only - it never downloads the installer artifact and
// never touches the filesystem. A concurrent call while one is already
// in flight is a no-op against the same check, never a second parallel
// request (docs/updater.md §11/§18's "no five clicks" rule).
func (m *Manager) CheckNow(ctx context.Context) error {
	if !m.releaseBuild {
		return ErrDisabled
	}
	if m.platformUnsupported {
		return ErrPlatformUnsupported
	}

	m.mu.Lock()
	if m.checking {
		m.mu.Unlock()
		return nil
	}
	m.checking = true
	m.state = StateChecking
	etag := m.etag
	m.mu.Unlock()

	result, err := m.client.FetchLatestRelease(ctx, etag)

	m.mu.Lock()
	defer m.mu.Unlock()
	m.checking = false

	if err != nil {
		m.state = StateError
		m.lastErrorCode = classifyCheckError(err)
		return err
	}

	if result.NotModified {
		m.etag = result.ETag
		m.lastSuccessfulCheckAt = m.clock()
		m.lastErrorCode = ""
		// A prior successful check already established the current
		// state (up_to_date/available) - nothing changed, so it is left
		// as-is.
		if m.state == StateChecking {
			m.state = m.previousSettledStateLocked()
		}
		return nil
	}

	release := result.Release
	m.etag = result.ETag

	// Defend against draft/prerelease client-side too - never trust the
	// endpoint's own filtering alone (docs/updater.md §2).
	if release.Draft || release.Prerelease {
		m.state = StateError
		m.lastErrorCode = ErrorCodeInvalidManifest
		return errors.New("latest release is a draft or prerelease")
	}

	manifestAsset, err := release.ManifestAsset()
	if err != nil {
		m.state = StateError
		m.lastErrorCode = ErrorCodeInvalidManifest
		return err
	}

	manifestBytes, err := m.client.FetchManifest(ctx, manifestAsset)
	if err != nil {
		m.state = StateError
		m.lastErrorCode = classifyCheckError(err)
		return err
	}

	parsed, err := manifest.Parse(manifestBytes)
	if err != nil {
		m.state = StateError
		m.lastErrorCode = ErrorCodeInvalidManifest
		return err
	}
	if err := manifest.Validate(parsed, release.TagName); err != nil {
		m.state = StateError
		m.lastErrorCode = ErrorCodeInvalidManifest
		return err
	}

	artifact, hasArtifact := parsed.ArtifactFor(m.identity)

	current, err := manifest.ParseVersion(m.currentVersion)
	if err != nil {
		m.state = StateError
		m.lastErrorCode = ErrorCodeInvalidManifest
		return err
	}
	latest, err := manifest.ParseVersion(parsed.Version)
	if err != nil {
		m.state = StateError
		m.lastErrorCode = ErrorCodeInvalidManifest
		return err
	}

	m.latestVersion = parsed.Version
	m.latestTag = release.TagName
	m.releaseNotes, m.releaseNotesTruncated = boundReleaseNotes(release.Body)
	m.publishedAt = release.PublishedAt
	m.lastSuccessfulCheckAt = m.clock()
	m.lastErrorCode = ""

	switch {
	case current.Compare(latest) >= 0:
		// Up to date, or installed is newer than latest stable - never
		// offer a downgrade (docs/updater.md §4).
		m.state = StateUpToDate
		m.latestRelease = nil
	case !hasArtifact:
		// A real newer version exists, but not for this platform yet -
		// reporting "available" would be misleading since nothing is
		// installable; report up to date from this build's own
		// perspective rather than dangling an unusable "available" state.
		m.state = StateUpToDate
		m.latestRelease = nil
	default:
		m.state = StateAvailable
		m.latestRelease = release
		m.latestArtifact = artifact
	}

	return nil
}

// previousSettledStateLocked returns to a sensible non-transient state
// after a 304, if the state machine was mid-check when the response
// arrived. Caller holds m.mu.
func (m *Manager) previousSettledStateLocked() State {
	if m.latestVersion == "" {
		return StateIdle
	}
	current, err1 := manifest.ParseVersion(m.currentVersion)
	latest, err2 := manifest.ParseVersion(m.latestVersion)
	if err1 != nil || err2 != nil || current.Compare(latest) >= 0 {
		return StateUpToDate
	}
	return StateAvailable
}

// classifyCheckError maps a client error to a stable, safe error code
// (docs/updater.md §30 - never a raw error string).
func classifyCheckError(err error) string {
	if errors.Is(err, ErrRateLimited) {
		return ErrorCodeRateLimited
	}
	return ErrorCodeCheckFailed
}

// boundReleaseNotes caps release-note length and reports truncation
// honestly (docs/updater.md §12).
func boundReleaseNotes(body string) (string, bool) {
	if utf8.RuneCountInString(body) <= maxReleaseNotesRunes {
		return body, false
	}
	runes := []rune(body)
	return string(runes[:maxReleaseNotesRunes]), true
}
