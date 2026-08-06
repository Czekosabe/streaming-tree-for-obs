package chatoverlay

import "errors"

var (
	// ErrStorage wraps any unexpected persistence failure.
	ErrStorage = errors.New("storage failure")
	// ErrNotFound means the referenced overlay profile does not exist.
	ErrNotFound = errors.New("chat overlay not found")
	// ErrPublicSlugNotFound means no overlay currently uses the given
	// public slug - either it never existed or it was rotated/deleted.
	ErrPublicSlugNotFound = errors.New("chat overlay public slug not found")
	// ErrAccountNotFound means the referenced connected account does not
	// exist.
	ErrAccountNotFound = errors.New("connected account not found")
	// ErrUserNotFound means no matching hidden-user entry exists to
	// remove.
	ErrUserNotFound = errors.New("hidden user entry not found")
	// ErrTermNotFound means no matching blocked-term entry exists to
	// remove.
	ErrTermNotFound = errors.New("blocked term not found")
)
