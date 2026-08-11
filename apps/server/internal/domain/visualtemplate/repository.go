package visualtemplate

import "context"

// Repository persists USER templates only - built-ins never pass
// through here (Stage 14A task Part 11/12).
type Repository interface {
	// Create inserts t (t.ID/CreatedAt/UpdatedAt already set by the
	// caller) and returns the stored value.
	Create(ctx context.Context, t Template) (Template, error)
	// Get returns the user template with id, or ErrNotFound.
	Get(ctx context.Context, id string) (Template, error)
	// List returns every user template, newest first.
	List(ctx context.Context) ([]Template, error)
	// UpdateMetadata replaces name/description/author/license for id,
	// leaving Document/CreatedAt untouched, and returns the updated
	// value. Returns ErrNotFound if id does not exist.
	UpdateMetadata(ctx context.Context, id, name, description, author, license string) (Template, error)
	// Delete removes id if it exists; idempotent.
	Delete(ctx context.Context, id string) error
}
