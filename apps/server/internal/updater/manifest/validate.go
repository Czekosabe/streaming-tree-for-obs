package manifest

import (
	"fmt"
	"strings"
)

// MaxArtifactSizeBytes is the hard download-size ceiling from
// docs/updater.md §13: generous headroom over any realistic near-future
// installer size, documented here so a later change is a deliberate,
// reviewed decision rather than a forgotten magic number.
const MaxArtifactSizeBytes int64 = 300 * 1024 * 1024

// sha256HexLength is the exact length of a lowercase-hex SHA-256 digest.
const sha256HexLength = 64

// Validate checks every rule in docs/updater.md §5 and returns an
// ErrInvalid-wrapped error describing the first violation found. tag is
// the release's own canonical Git tag (e.g. "v0.2.0"); m.Version must
// exactly equal tag's parsed version - a manifest is never trusted
// against a release it does not actually describe.
func Validate(m Manifest, tag string) error {
	if m.Format != Format {
		return fmt.Errorf("%w: format must be %q, got %q", ErrInvalid, Format, m.Format)
	}
	if m.SchemaVersion != SchemaVersion {
		return fmt.Errorf("%w: schemaVersion %d is not understood by this build (expected %d)", ErrInvalid, m.SchemaVersion, SchemaVersion)
	}

	manifestVersion, err := ParseVersion(m.Version)
	if err != nil {
		return err
	}
	tagVersion, err := ParseTag(tag)
	if err != nil {
		return err
	}
	if manifestVersion.Compare(tagVersion) != 0 {
		return fmt.Errorf("%w: manifest version %q does not match release tag %q", ErrInvalid, m.Version, tag)
	}

	if m.Channel != ChannelStable {
		return fmt.Errorf("%w: channel must be %q, got %q", ErrInvalid, ChannelStable, m.Channel)
	}

	if len(m.Artifacts) == 0 {
		return fmt.Errorf("%w: artifacts must not be empty", ErrInvalid)
	}

	seenIdentity := make(map[Identity]bool, len(m.Artifacts))
	seenName := make(map[string]bool, len(m.Artifacts))
	for i, a := range m.Artifacts {
		if err := validateArtifact(a); err != nil {
			return fmt.Errorf("artifact %d: %w", i, err)
		}

		id := a.Identity()
		if seenIdentity[id] {
			return fmt.Errorf("%w: duplicate artifact identity %s/%s/%s", ErrInvalid, a.OS, a.Arch, a.Kind)
		}
		seenIdentity[id] = true

		if seenName[a.Name] {
			return fmt.Errorf("%w: duplicate artifact name %q", ErrInvalid, a.Name)
		}
		seenName[a.Name] = true
	}

	return nil
}

func validateArtifact(a Artifact) error {
	if !a.OS.known() {
		return fmt.Errorf("%w: unknown os %q", ErrInvalid, a.OS)
	}
	if !a.Arch.known() {
		return fmt.Errorf("%w: unknown arch %q", ErrInvalid, a.Arch)
	}
	if !a.Kind.known() {
		return fmt.Errorf("%w: unknown kind %q", ErrInvalid, a.Kind)
	}

	if err := validateArtifactName(a.Name); err != nil {
		return err
	}

	if a.SizeBytes <= 0 {
		return fmt.Errorf("%w: sizeBytes must be positive, got %d", ErrInvalid, a.SizeBytes)
	}
	if a.SizeBytes > MaxArtifactSizeBytes {
		return fmt.Errorf("%w: sizeBytes %d exceeds the maximum of %d bytes", ErrInvalid, a.SizeBytes, MaxArtifactSizeBytes)
	}

	if err := validateSHA256(a.SHA256); err != nil {
		return err
	}

	return nil
}

// validateArtifactName rejects anything that is not a plain filename:
// this value later becomes part of a filesystem path (docs/updater.md
// §16), so no path separator, no relative segment, and no drive letter
// is ever accepted, regardless of platform.
func validateArtifactName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: artifact name must not be empty", ErrInvalid)
	}
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("%w: artifact name %q must not contain a path separator", ErrInvalid, name)
	}
	if name == "." || name == ".." {
		return fmt.Errorf("%w: artifact name %q is a relative path segment", ErrInvalid, name)
	}
	if strings.Contains(name, ":") {
		return fmt.Errorf("%w: artifact name %q must not contain ':'", ErrInvalid, name)
	}
	return nil
}

func validateSHA256(digest string) error {
	if len(digest) != sha256HexLength {
		return fmt.Errorf("%w: sha256 must be exactly %d lowercase hex characters, got %d", ErrInvalid, sha256HexLength, len(digest))
	}
	for _, r := range digest {
		isLowerHex := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')
		if !isLowerHex {
			return fmt.Errorf("%w: sha256 %q is not lowercase hexadecimal", ErrInvalid, digest)
		}
	}
	return nil
}
