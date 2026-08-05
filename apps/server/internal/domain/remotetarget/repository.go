package remotetarget

import (
	"context"
	"time"
)

// Repository is the persistence port for remote-broadcast-target
// associations.
type Repository interface {
	// Get returns the target set for a platform, if any.
	Get(ctx context.Context, platformID string) (Target, bool, error)

	// Set creates or replaces a platform's target.
	Set(ctx context.Context, t Target, now time.Time) (Target, error)

	// Delete removes a platform's target. Deleting an absent target is not
	// an error.
	Delete(ctx context.Context, platformID string) error
}
