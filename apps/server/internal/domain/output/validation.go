package output

import (
	"net/url"
	"strconv"
	"strings"
	"unicode"

	"github.com/streaming-tree/server/internal/domain/platform"
)

// MaxServerURLBytes bounds the stored value. No verified provider needs
// anything close to this; it exists to bound storage, not to encode a real
// provider limit.
const MaxServerURLBytes = 2048

// ValidateServerURL trims incidental surrounding whitespace and validates the
// destination RTMP/RTMPS server address.
//
// An empty value (after trimming) is valid and means "not configured yet" -
// the field is optional at the domain level, so a destination can exist
// without one and clearing it is a legitimate action. A non-empty value must
// be a well-formed rtmp:// or rtmps:// URL.
//
// Deliberately rejected even though technically parseable:
//   - userinfo ("user:pass@host") - a URL is not a place to smuggle a
//     credential, and accepting the shape would blur the line this package
//     exists to keep sharp,
//   - a fragment ("#...") - meaningless for a server address and never used
//     by any verified provider integration,
//   - a query string ("?...") - no verified provider requires one for the
//     base server address, and rejecting it keeps the field's shape simple
//     and unambiguous. If a provider is later found to need one, this is the
//     one place that would change.
//
// The returned error, when non-nil, is a *platform.ValidationError with a
// single "serverUrl" violation, reusing the same rendering pipeline and
// field/rule convention as platform metadata and the credential stream-key
// field - see internal/domain/credential/validation.go for the same
// trade-off made there.
func ValidateServerURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}

	verr := &platform.ValidationError{}

	for _, r := range trimmed {
		if unicode.IsControl(r) {
			verr.Add("serverUrl", platform.RuleInvalid,
				"Server address must not contain control characters or line breaks.", nil)
			return "", verr
		}
	}

	if len(trimmed) > MaxServerURLBytes {
		verr.Add("serverUrl", platform.RuleTooLong,
			"Server address exceeds the maximum allowed size.",
			map[string]any{"max": MaxServerURLBytes})
		return "", verr
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		verr.Add("serverUrl", platform.RuleInvalid, "Server address is not a valid URL.", nil)
		return "", verr
	}

	if parsed.Scheme != "rtmp" && parsed.Scheme != "rtmps" {
		verr.Add("serverUrl", platform.RuleInvalid,
			"Server address must start with rtmp:// or rtmps://.", nil)
		return "", verr
	}
	if parsed.Host == "" || parsed.Hostname() == "" {
		verr.Add("serverUrl", platform.RuleRequired, "Server address must include a host.", nil)
		return "", verr
	}
	if parsed.User != nil {
		verr.Add("serverUrl", platform.RuleInvalid,
			"Server address must not include a username or password.", nil)
		return "", verr
	}
	if parsed.Fragment != "" {
		verr.Add("serverUrl", platform.RuleInvalid,
			"Server address must not include a # fragment.", nil)
		return "", verr
	}
	if parsed.RawQuery != "" {
		verr.Add("serverUrl", platform.RuleInvalid,
			"Server address must not include a ? query string.", nil)
		return "", verr
	}
	if port := parsed.Port(); port != "" {
		value, err := strconv.Atoi(port)
		if err != nil || value < 1 || value > 65535 {
			verr.Add("serverUrl", platform.RuleInvalid,
				"Server address has an invalid port.", nil)
			return "", verr
		}
	}

	return trimmed, nil
}
