package backup

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/streaming-tree/server/internal/archivesafety"
)

// ValidatedAsset is one archive asset entry that has passed manifest
// cross-reference and hash agreement - mirrors
// internal/domain/visualpackage's own ValidatedAsset shape exactly.
type ValidatedAsset struct {
	Manifest ManifestAsset
	Data     []byte
}

// Validated is the fully-checked result of ReadArchive - nothing has
// been written to disk or to any database by this point (docs/backup-
// restore.md §5's "never blind extraction" pipeline, reused from
// visualpackage's own).
type Validated struct {
	Manifest Manifest
	Config   Config
	Assets   []ValidatedAsset
}

// ReadArchive validates data as a complete `.streaming-tree-backup`
// package: archive-level bounds, every entry's path grammar, that the
// manifest's own declared Product/FormatVersion are ones this build
// accepts, that the archive's real entries and the manifest's declared
// assets correspond exactly, and that every asset's actual SHA-256
// matches its manifest entry. data's own untrusted archive-entry names
// are used only as lookup keys against this already-validated
// allowed-entry set - never joined to a real filesystem path.
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
		if err := validateBackupEntryPath(f.Name); err != nil {
			return nil, err
		}
		norm := archivesafety.NormalizePath(f.Name)
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
		if err := archivesafety.CheckDecompressionRatio(f.UncompressedSize64, f.CompressedSize64, MaxDecompressionRatio); err != nil {
			return nil, fmt.Errorf("%w: entry %q: %v", ErrDecompressionLimit, f.Name, err)
		}

		entries[f.Name] = f
	}

	manifestFile, ok := entries[ManifestPath]
	if !ok {
		return nil, fmt.Errorf("%w: archive is missing %s", ErrManifestInvalid, ManifestPath)
	}
	configFile, ok := entries[ConfigPath]
	if !ok {
		return nil, fmt.Errorf("%w: archive is missing %s", ErrManifestInvalid, ConfigPath)
	}

	manifestBytes, err := archivesafety.ReadEntryBounded(manifestFile, MaxManifestBytes)
	if err != nil {
		return nil, err
	}
	var manifest Manifest
	dec := json.NewDecoder(bytes.NewReader(manifestBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("%w: decode manifest: %v", ErrManifestInvalid, err)
	}
	if manifest.Product != Product {
		return nil, fmt.Errorf("%w: manifest product %q", ErrProductMismatch, manifest.Product)
	}
	if manifest.FormatVersion != FormatVersion {
		return nil, fmt.Errorf("%w: manifest format version %d, this build supports %d", ErrVersionUnsupported, manifest.FormatVersion, FormatVersion)
	}

	configBytes, err := archivesafety.ReadEntryBounded(configFile, MaxConfigBytes)
	if err != nil {
		return nil, err
	}
	configSum := sha256.Sum256(configBytes)
	if hex.EncodeToString(configSum[:]) != manifest.ConfigSHA256 {
		return nil, fmt.Errorf("%w: config content does not match its declared sha256", ErrAssetHashMismatch)
	}

	var cfg Config
	cdec := json.NewDecoder(bytes.NewReader(configBytes))
	cdec.DisallowUnknownFields()
	if err := cdec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("%w: decode config: %v", ErrConfigInvalid, err)
	}

	manifestPaths := make(map[string]ManifestAsset, len(manifest.Assets))
	for _, a := range manifest.Assets {
		manifestPaths[a.Path] = a
	}

	for name := range entries {
		if name == ManifestPath || name == ConfigPath {
			continue
		}
		if _, ok := manifestPaths[name]; !ok {
			return nil, fmt.Errorf("%w: archive entry %q is not referenced by the manifest", ErrAssetUnreferenced, name)
		}
	}
	for path := range manifestPaths {
		if _, ok := entries[path]; !ok {
			return nil, fmt.Errorf("%w: manifest references %q, which is not present in the archive", ErrAssetMissing, path)
		}
	}

	assets := make([]ValidatedAsset, 0, len(manifest.Assets))
	for _, a := range manifest.Assets {
		f := entries[a.Path]
		if f.UncompressedSize64 > uint64(MaxAssetBytes) {
			return nil, fmt.Errorf("%w: asset %q is larger than the %d byte limit", ErrTooLarge, a.SHA256, MaxAssetBytes)
		}
		payload, err := archivesafety.ReadEntryBounded(f, MaxAssetBytes)
		if err != nil {
			return nil, err
		}
		if int64(len(payload)) != a.SizeBytes {
			return nil, fmt.Errorf("%w: asset %q declared size %d does not match its actual size %d", ErrConfigInvalid, a.SHA256, a.SizeBytes, len(payload))
		}
		sum := sha256.Sum256(payload)
		if hex.EncodeToString(sum[:]) != a.SHA256 {
			return nil, fmt.Errorf("%w: asset %q content does not match its declared sha256", ErrAssetHashMismatch, a.SHA256)
		}
		assets = append(assets, ValidatedAsset{Manifest: a, Data: payload})
	}

	return &Validated{Manifest: manifest, Config: cfg, Assets: assets}, nil
}

// MaxAssetBytes bounds one individual asset entry - generous enough
// for the largest media this application's own visualasset/audioasset
// domains already accept (docs/visual-template-packages.md's own
// per-kind bounds), never larger than the whole-package bound.
const MaxAssetBytes = 128 << 20 // 128 MiB

// validateBackupEntryPath is docs/backup-restore.md §5's own path
// grammar: exactly manifest.json/config.json at the root, or a
// "assets/<sha256>"/"audio/<sha256>" entry whose own segment is
// literally a 64-character lowercase hex SHA-256 - a strictly
// narrower, content-addressed grammar than visualpackage's own bounded
// ASCII filenames, since every asset entry name here IS the asset's
// own hash by construction.
func validateBackupEntryPath(name string) error {
	if err := archivesafety.ValidateNoTraversal(name); err != nil {
		return fmt.Errorf("%w: %v", ErrEntryInvalid, err)
	}

	switch name {
	case ManifestPath, ConfigPath:
		return nil
	}

	segments := strings.Split(name, "/")
	if len(segments) != 2 {
		return fmt.Errorf("%w: entry %q is outside the allowed root structure", ErrEntryInvalid, name)
	}
	root, seg := segments[0], segments[1]
	if root != "assets" && root != "audio" {
		return fmt.Errorf("%w: entry %q is outside the allowed root structure", ErrEntryInvalid, name)
	}
	if !isSHA256Hex(seg) {
		return fmt.Errorf("%w: entry %q's own filename is not a 64-character lowercase hex sha256", ErrEntryInvalid, name)
	}
	return nil
}

func isSHA256Hex(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}
