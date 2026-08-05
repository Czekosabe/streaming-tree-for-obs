package engagementsettings

import (
	"context"
	"time"
)

// Repository is the persistence port for per-account engagement-connector
// settings.
type Repository interface {
	// Get returns the settings for one account. found is false when no row
	// exists yet - callers treat this the same as Enabled: false.
	Get(ctx context.Context, accountID string) (Settings, bool, error)

	// Set creates or replaces one account's settings.
	Set(ctx context.Context, s Settings, now time.Time) (Settings, error)

	// Delete removes one account's settings. Deleting an absent row is not
	// an error. Also happens automatically via ON DELETE CASCADE when the
	// connected account itself is deleted - this method exists for the
	// explicit disable-and-forget case, not only cascade.
	Delete(ctx context.Context, accountID string) error

	// ListEnabled returns every account currently configured enabled, in a
	// stable order - used once at backend startup to decide which
	// connectors to start automatically.
	ListEnabled(ctx context.Context) ([]Settings, error)
}
