package branch

import (
	"net/url"
	"sort"
	"strings"
)

// redactedPlaceholder replaces every occurrence of a secret.
const redactedPlaceholder = "[REDACTED]"

// Redactor removes one branch's secrets - its stream key and its complete
// destination URL - from arbitrary text before that text is ever logged,
// buffered for diagnostics, or included in an error.
//
// Built fresh per launch from exactly the values that launch used, and
// discarded with everything else once the process exits: it never outlives
// the secrets it exists to hide, and it is never constructed from anything
// other than the values a launch actually used.
type Redactor struct {
	variants []string
}

// newRedactor builds a Redactor for one branch launch. Both the raw secret
// values and common URL-escaped variants are included, since FFmpeg's own
// log lines sometimes percent-encode a URL it prints.
func newRedactor(secrets ...string) *Redactor {
	seen := make(map[string]struct{}, len(secrets)*3)
	add := func(s string) {
		if s == "" {
			return
		}
		seen[s] = struct{}{}
	}

	for _, s := range secrets {
		add(s)
		add(url.QueryEscape(s))
		add(url.PathEscape(s))
	}

	variants := make([]string, 0, len(seen))
	for v := range seen {
		variants = append(variants, v)
	}
	// Longest first: a short secret that happens to be a substring of a
	// longer one (or of its own escaped form) must not partially mask the
	// longer one before it gets its turn.
	sort.Slice(variants, func(i, j int) bool { return len(variants[i]) > len(variants[j]) })

	return &Redactor{variants: variants}
}

// Redact returns text with every known secret variant replaced.
func (r *Redactor) Redact(text string) string {
	if r == nil || len(r.variants) == 0 {
		return text
	}
	for _, v := range r.variants {
		text = strings.ReplaceAll(text, v, redactedPlaceholder)
	}
	return text
}
