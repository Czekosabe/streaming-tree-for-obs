package audio

import (
	"context"
	"time"
)

// Repository is the persistence port for the singleton audio/TTS
// settings row.
type Repository interface {
	// GetSettings returns the singleton settings row. found is false
	// when no row has ever been written - callers treat that identically
	// to Default().
	GetSettings(ctx context.Context) (Settings, bool, error)

	// SetSettings replaces the singleton settings row in full.
	SetSettings(ctx context.Context, s Settings, now time.Time) (Settings, error)
}
