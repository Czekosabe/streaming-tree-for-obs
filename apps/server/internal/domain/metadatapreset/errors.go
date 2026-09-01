package metadatapreset

import (
	"errors"

	"github.com/streaming-tree/server/internal/domain/platform"
)

// Sentinel domain errors. Handlers map these to HTTP status codes; raw
// storage errors never reach the API surface.
var (
	// ErrNotFound is returned when a preset does not exist.
	ErrNotFound = errors.New("metadata preset not found")

	// ErrDuplicateName is returned when a create/rename would collide
	// with an existing preset's name (case-insensitive).
	ErrDuplicateName = errors.New("a preset with this name already exists")

	// ErrTooMany is returned when creating a preset would exceed
	// MaxPresets.
	ErrTooMany = errors.New("too many presets")

	// ErrStorage wraps any unexpected persistence failure. The
	// underlying driver error is kept for the logs but must not be
	// returned to a client.
	ErrStorage = errors.New("storage failure")
)

// Field-validation violations reuse platform.ValidationError/
// FieldViolation directly - already a shared, cross-domain mechanism
// in this codebase (account/chatoverlay/credential/output domains and
// httpapi/errors.go's writeValidationError all reuse it too), not
// reinvented here.

// ApplyValidationError reports why an Apply request was rejected in
// full. Apply is all-or-nothing (docs/metadata-presets.md §6/§15/§23):
// if any selected destination's projected candidate fails validation,
// the entire request is rejected with every destination's own field
// errors, and nothing is written for any of them.
type ApplyValidationError struct {
	// Destinations maps platform ID to that destination's own field
	// violations. Only destinations that actually failed appear here.
	Destinations map[string][]platform.FieldViolation
}

func (e *ApplyValidationError) Error() string {
	return "metadata preset apply validation failed"
}

// AsApplyValidationError extracts an *ApplyValidationError from an
// error chain.
func AsApplyValidationError(err error) (*ApplyValidationError, bool) {
	var target *ApplyValidationError
	if errors.As(err, &target) {
		return target, true
	}
	return nil, false
}
