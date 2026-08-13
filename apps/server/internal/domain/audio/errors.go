package audio

import "errors"

// Sentinel domain errors. The HTTP layer maps these to stable API error
// codes; no repository-level error ever reaches it directly.
var (
	// ErrValidation wraps every settings-validation failure - the HTTP
	// layer maps it to 422.
	ErrValidation = errors.New("invalid audio/TTS settings")

	// ErrStorage wraps any unexpected persistence failure. The
	// underlying driver error is kept for the logs but must never reach
	// a client.
	ErrStorage = errors.New("storage failure")
)
