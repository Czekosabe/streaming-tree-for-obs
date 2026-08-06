package chatoverlay

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// NewID returns a random, non-sequential overlay-profile identifier -
// mirrors account.NewID's own reasoning: no sequential integers as
// public IDs, and this one is never itself exposed to a viewer (see
// NewPublicSlug for the value that is).
func NewID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate chat overlay id: %w", err)
	}
	return "ov_" + hex.EncodeToString(buf), nil
}

// publicSlugEntropyBytes is chosen generously (160 bits) - the public
// slug is the one value a viewer's Browser Source URL actually carries,
// so it needs to be practically unguessable by brute force even though
// it is explicitly NOT a credential (see NewPublicSlug's own doc
// comment).
const publicSlugEntropyBytes = 20

// NewPublicSlug returns a fresh, high-entropy, opaque public locator for
// an overlay's Browser Source URL.
//
// This is deliberately NOT an OAuth token or credential, and is never
// stored in internal/secrets: it is a random, unguessable local locator
// that lets an operator get a stable URL for a Browser Source without
// exposing a sequential id or the count of overlays that exist. The
// application is loopback-only by default; a locator like this is not
// sufficient authentication for a server exposed to the public network -
// a future remote-server stage must add real authentication before that
// is safe. Rotating it (see the /rotate-public-slug endpoint) invalidates
// the previous URL immediately without losing any other profile setting.
func NewPublicSlug() (string, error) {
	buf := make([]byte, publicSlugEntropyBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate chat overlay public slug: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// NewUserRefID returns a random identifier for one hidden-user or
// blocked-term entry.
func NewUserRefID() (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate chat overlay entry id: %w", err)
	}
	return "ocu_" + hex.EncodeToString(buf), nil
}

// NewTermID returns a random identifier for one blocked-term entry.
func NewTermID() (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate chat overlay term id: %w", err)
	}
	return "term_" + hex.EncodeToString(buf), nil
}
