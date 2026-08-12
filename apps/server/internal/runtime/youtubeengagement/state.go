// Package youtubeengagement supervises one YouTube Live Chat polling
// connector per enabled connected YouTube account - the Stage 15A inbound
// engagement connector. See docs/provider-integrations/
// youtube-engagement.md for the researched REST polling contract this
// package implements, and internal/runtime/twitchengagement for the
// analogous Stage 8A connector this package's lifecycle/state-machine
// shape deliberately mirrors where the underlying transport is actually
// alike (both: one goroutine per enabled account, bounded exponential
// backoff on failure, an explicit Snapshot never carrying a token).
//
// The two providers' transports are not alike in one fundamental way:
// Twitch pushes over a WebSocket; YouTube is polled over plain HTTPS with
// a server-recommended interval and a continuation (page) token. This
// package's own states reflect that difference explicitly (StateWaitingForBroadcast/
// StateWaitingForLiveChat/StateChatEnded have no Twitch equivalent) rather
// than forcing YouTube's reality into Twitch's shape.
package youtubeengagement

import "time"

// State is a connector's explicit lifecycle state. Mutually exclusive.
type State string

const (
	// StateDisabled means the account's engagement setting is off.
	StateDisabled State = "disabled"
	// StateBlocked means engagement is enabled but cannot start at all
	// (e.g. the account is unhealthy) - see Snapshot.BlockerCodes.
	StateBlocked State = "blocked"
	// StateWaitingForBroadcast means no live-broadcast remote target is
	// currently selected for the destination this account is linked to.
	StateWaitingForBroadcast State = "waiting_for_broadcast"
	// StateWaitingForLiveChat means a broadcast is selected but it has no
	// liveChatId yet (not live yet, or chat disabled by the owner) - see
	// docs/provider-integrations/youtube-engagement.md §3.5.
	StateWaitingForLiveChat State = "waiting_for_live_chat"
	// StateConnecting means the baseline-establishing first poll is in
	// progress (docs/provider-integrations/youtube-engagement.md §7) -
	// nothing from this call is ever published.
	StateConnecting State = "connecting"
	// StateConnected means the connector is polling normally and any
	// newly-received message is published to the Event Bus.
	StateConnected State = "connected"
	// StateReconnecting means a poll failed transiently and the connector
	// is retrying with backoff, or a continuation token was invalidated
	// and a fresh baseline is being established.
	StateReconnecting State = "reconnecting"
	// StateChatEnded means the provider reported the chat/broadcast has
	// ended (a chatEndedEvent message, or the response's own offlineAt
	// field) - a real, honest terminal-for-now state, never treated as
	// equivalent to stream.offline (see docs/provider-integrations/
	// youtube-engagement.md §5). The connector stops polling; a broadcast
	// change or explicit Restart is required to resume.
	StateChatEnded State = "chat_ended"
	// StateStopping means Disable or Shutdown was called.
	StateStopping State = "stopping"
	// StateError means an unrecoverable condition occurred (most often
	// "reconnect_required" after every refresh attempt failed) - no
	// automatic retry; an explicit Restart or re-Enable is required.
	StateError State = "error"
)

// terminalForRetryLoop reports whether the connector's background retry
// loop should stop entirely rather than reconnect with backoff.
func (s State) terminalForRetryLoop() bool {
	return s == StateStopping || s == StateError || s == StateDisabled || s == StateBlocked || s == StateChatEnded
}

// waitingState reports whether s is a "nothing wrong, just not ready yet"
// state that should retry on a fixed short interval rather than
// exponential backoff (a selected broadcast or its live chat is expected
// to appear/disappear during ordinary streamer behavior, not a failure).
func (s State) waitingState() bool {
	return s == StateWaitingForBroadcast || s == StateWaitingForLiveChat
}

// Snapshot is one connector's public, non-secret status. Never carries an
// access/refresh token, a continuation (page) token, or a raw provider
// response body - see docs/provider-integrations/youtube-engagement.md §10.
type Snapshot struct {
	AccountID string
	Enabled   bool
	State     State

	BlockerCodes []string

	// SelectedBroadcastID is the broadcast this connector currently
	// targets - a management/private-surface-only field (not exposed
	// beyond this application's own operator-facing API), per the task's
	// own "current selected broadcast id only on management/private
	// surface" instruction. Never a liveChatId (an internal identifier
	// with no operator-facing meaning).
	SelectedBroadcastID string

	ConnectedAt   *time.Time
	LastEventAt   *time.Time
	LastPollAt    *time.Time
	LastDataGapAt *time.Time

	ReconnectCount        int
	PossibleGapCount      int
	UnsupportedEventCount int

	// LastError is a sanitized, stable error code.
	LastError string
}
