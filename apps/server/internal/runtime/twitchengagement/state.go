// Package twitchengagement supervises one Twitch EventSub WebSocket
// connector per enabled connected Twitch account - the Stage 8A inbound
// engagement connector. See docs/provider-integrations/twitch-engagement.md
// for the researched WebSocket contract this package implements.
package twitchengagement

import "time"

// State is a connector's explicit lifecycle state. Mutually exclusive - a
// connector is in exactly one of these at any moment, never represented as
// independent booleans.
type State string

const (
	// StateDisabled means the account's engagement setting is off (or was
	// never enabled). No connector goroutine is running.
	StateDisabled State = "disabled"
	// StateBlocked means engagement is enabled but cannot start: missing
	// scopes, no Client ID configured, or the account itself is unhealthy.
	// See Snapshot.BlockerCodes.
	StateBlocked State = "blocked"
	// StateConnecting means the WebSocket dial is in progress.
	StateConnecting State = "connecting"
	// StateWaitingForWelcome means the socket is open and this connector is
	// waiting for Twitch's session_welcome message.
	StateWaitingForWelcome State = "waiting_for_welcome"
	// StateSubscribing means the welcome was received and this connector is
	// creating EventSub subscriptions.
	StateSubscribing State = "subscribing"
	// StateConnected means at least one subscription is active and this
	// connector is receiving keepalives/notifications normally.
	StateConnected State = "connected"
	// StateReconnecting means a connection was lost (or an official
	// session_reconnect handoff is in progress) and this connector is
	// establishing a new session.
	StateReconnecting State = "reconnecting"
	// StateStopping means Disable or Shutdown was called and this connector
	// is closing its connection.
	StateStopping State = "stopping"
	// StateError means an unrecoverable condition occurred (for example,
	// authorization was revoked) and this connector will not retry on its
	// own - an explicit Restart or re-Enable is required.
	StateError State = "error"
)

// terminalForRetryLoop reports whether the connector's background retry
// loop should stop entirely rather than reconnect with backoff.
func (s State) terminalForRetryLoop() bool {
	return s == StateStopping || s == StateError || s == StateDisabled || s == StateBlocked
}

// Snapshot is one connector's public, non-secret status - see the stage
// task's explicit list of fields that must never appear here (WebSocket
// session id, reconnect URL, access token, full authorization headers, raw
// subscription response, internal endpoint override, secret-store
// identifiers).
type Snapshot struct {
	AccountID string
	Enabled   bool
	State     State

	// BlockerCodes are stable, English-message-free reasons this connector
	// cannot run right now (e.g. "engagement_scope_upgrade_required") - the
	// frontend/API layer attaches its own localized message.
	BlockerCodes []string
	// MissingScopes lists the specific engagement scopes not yet granted,
	// when BlockerCodes includes a scope-related blocker.
	MissingScopes []string

	ConnectedAt     *time.Time
	LastEventAt     *time.Time
	LastKeepaliveAt *time.Time
	LastDataGapAt   *time.Time

	ReconnectCount            int
	ActiveSubscriptionCount   int
	ExpectedSubscriptionCount int

	// LastError is a sanitized, stable error code - never a raw provider
	// error body or Go error string with request/response detail.
	LastError string
}
