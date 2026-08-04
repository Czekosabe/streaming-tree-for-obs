package ffmpeg

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// fakeBinary writes a placeholder file that stands in for the executable. It
// is never executed: the probe is injected in these tests.
func fakeBinary(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create directory: %v", err)
	}
	if err := os.WriteFile(path, []byte("not a real binary"), 0o700); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
}

func staticProbe(result probeResult, err error) func(context.Context, string) (probeResult, error) {
	return func(context.Context, string) (probeResult, error) { return result, err }
}

func compatibleResult(version string) probeResult {
	return probeResult{
		versionOutput: "ffmpeg version " + version + " Copyright (c) 2000-2026\n",
		protocols:     Capabilities{RTMPInput: true, RTMPOutput: true, RTMPSOutput: true},
		flvMuxer:      true,
		progressWorks: true,
	}
}

func newTestResolver(t *testing.T) *Resolver {
	t.Helper()
	return &Resolver{lookPath: func(string) (string, error) { return "", errors.New("not found") }}
}

func TestResolveReportsMissingWhenNothingIsFound(t *testing.T) {
	r := newTestResolver(t)

	resolution := r.Resolve(context.Background())

	if resolution.Source != SourceMissing {
		t.Errorf("source = %q, want missing", resolution.Source)
	}
	if resolution.Err == nil || resolution.Err.Code != CodeNotFound {
		t.Errorf("error = %+v, want %s", resolution.Err, CodeNotFound)
	}
}

func TestOverrideTakesPrecedenceOverEverything(t *testing.T) {
	override := filepath.Join(t.TempDir(), "ffmpeg")
	fakeBinary(t, override)

	r := newTestResolver(t)
	r.overridePath = override
	r.bundledPath = filepath.Join(t.TempDir(), "bundled-ffmpeg") // does not exist
	r.probe = staticProbe(compatibleResult("6.0"), nil)

	resolution := r.Resolve(context.Background())

	if resolution.Source != SourceOverride {
		t.Fatalf("source = %q, want override", resolution.Source)
	}
	if resolution.Path != override {
		t.Errorf("path = %q, want %q", resolution.Path, override)
	}
}

func TestBundledLocationIsUsedWhenNoOverrideIsSet(t *testing.T) {
	bundled := filepath.Join(t.TempDir(), "ffmpeg")
	fakeBinary(t, bundled)

	r := newTestResolver(t)
	r.bundledPath = bundled
	r.probe = staticProbe(compatibleResult("6.0"), nil)

	resolution := r.Resolve(context.Background())

	if resolution.Source != SourceBundled {
		t.Fatalf("source = %q, want bundled", resolution.Source)
	}
}

func TestBundledLosesToOverride(t *testing.T) {
	override := filepath.Join(t.TempDir(), "override-ffmpeg")
	fakeBinary(t, override)
	bundled := filepath.Join(t.TempDir(), "bundled-ffmpeg")
	fakeBinary(t, bundled)

	r := newTestResolver(t)
	r.overridePath = override
	r.bundledPath = bundled
	r.probe = staticProbe(compatibleResult("6.0"), nil)

	resolution := r.Resolve(context.Background())

	if resolution.Source != SourceOverride {
		t.Fatalf("source = %q, want override", resolution.Source)
	}
}

func TestPathFallbackIsUsedWhenNothingElseIsConfigured(t *testing.T) {
	pathBinary := filepath.Join(t.TempDir(), "ffmpeg")
	fakeBinary(t, pathBinary)

	r := &Resolver{
		lookPath: func(name string) (string, error) {
			if name == "ffmpeg" {
				return pathBinary, nil
			}
			return "", errors.New("not found")
		},
		probe: staticProbe(compatibleResult("6.0"), nil),
	}

	resolution := r.Resolve(context.Background())

	if resolution.Source != SourcePath {
		t.Fatalf("source = %q, want path", resolution.Source)
	}
}

func TestOverridePathThatDoesNotExistIsReportedMissing(t *testing.T) {
	r := newTestResolver(t)
	r.overridePath = filepath.Join(t.TempDir(), "absent")

	resolution := r.Resolve(context.Background())

	if resolution.Source != SourceMissing {
		t.Errorf("source = %q, want missing", resolution.Source)
	}
	if resolution.Err == nil || resolution.Err.Code != CodeNotFound {
		t.Errorf("error = %+v, want %s", resolution.Err, CodeNotFound)
	}
}

func TestOverridePathPointingAtADirectoryIsRejected(t *testing.T) {
	r := newTestResolver(t)
	r.overridePath = t.TempDir()

	resolution := r.Resolve(context.Background())

	if resolution.Err == nil || resolution.Err.Code != CodeNotExecutable {
		t.Fatalf("error = %+v, want %s", resolution.Err, CodeNotExecutable)
	}
}

func TestAGitBuildVersionStringDoesNotByItselfRejectAnOtherwiseCapableBinary(t *testing.T) {
	// Capability probing is authoritative; a version token with no numeric
	// meaning (a git/dev build) must not be treated as an automatic failure,
	// even though it is still shown to the user as the detected version.
	override := filepath.Join(t.TempDir(), "ffmpeg")
	fakeBinary(t, override)

	r := newTestResolver(t)
	r.overridePath = override
	result := compatibleResult("N-999-gabcdef")
	result.versionOutput = "ffmpeg version N-999-gabcdef Copyright\n"
	r.probe = staticProbe(result, nil)

	resolution := r.Resolve(context.Background())

	if resolution.Version != "N-999-gabcdef" {
		t.Errorf("version = %q, want the raw reported token", resolution.Version)
	}
	if !resolution.Compatible {
		t.Errorf("a git-build version with full capabilities was rejected: %+v", resolution.Err)
	}
}

func TestCompletelyUnreadableVersionOutputLeavesVersionEmpty(t *testing.T) {
	override := filepath.Join(t.TempDir(), "ffmpeg")
	fakeBinary(t, override)

	r := newTestResolver(t)
	r.overridePath = override
	result := compatibleResult("6.0")
	result.versionOutput = "this is not ffmpeg output at all\n"
	r.probe = staticProbe(result, nil)

	resolution := r.Resolve(context.Background())

	if resolution.Version != "" {
		t.Errorf("version = %q, want empty when no version line is present", resolution.Version)
	}
	// Still capability-gated, and every capability here reports true, so this
	// remains compatible - version-string readability is not itself a gate.
	if !resolution.Compatible {
		t.Errorf("capabilities were satisfied but the binary was still rejected: %+v", resolution.Err)
	}
}

func TestBelowMinimumVersionIsIncompatible(t *testing.T) {
	override := filepath.Join(t.TempDir(), "ffmpeg")
	fakeBinary(t, override)

	r := newTestResolver(t)
	r.overridePath = override
	r.probe = staticProbe(compatibleResult("3.4"), nil)

	resolution := r.Resolve(context.Background())

	if resolution.Compatible {
		t.Fatal("a below-minimum version was accepted")
	}
	if resolution.Err == nil || resolution.Err.Code != CodeBelowMinimumVersion {
		t.Fatalf("error = %+v, want %s", resolution.Err, CodeBelowMinimumVersion)
	}
}

func TestNewerVersionThanAnythingKnownIsAcceptedWhenCapable(t *testing.T) {
	override := filepath.Join(t.TempDir(), "ffmpeg")
	fakeBinary(t, override)

	r := newTestResolver(t)
	r.overridePath = override
	r.probe = staticProbe(compatibleResult("99.0"), nil)

	resolution := r.Resolve(context.Background())

	if !resolution.Compatible {
		t.Fatalf("a newer, capable version was rejected: %+v", resolution.Err)
	}
}

func TestMissingRTMPInputIsIncompatible(t *testing.T) {
	override := filepath.Join(t.TempDir(), "ffmpeg")
	fakeBinary(t, override)

	r := newTestResolver(t)
	r.overridePath = override
	result := compatibleResult("6.0")
	result.protocols.RTMPInput = false
	r.probe = staticProbe(result, nil)

	resolution := r.Resolve(context.Background())

	if resolution.Compatible {
		t.Fatal("missing RTMP input support was accepted")
	}
	if resolution.Err == nil || resolution.Err.Code != CodeMissingCapability {
		t.Fatalf("error = %+v, want %s", resolution.Err, CodeMissingCapability)
	}
}

func TestMissingRTMPSOutputIsIncompatible(t *testing.T) {
	override := filepath.Join(t.TempDir(), "ffmpeg")
	fakeBinary(t, override)

	r := newTestResolver(t)
	r.overridePath = override
	result := compatibleResult("6.0")
	result.protocols.RTMPSOutput = false
	r.probe = staticProbe(result, nil)

	resolution := r.Resolve(context.Background())

	if resolution.Compatible {
		t.Fatal("missing RTMPS output support was accepted")
	}
}

func TestMissingFLVMuxerIsIncompatible(t *testing.T) {
	override := filepath.Join(t.TempDir(), "ffmpeg")
	fakeBinary(t, override)

	r := newTestResolver(t)
	r.overridePath = override
	result := compatibleResult("6.0")
	result.flvMuxer = false
	r.probe = staticProbe(result, nil)

	resolution := r.Resolve(context.Background())

	if resolution.Compatible {
		t.Fatal("missing FLV muxer support was accepted")
	}
}

func TestMissingProgressSupportIsIncompatible(t *testing.T) {
	override := filepath.Join(t.TempDir(), "ffmpeg")
	fakeBinary(t, override)

	r := newTestResolver(t)
	r.overridePath = override
	result := compatibleResult("6.0")
	result.progressWorks = false
	r.probe = staticProbe(result, nil)

	resolution := r.Resolve(context.Background())

	if resolution.Compatible {
		t.Fatal("missing -progress support was accepted")
	}
}

func TestExecutionFailureIsReportedAsExecutionFailed(t *testing.T) {
	override := filepath.Join(t.TempDir(), "ffmpeg")
	fakeBinary(t, override)

	r := newTestResolver(t)
	r.overridePath = override
	r.probe = staticProbe(probeResult{}, errors.New("boom"))

	resolution := r.Resolve(context.Background())

	if resolution.Err == nil || resolution.Err.Code != CodeExecutionFailed {
		t.Fatalf("error = %+v, want %s", resolution.Err, CodeExecutionFailed)
	}
}
