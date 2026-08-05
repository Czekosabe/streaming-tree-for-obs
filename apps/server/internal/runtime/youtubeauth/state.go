// Package youtubeauth is the YouTube-specific OAuth attempt manager:
// Authorization Code Flow with PKCE and a temporary loopback callback
// listener, deliberately separate from internal/runtime/deviceflow's
// Twitch-specific polling state machine - see
// docs/provider-integrations/youtube.md's "Why Twitch's Device Code Flow is
// not reused" section and account.DeviceFlowProvider's own doc comment.
package youtubeauth

import "time"

// State is one YouTube OAuth attempt's current stage.
type State string

const (
	StateCreating              State = "creating"
	StateWaitingForBrowser     State = "waiting_for_browser"
	StateProcessingCallback    State = "processing_callback"
	StateAwaitingChannelSelect State = "awaiting_channel_selection"
	StateAuthorized            State = "authorized"
	StateDenied                State = "denied"
	StateExpired               State = "expired"
	StateCancelled             State = "cancelled"
	StateError                 State = "error"
)

func (s State) terminal() bool {
	switch s {
	case StateAuthorized, StateDenied, StateExpired, StateCancelled, StateError:
		return true
	default:
		return false
	}
}

// ChannelSummary is the non-secret information about one owned channel,
// shown to the operator only when more than one channel was returned and an
// explicit selection is required - see docs/provider-integrations/
// youtube.md's "Account/channel identity behavior" section. Deliberately
// excludes email, any raw API payload, and any token.
type ChannelSummary struct {
	ChannelID    string
	Title        string
	ThumbnailURL string
}

// Snapshot is the public, non-secret view of one attempt.
//
// Never carries: an authorization code, a PKCE verifier, a state value, a
// refresh token, an access token, an ID token, a client secret, a callback
// query string, or a raw Google response - see the task's OAuth attempt
// state machine requirements. AuthorizationURL is included only while
// waiting for the browser and is itself ephemeral, security-sensitive data
// that this Manager never persists past the attempt's own lifetime.
type Snapshot struct {
	AttemptID          string
	ProviderID         string
	State              State
	AuthorizationURL   string
	CreatedAt          time.Time
	ExpiresAt          time.Time
	ConnectedAccountID string
	Channels           []ChannelSummary
	ErrorCode          string
	ErrorMessage       string
}
