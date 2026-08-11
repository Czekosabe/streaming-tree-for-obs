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
)

// Format/CurrentManifestSchemaVersion identify the manifest schema
// (schema C, docs/visual-template-packages.md §3/§5) - independent of
// visualtemplate.CurrentTemplateSchemaVersion (schema B) and
// visualdesign.CurrentVersion (schema A). One counter per concern, never
// reused.
const (
	Format                       = "streaming-tree-template-package"
	CurrentManifestSchemaVersion = 1
)

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

// Manifest is the top-level manifest.json shape (docs/visual-template-
// packages.md §5).
type Manifest struct {
	Format        string          `json:"format"`
	SchemaVersion int             `json:"schemaVersion"`
	TemplatePath  string          `json:"templatePath"`
	Assets        []ManifestAsset `json:"assets"`
}

// Bounds (docs/visual-template-packages.md §10).
const (
	MaxPackageBytes            int64   = 96 << 20
	MaxTotalUncompressedBytes  int64   = 128 << 20
	MaxArchiveEntries                  = 64
	MaxAssets                          = 32
	MaxManifestBytes           int64   = 64 << 10
	MaxTemplateBytes           int64   = 128 << 10
	MaxDecompressionRatio      float64 = 100.0
	MaxAssetMetadataCodePoints         = 200
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
	if m.SchemaVersion != CurrentManifestSchemaVersion {
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
	return nil
}
