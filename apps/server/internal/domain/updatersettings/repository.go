package updatersettings

import (
	"context"
	"time"
)

// Repository is the persistence port for the singleton updater
// preferences row.
type Repository interface {
	// GetPreferences returns the singleton preferences row. found is
	// false when no row has ever been written - callers treat that
	// identically to Default().
	GetPreferences(ctx context.Context) (Preferences, bool, error)

	// SetPreferences replaces the singleton preferences row in full.
	SetPreferences(ctx context.Context, p Preferences, now time.Time) (Preferences, error)
}
