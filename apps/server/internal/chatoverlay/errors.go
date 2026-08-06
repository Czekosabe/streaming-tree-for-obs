package chatoverlay

import "errors"

var (
	// ErrClosed means the overlay's projection has been shut down (the
	// profile was deleted, or the backend is shutting down).
	ErrClosed = errors.New("chat overlay projection closed")
	// ErrNotFound means no active projection exists for the requested
	// overlay - either it was never created, or it has been shut down.
	ErrNotFound = errors.New("chat overlay projection not found")
)
