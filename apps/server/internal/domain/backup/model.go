// Package backup implements Stage 23 safe configuration backup/restore
// (docs/backup-restore.md). A backup is a versioned logical export of
// every genuinely portable, non-secret domain this application
// persists, packaged as a bounded, validated zip archive alongside the
// managed visual/audio assets that configuration references.
//
// What a backup never contains, by construction: a destination stream
// key, an OAuth token bundle, a donation-source credential, the
// administrator password verifier, the remote-ingest publisher
// verifier, or any remote-overlay capability token. This package's own
// Config type has no field shaped to carry any of them - see
// docs/backup-restore.md §1/§2/§3 for the full audit this design is
// based on, and security_test.go for the structural proof.
package backup

import (
	"github.com/streaming-tree/server/internal/domain/account"
	"github.com/streaming-tree/server/internal/domain/alerts"
	"github.com/streaming-tree/server/internal/domain/audio"
	"github.com/streaming-tree/server/internal/domain/audioasset"
	"github.com/streaming-tree/server/internal/domain/chatautomation"
	"github.com/streaming-tree/server/internal/domain/chatoverlay"
	"github.com/streaming-tree/server/internal/domain/donationsource"
	"github.com/streaming-tree/server/internal/domain/goals"
	"github.com/streaming-tree/server/internal/domain/metadatapreset"
	"github.com/streaming-tree/server/internal/domain/operatorchatprefs"
	"github.com/streaming-tree/server/internal/domain/output"
	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/domain/remotetarget"
	"github.com/streaming-tree/server/internal/domain/streamsetup"
	"github.com/streaming-tree/server/internal/domain/updatersettings"
	"github.com/streaming-tree/server/internal/domain/visualasset"
	"github.com/streaming-tree/server/internal/domain/visualdesign"
	"github.com/streaming-tree/server/internal/domain/visualtemplate"
)

// FormatVersion is this package's own logical-payload format version,
// distinct from the application's schema-migration number - see
// docs/backup-restore.md §12 on the two being separate concepts.
const FormatVersion = 1

// Product identifies the application that produced a package - a
// fixed, never-user-editable string, checked on restore so a package
// from an unrelated application is rejected before anything else is
// even parsed.
const Product = "streaming-tree-for-obs-backup"

// PlatformExport is one configured destination's portable
// configuration: identity/display fields, its stream metadata, its
// non-secret output (server URL) settings, and (YouTube destinations
// only, and only once one has actually been selected) its remote
// broadcast target. Deliberately excludes the stream key itself
// (internal/secrets, never read by this package).
//
// RemoteTarget carries no token, stream key, or ingestion field
// (internal/domain/remotetarget's own doc comment) - only which remote
// resource (a YouTube live broadcast id, today) this destination's
// metadata reads from and publishes to. Its ResourceID is an external
// provider identifier, never a backup-local id, so restore preserves it
// verbatim rather than remapping it: docs/backup-restore.md §1 already
// documents the accepted risk plainly - "a stale resource_id simply
// fails to resolve on next use, same as it would today if the
// broadcast ended". Nil when the destination never had one (most
// destinations, and every non-YouTube one).
type PlatformExport struct {
	Platform     platform.Platform    `json:"platform"`
	Output       output.Settings      `json:"output"`
	RemoteTarget *remotetarget.Target `json:"remoteTarget,omitempty"`
}

// ConnectedAccountExport is one connected account's portable identity.
// Deliberately excludes its OAuth token bundle (internal/secrets,
// never read by this package) - Status is always restored as
// account.StatusReconnectRequired regardless of what is recorded here
// (docs/backup-restore.md §8).
type ConnectedAccountExport struct {
	Account account.Account `json:"account"`
	// PlatformLinks are the destination platform ids (backup-local,
	// remapped like everything else) this account was linked to.
	PlatformLinks []string `json:"platformLinks"`
	// YouTubeRegion is the optional per-account category-region
	// override (migration 0008), empty when never set.
	YouTubeRegion string `json:"youTubeRegion,omitempty"`
	// EngagementEnabled mirrors connected_account_engagement_settings -
	// nil when no explicit preference was ever recorded (absent-row-
	// means-default, matching the table's own convention).
	EngagementEnabled *bool `json:"engagementEnabled,omitempty"`
}

// ChatOverlayExport composes one chat-overlay profile with its own
// child rows - account selection, hidden users, blocked terms, and
// visible activity types - all remapped/preserved per
// docs/backup-restore.md §1.
type ChatOverlayExport struct {
	Profile       chatoverlay.Profile       `json:"profile"`
	AccountIDs    []string                  `json:"accountIds"`
	HiddenUsers   []chatoverlay.HiddenUser  `json:"hiddenUsers"`
	BlockedTerms  []chatoverlay.BlockedTerm `json:"blockedTerms"`
	ActivityTypes []string                  `json:"activityTypes"`
}

// AlertProfileExport composes one alert-output profile with every
// rule that belongs to it.
type AlertProfileExport struct {
	Profile alerts.Profile `json:"profile"`
	Rules   []alerts.Rule  `json:"rules"`
}

// OperatorChatPreferencesExport composes the singleton preference row
// with its own account-scoped child lists.
type OperatorChatPreferencesExport struct {
	Preferences       operatorchatprefs.Preferences         `json:"preferences"`
	AccountVisibility []operatorchatprefs.AccountVisibility `json:"accountVisibility"`
	HiddenUsers       []operatorchatprefs.UserRef           `json:"hiddenUsers"`
	BotUsers          []operatorchatprefs.UserRef           `json:"botUsers"`
}

// VisualDesignExport is one visual-design record together with the
// backup-local owner it belongs to - OwnerKind/OwnerID inside
// visualdesign.Record are remapped at restore time exactly like every
// other cross-domain reference (docs/backup-restore.md §4).
type VisualDesignExport = visualdesign.Record

// Config is the complete versioned logical payload of one backup - the
// `config.json` archive entry. Every field here corresponds to a "Yes"
// row in docs/backup-restore.md §1's durable-state inventory; nothing
// else is included. Field order mirrors that table.
type Config struct {
	FormatVersion int `json:"formatVersion"`

	Platforms                   []PlatformExport               `json:"platforms"`
	ProviderIntegrationSettings []account.IntegrationSettings  `json:"providerIntegrationSettings"`
	ConnectedAccounts           []ConnectedAccountExport       `json:"connectedAccounts"`
	OperatorChatPreferences     *OperatorChatPreferencesExport `json:"operatorChatPreferences,omitempty"`
	ChatOverlays                []ChatOverlayExport            `json:"chatOverlays"`
	ChatSchedules               []chatautomation.Schedule      `json:"chatSchedules"`
	ChatCommands                []chatautomation.Command       `json:"chatCommands"`
	AlertProfiles               []AlertProfileExport           `json:"alertProfiles"`
	VisualDesigns               []VisualDesignExport           `json:"visualDesigns"`
	VisualTemplates             []visualtemplate.Template      `json:"visualTemplates"`
	VisualAssets                []visualasset.Asset            `json:"visualAssets"`
	AudioAssets                 []audioasset.Asset             `json:"audioAssets"`
	AudioSettings               *audio.Settings                `json:"audioSettings,omitempty"`
	Goals                       []goals.Goal                   `json:"goals"`
	WidgetProfiles              []goals.WidgetProfile          `json:"widgetProfiles"`
	MetadataPresets             []metadatapreset.Preset        `json:"metadataPresets"`
	// StreamSetupProfiles are Stage 25 reusable stream setup profiles
	// (docs/stream-setup-profiles.md §8) - ordinary non-secret creator
	// configuration, restored after Platforms and MetadataPresets so
	// both a profile's destination membership and its metadata-preset
	// reference can be remapped to their own freshly restored ids.
	StreamSetupProfiles []streamsetup.Profile        `json:"streamSetupProfiles"`
	DonationSources     []donationsource.Source      `json:"donationSources"`
	UpdatePreferences   *updatersettings.Preferences `json:"updatePreferences,omitempty"`
	// StreamSessionRetentionDays is the persisted stream-session-
	// history retention preference in days (docs/stream-session-
	// history.md §6, `stream_session_settings`) - nil when never
	// explicitly set (the operational default applies). This is a
	// PREFERENCE about how long to keep history, never the history
	// itself: stream_sessions/stream_session_destinations are
	// deliberately excluded from every backup (history/observability,
	// not configuration).
	StreamSessionRetentionDays *int `json:"streamSessionRetentionDays,omitempty"`
}
