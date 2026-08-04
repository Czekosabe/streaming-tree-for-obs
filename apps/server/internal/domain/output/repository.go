package output

import "context"

// Repository is the persistence port for output settings.
//
// Implementations translate storage failures into the sentinel errors
// declared in errors.go; no driver-specific error ever crosses this
// boundary.
type Repository interface {
	// Get returns one platform's output settings, or ErrNotFound.
	Get(ctx context.Context, platformID string) (Settings, error)

	// Update replaces the mutable fields. Returns ErrNotFound when the
	// platform (and therefore its settings row) does not exist.
	Update(ctx context.Context, platformID string, input UpdateInput, updatedAt string) (Settings, error)
}
