// Package metadatapreset holds the Stage 22 reusable stream metadata
// preset domain: a named, reusable bundle of stream CONTENT metadata
// (title, description, tags, language, and provider-scoped category
// data) a creator can apply to one or more configured destinations
// without retyping it every time. See docs/metadata-presets.md for the
// full contract.
//
// A preset is deliberately not a destination/stream-key configuration,
// an OBS profile, a credential bundle, a scheduler, or an auto-
// publisher. It never holds a stream key, an OAuth token, a client
// secret, or any other credential - there is no field shaped to carry
// one. Applying a preset only ever writes local metadata through the
// same validated save path manual editing already uses; publishing to
// a provider remains the existing, separate, explicit action.
package metadatapreset

import (
	"time"

	"github.com/streaming-tree/server/internal/domain/platform"
)

// CommonMetadata mirrors platform.Metadata's own shared, capability-
// gated fields exactly - everything except Category/CategoryID (the
// one genuinely provider-scoped concept, see ProviderMetadata) and
// UpdatedAt (a destination's own, not the preset's).
type CommonMetadata struct {
	Title         string
	Description   string
	Tags          []string
	Language      string
	Visibility    string
	MatureContent bool
	DVR           bool
	LatencyMode   string
}

// ProviderMetadata is a provider-scoped category, captured from - and
// only ever applied to - the exact provider it is keyed under in
// Preset.Providers. A Twitch category ID is never applied to a YouTube
// destination, and vice versa (docs/metadata-presets.md §1's own real
// schema audit: platform.ProviderDefinition.CategoryRequiresRemoteID is
// true for Twitch and YouTube, so a bare category string with no
// matching ID from a different provider is a publish blocker, not a
// harmless approximation).
type ProviderMetadata struct {
	Category   string
	CategoryID string
}

// Preset is the persisted, named, reusable metadata bundle.
type Preset struct {
	ID   string
	Name string
	// Note is the preset's own optional short annotation (e.g. "for
	// Just Chatting streams") - never the stream description itself,
	// which lives in Common.Description.
	Note      string
	Common    CommonMetadata
	Providers map[platform.ProviderID]ProviderMetadata
	CreatedAt time.Time
	UpdatedAt time.Time
}

// CreateInput is the accepted payload for creating a new preset.
type CreateInput struct {
	Name      string
	Note      string
	Common    CommonMetadata
	Providers map[platform.ProviderID]ProviderMetadata
}

// UpdateInput is a full replacement of a preset's mutable fields.
type UpdateInput struct {
	Name      string
	Note      string
	Common    CommonMetadata
	Providers map[platform.ProviderID]ProviderMetadata
}
