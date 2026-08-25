package mediamtx

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/streaming-tree/server/internal/runtime/procutil"
)

// BinarySource records where the executable in use came from.
type BinarySource string

const (
	// SourceManaged is the application-installed copy under the data directory.
	SourceManaged BinarySource = "managed"
	// SourceOverride is an operator-supplied path.
	SourceOverride BinarySource = "override"
	// SourceMissing means no usable binary was found.
	SourceMissing BinarySource = "missing"
)

// InstallationMetadata is written next to a managed installation.
type InstallationMetadata struct {
	Version     string `json:"version"`
	Platform    string `json:"platform"`
	AssetName   string `json:"assetName"`
	SHA256      string `json:"sha256"`
	InstalledAt string `json:"installedAt"`
}

// MetadataFileName is the installation manifest inside a managed install.
const MetadataFileName = "installation.json"

// LicenseFileName is the upstream license preserved beside the binary.
const LicenseFileName = "LICENSE"

// Resolution is the outcome of looking for a usable MediaMTX binary.
//
// Path is intentionally NOT part of any API response: a filesystem path tells a
// browser about the machine's layout and is of no use to the interface.
type Resolution struct {
	Source BinarySource
	// Path is empty when Source is SourceMissing.
	Path string
	// Version is the version the binary reported, empty when unknown.
	Version string
	// Compatible is true only when Version equals SupportedVersion.
	Compatible bool
	// Err carries the reportable reason when the binary is unusable.
	Err *RuntimeError
}

// Resolver locates the MediaMTX executable.
//
// Resolution order is deliberately narrow:
//  1. the explicit override path,
//  2. the application-managed installation,
//  3. missing.
//
// The system PATH is NOT searched. On a developer machine that could pick up an
// unrelated or unsupported build, and this application starts the binary as a
// long-lived child process with a generated configuration - it should only ever
// run a copy it can identify.
type Resolver struct {
	// dataDir is the application data directory holding managed installs.
	dataDir string
	// overridePath is the configured explicit path, empty when unset.
	overridePath string
	// goos and goarch are injectable so tests can exercise other platforms.
	goos   string
	goarch string
	// versionProbe runs `--version`; injectable for tests.
	versionProbe func(ctx context.Context, path string) (string, error)
}

// NewResolver builds a resolver for the running platform.
func NewResolver(dataDir, overridePath string) *Resolver {
	return &Resolver{
		dataDir:      dataDir,
		overridePath: overridePath,
		goos:         runtime.GOOS,
		goarch:       runtime.GOARCH,
		versionProbe: probeVersion,
	}
}

// RuntimeDir is the root of managed runtime dependencies.
func RuntimeDir(dataDir string) string {
	return filepath.Join(dataDir, "runtime")
}

// InstallDir is the versioned, platform-specific managed installation folder.
//
// Version and platform are separate path segments so several versions can sit
// side by side, which is what makes a future upgrade a install-then-switch
// rather than an in-place overwrite of a running binary.
func InstallDir(dataDir, platformDir string) string {
	return filepath.Join(RuntimeDir(dataDir), "mediamtx", SupportedVersion, platformDir)
}

// ManagedExecutablePath is where a managed installation puts the executable.
func ManagedExecutablePath(dataDir, platformDir, executableName string) string {
	return filepath.Join(InstallDir(dataDir, platformDir), executableName)
}

// Resolve looks for a usable binary and reports what it found.
//
// It never returns an error for "missing": that is a normal state the interface
// renders, not a backend failure.
func (r *Resolver) Resolve(ctx context.Context) Resolution {
	if r.overridePath != "" {
		return r.resolveOverride(ctx)
	}
	return r.resolveManaged(ctx)
}

func (r *Resolver) resolveOverride(ctx context.Context) Resolution {
	resolution := Resolution{Source: SourceOverride, Path: r.overridePath}

	info, err := os.Stat(r.overridePath)
	if err != nil {
		resolution.Source = SourceMissing
		resolution.Path = ""
		resolution.Err = NewRuntimeError(CodeNotInstalled,
			"STREAMING_TREE_MEDIAMTX_PATH points at a file that does not exist.")
		return resolution
	}
	if info.IsDir() || !info.Mode().IsRegular() {
		resolution.Err = NewRuntimeError(CodeNotInstalled,
			"STREAMING_TREE_MEDIAMTX_PATH must point at the MediaMTX executable file.")
		return resolution
	}
	if err := checkExecutable(info, r.goos); err != nil {
		resolution.Err = NewRuntimeError(CodePermissionDenied,
			"The file at STREAMING_TREE_MEDIAMTX_PATH is not executable.")
		return resolution
	}

	r.fillVersion(ctx, &resolution)
	return resolution
}

func (r *Resolver) resolveManaged(ctx context.Context) Resolution {
	asset, err := AssetFor(r.goos, r.goarch)
	if err != nil {
		return Resolution{
			Source: SourceMissing,
			Err: NewRuntimeError(CodeUnsupportedPlatform, fmt.Sprintf(
				"MediaMTX %s has no official release for %s/%s. "+
					"Set STREAMING_TREE_MEDIAMTX_PATH to a compatible binary instead.",
				SupportedVersion, r.goos, r.goarch)),
		}
	}

	path := ManagedExecutablePath(r.dataDir, asset.PlatformDir, asset.ExecutableName)
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return Resolution{
			Source: SourceMissing,
			Err: NewRuntimeError(CodeNotInstalled,
				"MediaMTX "+SupportedVersion+" is not installed yet."),
		}
	}

	resolution := Resolution{Source: SourceManaged, Path: path}
	r.fillVersion(ctx, &resolution)
	return resolution
}

// fillVersion probes the binary and decides whether it may be started.
func (r *Resolver) fillVersion(ctx context.Context, resolution *Resolution) {
	output, err := r.versionProbe(ctx, resolution.Path)
	if err != nil {
		resolution.Err = NewRuntimeError(CodeIncompatibleVersion,
			"The MediaMTX executable could not be run to read its version.")
		return
	}

	version, err := ParseVersionOutput(output)
	if err != nil {
		resolution.Err = NewRuntimeError(CodeIncompatibleVersion,
			"The executable did not report a recognisable MediaMTX version.")
		return
	}

	resolution.Version = version
	resolution.Compatible = IsSupportedVersion(version)

	if !resolution.Compatible {
		// An unsupported build is never started by default: the generated
		// configuration targets one schema, and MediaMTX rejects unknown keys.
		resolution.Err = NewRuntimeError(CodeIncompatibleVersion, fmt.Sprintf(
			"MediaMTX %s is installed but this application supports %s. "+
				"Install the supported version or point STREAMING_TREE_MEDIAMTX_PATH at it.",
			version, SupportedVersion))
	}
}

// versionProbeTimeout bounds `mediamtx --version`, which should be instant.
const versionProbeTimeout = 10 * time.Second

// probeVersion executes the binary with --version and returns its output.
func probeVersion(ctx context.Context, path string) (string, error) {
	probeCtx, cancel := context.WithTimeout(ctx, versionProbeTimeout)
	defer cancel()

	cmd := exec.CommandContext(probeCtx, path, "--version")
	procutil.HideConsoleWindow(cmd)
	// A hostile or broken binary could print a great deal; the output is
	// bounded by reading it into memory only through CombinedOutput and then
	// truncating before it reaches a message.
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("run %s --version: %w", filepath.Base(path), err)
	}
	return string(output), nil
}

// checkExecutable verifies the file can plausibly be executed.
func checkExecutable(info os.FileInfo, goos string) error {
	if goos == "windows" {
		// Windows has no executable bit; extension and regularity are all that
		// can be checked without running the file.
		return nil
	}
	if info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("file mode %s has no executable bit", info.Mode().Perm())
	}
	return nil
}

// ReadInstallationMetadata loads the manifest of a managed installation.
func ReadInstallationMetadata(dataDir, platformDir string) (InstallationMetadata, error) {
	path := filepath.Join(InstallDir(dataDir, platformDir), MetadataFileName)

	raw, err := os.ReadFile(path)
	if err != nil {
		return InstallationMetadata{}, fmt.Errorf("read installation metadata: %w", err)
	}

	var metadata InstallationMetadata
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return InstallationMetadata{}, fmt.Errorf("parse installation metadata: %w", err)
	}
	if strings.TrimSpace(metadata.Version) == "" {
		return InstallationMetadata{}, fmt.Errorf("installation metadata has no version")
	}

	return metadata, nil
}
