package goals

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// NewGoalID returns a random, non-sequential goal identifier - mirrors
// alerts.NewRuleID's own reasoning.
func NewGoalID() (string, error) {
	return newID("goal_", 12)
}

// NewWidgetProfileID returns a random, non-sequential widget-profile
// identifier.
func NewWidgetProfileID() (string, error) {
	return newID("widget_", 12)
}

// publicSlugEntropyBytes matches alerts.NewPublicSlug's own choice (160
// bits) - see that function's doc comment for the full reasoning, which
// applies unchanged here.
const publicSlugEntropyBytes = 20

// NewPublicSlug returns a fresh, high-entropy, opaque public locator for
// a widget profile's Browser Source URL. Not a credential - see
// alerts.NewPublicSlug's own doc comment for the full reasoning.
func NewPublicSlug() (string, error) {
	buf := make([]byte, publicSlugEntropyBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate goal widget public slug: %w", err)
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
