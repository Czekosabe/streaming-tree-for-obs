package manifest

import (
	"strings"
	"testing"
)

func TestParseRoundTrip(t *testing.T) {
	m := validManifest()
	raw := MustMarshal(m)

	got, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got.Format != m.Format || got.Version != m.Version || got.Channel != m.Channel {
		t.Fatalf("Parse() round-trip mismatch: got %+v, want %+v", got, m)
	}
	if len(got.Artifacts) != 1 || got.Artifacts[0] != m.Artifacts[0] {
		t.Fatalf("Parse() artifacts mismatch: got %+v", got.Artifacts)
	}
}

func TestParseRejectsUnknownField(t *testing.T) {
	raw := []byte(`{
		"format": "streaming-tree-release",
		"schemaVersion": 1,
		"version": "0.2.0",
		"channel": "stable",
		"artifacts": [],
		"downloadUrl": "https://evil.example/payload.exe"
	}`)
	if _, err := Parse(raw); err == nil {
		t.Fatal("Parse() accepted an unknown field, want rejection")
	}
}

func TestParseRejectsTrailingContent(t *testing.T) {
	raw := []byte(`{"format":"streaming-tree-release","schemaVersion":1,"version":"0.2.0","channel":"stable","artifacts":[]}{}`)
	if _, err := Parse(raw); err == nil {
		t.Fatal("Parse() accepted trailing content, want rejection")
	}
}

func TestParseRejectsMalformedJSON(t *testing.T) {
	if _, err := Parse([]byte(`{not json`)); err == nil {
		t.Fatal("Parse() accepted malformed JSON, want rejection")
	}
}

func TestParseRejectsEmptyInput(t *testing.T) {
	if _, err := Parse([]byte("")); err == nil {
		t.Fatal("Parse() accepted empty input, want rejection")
	}
}

func TestArtifactForFindsMatch(t *testing.T) {
	m := validManifest()
	a, ok := m.ArtifactFor(Identity{OS: OSWindows, Arch: ArchAMD64, Kind: KindInstaller})
	if !ok {
		t.Fatal("ArtifactFor() = not found, want a match")
	}
	if a.Name != m.Artifacts[0].Name {
		t.Fatalf("ArtifactFor() = %+v, want %+v", a, m.Artifacts[0])
	}
}

func TestArtifactForNoMatch(t *testing.T) {
	m := validManifest()
	if _, ok := m.ArtifactFor(Identity{OS: OSDarwin, Arch: ArchARM64, Kind: KindDMG}); ok {
		t.Fatal("ArtifactFor() = found, want no match")
	}
}

// multiPlatformManifest is a real Stage 20C1 shape: one release
// describing a Windows installer and both macOS DMG architectures
// together (docs/macos-packaging.md §22).
func multiPlatformManifest() Manifest {
	m := validManifest() // windows/amd64/installer
	m.Artifacts = append(m.Artifacts,
		Artifact{
			OS: OSDarwin, Arch: ArchARM64, Kind: KindDMG,
			Name:      "StreamingTreeForOBS-0.2.0-darwin-arm64.dmg",
			SizeBytes: 23456789,
			SHA256:    strings.Repeat("b", 64),
		},
		Artifact{
			OS: OSDarwin, Arch: ArchAMD64, Kind: KindDMG,
			Name:      "StreamingTreeForOBS-0.2.0-darwin-amd64.dmg",
			SizeBytes: 23456790,
			SHA256:    strings.Repeat("c", 64),
		},
	)
	return m
}

func TestMultiPlatformManifestIsValid(t *testing.T) {
	if err := Validate(multiPlatformManifest(), "v0.2.0"); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

// TestMultiPlatformManifestSelectsExactIdentityOnly proves the real
// selection property the updater's own security model depends on
// (docs/updater.md §7, docs/macos-packaging.md §22): a Windows build's
// identity only ever resolves the Windows artifact, each macOS
// architecture only resolves its own artifact, and no identity ever
// fuzzy-matches a different OS or a different architecture on the same
// OS, even though all three coexist in one manifest for one release.
func TestMultiPlatformManifestSelectsExactIdentityOnly(t *testing.T) {
	m := multiPlatformManifest()

	windows, ok := m.ArtifactFor(Identity{OS: OSWindows, Arch: ArchAMD64, Kind: KindInstaller})
	if !ok || windows.Name != "StreamingTreeForOBS-Setup-0.2.0.exe" {
		t.Fatalf("windows/amd64/installer = %+v, ok=%v, want the Windows artifact", windows, ok)
	}

	macARM, ok := m.ArtifactFor(Identity{OS: OSDarwin, Arch: ArchARM64, Kind: KindDMG})
	if !ok || macARM.Name != "StreamingTreeForOBS-0.2.0-darwin-arm64.dmg" {
		t.Fatalf("darwin/arm64/dmg = %+v, ok=%v, want the Mac ARM64 artifact", macARM, ok)
	}

	macIntel, ok := m.ArtifactFor(Identity{OS: OSDarwin, Arch: ArchAMD64, Kind: KindDMG})
	if !ok || macIntel.Name != "StreamingTreeForOBS-0.2.0-darwin-amd64.dmg" {
		t.Fatalf("darwin/amd64/dmg = %+v, ok=%v, want the Mac Intel artifact", macIntel, ok)
	}

	// No cross-platform/cross-architecture fuzzy match of any kind.
	noMatchIdentities := []Identity{
		{OS: OSDarwin, Arch: ArchAMD64, Kind: KindInstaller},  // right OS/arch, wrong kind
		{OS: OSDarwin, Arch: ArchARM64, Kind: KindInstaller},  // right OS/arch, wrong kind
		{OS: OSWindows, Arch: ArchARM64, Kind: KindInstaller}, // right OS, wrong arch
		{OS: OSLinux, Arch: ArchAMD64, Kind: KindInstaller},   // wrong OS entirely
	}
	for _, id := range noMatchIdentities {
		if _, ok := m.ArtifactFor(id); ok {
			t.Errorf("ArtifactFor(%+v) unexpectedly matched an artifact from a multi-platform manifest", id)
		}
	}
}

// TestMultiPlatformManifestStillRejectsDuplicateIdentity proves the
// existing duplicate-identity guard still holds once a manifest
// describes more than one platform - a second darwin/arm64/dmg artifact
// must be rejected exactly like a second windows/amd64/installer would
// be.
func TestMultiPlatformManifestStillRejectsDuplicateIdentity(t *testing.T) {
	m := multiPlatformManifest()
	dup := m.Artifacts[1] // darwin/arm64/dmg
	dup.Name = "a-different-name.dmg"
	m.Artifacts = append(m.Artifacts, dup)
	requireInvalid(t, Validate(m, "v0.2.0"))
}

func TestMustMarshalProducesNoDownloadURLField(t *testing.T) {
	raw := string(MustMarshal(validManifest()))
	if strings.Contains(strings.ToLower(raw), "url") {
		t.Fatalf("manifest JSON unexpectedly contains a URL-like field: %s", raw)
	}
}
