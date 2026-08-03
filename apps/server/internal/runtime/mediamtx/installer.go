package mediamtx

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DefaultReleaseBaseURL is the official MediaMTX download location.
//
// It is a constant rather than a request parameter on purpose: an installation
// endpoint that accepted a URL would let anyone who can reach the local API
// make the backend download and execute an arbitrary binary.
const DefaultReleaseBaseURL = "https://github.com/bluenviron/mediamtx/releases/download/" + SupportedVersion

// ChecksumFileName is the official checksum manifest published per release.
const ChecksumFileName = "checksums.sha256"

const (
	// downloadTimeout bounds the whole install, archive included.
	downloadTimeout = 10 * time.Minute
	// checksumTimeout bounds the small manifest download.
	checksumTimeout = 60 * time.Second
	// maxArchiveBytes bounds the archive download. Releases are ~30 MB.
	maxArchiveBytes = 128 << 20
	// maxChecksumBytes bounds the manifest, which is a few hundred bytes.
	maxChecksumBytes = 64 << 10
	// maxRedirects bounds redirect following; GitHub redirects to a CDN.
	maxRedirects = 5
)

// Installer downloads and installs the pinned MediaMTX release.
type Installer struct {
	dataDir string
	baseURL string
	client  *http.Client
	goos    string
	goarch  string
	// versionProbe verifies the installed binary; injectable for tests.
	versionProbe func(ctx context.Context, path string) (string, error)
}

// InstallerOption customises an Installer, mainly for tests.
type InstallerOption func(*Installer)

// WithReleaseBaseURL points the installer at a different release source.
//
// This exists so automated tests can serve fixtures from an httptest server.
// It is NOT reachable from any HTTP request: the install endpoint accepts no
// body, so a browser cannot influence where a download comes from.
func WithReleaseBaseURL(baseURL string) InstallerOption {
	return func(i *Installer) { i.baseURL = strings.TrimRight(baseURL, "/") }
}

// WithHTTPClient overrides the HTTP client.
func WithHTTPClient(client *http.Client) InstallerOption {
	return func(i *Installer) { i.client = client }
}

// WithPlatform overrides the target platform, for tests.
func WithPlatform(goos, goarch string) InstallerOption {
	return func(i *Installer) {
		i.goos = goos
		i.goarch = goarch
	}
}

// WithVersionProbe overrides how the installed binary is verified, for tests.
func WithVersionProbe(probe func(ctx context.Context, path string) (string, error)) InstallerOption {
	return func(i *Installer) { i.versionProbe = probe }
}

// NewInstaller builds an installer writing into the given data directory.
func NewInstaller(dataDir string, opts ...InstallerOption) *Installer {
	installer := &Installer{
		dataDir: dataDir,
		baseURL: DefaultReleaseBaseURL,
		client: &http.Client{
			Timeout: downloadTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= maxRedirects {
					return fmt.Errorf("stopped after %d redirects", maxRedirects)
				}
				return nil
			},
		},
		goos:         currentGOOS(),
		goarch:       currentGOARCH(),
		versionProbe: probeVersion,
	}

	for _, opt := range opts {
		opt(installer)
	}
	return installer
}

// Install downloads, verifies and installs the pinned release.
//
// The sequence never executes anything it has not verified: the archive is
// checksummed before it is opened, and the extracted binary is only run to read
// its version, after extraction has already been constrained.
//
// On any failure the temporary directory is removed and an existing valid
// installation is left untouched, so a failed reinstall cannot degrade a
// working setup.
func (i *Installer) Install(ctx context.Context) (InstallationMetadata, error) {
	asset, err := AssetFor(i.goos, i.goarch)
	if err != nil {
		return InstallationMetadata{}, err
	}

	// Staging happens inside the runtime directory so the final move is a
	// rename on the same filesystem rather than a cross-device copy.
	stagingRoot := filepath.Join(RuntimeDir(i.dataDir), "tmp")
	if err := os.MkdirAll(stagingRoot, 0o700); err != nil {
		return InstallationMetadata{}, fmt.Errorf("create staging directory: %w", err)
	}

	tempDir, err := os.MkdirTemp(stagingRoot, "install-")
	if err != nil {
		return InstallationMetadata{}, fmt.Errorf("create temporary directory: %w", err)
	}
	// Removed on success and on failure alike.
	defer func() { _ = os.RemoveAll(tempDir) }()

	expectedSum, err := i.fetchChecksum(ctx, asset.FileName)
	if err != nil {
		return InstallationMetadata{}, err
	}

	archivePath := filepath.Join(tempDir, asset.FileName)
	actualSum, err := i.downloadArchive(ctx, asset.FileName, archivePath)
	if err != nil {
		return InstallationMetadata{}, err
	}

	if !strings.EqualFold(actualSum, expectedSum) {
		return InstallationMetadata{}, fmt.Errorf(
			"%w: the downloaded %s does not match the official checksum", ErrChecksumMismatch, asset.FileName)
	}

	extractDir := filepath.Join(tempDir, "extracted")
	if err := os.MkdirAll(extractDir, 0o700); err != nil {
		return InstallationMetadata{}, fmt.Errorf("create extraction directory: %w", err)
	}

	files, err := extractArchive(archivePath, extractDir, asset.Format)
	if err != nil {
		return InstallationMetadata{}, err
	}

	executable, ok := findExtracted(files, asset.ExecutableName)
	if !ok {
		return InstallationMetadata{}, fmt.Errorf(
			"%w: the archive does not contain %s", ErrArchiveInvalid, asset.ExecutableName)
	}
	license, ok := findExtracted(files, LicenseFileName)
	if !ok {
		return InstallationMetadata{}, fmt.Errorf(
			"%w: the archive does not contain its %s file", ErrArchiveInvalid, LicenseFileName)
	}

	// tar preserves the executable bit; zip on Windows does not carry one.
	if err := os.Chmod(executable.Path, 0o700); err != nil {
		return InstallationMetadata{}, fmt.Errorf("make the executable runnable: %w", err)
	}

	installedVersion, err := i.verifyExtractedVersion(ctx, executable.Path)
	if err != nil {
		return InstallationMetadata{}, err
	}

	metadata := InstallationMetadata{
		Version:     installedVersion,
		Platform:    asset.PlatformDir,
		AssetName:   asset.FileName,
		SHA256:      strings.ToLower(actualSum),
		InstalledAt: time.Now().UTC().Format(time.RFC3339Nano),
	}

	if err := i.publish(tempDir, extractDir, asset, executable, license, metadata); err != nil {
		return InstallationMetadata{}, err
	}

	return metadata, nil
}

// verifyExtractedVersion runs the freshly extracted binary once.
func (i *Installer) verifyExtractedVersion(ctx context.Context, path string) (string, error) {
	output, err := i.versionProbe(ctx, path)
	if err != nil {
		return "", fmt.Errorf(
			"%w: the extracted executable could not be run to verify its version", ErrArchiveInvalid)
	}

	version, err := ParseVersionOutput(output)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrArchiveInvalid, err)
	}
	if !IsSupportedVersion(version) {
		return "", fmt.Errorf(
			"%w: the downloaded archive reports %s but %s was expected",
			ErrIncompatibleVersion, version, SupportedVersion)
	}

	return version, nil
}

// publish assembles the final layout and moves it into place atomically.
//
// The staged directory is renamed onto the target, so an interrupted install
// never leaves a half-populated directory that would look valid to the resolver.
func (i *Installer) publish(
	tempDir, extractDir string,
	asset ReleaseAsset,
	executable, license extractedFile,
	metadata InstallationMetadata,
) error {
	staged := filepath.Join(tempDir, "staged")
	if err := os.MkdirAll(staged, 0o700); err != nil {
		return fmt.Errorf("create staging layout: %w", err)
	}

	if err := os.Rename(executable.Path, filepath.Join(staged, asset.ExecutableName)); err != nil {
		return fmt.Errorf("stage the executable: %w", err)
	}
	if err := os.Rename(license.Path, filepath.Join(staged, LicenseFileName)); err != nil {
		return fmt.Errorf("stage the license: %w", err)
	}

	if err := writeJSONFile(filepath.Join(staged, MetadataFileName), metadata); err != nil {
		return fmt.Errorf("write installation metadata: %w", err)
	}

	target := InstallDir(i.dataDir, asset.PlatformDir)
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return fmt.Errorf("create installation directory: %w", err)
	}

	// Replacing an existing installation: move it aside first, so a failed
	// rename leaves the previous copy recoverable rather than deleted.
	backup := target + ".previous"
	_ = os.RemoveAll(backup)
	if _, err := os.Stat(target); err == nil {
		if err := os.Rename(target, backup); err != nil {
			return fmt.Errorf("move the previous installation aside: %w", err)
		}
	}

	if err := os.Rename(staged, target); err != nil {
		// Put the previous installation back before reporting the failure.
		if _, statErr := os.Stat(backup); statErr == nil {
			_ = os.Rename(backup, target)
		}
		return fmt.Errorf("install into the runtime directory: %w", err)
	}

	_ = os.RemoveAll(backup)
	return nil
}

// fetchChecksum downloads the official manifest and returns the entry for one
// asset. A manifest without an exact entry is a hard failure: installing an
// archive that nothing vouches for is precisely what this step prevents.
func (i *Installer) fetchChecksum(ctx context.Context, assetName string) (string, error) {
	requestCtx, cancel := context.WithTimeout(ctx, checksumTimeout)
	defer cancel()

	body, err := i.get(requestCtx, ChecksumFileName, maxChecksumBytes)
	if err != nil {
		return "", err
	}

	sum, err := findChecksum(string(body), assetName)
	if err != nil {
		return "", err
	}
	return sum, nil
}

// findChecksum locates one entry in a sha256sum-style manifest.
//
// Lines look like "<64 hex chars> *<file name>"; the "*" marks binary mode.
func findChecksum(manifest, assetName string) (string, error) {
	for _, line := range strings.Split(manifest, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		fields := strings.Fields(trimmed)
		if len(fields) != 2 {
			continue
		}

		name := strings.TrimPrefix(fields[1], "*")
		if name != assetName {
			continue
		}

		sum := strings.ToLower(fields[0])
		if len(sum) != 64 {
			return "", fmt.Errorf(
				"%w: the checksum entry for %s is malformed", ErrChecksumMismatch, assetName)
		}
		if _, err := hex.DecodeString(sum); err != nil {
			return "", fmt.Errorf(
				"%w: the checksum entry for %s is not hexadecimal", ErrChecksumMismatch, assetName)
		}
		return sum, nil
	}

	return "", fmt.Errorf(
		"%w: the official checksum file has no entry for %s", ErrChecksumMismatch, assetName)
}

// downloadArchive streams the asset to disk and returns its SHA-256.
//
// The hash is computed while streaming, so the file is never loaded into memory
// and never read twice.
func (i *Installer) downloadArchive(ctx context.Context, assetName, target string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, i.baseURL+"/"+assetName, nil)
	if err != nil {
		return "", fmt.Errorf("build the download request: %w", err)
	}

	response, err := i.client.Do(request)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", assetName, err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: the server answered %d", assetName, response.StatusCode)
	}

	file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", fmt.Errorf("create the download file: %w", err)
	}
	defer file.Close()

	hasher := sha256.New()
	// Read one byte past the budget so an oversized body is detectable.
	written, err := io.Copy(io.MultiWriter(file, hasher), io.LimitReader(response.Body, maxArchiveBytes+1))
	if err != nil {
		return "", fmt.Errorf("download %s: %w", assetName, err)
	}
	if written > maxArchiveBytes {
		return "", fmt.Errorf("download %s: the response exceeds the %d byte limit",
			assetName, maxArchiveBytes)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("finish writing the download: %w", err)
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// get fetches a small resource with a hard size limit.
//
// The body is never logged: it is either a checksum manifest or, if something
// upstream went wrong, an arbitrary error page.
func (i *Installer) get(ctx context.Context, name string, limit int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, i.baseURL+"/"+name, nil)
	if err != nil {
		return nil, fmt.Errorf("build the request for %s: %w", name, err)
	}

	response, err := i.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", name, err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: the server answered %d", name, response.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("download %s: the response exceeds the %d byte limit", name, limit)
	}

	return body, nil
}

// ClassifyInstallError maps an installation failure to a stable code.
func ClassifyInstallError(err error) *RuntimeError {
	switch {
	case errors.Is(err, ErrUnsupportedPlatform):
		return NewRuntimeError(CodeUnsupportedPlatform, err.Error())
	case errors.Is(err, ErrChecksumMismatch):
		return NewRuntimeError(CodeChecksumMismatch,
			"The downloaded MediaMTX archive did not match the official checksum and was discarded.")
	case errors.Is(err, ErrArchiveInvalid):
		return NewRuntimeError(CodeArchiveInvalid,
			"The downloaded MediaMTX archive could not be used and was discarded.")
	case errors.Is(err, ErrIncompatibleVersion):
		return NewRuntimeError(CodeIncompatibleVersion, err.Error())
	case errors.Is(err, ErrInstallInProgress):
		return NewRuntimeError(CodeInstallInProgress,
			"A MediaMTX installation is already running.")
	case errors.Is(err, os.ErrPermission):
		return NewRuntimeError(CodePermissionDenied,
			"MediaMTX could not be installed because of a file permission error.")
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return NewRuntimeError(CodeDownloadFailed,
			"The MediaMTX download did not finish in time.")
	default:
		return NewRuntimeError(CodeDownloadFailed,
			"MediaMTX could not be downloaded. Check the network connection and try again.")
	}
}

func writeJSONFile(path string, value any) error {
	encoded, err := marshalIndent(value)
	if err != nil {
		return err
	}
	return os.WriteFile(path, encoded, 0o600)
}
