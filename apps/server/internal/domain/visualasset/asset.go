// Package visualasset holds Stage 14B's managed visual asset domain: the
// content-addressed, deduplicated, immutable binary blob store and the
// logical asset metadata layered on top of it, backing the visualdesign
// package's new `image`/`video` layer kinds and optional custom-font
// reference (Stage 14B, docs/visual-template-packages.md §11-§17).
//
// This is the first project domain package that intentionally accepts
// untrusted binary input - a manual upload or a package-imported asset -
// so every exported validation entry point in this package treats its
// input as hostile: independent magic-byte signature detection (never
// trusting a filename extension or a caller-declared media type alone),
// bounded reads, and safe metadata-only decoding (never full-frame image
// decoding, never shelling out to FFmpeg/ffprobe).
//
// Deliberately excludes: audio assets of any kind (Stage 17 owns the
// application's one audio/playback subsystem - see
// docs/visual-template-packages.md §2), SVG/HTML/CSS/JavaScript/
// executable/archive content, and any notion of a remote/URL-based asset
// source. This package never imports internal/domain/alerts,
// internal/domain/chatoverlay, internal/domain/visualtemplate,
// internal/provider/twitch, or any owner-specific package - it is a
// sibling of internal/domain/visualdesign, not a dependent of it, and
// neither package imports the other (a layer's asset reference is
// resolved against this package's own service by the httpapi bridge
// layer, exactly like owner-binding compatibility is resolved outside
// internal/domain/visualtemplate itself).
//
// See docs/visual-template-packages.md for the full contract this
// package implements.
package visualasset

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// Kind is the closed set of managed asset families Stage 14B accepts
// (docs/visual-template-packages.md §11). There is deliberately no
// "audio" kind.
type Kind string

const (
	KindImage Kind = "image"
	KindVideo Kind = "video"
	KindFont  Kind = "font"
)

var validKinds = []Kind{KindImage, KindVideo, KindFont}

func (k Kind) Valid() bool {
	for _, v := range validKinds {
		if k == v {
			return true
		}
	}
	return false
}

// MediaType is the closed set of exact media types this package accepts,
// one per allowed concrete format (docs/visual-template-packages.md §11).
// A caller-declared or extension-derived media type is only ever trusted
// once it agrees with the signature independently detected from the
// asset's own bytes - see validation.go's DetectSignature.
type MediaType string

const (
	MediaPNG   MediaType = "image/png"
	MediaJPEG  MediaType = "image/jpeg"
	MediaGIF   MediaType = "image/gif"
	MediaWebP  MediaType = "image/webp"
	MediaWebM  MediaType = "video/webm"
	MediaMP4   MediaType = "video/mp4"
	MediaWOFF2 MediaType = "font/woff2"
)

// kindForMediaType maps every accepted MediaType to its own Kind - the
// one place that mapping is defined, reused by both signature detection
// and manifest/upload cross-checks.
var kindForMediaType = map[MediaType]Kind{
	MediaPNG:   KindImage,
	MediaJPEG:  KindImage,
	MediaGIF:   KindImage,
	MediaWebP:  KindImage,
	MediaWebM:  KindVideo,
	MediaMP4:   KindVideo,
	MediaWOFF2: KindFont,
}

func (m MediaType) Valid() bool {
	_, ok := kindForMediaType[m]
	return ok
}

// KindOf returns the Kind an accepted MediaType belongs to. Panics-free:
// callers must check Valid() first (mirrors this project's other closed
// enums, which never silently coerce an unrecognized value).
func (m MediaType) KindOf() (Kind, bool) {
	k, ok := kindForMediaType[m]
	return k, ok
}

// Blob is one immutable, content-addressed binary payload - never
// mutated once created, only referenced (by one or more Asset rows) or
// physically garbage-collected once unreferenced and safe to remove
// (docs/visual-template-packages.md §13/§15).
type Blob struct {
	SHA256      string
	MediaType   MediaType
	ByteSize    int64
	StorageName string
	PublicToken string
	CreatedAt   time.Time
}

// Asset is one logical managed visual asset - user/package-supplied
// metadata pointing at an immutable Blob (docs/visual-template-
// packages.md §13). Two Asset rows may share one BlobSHA256 (content
// dedup) while carrying entirely different metadata - they are never
// silently merged.
type Asset struct {
	ID          string
	BlobSHA256  string
	Kind        Kind
	DisplayName string
	Author      string
	License     string
	Notice      string
	// Source records where this logical asset came from - "upload" (a
	// direct manual upload, docs/visual-template-packages.md §17) or
	// "package" (created during a package import,
	// docs/visual-template-packages.md §20). Presentation-only, exactly
	// like visualtemplate.Source - never itself a security boundary.
	Source    string
	CreatedAt time.Time
	UpdatedAt time.Time

	// Blob carries the resolved blob this asset points at, populated by
	// the repository/service on read - never required on write (Create
	// takes the blob bytes directly). Nil is a valid zero value only
	// before the first successful read.
	Blob *Blob
}

const (
	SourceUpload  = "upload"
	SourcePackage = "package"
)

// AssetIDPrefix / NewAssetID: local managed asset IDs are always
// server-generated (docs/visual-template-packages.md §13/§26) - never
// accepted as caller input, and never equal to a package-supplied
// logical asset ID (visualpackage's own pkgasset_ prefix is disjoint by
// construction). Matches visualdesign's own assetRefPattern
// (`^asset_[A-Za-z0-9]{1,64}$`) and visualtemplate.NewTemplateID's own
// "tpl_ + 16 random bytes hex" shape.
const AssetIDPrefix = "asset_"

func NewAssetID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate asset id: %w", err)
	}
	return AssetIDPrefix + hex.EncodeToString(buf), nil
}

// NewPublicToken returns a fresh, random, high-entropy public blob
// token (docs/visual-template-packages.md §18) - 32 random bytes, wider
// than a local asset id, since it is the only credential-shaped value
// this package ever exposes on an unauthenticated public route.
func NewPublicToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate public token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
