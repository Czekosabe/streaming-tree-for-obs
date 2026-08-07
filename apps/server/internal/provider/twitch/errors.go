package twitch

import (
	"errors"
	"fmt"
)

// Sentinel errors this adapter returns. internal/domain/account maps these
// onto account.Err* / account.Rule* at its own boundary; internal/httpapi
// never sees a Twitch-specific error directly.
var (
	// ErrUnavailable wraps a transient failure: network error, timeout, or
	// any 5xx response.
	ErrUnavailable = errors.New("twitch unavailable")

	// ErrRateLimited means Twitch answered 429.
	ErrRateLimited = errors.New("twitch rate limited")

	// ErrInvalidResponse means Twitch answered with something this adapter
	// could not parse or trust (malformed JSON, a required field missing,
	// an empty data array where one item was expected).
	ErrInvalidResponse = errors.New("invalid twitch response")

	// ErrUnauthorized means Twitch rejected the credentials (401) - the
	// caller (account.Service) is expected to attempt exactly one refresh
	// and retry; see WithFreshToken.
	ErrUnauthorized = errors.New("twitch rejected the credentials")

	// ErrForbidden means Twitch answered 403 - most often a missing scope.
	ErrForbidden = errors.New("twitch forbidden")

	// ErrTransportUncertain means the request may have left this process
	// but no trustworthy HTTP response was ever received - a network
	// failure, a timeout, or a connection reset mid-response. Deliberately
	// distinct from ErrUnavailable (which means Twitch gave a definite 5xx
	// answer): only outbound_chat_client.go's SendChatMessage returns this,
	// since every other call in this package is safely retryable and never
	// needed the distinction before Stage 11A - see
	// docs/provider-integrations/twitch-outbound-chat.md's "Uncertain-
	// outcome and retry policy".
	ErrTransportUncertain = errors.New("twitch response uncertain")
)

// wireErr builds a sanitized error for an unexpected HTTP status: it names
// the status and the endpoint, never the response body, which could in
// principle echo back request data.
func wireErr(base error, status int, endpoint string) error {
	return fmt.Errorf("%w: %s returned status %d", base, endpoint, status)
}
