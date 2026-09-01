package platform

import "context"

// Repository is the persistence port for configured platforms.
//
// Implementations translate storage failures into the sentinel errors declared
// in errors.go; no driver-specific error ever crosses this boundary.
type Repository interface {
	// List returns every configured platform with its metadata and tags,
	// ordered by sort order and then by a deterministic tie-breaker.
	List(ctx context.Context) ([]Platform, error)

	// Get returns one configured platform, or ErrNotFound.
	Get(ctx context.Context, id string) (Platform, error)

	// Create inserts the platform together with its initial metadata row in a
	// single transaction.
	Create(ctx context.Context, p Platform) error

	// Update replaces the mutable configuration fields. Returns ErrNotFound
	// when the platform does not exist.
	Update(ctx context.Context, id string, input UpdateInput, updatedAt string) error

	// Delete removes the platform; metadata and tags cascade. Returns
	// ErrNotFound when nothing was removed.
	Delete(ctx context.Context, id string) error

	// SaveMetadata replaces the metadata row and the whole ordered tag list in
	// a single transaction.
	SaveMetadata(ctx context.Context, platformID string, m Metadata) error

	// SaveMetadataBatch replaces the metadata (and tag lists) of every named
	// platform in one transaction spanning all of them: either every update
	// lands, or none does. Returns ErrNotFound if any platform ID does not
	// exist.
	SaveMetadataBatch(ctx context.Context, updates map[string]Metadata) error

	// NextSortOrder returns the sort order to use when appending a platform.
	NextSortOrder(ctx context.Context) (int, error)
}
