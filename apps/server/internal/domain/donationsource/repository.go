package donationsource

import "context"

// Repository is the persistence port for donation sources' safe metadata
// only - no implementation of this interface ever reads or writes a
// credential; that lives in the OS credential store via SecretStore,
// addressed only by source ID (see credential.go).
type Repository interface {
	// GetSource returns one source, or ErrNotFound.
	GetSource(ctx context.Context, id string) (Source, error)

	// ListSources returns every donation source, ordered by creation time.
	ListSources(ctx context.Context) ([]Source, error)

	// CreateSource inserts a new source.
	CreateSource(ctx context.Context, src Source) error

	// UpdateSource replaces an existing source's mutable fields.
	UpdateSource(ctx context.Context, src Source) error

	// DeleteSource removes the source row.
	DeleteSource(ctx context.Context, id string) error
}
