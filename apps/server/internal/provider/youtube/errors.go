package youtube

import (
	"errors"
	"fmt"
)

// Sentinel errors this adapter returns. internal/domain/account maps these
// onto account.Err* at its own boundary; internal/httpapi never sees a
// YouTube-specific error directly.
var (
	// ErrUnavailable wraps a transient failure: network error, timeout, or
	// any 5xx response.
	ErrUnavailable = errors.New("youtube unavailable")

	// ErrRateLimited means Google answered 429, or a quotaExceeded /
	// rateLimitExceeded errors.reason at 403.
	ErrRateLimited = errors.New("youtube rate limited")

	// ErrQuotaExceeded means Google specifically reported quotaExceeded,
	// distinct from a generic rate limit.
	ErrQuotaExceeded = errors.New("youtube quota exceeded")

	// ErrInvalidResponse means Google answered with something this adapter
	// could not parse or trust.
	ErrInvalidResponse = errors.New("invalid youtube response")

	// ErrUnauthorized means Google rejected the credentials (401) - the
	// caller (account.Service) is expected to attempt exactly one refresh
	// and retry; see account.Service.WithFreshToken.
	ErrUnauthorized = errors.New("youtube rejected the credentials")

	// ErrForbidden means Google answered 403 for a reason other than quota
	// or rate limiting - most often a missing scope.
	ErrForbidden = errors.New("youtube forbidden")

	// ErrLiveStreamingNotEnabled means the channel has not enabled live
	// streaming - Google's own liveStreamingNotEnabled errors.reason.
	ErrLiveStreamingNotEnabled = errors.New("live streaming is not enabled for this channel")

	// ErrInvalidGrant means a refresh failed because the refresh token
	// itself is no longer usable (revoked, or Google's Testing-mode 7-day
	// expiry) - the caller marks the account reconnect_required rather than
	// treating this as a transient failure.
	ErrInvalidGrant = errors.New("youtube refresh token is no longer valid")

	// Stage 15A (live chat). Each is Google's own documented errors[].reason
	// for liveChatMessages.list/streamList/insert - see
	// docs/provider-integrations/youtube-engagement.md §3.2-§3.4.

	// ErrLiveChatDisabled means the broadcast owner has disabled live chat.
	ErrLiveChatDisabled = errors.New("youtube live chat is disabled for this broadcast")
	// ErrLiveChatEnded means the specified live chat is no longer live -
	// a real, honest, non-error connector lifecycle signal, never treated
	// as a transient failure to retry.
	ErrLiveChatEnded = errors.New("youtube live chat has ended")
	// ErrLiveChatNotFound means the chat ID does not exist (or was
	// deleted) - distinct from ErrLiveChatEnded, which is a real chat that
	// is simply no longer live.
	ErrLiveChatNotFound = errors.New("youtube live chat not found")
	// ErrMessageInvalid means Google rejected the outgoing message body
	// itself (messageTextInvalid/messageTextRequired/liveChatIdRequired/
	// typeRequired) - a client-side validation failure, never retried.
	ErrMessageInvalid = errors.New("youtube rejected the message")
)

// wireErr builds a sanitized error for an unexpected HTTP status: it names
// the status and the endpoint, never the response body, which could in
// principle echo back request data.
func wireErr(base error, status int, endpoint string) error {
	return fmt.Errorf("%w: %s returned status %d", base, endpoint, status)
}
