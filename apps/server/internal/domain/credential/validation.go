package credential

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/streaming-tree/server/internal/domain/platform"
)

// MaxStreamKeyBytes bounds the value this package will store.
//
// No destination platform's stream-key format has been verified against a
// real integration yet, so this is a conservative ceiling to bound memory
// and storage - not an attempt to encode a real provider limit that has not
// been confirmed.
const MaxStreamKeyBytes = 4096

// ValidateStreamKey trims incidental surrounding whitespace and rejects a
// value that is empty, oversized, not valid text, or contains a control
// character (which includes every line-break form).
//
// The rejected value is never included in the returned error: callers must
// not format raw into a log line or an API error message either.
//
// The returned error, when non-nil, is a *platform.ValidationError with a
// single "streamKey" violation. Reusing that type - rather than a
// credential-specific one - lets the existing validation-error rendering and
// the frontend's existing field/rule mapping serve this field with no new
// machinery, at the cost of this package depending on platform's error
// vocabulary. See docs/progress.md for that trade-off.
func ValidateStreamKey(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)

	verr := &platform.ValidationError{}

	if trimmed == "" {
		verr.Add("streamKey", platform.RuleRequired,
			"Stream key must not be empty.", nil)
		return "", verr
	}

	if !utf8.ValidString(trimmed) {
		verr.Add("streamKey", platform.RuleInvalid,
			"Stream key is not valid text.", nil)
		return "", verr
	}

	if len(trimmed) > MaxStreamKeyBytes {
		verr.Add("streamKey", platform.RuleTooLong,
			"Stream key exceeds the maximum allowed size.",
			map[string]any{"max": MaxStreamKeyBytes})
		return "", verr
	}

	for _, r := range trimmed {
		if unicode.IsControl(r) {
			verr.Add("streamKey", platform.RuleInvalid,
				"Stream key must not contain control characters or line breaks.", nil)
			return "", verr
		}
	}

	return trimmed, nil
}
