package operatorchatprefs

import "errors"

var (
	// ErrStorage wraps any unexpected persistence failure.
	ErrStorage = errors.New("storage failure")
	// ErrAccountNotFound means the referenced connected account does not
	// exist - returned instead of a raw foreign-key error.
	ErrAccountNotFound = errors.New("connected account not found")
	// ErrUserNotFound means no matching hidden/bot user entry exists to
	// remove.
	ErrUserNotFound = errors.New("user entry not found")
)
