// Package audioasset holds Stage 17B's managed persistent audio asset
// domain: the content-addressed, deduplicated, immutable binary blob store
// and the logical asset metadata layered on top of it, backing an alert
// rule's optional persistent sound (docs/alert-audio.md §5).
//
// This package deliberately reuses internal/domain/visualasset's own
// generic *visualasset.FileStore blob-storage primitive directly (a second
// instance, rooted at a sibling directory) rather than duplicating it -
// see docs/alert-audio.md §5.1 for why that primitive is safe and narrow
// enough to share while the two packages' own Kind/MediaType/Asset
// metadata models, validation rules, and Repository ports stay entirely
// independent. This package never imports internal/domain/alerts,
// internal/domain/visualtemplate, or internal/audio - it is a sibling
// domain, resolved against by the httpapi bridge layer and by
// internal/audio's own injected AudioAssetResolver, exactly like
// visualasset is resolved against visualdesign.
//
// This is Stage 17B's own intentionally-accepts-untrusted-binary-input
// domain package (a manual upload or a package-imported sound), so every
// exported validation entry point here treats its input as hostile:
// independent magic-byte signature detection (never trusting a filename
// extension or a caller-declared media type alone), bounded reads, and a
// closed WAV/PCM structural parser - never a general audio decoder, never
// shelling out to FFmpeg/ffprobe.
//
// See docs/alert-audio.md for the full contract this package implements.
package audioasset

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// Kind is the closed set of managed audio asset families Stage 17B
// accepts (docs/alert-audio.md §5.2). There is deliberately only one.
type Kind string

const KindSound Kind = "sound"

func (k Kind) Valid() bool { return k == KindSound }

// MediaType is the closed set of exact media types this package accepts -
// exactly one, per docs/alert-audio.md §2.3's own researched format
// decision.
type MediaType string

const MediaWAV MediaType = "audio/wav"

func (m MediaType) Valid() bool { return m == MediaWAV }

// KindOf returns the Kind an accepted MediaType belongs to - trivial today
// (one media type, one kind) but kept as its own method for the same
// forward-compatible shape visualasset.MediaType.KindOf already has.
func (m MediaType) KindOf() (Kind, bool) {
	if m == MediaWAV {
		return KindSound, true
	}
	return "", false
}

// Blob is one immutable, content-addressed binary payload - never mutated
// once created, only referenced (by one or more Asset rows) or physically
// garbage-collected once unreferenced and safe to remove
// (docs/alert-audio.md §5.4/§5.6).
type Blob struct {
	SHA256      string
	MediaType   MediaType
	ByteSize    int64
	DurationMS  int64
	StorageName string
	PublicToken string
	CreatedAt   time.Time
}

// Asset is one logical managed audio asset - user-supplied metadata
// pointing at an immutable Blob (docs/alert-audio.md §5.2). Two Asset rows
// may share one BlobSHA256 (content dedup) while carrying different
// metadata - they are never silently merged.
type Asset struct {
	ID          string
	BlobSHA256  string
	Kind        Kind
	DisplayName string
	// Source records where this logical asset came from - "upload" (a
	// direct manual upload) or "package" (created during a package v2
	// import, docs/alert-audio.md §10.4). Presentation-only, exactly like
	// visualasset.Asset.Source - never itself a security boundary.
	Source    string
	CreatedAt time.Time
	UpdatedAt time.Time

	// Blob carries the resolved blob this asset points at, populated by
	// the repository/service on read - never required on write (Upload
	// takes the blob bytes directly). Nil is a valid zero value only
	// before the first successful read.
	Blob *Blob
}

const (
	SourceUpload  = "upload"
	SourcePackage = "package"
)

// AssetIDPrefix / NewAssetID: local managed audio asset IDs are always
// server-generated - never accepted as caller input, and never equal to a
// package-supplied logical asset ID (visualpackage's own pkgaudio_ prefix,
// docs/alert-audio.md §10.2, is disjoint by construction). Matches
// visualasset.NewAssetID's own "prefix + 16 random bytes hex" shape, with
// its own distinct prefix so an audio asset ID can never collide with, or
// be confused for, a visual asset ID.
const AssetIDPrefix = "audioasset_"

func NewAssetID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate audio asset id: %w", err)
	}
	return AssetIDPrefix + hex.EncodeToString(buf), nil
}

// NewPublicToken returns a fresh, random, high-entropy public blob token -
// unused by the public route today (Stage 17B's own renderer always
// reaches audio bytes through internal/audio's existing
// /api/public/audio/{slug}/bytes/{token} route instead, docs/alert-
// audio.md §5.2), generated for structural symmetry with visualasset and
// reserved for a possible future direct-serving need.
func NewPublicToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate public token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
