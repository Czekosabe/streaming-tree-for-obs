package outboundchat

import (
	"errors"
	"time"
)

// Sentinel domain errors. The HTTP layer maps these to stable API error
// codes; no provider-level error, raw response, or secret value ever
// reaches it directly. account.ErrNotFound, account.ErrReconnectRequired,
// and account.ErrMissingScope are reused directly rather than redefined
// here, matching how internal/provider/twitch's own metadata publishing
// already reuses them.
var (
	// ErrUnauthorized means the provider rejected the token outright (for
	// example Twitch's own 401). Provider-independent by design - see
	// Provider's own doc comment - so the dispatcher can drive a uniform
	// refresh-and-retry-once policy without ever importing a concrete
	// provider package's own sentinel error.
	ErrUnauthorized = errors.New("outbound chat token rejected")

	// ErrUnsupportedProvider means the connected account's provider has no
	// outbound-chat Provider registered with the dispatcher's Manager.
	ErrUnsupportedProvider = errors.New("provider does not support outbound chat")

	// ErrPermissionRequired means the account's currently-granted scopes do
	// not satisfy the provider's outbound-chat capability profile - see
	// Capability.PermissionUpgradeRequired.
	ErrPermissionRequired = errors.New("outbound chat permission required")

	// ErrForbidden means the provider rejected the send because the sender
	// is not permitted to post in that chat room right now (banned, timed
	// out, or otherwise lacks permission) - never automatically retried.
	ErrForbidden = errors.New("outbound chat forbidden")

	// ErrProviderFailure means the provider returned a definite error
	// response (for example an HTTP 5xx) - a real answer was received, but
	// it was not success. Never automatically retried.
	ErrProviderFailure = errors.New("outbound chat provider failure")

	// ErrDeliveryUnknown means the request may have reached the provider
	// but no trustworthy result was ever received - a transport failure, a
	// timeout after the request may have left this process, or a
	// malformed/non-provider-shaped "success" response. Never automatically
	// retried, since retrying an uncertain send risks a real duplicate
	// message in the broadcaster's own chat.
	ErrDeliveryUnknown = errors.New("outbound chat delivery outcome unknown")

	// ErrQueueFull means an account's bounded dispatch queue is already at
	// capacity - a queue-full error, never an unbounded memory grow.
	ErrQueueFull = errors.New("outbound chat queue is full")

	// ErrCancelled means the send was cancelled before it started (caller
	// context cancelled, or the dispatcher is shutting down).
	ErrCancelled = errors.New("outbound chat send was cancelled")

	// ErrChatUnavailable means the provider has no currently-writable chat
	// for this account to send into (Stage 15A: YouTube has no selected
	// broadcast, no live chat yet, or the chat has ended) - a stable,
	// provider-independent "not live" outcome, never automatically
	// retried, and never confused with ErrForbidden (which means a real
	// chat rejected this specific sender).
	ErrChatUnavailable = errors.New("outbound chat has no writable chat available")

	// ErrReplyUnsupported means the target provider's send API has no
	// reply/parent-message concept at all (Stage 15A: YouTube
	// liveChatMessages.insert has no such field) - the backend rejects an
	// attempted reply outright rather than silently sending it as a plain
	// message or fabricating an @mention prefix.
	ErrReplyUnsupported = errors.New("this provider does not support replying to a message")
)

// RateLimitedError signals a send was rejected due to rate limiting -
// either this application's own conservative local ceiling, Twitch's
// standard Helix API bucket (HTTP 429), or Twitch's own chat-backend "too
// fast" limit (HTTP 420 - see
// docs/provider-integrations/twitch-outbound-chat.md). Never automatically
// retried. RetryAt is the zero time when no usable hint is available.
type RateLimitedError struct {
	RetryAt time.Time
}

func (e *RateLimitedError) Error() string { return "outbound chat rate limited" }
