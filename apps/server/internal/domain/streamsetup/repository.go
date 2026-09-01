package streamsetup

import (
	"context"
	"errors"
)

// ErrNotFound means no profile exists with the given id.
var ErrNotFound = errors.New("stream setup profile not found")

// ErrDuplicateName means a profile with that name (case-insensitively)
// already exists.
var ErrDuplicateName = errors.New("a stream setup profile with that name already exists")

// Repository is the persistence port this domain depends on.
type Repository interface {
	List(ctx context.Context) ([]Profile, error)
	Get(ctx context.Context, id string) (Profile, error)
	Create(ctx context.Context, p Profile) error
	// Update replaces name/note/metadata-preset-reference and the full
	// destination list in one transaction.
	Update(ctx context.Context, p Profile) error
	Delete(ctx context.Context, id string) error
}
