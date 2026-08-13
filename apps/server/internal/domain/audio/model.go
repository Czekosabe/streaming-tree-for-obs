// Package audio holds the Stage 17A global text-to-speech/audio
// configuration - the persisted settings singleton for the shared audio
// runtime (internal/audio) and its provider abstraction
// (internal/provider/tts). See docs/audio-tts.md for the full researched
// contract this package implements.
//
// Deliberately minimal, mirroring internal/domain/operatorchatprefs's own
// reasoning: this package holds exactly one configuration row, never a
// queue, a cooldown, a generated audio byte, or any user-authored text.
// Everything runtime-shaped lives only in internal/audio's own in-memory
// manager and is gone on restart - see docs/audio-tts.md §6/§12 for the
// exhaustive persisted-vs-never-persisted list.
package audio

import (
	"time"

	engagement "github.com/streaming-tree/server/internal/domain/engagement"
)

// ProviderMode selects which TTS provider family Settings.Enabled speech
// uses. Deliberately its own type, never shared with
// internal/provider/tts's own Provider identifiers - a mode is a
// persisted operator choice, a Provider is a runtime implementation.
type ProviderMode string

const (
	// ProviderModeDisabled means no speech is ever synthesized - the
	// default, and the only mode guaranteed to work on every platform.
	ProviderModeDisabled ProviderMode = "disabled"
	// ProviderModeSystem uses the local operating system's own installed
	// TTS engine - real on Windows (docs/audio-tts.md §2), honestly
	// unavailable elsewhere (§3).
	ProviderModeSystem ProviderMode = "system"
	// ProviderModeLocal names a future local neural voice engine - not
	// implemented in Stage 17A (needs its own licensing/model review),
	// and rejected by ValidateSettings until it is.
	ProviderModeLocal ProviderMode = "local"
	// ProviderModeCloud names a future cloud TTS provider - not
	// implemented in Stage 17A (needs its own credentials/privacy/network
	// contract), and rejected by ValidateSettings until it is.
	ProviderModeCloud ProviderMode = "cloud"
)

// KnownProviderModes lists every mode this package's type system
// recognizes at all - matches the architecture this stage was originally
// planned around (disabled/system/local/cloud). See
// ImplementedProviderModes for the subset Stage 17A actually accepts on
// save.
var KnownProviderModes = []ProviderMode{ProviderModeDisabled, ProviderModeSystem, ProviderModeLocal, ProviderModeCloud}

// ImplementedProviderModes are the only modes ValidateSettings currently
// accepts - see docs/audio-tts.md §4: Stage 17A never permits saving
// "local" or "cloud" merely because the enum names them.
var ImplementedProviderModes = []ProviderMode{ProviderModeDisabled, ProviderModeSystem}

func (m ProviderMode) known() bool {
	for _, k := range KnownProviderModes {
		if m == k {
			return true
		}
	}
	return false
}

func (m ProviderMode) implemented() bool {
	for _, k := range ImplementedProviderModes {
		if m == k {
			return true
		}
	}
	return false
}

// Settings is the singleton, global Stage 17A audio/TTS configuration.
// Persisted in full on every save (a full-replacement PUT, mirroring
// operatorchatprefs.Preferences's own contract), never a partial patch.
type Settings struct {
	Enabled      bool
	ProviderMode ProviderMode

	// EnabledEventTypes is the closed set of engagement.Type values TTS
	// may speak. Empty means none - Stage 17A is opt-in per event type,
	// never "everything by default" (docs/audio-tts.md §9/§41's own
	// "chat is opt-in, not spoken on first install" requirement).
	EnabledEventTypes []engagement.Type
	// EnabledProviderIDs is the closed set of engagement.ProviderID
	// values TTS may speak from. Empty means every provider - mirrors
	// internal/domain/alerts.Rule.Providers's own "empty = any" contract.
	EnabledProviderIDs []engagement.ProviderID
	// EnabledSourceIDs optionally narrows further to specific connected
	// accounts and/or donation sources (both id spaces accepted
	// interchangeably, exactly like internal/domain/alerts.Rule.Accounts
	// already does via its own combined AccountLookupAdapter - see
	// internal/alerts/wiring.go). Empty means every source.
	EnabledSourceIDs []string

	// SupporterOnlyMode, when true, restricts eligible events to the
	// closed supporter-family set (see internal/audio's own capability
	// table) regardless of EnabledEventTypes - see docs/audio-tts.md §9.
	SupporterOnlyMode bool

	// ThresholdCurrency/ThresholdMinimumAmountMicros mirror
	// internal/domain/alerts.Rule's own exact-currency threshold model:
	// empty currency means no monetary threshold configured; a threshold
	// is never compared across currencies (docs/audio-tts.md §8).
	ThresholdCurrency            string
	ThresholdMinimumAmountMicros *int64

	// MinimumBits is a plain integer threshold against
	// engagement.Event.Quantity for TypeBits - never a Money value
	// (Bits are not currently normalized as money anywhere in this
	// codebase).
	MinimumBits *int64

	MaxTextLengthCodePoints int
	PerUserCooldownSeconds  int
	GlobalCooldownSeconds   int
	BlockedWords            []string
	RemoveURLs              bool
	NormalizeRepeatedChars  bool
	SuppressCommands        bool

	QueueCapacity  int
	ManualApproval bool

	// VoiceID is the provider's own stable voice identifier, never a
	// localized display name - empty means "system default voice",
	// itself a distinct, explicit concept (docs/audio-tts.md §10.4,
	// governing task §49).
	VoiceID  string
	Language string
	// Speed/Volume are provider-independent, canonical app ranges - see
	// SpeedBounds/VolumeBounds. The provider adapter translates these
	// into whatever native range its own engine uses.
	Speed  float64
	Volume float64

	// PublicSlug is the unguessable locator for the public Browser
	// Source audio route (/overlay/audio/{PublicSlug}) - generated by
	// NewPublicSlug, rotatable, never a credential. Empty only for a
	// Settings value that has never been saved yet (Default's own
	// zero-value before the service assigns one on first write).
	PublicSlug string

	CreatedAt time.Time
	UpdatedAt time.Time
}

// Default returns the documented out-of-the-box settings: disabled,
// provider mode disabled, no event types enabled, safe preprocessing
// defaults on, manual approval off, normal speed/volume. PublicSlug is
// left empty here - Service.Get generates and persists one the first
// time settings are ever read, exactly once, never regenerated
// implicitly afterward.
func Default() Settings {
	return Settings{
		Enabled:                 false,
		ProviderMode:            ProviderModeDisabled,
		EnabledEventTypes:       nil,
		EnabledProviderIDs:      nil,
		EnabledSourceIDs:        nil,
		SupporterOnlyMode:       false,
		MaxTextLengthCodePoints: DefaultMaxTextLengthCodePoints,
		PerUserCooldownSeconds:  DefaultPerUserCooldownSeconds,
		GlobalCooldownSeconds:   DefaultGlobalCooldownSeconds,
		BlockedWords:            nil,
		RemoveURLs:              true,
		NormalizeRepeatedChars:  true,
		SuppressCommands:        true,
		QueueCapacity:           DefaultQueueCapacity,
		ManualApproval:          false,
		VoiceID:                 "",
		Language:                "",
		Speed:                   1.0,
		Volume:                  1.0,
	}
}
