package remotetarget

import "errors"

// Sentinel domain errors. The HTTP layer maps these to stable API error
// codes.
var (
	// ErrNotFound means no remote target is set for the given platform.
	ErrNotFound = errors.New("remote target not found")

	// ErrProviderMismatch means the target's provider does not match the
	// destination platform's own provider.
	ErrProviderMismatch = errors.New("provider mismatch")

	// ErrStorage wraps any unexpected persistence failure.
	ErrStorage = errors.New("storage failure")
)
