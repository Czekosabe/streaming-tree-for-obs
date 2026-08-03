package mediamtx

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// fakeBinary writes a placeholder file that stands in for the executable.
// It is never executed: the version probe is injected in these tests.
func fakeBinary(t *testing.T, path string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create directory: %v", err)
	}
	if err := os.WriteFile(path, []byte("not a real binary"), 0o700); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
}

func staticProbe(output string, err error) func(context.Context, string) (string, error) {
	return func(context.Context, string) (string, error) { return output, err }
}

func TestResolveReportsMissingWhenNothingIsInstalled(t *testing.T) {
	resolver := NewResolver(t.TempDir(), "")

	resolution := resolver.Resolve(context.Background())

	if resolution.Source != SourceMissing {
		t.Errorf("source = %q, want missing", resolution.Source)
	}
	if resolution.Err == nil || resolution.Err.Code != CodeNotInstalled {
		t.Errorf("error = %+v, want %s", resolution.Err, CodeNotInstalled)
	}
}

func TestResolveFindsAManagedInstallation(t *testing.T) {
	dataDir := t.TempDir()
	asset, err := CurrentAsset()
	if err != nil {
		t.Skipf("this platform has no managed asset: %v", err)
	}

	fakeBinary(t, ManagedExecutablePath(dataDir, asset.PlatformDir, asset.ExecutableName))

	resolver := NewResolver(dataDir, "")
	resolver.versionProbe = staticProbe(SupportedVersion+"\n", nil)

	resolution := resolver.Resolve(context.Background())

	if resolution.Source != SourceManaged {
		t.Fatalf("source = %q, want managed", resolution.Source)
	}
	if !resolution.Compatible {
		t.Errorf("the pinned version was reported incompatible: %+v", resolution.Err)
	}
	if resolution.Version != SupportedVersion {
		t.Errorf("version = %q, want %q", resolution.Version, SupportedVersion)
	}
}

func TestExplicitPathWinsOverTheManagedInstallation(t *testing.T) {
	dataDir := t.TempDir()
	asset, err := CurrentAsset()
	if err != nil {
		t.Skipf("this platform has no managed asset: %v", err)
	}
	fakeBinary(t, ManagedExecutablePath(dataDir, asset.PlatformDir, asset.ExecutableName))

	override := filepath.Join(t.TempDir(), ExecutableNameFor(runtime.GOOS))
	fakeBinary(t, override)

	resolver := NewResolver(dataDir, override)
	resolver.versionProbe = staticProbe(SupportedVersion+"\n", nil)

	resolution := resolver.Resolve(context.Background())

	if resolution.Source != SourceOverride {
		t.Fatalf("source = %q, want override", resolution.Source)
	}
	if resolution.Path != override {
		t.Errorf("path = %q, want the override path", resolution.Path)
	}
}

func TestOverridePathThatDoesNotExistIsReportedMissing(t *testing.T) {
	resolver := NewResolver(t.TempDir(), filepath.Join(t.TempDir(), "absent"))

	resolution := resolver.Resolve(context.Background())

	if resolution.Source != SourceMissing {
		t.Errorf("source = %q, want missing", resolution.Source)
	}
	if resolution.Err == nil || resolution.Err.Code != CodeNotInstalled {
		t.Errorf("error = %+v, want %s", resolution.Err, CodeNotInstalled)
	}
}

func TestOverridePathPointingAtADirectoryIsRejected(t *testing.T) {
	directory := t.TempDir()
	resolver := NewResolver(t.TempDir(), directory)

	resolution := resolver.Resolve(context.Background())

	if resolution.Err == nil {
		t.Fatal("a directory was accepted as the MediaMTX executable")
	}
}

func TestUnsupportedVersionIsReportedIncompatible(t *testing.T) {
	dataDir := t.TempDir()
	asset, err := CurrentAsset()
	if err != nil {
		t.Skipf("this platform has no managed asset: %v", err)
	}
	fakeBinary(t, ManagedExecutablePath(dataDir, asset.PlatformDir, asset.ExecutableName))

	resolver := NewResolver(dataDir, "")
	resolver.versionProbe = staticProbe("v1.18.0\n", nil)

	resolution := resolver.Resolve(context.Background())

	if resolution.Compatible {
		t.Fatal("an unsupported version was accepted")
	}
	if resolution.Err == nil || resolution.Err.Code != CodeIncompatibleVersion {
		t.Fatalf("error = %+v, want %s", resolution.Err, CodeIncompatibleVersion)
	}
	// The message must name both versions so the user knows what to do.
	if !contains(resolution.Err.Message, "v1.18.0") || !contains(resolution.Err.Message, SupportedVersion) {
		t.Errorf("message %q should mention both the found and the expected version",
			resolution.Err.Message)
	}
}

func TestMalformedVersionOutputIsRejected(t *testing.T) {
	dataDir := t.TempDir()
	asset, err := CurrentAsset()
	if err != nil {
		t.Skipf("this platform has no managed asset: %v", err)
	}
	fakeBinary(t, ManagedExecutablePath(dataDir, asset.PlatformDir, asset.ExecutableName))

	resolver := NewResolver(dataDir, "")
	resolver.versionProbe = staticProbe("this is not a version", nil)

	resolution := resolver.Resolve(context.Background())

	if resolution.Compatible {
		t.Fatal("unreadable version output was accepted")
	}
	if resolution.Err == nil || resolution.Err.Code != CodeIncompatibleVersion {
		t.Errorf("error = %+v, want %s", resolution.Err, CodeIncompatibleVersion)
	}
}

func TestUnsupportedPlatformIsReportedClearly(t *testing.T) {
	resolver := NewResolver(t.TempDir(), "")
	resolver.goos = "plan9"
	resolver.goarch = "mips"

	resolution := resolver.Resolve(context.Background())

	if resolution.Source != SourceMissing {
		t.Errorf("source = %q, want missing", resolution.Source)
	}
	if resolution.Err == nil || resolution.Err.Code != CodeUnsupportedPlatform {
		t.Fatalf("error = %+v, want %s", resolution.Err, CodeUnsupportedPlatform)
	}
	// The message must point at the escape hatch.
	if !contains(resolution.Err.Message, "STREAMING_TREE_MEDIAMTX_PATH") {
		t.Errorf("message %q should mention the override variable", resolution.Err.Message)
	}
}

func TestInstallDirIsVersionedAndPlatformSpecific(t *testing.T) {
	dir := InstallDir("/data", "linux-amd64")

	// Side-by-side versions and platforms are what make a future upgrade a
	// install-then-switch rather than an in-place overwrite.
	if !contains(dir, SupportedVersion) {
		t.Errorf("install dir %q should contain the version", dir)
	}
	if !contains(dir, "linux-amd64") {
		t.Errorf("install dir %q should contain the platform", dir)
	}
	if !contains(dir, "runtime") {
		t.Errorf("install dir %q should live under the runtime directory", dir)
	}
}

func TestReadInstallationMetadata(t *testing.T) {
	dataDir := t.TempDir()
	installDir := InstallDir(dataDir, "linux-amd64")
	if err := os.MkdirAll(installDir, 0o700); err != nil {
		t.Fatalf("create install dir: %v", err)
	}

	body := `{"version":"v1.19.3","platform":"linux-amd64","assetName":"a.tar.gz","sha256":"abc","installedAt":"2026-08-03T00:00:00Z"}`
	if err := os.WriteFile(filepath.Join(installDir, MetadataFileName), []byte(body), 0o600); err != nil {
		t.Fatalf("write metadata: %v", err)
	}

	metadata, err := ReadInstallationMetadata(dataDir, "linux-amd64")
	if err != nil {
		t.Fatalf("ReadInstallationMetadata() returned an error: %v", err)
	}
	if metadata.Version != "v1.19.3" {
		t.Errorf("version = %q, want v1.19.3", metadata.Version)
	}
}

func TestReadInstallationMetadataRejectsMalformedContent(t *testing.T) {
	dataDir := t.TempDir()
	installDir := InstallDir(dataDir, "linux-amd64")
	if err := os.MkdirAll(installDir, 0o700); err != nil {
		t.Fatalf("create install dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(installDir, MetadataFileName), []byte("{"), 0o600); err != nil {
		t.Fatalf("write metadata: %v", err)
	}

	if _, err := ReadInstallationMetadata(dataDir, "linux-amd64"); err == nil {
		t.Fatal("malformed metadata was accepted")
	}
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
