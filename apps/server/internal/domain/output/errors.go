package output

import "errors"

// Sentinel domain errors.
var (
	// ErrNotFound means the platform (and therefore its output-settings row,
	// which is created and deleted alongside it) does not exist. In
	// practice the HTTP layer checks platform existence before reaching this
	// package at all - see httpapi.requirePlatform - so this is reserved for
	// completeness rather than a path normally exercised.
	ErrNotFound = errors.New("output settings not found")

	// ErrStorage wraps an unexpected persistence failure. The underlying
	// driver error is kept for the logs but must not be returned to a client.
	ErrStorage = errors.New("storage failure")
)
