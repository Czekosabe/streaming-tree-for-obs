// Package visualpackage implements Stage 14B's portable, secure
// `.streaming-tree-template` archive package: the ZIP-only container
// format, its manifest schema (schema C in docs/visual-template-
// packages.md §3), the "never blind extraction" archive validation
// pipeline, and package import/export/preview. See
// docs/visual-template-packages.md for the full contract this package
// implements - every bound, error code, and validation rule here is
// named there first.
//
// This package sits one architectural layer above internal/domain/
// visualasset and internal/domain/visualtemplate (both of which it
// imports) - it is the integration domain that bridges "a portable
// archive" to "a local template plus local managed assets", exactly the
// same relationship internal/domain/visualtemplate already has to
// internal/domain/visualdesign. It never imports internal/domain/alerts,
// internal/domain/chatoverlay, internal/provider/twitch, or any other
// owner-specific package.
package visualpackage

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/streaming-tree/server/internal/domain/visualtemplate"
)

// Format identifies the package archive container. ManifestSchemaVersionV1/
// V2 are the two manifest schema versions this codebase understands
// (schema C, docs/visual-template-packages.md §3/§5, docs/alert-audio.md
// §10.1) - independent of visualtemplate.CurrentTemplateSchemaVersion
// (schema B) and visualdesign.CurrentVersion (schema A). One counter per
// concern, never reused. CurrentManifestSchemaVersion is the highest
// version this codebase ever writes - ExportTemplate still writes v1 for
// a purely visual template with no alert-audio preset (docs/alert-
// audio.md §10.1: "never silently upgrading a visual-only template's own
// export format"); v2 is written only when one is present.
const (
	Format                       = "streaming-tree-template-package"
	ManifestSchemaVersionV1      = 1
	ManifestSchemaVersionV2      = 2
	CurrentManifestSchemaVersion = ManifestSchemaVersionV2
)

func manifestSchemaVersionSupported(v int) bool {
	return v == ManifestSchemaVersionV1 || v == ManifestSchemaVersionV2
}

// TemplatePath is the only value manifest.templatePath may ever equal
// (docs/visual-template-packages.md §5).
const TemplatePath = "template.json"

// ManifestAsset is one archive-local asset entry (docs/visual-template-
// packages.md §5) - exactly the eight fields a manifest asset may carry;
// unknown fields are rejected by the strict JSON decode in reader.go.
type ManifestAsset struct {
	ID          string `json:"id"`
	Path        string `json:"path"`
	Kind        string `json:"kind"`
	MediaType   string `json:"mediaType"`
	SHA256      string `json:"sha256"`
	SizeBytes   int64  `json:"sizeBytes"`
	DisplayName string `json:"displayName"`
	Author      string `json:"author"`
	License     string `json:"license"`
	Notice      string `json:"notice"`
}

// ManifestAlertAudio is the optional v2-only alertAudio manifest object
// (docs/alert-audio.md §10.2) - a package-local mirror of
// visualtemplate.RuleAudioPreset, referencing a package-local audio
// asset id (SoundAssetID, pkgaudio_-prefixed) rather than a real local
// one until import remaps it.
type ManifestAlertAudio struct {
	SoundEnabled bool    `json:"soundEnabled"`
	SoundAssetID string  `json:"soundAssetId"`
	SoundVolume  float64 `json:"soundVolume"`
	TTSEnabled   bool    `json:"ttsEnabled"`
	TTSTemplate  string  `json:"ttsTemplate"`
	TTSVolume    float64 `json:"ttsVolume"`
}

// ManifestAudioAsset is one archive-local audio asset entry (docs/alert-
// audio.md §10.2) - a separate, sibling array from Assets (visual assets)
// with its own package-local pkgaudio_ id namespace, disjoint by
// construction from pkgasset_.
type ManifestAudioAsset struct {
	ID          string `json:"id"`
	Path        string `json:"path"`
	MediaType   string `json:"mediaType"`
	SHA256      string `json:"sha256"`
	SizeBytes   int64  `json:"sizeBytes"`
	DurationMS  int64  `json:"durationMs"`
	DisplayName string `json:"displayName"`
}

// Manifest is the top-level manifest.json shape (docs/visual-template-
// packages.md §5, docs/alert-audio.md §10.2).
type Manifest struct {
	Format        string               `json:"format"`
	SchemaVersion int                  `json:"schemaVersion"`
	TemplatePath  string               `json:"templatePath"`
	Assets        []ManifestAsset      `json:"assets"`
	AlertAudio    *ManifestAlertAudio  `json:"alertAudio,omitempty"`
	AudioAssets   []ManifestAudioAsset `json:"audioAssets,omitempty"`
}

// Bounds (docs/visual-template-packages.md §10, docs/alert-audio.md
// §10.2).
const (
	MaxPackageBytes            int64   = 96 << 20
	MaxTotalUncompressedBytes  int64   = 128 << 20
	MaxArchiveEntries                  = 64
	MaxAssets                          = 32
	MaxManifestBytes           int64   = 64 << 10
	MaxTemplateBytes           int64   = 128 << 10
	MaxDecompressionRatio      float64 = 100.0
	MaxAssetMetadataCodePoints         = 200
	// MaxAudioAssets bounds a v2 package's own audioAssets array - an
	// alert template needs at most one sound; four is a generous
	// ceiling against a future multi-preset feature without inviting
	// abuse today (docs/alert-audio.md §10.2).
	MaxAudioAssets = 4
)

// decodeManifestStrict parses raw as a Manifest, rejecting any unknown
// top-level or per-asset field (docs/visual-template-packages.md §5:
// "strict JSON, unknown fields rejected").
func decodeManifestStrict(raw []byte) (Manifest, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var m Manifest
	if err := dec.Decode(&m); err != nil {
		return Manifest{}, fmt.Errorf("%w: %v", ErrManifestInvalid, err)
	}
	if dec.More() {
		return Manifest{}, fmt.Errorf("%w: trailing content after manifest JSON", ErrManifestInvalid)
	}
	return m, nil
}

func codePointLen(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}

// validateManifest checks manifest's own structural shape - format,
// schema version, templatePath, asset count, and every asset entry's own
// bounded fields (docs/visual-template-packages.md §5/§10/§16). It does
// NOT check that manifest assets correspond 1:1 with real archive
// entries, or validate any asset's actual bytes - see reader.go for the
// cross-checks that need the rest of the archive.
func validateManifest(m Manifest) error {
	if m.Format != Format {
		return fmt.Errorf("%w: format %q is not %q", ErrManifestInvalid, m.Format, Format)
	}
	if !manifestSchemaVersionSupported(m.SchemaVersion) {
		return fmt.Errorf("%w: manifest schema version %d is not supported", ErrVersionUnsupported, m.SchemaVersion)
	}
	if m.TemplatePath != TemplatePath {
		return fmt.Errorf("%w: templatePath must equal %q", ErrManifestInvalid, TemplatePath)
	}
	if len(m.Assets) > MaxAssets {
		return fmt.Errorf("%w: package has %d assets, exceeding the maximum of %d", ErrTooManyAssets, len(m.Assets), MaxAssets)
	}
	seenIDs := make(map[string]bool, len(m.Assets))
	seenPaths := make(map[string]bool, len(m.Assets))
	for i, a := range m.Assets {
		if err := validatePackageAssetID(a.ID); err != nil {
			return fmt.Errorf("asset %d: %w", i, err)
		}
		if seenIDs[a.ID] {
			return fmt.Errorf("%w: duplicate manifest asset id %q", ErrManifestInvalid, a.ID)
		}
		seenIDs[a.ID] = true

		if err := validateAssetPath(a.Path); err != nil {
			return fmt.Errorf("asset %q: %w", a.ID, err)
		}
		norm := normalizePath(a.Path)
		if seenPaths[norm] {
			return fmt.Errorf("%w: duplicate (case-insensitive) manifest asset path %q", ErrManifestInvalid, a.Path)
		}
		seenPaths[norm] = true

		if a.Kind != "image" && a.Kind != "video" && a.Kind != "font" {
			return fmt.Errorf("%w: asset %q has unrecognized kind %q", ErrAssetUnsupported, a.ID, a.Kind)
		}
		if a.MediaType == "" {
			return fmt.Errorf("%w: asset %q is missing a media type", ErrManifestInvalid, a.ID)
		}
		if len(a.SHA256) != 64 {
			return fmt.Errorf("%w: asset %q has a malformed sha256 field", ErrManifestInvalid, a.ID)
		}
		if a.SizeBytes < 0 {
			return fmt.Errorf("%w: asset %q has a negative sizeBytes", ErrManifestInvalid, a.ID)
		}
		if codePointLen(a.DisplayName) > MaxAssetMetadataCodePoints ||
			codePointLen(a.Author) > MaxAssetMetadataCodePoints ||
			codePointLen(a.License) > MaxAssetMetadataCodePoints ||
			codePointLen(a.Notice) > MaxAssetMetadataCodePoints {
			return fmt.Errorf("%w: asset %q metadata field exceeds %d characters", ErrManifestInvalid, a.ID, MaxAssetMetadataCodePoints)
		}
	}

	return validateManifestAudio(m)
}

// validateManifestAudio checks the optional v2-only alertAudio/
// audioAssets manifest objects (docs/alert-audio.md §10.2): legal only
// under schemaVersion 2, audioAssets' own bounded count/id/path/hash/
// metadata fields, alertAudio's own structural bounds (reusing
// visualtemplate.ValidateRuleAudioPreset - "the exact same validator §7
// uses for a live rule's audio object"), and the bidirectional
// alertAudio<->audioAssets cross-reference ("a package never contains
// audio bytes it doesn't use"). It does NOT check that template.json's
// own target is "alert" - that needs the parsed template file, checked
// by the caller (service.go) before any asset is staged, nor does it
// check any audio asset's actual bytes - see reader.go.
func validateManifestAudio(m Manifest) error {
	if m.SchemaVersion == ManifestSchemaVersionV1 && (m.AlertAudio != nil || len(m.AudioAssets) > 0) {
		return fmt.Errorf("%w: alertAudio/audioAssets require manifest schema version %d", ErrManifestInvalid, ManifestSchemaVersionV2)
	}
	if len(m.AudioAssets) > MaxAudioAssets {
		return fmt.Errorf("%w: package has %d audio assets, exceeding the maximum of %d", ErrTooManyAssets, len(m.AudioAssets), MaxAudioAssets)
	}

	audioIDs := make(map[string]bool, len(m.AudioAssets))
	seenPaths := make(map[string]bool, len(m.AudioAssets))
	for i, a := range m.AudioAssets {
		if err := validatePackageAudioAssetID(a.ID); err != nil {
			return fmt.Errorf("audio asset %d: %w", i, err)
		}
		if audioIDs[a.ID] {
			return fmt.Errorf("%w: duplicate manifest audio asset id %q", ErrManifestInvalid, a.ID)
		}
		audioIDs[a.ID] = true

		if err := validateAudioAssetPath(a.Path); err != nil {
			return fmt.Errorf("audio asset %q: %w", a.ID, err)
		}
		norm := normalizePath(a.Path)
		if seenPaths[norm] {
			return fmt.Errorf("%w: duplicate (case-insensitive) manifest audio asset path %q", ErrManifestInvalid, a.Path)
		}
		seenPaths[norm] = true

		if a.MediaType == "" {
			return fmt.Errorf("%w: audio asset %q is missing a media type", ErrManifestInvalid, a.ID)
		}
		if len(a.SHA256) != 64 {
			return fmt.Errorf("%w: audio asset %q has a malformed sha256 field", ErrManifestInvalid, a.ID)
		}
		if a.SizeBytes < 0 {
			return fmt.Errorf("%w: audio asset %q has a negative sizeBytes", ErrManifestInvalid, a.ID)
		}
		if a.DurationMS < 0 {
			return fmt.Errorf("%w: audio asset %q has a negative durationMs", ErrManifestInvalid, a.ID)
		}
		if codePointLen(a.DisplayName) > MaxAssetMetadataCodePoints {
			return fmt.Errorf("%w: audio asset %q display name exceeds %d characters", ErrManifestInvalid, a.ID, MaxAssetMetadataCodePoints)
		}
	}

	if m.AlertAudio == nil {
		if len(m.AudioAssets) > 0 {
			return fmt.Errorf("%w: audioAssets is present but alertAudio is not", ErrAssetUnreferenced)
		}
		return nil
	}

	preset := visualtemplate.RuleAudioPreset{
		SoundEnabled: m.AlertAudio.SoundEnabled, SoundAssetID: m.AlertAudio.SoundAssetID, SoundVolume: m.AlertAudio.SoundVolume,
		TTSEnabled: m.AlertAudio.TTSEnabled, TTSTemplate: m.AlertAudio.TTSTemplate, TTSVolume: m.AlertAudio.TTSVolume,
	}
	if err := visualtemplate.ValidateRuleAudioPreset(&preset); err != nil {
		return err
	}
	if m.AlertAudio.SoundAssetID != "" && !audioIDs[m.AlertAudio.SoundAssetID] {
		return fmt.Errorf("%w: alertAudio references audio asset %q, which is not in audioAssets", ErrAssetMissing, m.AlertAudio.SoundAssetID)
	}
	for id := range audioIDs {
		if id != m.AlertAudio.SoundAssetID {
			return fmt.Errorf("%w: audioAssets entry %q is not referenced by alertAudio", ErrAssetUnreferenced, id)
		}
	}
	return nil
}
