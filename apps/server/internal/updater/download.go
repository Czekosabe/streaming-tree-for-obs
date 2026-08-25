package updater

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/streaming-tree/server/internal/updater/manifest"
)

// updatesSubdir is the application-owned subtree downloads stage under,
// inside the existing per-user data directory (docs/updater.md §16) -
// never the install directory, never the current working directory,
// never a frontend-supplied path.
const updatesSubdir = "updates"

// errHashMismatch is a local sentinel wrapped into ErrorCodeHashMismatch -
// distinct from ErrDigestMismatch, which is specifically the GitHub-
// digest cross-check (docs/updater.md §14).
var errHashMismatch = errors.New("downloaded artifact sha256 does not match the release manifest")

// Download fetches, verifies, and stages the currently-available
// update's installer artifact (docs/updater.md §13/§14/§16). Allowed
// while streaming - only Install is gated (docs/updater.md §17).
func (m *Manager) Download(ctx context.Context) error {
	if !m.releaseBuild {
		return ErrDisabled
	}
	if m.manualBuild {
		return ErrManualBuild
	}
	if m.platformUnsupported {
		return ErrPlatformUnsupported
	}

	m.mu.Lock()
	switch m.state {
	case StateDownloading, StateReadyToInstall:
		// Already downloading or already have a verified candidate -
		// not an error, just nothing new to do.
		m.mu.Unlock()
		return nil
	case StateAvailable:
		// proceed below
	default:
		m.mu.Unlock()
		return errors.New("no update is available to download")
	}
	release := m.latestRelease
	artifact := m.latestArtifact
	version := m.latestVersion
	m.state = StateDownloading
	m.downloadedBytes = 0
	m.totalBytes = artifact.SizeBytes
	m.mu.Unlock()

	if release == nil {
		m.failDownload(ErrorCodeDownloadFailed, errors.New("no cached release to download from"))
		return errors.New("no cached release to download from")
	}

	asset, err := release.AssetByName(artifact.Name)
	if err != nil {
		m.failDownload(ErrorCodeDownloadFailed, err)
		return err
	}

	path, err := m.downloadAndVerify(ctx, asset, artifact, version)
	if err != nil {
		code := ErrorCodeDownloadFailed
		switch {
		case errors.Is(err, ErrResponseTooLarge):
			code = ErrorCodeSizeExceeded
		case errors.Is(err, errHashMismatch), errors.Is(err, ErrDigestMismatch):
			code = ErrorCodeHashMismatch
		}
		m.failDownload(code, err)
		return err
	}

	m.mu.Lock()
	m.verifiedCandidatePath = path
	m.verifiedCandidateVersion = version
	m.state = StateReadyToInstall
	m.mu.Unlock()
	return nil
}

func (m *Manager) failDownload(code string, err error) {
	m.mu.Lock()
	m.state = StateError
	m.lastErrorCode = code
	m.mu.Unlock()
	m.logger.Warn("update download failed", "code", code, "error", err)
}

// downloadAndVerify implements docs/updater.md §16: write to a
// ".part" file, verify size and SHA-256 (plus the GitHub digest
// cross-check, already performed by the caller before this - see
// Download), then atomically rename to a verified candidate name that
// encodes its own identity so a stale candidate can never be confused
// with a newer one.
func (m *Manager) downloadAndVerify(ctx context.Context, asset Asset, artifact manifest.Artifact, version string) (string, error) {
	dir := filepath.Join(m.dataDir, updatesSubdir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create update staging directory: %w", err)
	}

	partPath := filepath.Join(dir, "artifact.part")
	f, err := os.Create(partPath) // #nosec G304 -- dir is application-owned, not user-supplied.
	if err != nil {
		return "", fmt.Errorf("create partial download file: %w", err)
	}

	sha, total, err := m.client.DownloadAssetTo(ctx, asset, f, artifact.SizeBytes, func(written int64) {
		m.mu.Lock()
		m.downloadedBytes = written
		m.mu.Unlock()
	})
	closeErr := f.Close()
	if err != nil {
		_ = os.Remove(partPath)
		return "", err
	}
	if closeErr != nil {
		_ = os.Remove(partPath)
		return "", fmt.Errorf("finalize downloaded file: %w", closeErr)
	}

	if total != artifact.SizeBytes {
		_ = os.Remove(partPath)
		return "", fmt.Errorf("%w: downloaded %d bytes, manifest declares %d", errHashMismatch, total, artifact.SizeBytes)
	}
	if sha != artifact.SHA256 {
		_ = os.Remove(partPath)
		return "", errHashMismatch
	}

	finalName := fmt.Sprintf("verified-%s-%s%s", version, sha[:12], filepath.Ext(artifact.Name))
	finalPath := filepath.Join(dir, finalName)
	if err := os.Rename(partPath, finalPath); err != nil {
		_ = os.Remove(partPath)
		return "", fmt.Errorf("promote verified candidate: %w", err)
	}

	return finalPath, nil
}
