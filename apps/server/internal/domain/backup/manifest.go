package backup

import "time"

// Extension is the file extension a backup is saved/opened with.
const Extension = ".streaming-tree-backup"

// ConfigPath/ManifestPath are the two fixed, always-present archive
// entries every backup package has - never manifest-declared, never
// user-supplied, checked by exact name.
const (
	ManifestPath = "manifest.json"
	ConfigPath   = "config.json"
)

// Archive bounds. Deliberately larger than
// internal/domain/visualpackage's own template-package bounds (a
// backup aggregates every domain this application persists, not one
// template's own small asset set) - see docs/backup-restore.md §5.
// The SAFETY LOGIC these bounds are checked with is shared
// (internal/archivesafety); the numbers themselves are not.
const (
	MaxPackageBytes           int64   = 512 << 20 // 512 MiB
	MaxTotalUncompressedBytes int64   = 768 << 20 // 768 MiB
	MaxArchiveEntries                 = 20_000
	MaxManifestBytes          int64   = 256 << 10 // 256 KiB
	MaxConfigBytes            int64   = 64 << 20  // 64 MiB
	MaxDecompressionRatio     float64 = 100.0
)

// ManifestAsset is one archive asset entry's manifest record - present
// for both visual and audio blobs, distinguished by Path's own root
// ("assets/" or "audio/").
type ManifestAsset struct {
	SHA256    string `json:"sha256"`
	Path      string `json:"path"`
	SizeBytes int64  `json:"sizeBytes"`
}

// Manifest is a backup package's own top-level, always-first-validated
// entry (docs/backup-restore.md §5).
type Manifest struct {
	// FormatVersion is this package's own logical-payload format
	// version - see model.go's own FormatVersion constant.
	FormatVersion int `json:"formatVersion"`
	// Product is a fixed, never-user-editable identity string - a
	// package from an unrelated application is rejected the moment
	// this does not match, before anything else is parsed.
	Product          string          `json:"product"`
	CreatedAt        time.Time       `json:"createdAt"`
	SourceAppVersion string          `json:"sourceAppVersion"`
	SourcePlatform   string          `json:"sourcePlatform"`
	ConfigSHA256     string          `json:"configSha256"`
	ConfigSizeBytes  int64           `json:"configSizeBytes"`
	Assets           []ManifestAsset `json:"assets"`
}
