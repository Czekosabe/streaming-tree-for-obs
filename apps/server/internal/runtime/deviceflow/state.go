// Package deviceflow orchestrates OAuth device-authorization attempts:
// starting one, polling Twitch (or a future provider) at the documented
// interval, and finalizing a successful authorization into a connected
// account via internal/domain/account.
//
// Attempts live only in memory - never in SQLite, never as a device code
// anywhere persistent - and are bounded, cancellable, and cleaned up after
// completion or expiration. See Manager's own doc comment for the full
// lifecycle.
package deviceflow

import (
	"time"

	"github.com/streaming-tree/server/internal/domain/account"
)

// State is one device-flow attempt's explicit lifecycle state.
type State string

const (
	StateRequestingCode State = "requesting_code"
	StateWaitingForUser State = "waiting_for_user"
	StatePolling        State = "polling"
	StateAuthorized     State = "authorized"
	StateDenied         State = "denied"
	StateExpired        State = "expired"
	StateCancelled      State = "cancelled"
	StateError          State = "error"
)

// terminal reports whether a state can never transition again.
func (s State) terminal() bool {
	switch s {
	case StateAuthorized, StateDenied, StateExpired, StateCancelled, StateError:
		return true
	default:
		return false
	}
}

// Snapshot is the public, non-secret view of one device-flow attempt.
//
// Deliberately excludes: the device code, any access or refresh token, any
// client secret, and any raw provider response - see Manager's internal
// attempt type for what is tracked privately instead.
type Snapshot struct {
	AttemptID  string
	ProviderID account.ProviderID
	State      State
	// UserCode is safe to display and safe to copy to the clipboard.
	UserCode        string
	VerificationURI string
	CreatedAt       time.Time
	ExpiresAt       time.Time
	Interval        time.Duration
	// ConnectedAccountID is set only once State is StateAuthorized.
	ConnectedAccountID string
	// ErrorCode/ErrorMessage are set only for StateError - a stable
	// identifier plus an English fallback, matching this project's other
	// error envelopes.
	ErrorCode    string
	ErrorMessage string
}
