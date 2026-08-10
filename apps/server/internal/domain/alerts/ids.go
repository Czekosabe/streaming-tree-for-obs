package alerts

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// NewProfileID returns a random, non-sequential alert-profile identifier -
// mirrors chatoverlay.NewID's own reasoning: never a sequential integer,
// never itself exposed to a public Browser Source viewer (see
// NewPublicSlug for the value that is).
func NewProfileID() (string, error) {
	return newID("alprof_", 16)
}

// NewRuleID returns a random, non-sequential alert-rule identifier.
func NewRuleID() (string, error) {
	return newID("alrule_", 12)
}

// publicSlugEntropyBytes matches chatoverlay.NewPublicSlug's own choice
// (160 bits) - the one value a viewer's Browser Source URL actually
// carries needs to be practically unguessable by brute force, even
// though it is explicitly not a credential - see NewPublicSlug's own doc
// comment.
const publicSlugEntropyBytes = 20

// NewPublicSlug returns a fresh, high-entropy, opaque public locator for
// an alert profile's Browser Source URL.
//
// This is deliberately NOT an OAuth token or credential, and is never
// stored in internal/secrets: it is a random, unguessable local locator,
// exactly like a chat-overlay profile's own public slug (see
// internal/domain/chatoverlay/ids.go's own doc comment for the full
// reasoning, which applies unchanged here). The application is
// loopback-only by default; this locator is not sufficient
// authentication for a server exposed to the public network. Rotating it
// invalidates the previous URL immediately without losing any other
// profile setting.
func NewPublicSlug() (string, error) {
	buf := make([]byte, publicSlugEntropyBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate alert profile public slug: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func newID(prefix string, n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate %sid: %w", prefix, err)
	}
	return prefix + hex.EncodeToString(buf), nil
}
