// Package ffmpeg resolves and probes the FFmpeg executable this application
// uses to run destination branches.
//
// Unlike MediaMTX, there is no approved managed binary source for FFmpeg in
// this stage: FFmpeg publishes source releases only, and executable builds
// come from independent distributors whose provenance this application has
// not reviewed. Nothing here downloads a binary. Every discovered executable,
// wherever it came from, is probed for the specific capabilities this
// application needs before it is ever trusted - see capabilities.go.
package ffmpeg

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Source records where a resolved executable came from.
type Source string

const (
	// SourceOverride is an operator-supplied path (STREAMING_TREE_FFMPEG_PATH).
	SourceOverride Source = "override"
	// SourceBundled is a copy placed beside the backend executable, for a
	// future packaged distribution. Nothing places a binary there today.
	SourceBundled Source = "bundled"
	// SourcePath is an executable found on the system PATH.
	SourcePath Source = "path"
	// SourceMissing means no usable executable was found anywhere.
	SourceMissing Source = "missing"
)

// Resolution is the outcome of looking for a usable FFmpeg binary.
//
// Path is intentionally never part of any API response: a filesystem path
// tells a browser about the machine's layout and is of no use to the
// interface. See httpapi for the DTO that deliberately omits it.
type Resolution struct {
	Source Source
	// Path is empty when Source is SourceMissing.
	Path string
	// Version is the raw version token FFmpeg reported, empty when it could
	// not be parsed (for example a git/dev build) or the binary is missing.
	Version string
	// Compatible is true only once the binary ran successfully, its version
	// (when parseable) is at least MinimumVersion, and every required
	// capability probed successfully.
	Compatible   bool
	Capabilities Capabilities
	Err          *RuntimeError
}

// Resolver locates the FFmpeg executable.
//
// Resolution order:
//  1. the explicit override (STREAMING_TREE_FFMPEG_PATH),
//  2. a bundled copy beside the backend executable, if one exists,
//  3. the system PATH,
//  4. missing.
//
// PATH is searched here - unlike the MediaMTX resolver, which deliberately
// does not - because there is no managed FFmpeg source to prefer it over in
// this stage. Every candidate, from every step, is still probed for the
// capabilities this application requires before it is considered usable.
type Resolver struct {
	overridePath string
	bundledPath  string
	// lookPath and probe are injectable so tests do not depend on the real
	// PATH or a real FFmpeg binary.
	lookPath func(name string) (string, error)
	probe    func(ctx context.Context, path string) (probeResult, error)
}

// NewResolver builds a resolver. overridePath is
// STREAMING_TREE_FFMPEG_PATH's resolved absolute value, or empty when unset.
func NewResolver(overridePath string) *Resolver {
	return &Resolver{
		overridePath: overridePath,
		bundledPath:  defaultBundledPath(),
		lookPath:     exec.LookPath,
		probe:        probeExecutable,
	}
}

// defaultBundledPath is the convention for a future packaged distribution: an
// "ffmpeg" (or "ffmpeg.exe") executable beside the running backend binary.
// Nothing in this stage places a binary there, and nothing commits one to the
// repository; this only defines where a later packaging step could put one.
func defaultBundledPath() string {
	exePath, err := os.Executable()
	if err != nil {
		return ""
	}
	name := "ffmpeg"
	if runtime.GOOS == "windows" {
		name = "ffmpeg.exe"
	}
	return filepath.Join(filepath.Dir(exePath), name)
}

// Resolve looks for a usable binary and reports what it found.
//
// It never returns a Go error for "missing" or "incompatible": both are
// normal states the interface renders, not backend failures. The backend
// stays running either way.
func (r *Resolver) Resolve(ctx context.Context) Resolution {
	if r.overridePath != "" {
		return r.resolveCandidate(ctx, SourceOverride, r.overridePath)
	}
	if r.bundledPath != "" {
		if info, err := os.Stat(r.bundledPath); err == nil && info.Mode().IsRegular() {
			return r.resolveCandidate(ctx, SourceBundled, r.bundledPath)
		}
	}
	if path, err := r.lookPath("ffmpeg"); err == nil {
		return r.resolveCandidate(ctx, SourcePath, path)
	}

	return Resolution{
		Source: SourceMissing,
		Err: NewRuntimeError(CodeNotFound,
			"No FFmpeg executable was found. Set STREAMING_TREE_FFMPEG_PATH, "+
				"or install FFmpeg and make sure it is on PATH."),
	}
}

// resolveCandidate validates and probes one candidate path.
func (r *Resolver) resolveCandidate(ctx context.Context, source Source, path string) Resolution {
	info, err := os.Stat(path)
	if err != nil {
		return Resolution{
			Source: SourceMissing,
			Err: NewRuntimeError(CodeNotFound,
				"The configured FFmpeg path does not exist."),
		}
	}
	if info.IsDir() || !info.Mode().IsRegular() {
		return Resolution{
			Source: source,
			Err: NewRuntimeError(CodeNotExecutable,
				"The FFmpeg path must point at the executable file, not a directory."),
		}
	}
	if err := checkExecutableBit(info); err != nil {
		return Resolution{
			Source: source,
			Err: NewRuntimeError(CodeNotExecutable,
				"The file at the configured FFmpeg path is not executable."),
		}
	}

	result, err := r.probe(ctx, path)
	if err != nil {
		return Resolution{
			Source: source,
			Err: NewRuntimeError(CodeExecutionFailed,
				"The FFmpeg executable could not be run to probe its capabilities."),
		}
	}

	resolution := Resolution{Source: source, Path: path}

	version, parsed := ParseVersion(result.versionOutput)
	if parsed {
		resolution.Version = version
	}

	resolution.Capabilities = Capabilities{
		RTMPInput:   result.protocols.RTMPInput,
		RTMPOutput:  result.protocols.RTMPOutput,
		RTMPSOutput: result.protocols.RTMPSOutput,
		FLVMuxer:    result.flvMuxer,
		Progress:    result.progressWorks,
	}

	belowMinimum := parsed && !AtLeastMinimum(version)
	capabilitiesOK := resolution.Capabilities.Satisfied()

	switch {
	case belowMinimum:
		resolution.Err = NewRuntimeError(CodeBelowMinimumVersion,
			"FFmpeg "+version+" is older than the minimum supported version "+MinimumVersion+".")
	case !capabilitiesOK:
		resolution.Err = NewRuntimeError(CodeMissingCapability,
			"The FFmpeg executable is missing required capabilities: "+
				joinMissing(resolution.Capabilities.Missing())+".")
	default:
		resolution.Compatible = true
	}

	return resolution
}

func joinMissing(items []string) string {
	out := ""
	for i, item := range items {
		if i > 0 {
			out += ", "
		}
		out += item
	}
	return out
}

// checkExecutableBit verifies the file can plausibly be executed.
//
// Windows has no executable bit; the extension and regularity checks already
// performed are all that can be checked without running the file.
func checkExecutableBit(info os.FileInfo) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	if info.Mode().Perm()&0o111 == 0 {
		return os.ErrPermission
	}
	return nil
}
