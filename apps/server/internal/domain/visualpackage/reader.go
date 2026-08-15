package visualpackage

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/streaming-tree/server/internal/domain/audioasset"
	"github.com/streaming-tree/server/internal/domain/visualasset"
)

// ValidatedAsset is one archive asset entry that has passed every check
// in this file: manifest cross-reference, size/hash/type agreement, and
// (for an image) dimension bounds. Data holds the asset's own verified
// bytes, read fully into memory only after every bound in
// docs/visual-template-packages.md §10 already passed - never before.
type ValidatedAsset struct {
	Manifest  ManifestAsset
	Data      []byte
	MediaType visualasset.MediaType
	Kind      visualasset.Kind
}

// ValidatedAudioAsset is ValidatedAsset's own audio-asset counterpart
// (docs/alert-audio.md §10.3) - streamed through the exact same
// audioasset structural/type-agreement validator a manual upload uses,
// never a separately-implemented, potentially weaker package-path
// validator.
type ValidatedAudioAsset struct {
	Manifest  ManifestAudioAsset
	Data      []byte
	MediaType audioasset.MediaType
}

// Validated is the fully-checked result of ReadArchive - nothing has
// been written to disk or to any database by this point; ReadArchive
// performs validation only (docs/visual-template-packages.md §9's
// pipeline step 1-4). A caller (service.go) is responsible for steps
// 5-7: staging/installing verified bytes and rewriting asset references.
type Validated struct {
	Manifest     Manifest
	TemplateJSON []byte
	Assets       []ValidatedAsset
	AudioAssets  []ValidatedAudioAsset
}

// ReadArchive validates data as a complete `.streaming-tree-template`
// package (docs/visual-template-packages.md §9's "never blind
// extraction" pipeline): archive-level bounds, every entry's path
// grammar and file mode, the manifest's own structural validity, that
// the archive's real entries and the manifest's declared assets
// correspond exactly (no extra file, no missing file, no unreferenced
// file), and - per asset - that declared size/hash/kind/media type all
// agree with the asset's own independently detected signature. data's
// own untrusted archive-entry names are used only as lookup keys against
// this already-validated allowed-entry set; they are never joined to a
// real filesystem path or written anywhere by this function.
func ReadArchive(data []byte) (*Validated, error) {
	if int64(len(data)) > MaxPackageBytes {
		return nil, fmt.Errorf("%w: package is %d bytes, exceeding the %d byte limit", ErrTooLarge, len(data), MaxPackageBytes)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("%w: not a valid ZIP archive", ErrInvalidArchive)
	}
	if len(zr.File) == 0 {
		return nil, fmt.Errorf("%w: archive is empty", ErrInvalidArchive)
	}
	if len(zr.File) > MaxArchiveEntries {
		return nil, fmt.Errorf("%w: archive has %d entries, exceeding the maximum of %d", ErrTooManyEntries, len(zr.File), MaxArchiveEntries)
	}

	entries := make(map[string]*zip.File, len(zr.File))
	seenNormalized := make(map[string]bool, len(zr.File))
	var totalUncompressed uint64

	for _, f := range zr.File {
		if err := validateEntryPath(f.Name); err != nil {
			return nil, err
		}
		norm := normalizePath(f.Name)
		if seenNormalized[norm] {
			return nil, fmt.Errorf("%w: duplicate (case-insensitive) archive entry %q", ErrEntryInvalid, f.Name)
		}
		seenNormalized[norm] = true

		if !f.Mode().IsRegular() {
			return nil, fmt.Errorf("%w: entry %q is not a regular file (symlink/special/directory entries are rejected)", ErrEntryInvalid, f.Name)
		}

		totalUncompressed += f.UncompressedSize64
		if totalUncompressed > uint64(MaxTotalUncompressedBytes) {
			return nil, fmt.Errorf("%w: total uncompressed size exceeds %d bytes", ErrDecompressionLimit, MaxTotalUncompressedBytes)
		}
		if f.CompressedSize64 > 0 {
			ratio := float64(f.UncompressedSize64) / float64(f.CompressedSize64)
			if ratio > MaxDecompressionRatio {
				return nil, fmt.Errorf("%w: entry %q has a decompression ratio of %.1f, exceeding the limit of %.1f", ErrDecompressionLimit, f.Name, ratio, MaxDecompressionRatio)
			}
		}

		entries[f.Name] = f
	}

	manifestFile, ok := entries["manifest.json"]
	if !ok {
		return nil, fmt.Errorf("%w: archive is missing manifest.json", ErrManifestInvalid)
	}
	templateFile, ok := entries[TemplatePath]
	if !ok {
		return nil, fmt.Errorf("%w: archive is missing template.json", ErrManifestInvalid)
	}

	manifestBytes, err := readEntryBounded(manifestFile, MaxManifestBytes)
	if err != nil {
		return nil, err
	}
	manifest, err := decodeManifestStrict(manifestBytes)
	if err != nil {
		return nil, err
	}
	if err := validateManifest(manifest); err != nil {
		return nil, err
	}

	templateBytes, err := readEntryBounded(templateFile, MaxTemplateBytes)
	if err != nil {
		return nil, err
	}

	manifestPaths := make(map[string]ManifestAsset, len(manifest.Assets))
	for _, a := range manifest.Assets {
		manifestPaths[a.Path] = a
	}
	manifestAudioPaths := make(map[string]ManifestAudioAsset, len(manifest.AudioAssets))
	for _, a := range manifest.AudioAssets {
		manifestAudioPaths[a.Path] = a
	}

	// Every real entry outside manifest.json/template.json must be a
	// manifest-declared asset (visual or audio) - "a package should
	// contain exactly the bytes it needs" (docs/visual-template-
	// packages.md §58): no hidden payload file is ever accepted.
	for name := range entries {
		if name == "manifest.json" || name == TemplatePath {
			continue
		}
		_, isAsset := manifestPaths[name]
		_, isAudioAsset := manifestAudioPaths[name]
		if !isAsset && !isAudioAsset {
			return nil, fmt.Errorf("%w: archive entry %q is not referenced by the manifest", ErrAssetUnreferenced, name)
		}
	}
	// Every manifest-declared asset must correspond to a real entry.
	for path := range manifestPaths {
		if _, ok := entries[path]; !ok {
			return nil, fmt.Errorf("%w: manifest references %q, which is not present in the archive", ErrAssetMissing, path)
		}
	}
	for path := range manifestAudioPaths {
		if _, ok := entries[path]; !ok {
			return nil, fmt.Errorf("%w: manifest references %q, which is not present in the archive", ErrAssetMissing, path)
		}
	}

	assets := make([]ValidatedAsset, 0, len(manifest.Assets))
	for _, a := range manifest.Assets {
		f := entries[a.Path]
		declaredKind := visualasset.Kind(a.Kind)
		bound := visualasset.MaxBytesFor(declaredKind)
		if bound == 0 {
			return nil, fmt.Errorf("%w: asset %q has unrecognized kind %q", ErrAssetUnsupported, a.ID, a.Kind)
		}
		if f.UncompressedSize64 > uint64(bound) {
			return nil, fmt.Errorf("%w: asset %q is larger than the %d byte limit for %s assets", ErrTooLarge, a.ID, bound, a.Kind)
		}
		payload, err := readEntryBounded(f, bound)
		if err != nil {
			return nil, err
		}
		if int64(len(payload)) != a.SizeBytes {
			return nil, fmt.Errorf("%w: asset %q declared size %d does not match its actual size %d", visualasset.ErrUnsupported, a.ID, a.SizeBytes, len(payload))
		}
		sum := sha256.Sum256(payload)
		if hex.EncodeToString(sum[:]) != a.SHA256 {
			return nil, fmt.Errorf("%w: asset %q content does not match its declared sha256", ErrAssetHashMismatch, a.ID)
		}
		detected, err := visualasset.VerifyTypeAgreement(payload, "", a.MediaType)
		if err != nil {
			return nil, fmt.Errorf("%w: asset %q: %v", ErrAssetTypeMismatch, a.ID, err)
		}
		kind, _ := detected.KindOf()
		if kind != declaredKind {
			return nil, fmt.Errorf("%w: asset %q declared kind %q does not match its detected content type", ErrAssetTypeMismatch, a.ID, a.Kind)
		}
		if kind == visualasset.KindImage {
			if err := visualasset.ValidateImageDimensions(payload, detected); err != nil {
				return nil, err
			}
		}
		assets = append(assets, ValidatedAsset{Manifest: a, Data: payload, MediaType: detected, Kind: kind})
	}

	audioAssets := make([]ValidatedAudioAsset, 0, len(manifest.AudioAssets))
	for _, a := range manifest.AudioAssets {
		f := entries[a.Path]
		bound := audioasset.MaxBytesFor(audioasset.KindSound)
		if f.UncompressedSize64 > uint64(bound) {
			return nil, fmt.Errorf("%w: audio asset %q is larger than the %d byte limit", ErrTooLarge, a.ID, bound)
		}
		payload, err := readEntryBounded(f, bound)
		if err != nil {
			return nil, err
		}
		if int64(len(payload)) != a.SizeBytes {
			return nil, fmt.Errorf("%w: audio asset %q declared size %d does not match its actual size %d", audioasset.ErrUnsupported, a.ID, a.SizeBytes, len(payload))
		}
		sum := sha256.Sum256(payload)
		if hex.EncodeToString(sum[:]) != a.SHA256 {
			return nil, fmt.Errorf("%w: audio asset %q content does not match its declared sha256", ErrAssetHashMismatch, a.ID)
		}
		// Streamed through the exact same audioasset.VerifyTypeAgreement/
		// ValidateWAV a manual upload uses (docs/alert-audio.md §10.3) -
		// never a separately-implemented package-path validator.
		detected, err := audioasset.VerifyTypeAgreement(payload, "", a.MediaType)
		if err != nil {
			return nil, fmt.Errorf("%w: audio asset %q: %v", ErrAssetTypeMismatch, a.ID, err)
		}
		if _, err := audioasset.ValidateWAV(payload); err != nil {
			return nil, fmt.Errorf("%w: audio asset %q: %v", ErrAssetTypeMismatch, a.ID, err)
		}
		audioAssets = append(audioAssets, ValidatedAudioAsset{Manifest: a, Data: payload, MediaType: detected})
	}

	return &Validated{Manifest: manifest, TemplateJSON: templateBytes, Assets: assets, AudioAssets: audioAssets}, nil
}

// readEntryBounded opens and fully reads one already-validated ZIP entry,
// enforcing max as a hard streaming bound - the entry's own declared
// UncompressedSize64 is never trusted alone (docs/visual-template-
// packages.md §10/§16: "enforce declared sizes AND actual streamed
// sizes... never allocate UncompressedSize64 bytes directly").
func readEntryBounded(f *zip.File, max int64) ([]byte, error) {
	if int64(f.UncompressedSize64) > max {
		return nil, fmt.Errorf("%w: entry %q declares %d bytes, exceeding the %d byte limit", ErrTooLarge, f.Name, f.UncompressedSize64, max)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("%w: open entry %q: %v", ErrInvalidArchive, f.Name, err)
	}
	defer rc.Close()

	limited := io.LimitReader(rc, max+1)
	buf, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("%w: read entry %q: %v", ErrInvalidArchive, f.Name, err)
	}
	if int64(len(buf)) > max {
		return nil, fmt.Errorf("%w: entry %q streamed more than its declared/allowed %d bytes", ErrDecompressionLimit, f.Name, max)
	}
	return buf, nil
}
