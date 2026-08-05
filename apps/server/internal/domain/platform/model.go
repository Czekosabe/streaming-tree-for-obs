// Package platform holds the platform configuration domain: built-in provider
// definitions, user-configured destination branches and their stream metadata.
//
// Three concepts are kept strictly separate:
//
//   - A ProviderDefinition describes a built-in integration type (Twitch,
//     YouTube, ...). It is compiled into the binary, never stored in the
//     database and never created or deleted by a user.
//   - A Platform is a destination branch the user configured. It references a
//     provider and carries configuration only.
//   - Runtime state (offline/starting/live, viewer counts, FFmpeg processes) is
//     deliberately absent. No streaming engine exists yet, and runtime state
//     must not be persisted in the configuration tables.
//
// Nothing in this package stores stream keys, tokens or any other credential.
package platform

import "time"

// ProviderID identifies a built-in integration type.
type ProviderID string

const (
	ProviderTwitch  ProviderID = "twitch"
	ProviderYouTube ProviderID = "youtube"
	ProviderKick    ProviderID = "kick"
	ProviderTikTok  ProviderID = "tiktok"
)

// CategoryFieldType names the concept a provider uses for its category-like
// field. It is a semantic identifier, never display text: the frontend maps it
// to a localized label.
type CategoryFieldType string

const (
	CategoryFieldCategory CategoryFieldType = "category"
	CategoryFieldTopic    CategoryFieldType = "topic"
)

// Visibility option identifiers.
const (
	VisibilityPublic   = "public"
	VisibilityUnlisted = "unlisted"
	VisibilityPrivate  = "private"
)

// Latency option identifiers.
const (
	LatencyNormal   = "normal"
	LatencyLow      = "low"
	LatencyUltraLow = "ultra-low"
)

// Capabilities records which metadata fields a provider exposes.
//
// A false value means the provider has no equivalent concept at all, so the
// field must not be rendered and must not be submitted with a meaningful value.
type Capabilities struct {
	Title         bool `json:"title"`
	Description   bool `json:"description"`
	Category      bool `json:"category"`
	Tags          bool `json:"tags"`
	Language      bool `json:"language"`
	Visibility    bool `json:"visibility"`
	MatureContent bool `json:"matureContent"`
	DVR           bool `json:"dvr"`
	LatencyMode   bool `json:"latencyMode"`
}

// Limits records the size constraints a provider applies to metadata.
type Limits struct {
	TitleMaxLength       int `json:"titleMaxLength"`
	DescriptionMaxLength int `json:"descriptionMaxLength"`
	// DescriptionMaxLengthInBytes means DescriptionMaxLength counts UTF-8
	// bytes rather than runes - true for YouTube, whose videos.snippet.
	// description limit is documented as 5000 bytes, not 5000 characters
	// (docs/provider-integrations/youtube.md). Every other length in this
	// struct, for every provider, counts runes.
	DescriptionMaxLengthInBytes bool `json:"descriptionMaxLengthInBytes"`
	MaxTags                     int  `json:"maxTags"`
	TagMaxLength                int  `json:"tagMaxLength"`
	// TagsCombinedMaxLength, when non-zero, additionally bounds the total
	// UTF-8 byte length of every tag combined (separators included) -
	// YouTube's videos.snippet.tags limit (500 bytes total,
	// docs/provider-integrations/youtube.md) is a budget across all tags
	// together, not a per-tag or per-count limit the way Twitch's is. Zero
	// means this combined check does not apply.
	TagsCombinedMaxLength int `json:"tagsCombinedMaxLength"`
}

// ProviderDefinition is the built-in description of an integration type.
//
// Every option list holds stable semantic identifiers ("public", "ultra-low"),
// never localized labels. BrandName is the only human-readable string and is a
// proper noun that stays identical in every language.
type ProviderDefinition struct {
	ID         ProviderID `json:"id"`
	BrandName  string     `json:"brandName"`
	ShortLabel string     `json:"shortLabel"`

	CategoryFieldType CategoryFieldType `json:"categoryFieldType"`
	// CategoryRequiresRemoteID means this provider's category field cannot
	// be published from display text alone - a category selected through
	// the provider's own search must supply CategoryID too, and free-typed
	// or stale category text with no ID is a publish blocker rather than a
	// best-effort guess. False for a provider that has no metadata-publish
	// integration at all yet, or whose category concept is display-only.
	CategoryRequiresRemoteID bool         `json:"categoryRequiresRemoteId"`
	Capabilities             Capabilities `json:"capabilities"`
	Limits                   Limits       `json:"limits"`

	VisibilityOptions []string `json:"visibilityOptions"`
	LatencyOptions    []string `json:"latencyOptions"`
	LanguageOptions   []string `json:"languageOptions"`
}

// Metadata is the stream metadata stored for one configured platform.
//
// Fields the provider does not support are stored as SQL NULL and surface here
// as their zero value. The API always emits the whole object; the frontend
// renders only the fields the provider's capability table enables.
//
// Every value here is authored by the user and is stored exactly as entered -
// it is never translated, normalised beyond trimming, or interpreted.
type Metadata struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Category    string `json:"category"`
	// CategoryID is the provider's own stable remote identifier for
	// Category (a Twitch game/category ID, for instance) - distinct from
	// Category, which is display text a user may type freely. Empty means
	// no remote category has been selected, even when Category holds text:
	// stale, ID-less text is a publish blocker, never guessed at, per
	// docs/provider-integrations/twitch.md.
	CategoryID    string    `json:"categoryId"`
	Tags          []string  `json:"tags"`
	Language      string    `json:"language"`
	Visibility    string    `json:"visibility"`
	MatureContent bool      `json:"matureContent"`
	DVR           bool      `json:"dvr"`
	LatencyMode   string    `json:"latencyMode"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// Platform is a destination branch configured by the user.
//
// ID is generated by the backend and is not derived from the provider, because
// a user may configure several destinations for the same provider.
type Platform struct {
	ID          string     `json:"id"`
	ProviderID  ProviderID `json:"providerId"`
	DisplayName string     `json:"displayName"`
	Enabled     bool       `json:"enabled"`
	SortOrder   int        `json:"sortOrder"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	Metadata    Metadata   `json:"metadata"`
}

// CreateInput is the accepted payload for configuring a new platform.
//
// There is deliberately no field for a stream key or any other credential.
type CreateInput struct {
	ProviderID  ProviderID
	DisplayName string
	Enabled     bool
	// SortOrder is optional; when nil the service appends to the end.
	SortOrder *int
}

// UpdateInput is a full replacement of the mutable configuration fields.
type UpdateInput struct {
	DisplayName string
	Enabled     bool
	SortOrder   int
}
