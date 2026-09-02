// Package preflight is Stage 26's readiness-aggregation domain
// (docs/stream-preflight.md): "Is this setup actually ready to
// stream?", answered by composing state every other domain already
// tracks - never a parallel truth for any of them, and never a fake
// score. It never starts, stops, or configures anything itself.
package preflight

// Severity classifies one Finding.
type Severity string

const (
	// SeverityBlocker means the intended local stream operation would
	// not actually work (docs/stream-preflight.md §2).
	SeverityBlocker Severity = "blocker"
	// SeverityWarning means it may work, but needs attention -
	// optional metadata/engagement readiness, never video routing.
	SeverityWarning Severity = "warning"
)

// ActionCode names an existing corrective route/action a Finding can
// point at - preflight never duplicates a configuration form of its
// own (docs/stream-preflight.md §2).
type ActionCode string

const (
	ActionOpenDestinationSettings ActionCode = "open_destination_settings"
	ActionAddStreamKey            ActionCode = "add_stream_key"
	ActionInstallFFmpeg           ActionCode = "install_ffmpeg"
	ActionStartMediaMTX           ActionCode = "start_mediamtx"
	ActionReconnectAccount        ActionCode = "reconnect_account"
	ActionFixMetadata             ActionCode = "fix_metadata"
	ActionRepairSetupProfile      ActionCode = "repair_setup_profile"
)

// Action is a pointer at an existing corrective action. PlatformID is
// "" when the action is not scoped to one destination (e.g.
// installing FFmpeg).
type Action struct {
	Code       ActionCode
	PlatformID string
}

// Finding is one concrete, actionable readiness fact - never a
// summary or a score.
type Finding struct {
	// Code is a stable identifier. For a SeverityBlocker finding this
	// is exactly one of branch.Blocker* (docs/stream-preflight.md
	// §1.1) - reused verbatim, never re-encoded.
	Code     string
	Severity Severity
	// PlatformID is "" for a finding not scoped to one destination
	// (e.g. a Stage 25 setup-profile reference problem).
	PlatformID string
	// Action is nil when there is genuinely nothing to click (e.g.
	// "ingest not receiving" - the operator must start OBS itself;
	// "credential store unavailable" - an OS-level condition).
	Action *Action
}

// DestinationReadiness is one destination's own findings.
type DestinationReadiness struct {
	PlatformID  string
	ProviderID  string
	DisplayName string
	Findings    []Finding
}

// Status is the deterministic overall readiness verdict - never a
// percentage or a gamified score (docs/stream-preflight.md §0/§2).
type Status string

const (
	StatusReady             Status = "ready"
	StatusReadyWithWarnings Status = "ready_with_warnings"
	StatusNotReady          Status = "not_ready"
)

// Report is the full result of one Evaluate call.
type Report struct {
	Status Status
	// Findings is every finding this report produced, flattened
	// (destination-scoped findings appear here too, for a caller that
	// wants the flat list rather than walking Destinations).
	Findings     []Finding
	Destinations []DestinationReadiness
	// SelectedProfileID is the Stage 25 profile this report evaluated,
	// nil when the currently-enabled destination set was evaluated
	// instead (docs/stream-preflight.md §3).
	SelectedProfileID *string
	// StreamingActive mirrors updater.StreamingActive(snapshots) -
	// the frontend swaps this view to "unavailable while streaming"
	// rather than presenting a confusing status for a destination
	// that is, in fact, already live (docs/stream-preflight.md §7).
	StreamingActive bool
}
