package chatautomation

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// NewScheduleID returns a random, non-sequential schedule identifier -
// mirrors chatoverlay.NewID's own reasoning.
func NewScheduleID() (string, error) {
	return newID("sched_")
}

// NewScheduleMessageID returns a random identifier for one message
// alternative within a schedule.
func NewScheduleMessageID() (string, error) {
	return newID("schedmsg_")
}

// NewCommandID returns a random, non-sequential command identifier.
func NewCommandID() (string, error) {
	return newID("cmd_")
}

func newID(prefix string) (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate %sid: %w", prefix, err)
	}
	return prefix + hex.EncodeToString(buf), nil
}
