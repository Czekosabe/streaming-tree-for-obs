package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/streaming-tree/server/internal/updater/manifest"
)

func TestHashFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "artifact.bin")
	content := []byte("some release artifact content")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	size, sha, err := hashFile(path)
	if err != nil {
		t.Fatalf("hashFile() error = %v", err)
	}
	if size != int64(len(content)) {
		t.Errorf("size = %d, want %d", size, len(content))
	}

	want := sha256.Sum256(content)
	if sha != hex.EncodeToString(want[:]) {
		t.Errorf("sha256 = %s, want %s", sha, hex.EncodeToString(want[:]))
	}
}

func TestHashFileMissing(t *testing.T) {
	if _, _, err := hashFile(filepath.Join(t.TempDir(), "does-not-exist.bin")); err == nil {
		t.Fatal("hashFile() accepted a missing file, want an error")
	}
}

func TestBaseManifestFreshWhenNoInPath(t *testing.T) {
	m, err := baseManifest("", "0.2.0")
	if err != nil {
		t.Fatalf("baseManifest() error = %v", err)
	}
	if m.Format != manifest.Format || m.SchemaVersion != manifest.SchemaVersion {
		t.Fatalf("baseManifest() = %+v, want fresh Format/SchemaVersion set", m)
	}
	if m.Version != "0.2.0" || m.Channel != manifest.ChannelStable {
		t.Fatalf("baseManifest() = %+v, want version 0.2.0 on the stable channel", m)
	}
	if len(m.Artifacts) != 0 {
		t.Fatalf("baseManifest() Artifacts = %v, want empty (the caller appends the new one)", m.Artifacts)
	}
}

// TestBaseManifestExtendsExistingFile proves the real multi-invocation
// assembly path docs/macos-packaging.md §22 describes: a Windows build
// writes the first manifest, then a macOS build (on a different
// machine, hours or days later) points -in at that same file and gets
// its own artifact appended to the existing one, never a second format
// and never losing what was already there.
func TestBaseManifestExtendsExistingFile(t *testing.T) {
	existing := manifest.Manifest{
		Format: manifest.Format, SchemaVersion: manifest.SchemaVersion,
		Version: "0.2.0", Channel: manifest.ChannelStable,
		Artifacts: []manifest.Artifact{
			{
				OS: manifest.OSWindows, Arch: manifest.ArchAMD64, Kind: manifest.KindInstaller,
				Name:      "StreamingTreeForOBS-0.2.0-windows-amd64-setup.exe",
				SizeBytes: 12345678,
				SHA256:    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			},
		},
	}
	path := filepath.Join(t.TempDir(), "release.json")
	if err := os.WriteFile(path, manifest.MustMarshal(existing), 0o600); err != nil {
		t.Fatalf("write existing manifest: %v", err)
	}

	m, err := baseManifest(path, "0.2.0")
	if err != nil {
		t.Fatalf("baseManifest() error = %v", err)
	}
	if len(m.Artifacts) != 1 || m.Artifacts[0].Name != existing.Artifacts[0].Name {
		t.Fatalf("baseManifest() Artifacts = %+v, want the one existing Windows artifact carried over", m.Artifacts)
	}
}

func TestBaseManifestRejectsVersionMismatch(t *testing.T) {
	existing := manifest.Manifest{
		Format: manifest.Format, SchemaVersion: manifest.SchemaVersion,
		Version: "0.2.0", Channel: manifest.ChannelStable,
		Artifacts: []manifest.Artifact{
			{
				OS: manifest.OSWindows, Arch: manifest.ArchAMD64, Kind: manifest.KindInstaller,
				Name:      "StreamingTreeForOBS-0.2.0-windows-amd64-setup.exe",
				SizeBytes: 12345678,
				SHA256:    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			},
		},
	}
	path := filepath.Join(t.TempDir(), "release.json")
	if err := os.WriteFile(path, manifest.MustMarshal(existing), 0o600); err != nil {
		t.Fatalf("write existing manifest: %v", err)
	}

	// A different release's artifact must never be silently folded into
	// an unrelated manifest just because -in happened to point at one.
	if _, err := baseManifest(path, "0.3.0"); err == nil {
		t.Fatal("baseManifest() accepted a version mismatch between -in and -version, want an error")
	}
}
