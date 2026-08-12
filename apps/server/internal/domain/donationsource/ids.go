package donationsource

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// NewID returns a random, non-sequential donation-source identifier -
// mirrors internal/domain/alerts.NewProfileID's own convention.
func NewID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate donation source id: %w", err)
	}
	return "donsrc_" + hex.EncodeToString(buf), nil
}
