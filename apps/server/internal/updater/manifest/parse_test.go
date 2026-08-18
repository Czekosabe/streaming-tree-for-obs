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

func TestMustMarshalProducesNoDownloadURLField(t *testing.T) {
	raw := string(MustMarshal(validManifest()))
	if strings.Contains(strings.ToLower(raw), "url") {
		t.Fatalf("manifest JSON unexpectedly contains a URL-like field: %s", raw)
	}
}
