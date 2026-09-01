package metadatapreset

import "context"

// Repository is the persistence port for metadata presets.
type Repository interface {
	// List returns every preset, most-recently-updated first.
	List(ctx context.Context) ([]Preset, error)
	// Get returns one preset. Returns ErrNotFound if absent.
	Get(ctx context.Context, id string) (Preset, error)
	// Count returns the total number of presets - used to enforce
	// MaxPresets before Create.
	Count(ctx context.Context) (int, error)
	// Create inserts a new preset. Returns ErrDuplicateName if the
	// (case-insensitive) name is already taken.
	Create(ctx context.Context, p Preset) error
	// Update replaces a preset's mutable fields in full. Returns
	// ErrDuplicateName if the new name collides with a different
	// preset, ErrNotFound if id does not exist.
	Update(ctx context.Context, id string, input UpdateInput, updatedAt string) error
	// Delete removes a preset together with its tags and provider
	// overrides. Deleting a preset never touches any destination's
	// own metadata.
	Delete(ctx context.Context, id string) error
}
