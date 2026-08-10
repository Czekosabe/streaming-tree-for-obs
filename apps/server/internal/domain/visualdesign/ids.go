package visualdesign

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// NewDesignID returns a fresh, opaque, random visual-design id
// ("design_" + 16 random bytes hex-encoded) - stable across editing,
// never derived from an array index or the owning rule's own id (Stage
// 13A task Part 7).
func NewDesignID() (string, error) {
	return randomID("design_", 16)
}

// NewLayerID returns a fresh, opaque, random layer id ("layer_" + 12
// random bytes hex-encoded) - unique within one document, stable across
// reordering, and a duplicated layer always gets a new one (Stage 13A
// task Part 7/34).
func NewLayerID() (string, error) {
	return randomID("layer_", 12)
}

func randomID(prefix string, n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate %s id: %w", prefix, err)
	}
	return prefix + hex.EncodeToString(buf), nil
}
