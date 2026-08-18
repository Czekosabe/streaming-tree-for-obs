// Package updatersettings holds the one persisted Stage 20B updater
// preference: whether the application automatically checks GitHub for a
// newer Stable release (docs/updater.md §27). Deliberately minimal,
// mirroring internal/domain/operatorchatprefs's own singleton-row
// reasoning - this is configuration only, never identity, never a
// machine/installation id, never a check/download history.
package updatersettings

import "time"

// Preferences is the singleton set of updater preferences.
type Preferences struct {
	// AutoCheck enables the automatic startup + hourly metadata check
	// (docs/updater.md §10) in a packaged release build. Has no effect
	// in a development build regardless of its value - see
	// docs/updater.md §35.
	AutoCheck bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Default returns the documented out-of-the-box preference:
// automatically check for updates is on (docs/updater.md §10/§27).
func Default() Preferences {
	return Preferences{AutoCheck: true}
}
