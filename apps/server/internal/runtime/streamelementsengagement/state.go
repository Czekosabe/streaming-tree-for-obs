// Package streamelementsengagement supervises one StreamElements Astro
// WebSocket connector per enabled donation source - the Stage 16A inbound
// engagement connector. See docs/provider-integrations/
// external-donations.md for the researched Astro contract this package
// implements, and internal/runtime/youtubeengagement for the closer of the
// two existing connector-manager analogues this package's lifecycle/state
// shape deliberately mirrors (both: one goroutine per enabled source,
// bounded exponential backoff on transient failure, an explicit Snapshot
// never carrying a credential/token).
//
// Unlike internal/runtime/account-backed connectors, a donation source has
// no OAuth token to refresh and no linked streaming destination (see
// internal/domain/donationsource's own package doc comment) - so this
// package has no destinationLookup/broadcastLookup equivalent and no
// WithFreshToken-style retry; its own credential is a single opaque JWT
// loaded once per connection attempt from SecretStore.
package streamelementsengagement

import "time"

// State is a connector's explicit lifecycle state. Mutually exclusive.
// Matches docs/provider-integrations/external-donations.md §33's
// operator-facing state list exactly.
type State string

const (
	// StateDisabled means the source's Enabled flag is off.
	StateDisabled State = "disabled"
	// StateConnecting means a WebSocket dial/welcome/subscribe attempt is
	// in progress - nothing has been published yet.
	StateConnecting State = "connecting"
	// StateConnected means the connector is subscribed and receiving
	// normally.
	StateConnected State = "connected"
	// StateReconnecting means the connection was lost and the connector is
	// retrying with bounded exponential backoff.
	StateReconnecting State = "reconnecting"
	// StatePossibleGap means the connector just re-established a fresh
	// (non-graceful-resume) connection after an unexpected disconnect -
	// StreamElements documents no event-replay guarantee for that path, so
	// this honestly signals a donation may have been missed while
	// disconnected. Cleared on the next real tip received, or on a later
	// graceful reconnect.
	StatePossibleGap State = "possible_gap"
	// StateReconnectRequired means the provider rejected the stored
	// credential outright (subscribe failed) - retrying with the same
	// token would loop forever, so the connector stops and waits for the
	// operator to replace the credential or explicitly restart.
	StateReconnectRequired State = "reconnect_required"
	// StateError means an unrecoverable, non-credential condition occurred
	// (e.g. the credential is missing from SecretStore entirely).
	StateError State = "error"
	// StateStopping means Disable/Delete/Shutdown was called.
	StateStopping State = "stopping"
)

// terminalForRetryLoop reports whether the connector's background retry
// loop should stop entirely rather than reconnect with backoff.
func (s State) terminalForRetryLoop() bool {
	return s == StateStopping || s == StateError || s == StateDisabled || s == StateReconnectRequired
}

// Snapshot is one connector's public, non-secret status. Never carries the
// JWT, a reconnect token, or a raw provider/error payload - see
// docs/provider-integrations/external-donations.md §33.
type Snapshot struct {
	SourceID string
	Enabled  bool
	State    State

	ConnectedAt   *time.Time
	LastEventAt   *time.Time
	LastDataGapAt *time.Time

	ReconnectCount   int
	PossibleGapCount int

	// LastError is a sanitized, stable error code.
	LastError string
}
