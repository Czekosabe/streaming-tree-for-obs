package manifest

import (
	"errors"
	"strings"
	"testing"
)

func validManifest() Manifest {
	return Manifest{
		Format:        Format,
		SchemaVersion: SchemaVersion,
		Version:       "0.2.0",
		Channel:       ChannelStable,
		Artifacts: []Artifact{
			{
				OS: OSWindows, Arch: ArchAMD64, Kind: KindInstaller,
				Name:      "StreamingTreeForOBS-Setup-0.2.0.exe",
				SizeBytes: 12345678,
				SHA256:    strings.Repeat("a", 64),
			},
		},
	}
}

func TestValidateAcceptsValidManifest(t *testing.T) {
	if err := Validate(validManifest(), "v0.2.0"); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateAcceptsTagWithoutLeadingV(t *testing.T) {
	if err := Validate(validManifest(), "0.2.0"); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsWrongFormat(t *testing.T) {
	m := validManifest()
	m.Format = "something-else"
	requireInvalid(t, Validate(m, "v0.2.0"))
}

func TestValidateRejectsUnknownSchemaVersion(t *testing.T) {
	m := validManifest()
	m.SchemaVersion = 2
	requireInvalid(t, Validate(m, "v0.2.0"))
}

func TestValidateRejectsVersionTagMismatch(t *testing.T) {
	m := validManifest()
	requireInvalid(t, Validate(m, "v0.3.0"))
}

func TestValidateRejectsMalformedVersion(t *testing.T) {
	m := validManifest()
	m.Version = "latest"
	requireInvalid(t, Validate(m, "v0.2.0"))
}

func TestValidateRejectsNonStableChannel(t *testing.T) {
	for _, ch := range []Channel{"beta", "nightly", "canary", "dev", ""} {
		m := validManifest()
		m.Channel = ch
		if err := Validate(m, "v0.2.0"); err == nil {
			t.Errorf("Validate() with channel %q accepted, want rejection", ch)
		}
	}
}

func TestValidateRejectsEmptyArtifacts(t *testing.T) {
	m := validManifest()
	m.Artifacts = nil
	requireInvalid(t, Validate(m, "v0.2.0"))
}

func TestValidateRejectsDuplicateIdentity(t *testing.T) {
	m := validManifest()
	dup := m.Artifacts[0]
	dup.Name = "a-different-name.exe"
	m.Artifacts = append(m.Artifacts, dup)
	requireInvalid(t, Validate(m, "v0.2.0"))
}

func TestValidateRejectsDuplicateName(t *testing.T) {
	m := validManifest()
	dup := m.Artifacts[0]
	dup.OS = OSLinux
	dup.Arch = ArchARM64
	dup.Kind = KindAppImage
	// Same Name as the first artifact - identity differs, name does not.
	m.Artifacts = append(m.Artifacts, dup)
	requireInvalid(t, Validate(m, "v0.2.0"))
}

func TestValidateRejectsUnknownOS(t *testing.T) {
	m := validManifest()
	m.Artifacts[0].OS = "freebsd"
	requireInvalid(t, Validate(m, "v0.2.0"))
}

func TestValidateRejectsUnknownArch(t *testing.T) {
	m := validManifest()
	m.Artifacts[0].Arch = "arm"
	requireInvalid(t, Validate(m, "v0.2.0"))
}

func TestValidateRejectsUnknownKind(t *testing.T) {
	m := validManifest()
	m.Artifacts[0].Kind = "msi"
	requireInvalid(t, Validate(m, "v0.2.0"))
}

func TestValidateRejectsBadArtifactNames(t *testing.T) {
	cases := []string{"", "..", ".", "sub/dir.exe", `sub\dir.exe`, "C:\\evil.exe", "a:b"}
	for _, name := range cases {
		m := validManifest()
		m.Artifacts[0].Name = name
		if err := Validate(m, "v0.2.0"); err == nil {
			t.Errorf("Validate() with artifact name %q accepted, want rejection", name)
		}
	}
}

func TestValidateRejectsInvalidSize(t *testing.T) {
	for _, size := range []int64{0, -1, MaxArtifactSizeBytes + 1} {
		m := validManifest()
		m.Artifacts[0].SizeBytes = size
		if err := Validate(m, "v0.2.0"); err == nil {
			t.Errorf("Validate() with size %d accepted, want rejection", size)
		}
	}
}

func TestValidateAcceptsMaxSize(t *testing.T) {
	m := validManifest()
	m.Artifacts[0].SizeBytes = MaxArtifactSizeBytes
	if err := Validate(m, "v0.2.0"); err != nil {
		t.Fatalf("Validate() at exactly the max size rejected: %v", err)
	}
}

func TestValidateRejectsBadSHA256(t *testing.T) {
	cases := []string{
		"",
		strings.Repeat("a", 63),
		strings.Repeat("a", 65),
		strings.Repeat("A", 64), // uppercase not accepted
		strings.Repeat("g", 64), // not hex
	}
	for _, digest := range cases {
		m := validManifest()
		m.Artifacts[0].SHA256 = digest
		if err := Validate(m, "v0.2.0"); err == nil {
			t.Errorf("Validate() with sha256 %q accepted, want rejection", digest)
		}
	}
}

func requireInvalid(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("Validate() = nil, want an error")
	}
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("Validate() error = %v, want it to wrap ErrInvalid", err)
	}
}
