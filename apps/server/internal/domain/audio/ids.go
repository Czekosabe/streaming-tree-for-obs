package audio

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// publicSlugEntropyBytes mirrors internal/domain/chatoverlay.NewPublicSlug's
// own exact entropy choice (160 bits) - see that function's own doc
// comment for the full "unguessable locator, not a credential" reasoning,
// which applies identically here. Implemented as its own function rather
// than imported, matching this codebase's existing per-domain-package
// convention (donationsource, alerts, and chatoverlay each define their
// own ID/slug generation rather than sharing one).
const publicSlugEntropyBytes = 20

// NewPublicSlug returns a fresh, high-entropy, opaque public locator for
// the audio Browser Source URL (/overlay/audio/{slug}). Rotating it
// (see Service.RotatePublicSlug) invalidates the previous URL
// immediately.
func NewPublicSlug() (string, error) {
	buf := make([]byte, publicSlugEntropyBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate audio public slug: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
