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
)

// wireErr builds a sanitized error for an unexpected HTTP status: it names
// the status and the endpoint, never the response body, which could in
// principle echo back request data.
func wireErr(base error, status int, endpoint string) error {
	return fmt.Errorf("%w: %s returned status %d", base, endpoint, status)
}
