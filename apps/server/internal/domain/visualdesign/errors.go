package visualdesign

import "errors"

var (
	// ErrStorage wraps any unexpected persistence failure.
	ErrStorage = errors.New("visual design storage failure")
	// ErrNotFound means no design is currently saved for the given
	// owner - a normal, expected state (Stage 13A task Part 19: every
	// existing Stage 12 alert rule currently has no design), never
	// itself an error condition the caller should treat as failure.
	ErrNotFound = errors.New("visual design not found")
	// ErrValidation wraps any semantic document validation failure -
	// see validation.go for the exact rules.
	ErrValidation = errors.New("visual design validation failed")
	// ErrRevisionConflict means a PUT's expected revision did not match
	// the currently persisted revision - another writer saved first
	// (Stage 13A task Part 7/41). The caller must reload and never
	// silently overwrite.
	ErrRevisionConflict = errors.New("visual design revision conflict")
	// ErrTooLarge means the serialized document exceeds
	// MaxDocumentBytes.
	ErrTooLarge = errors.New("visual design document is too large")
)
