package visualtemplate

import "errors"

var (
	// ErrStorage wraps any unexpected persistence failure.
	ErrStorage = errors.New("visual template storage failure")
	// ErrNotFound means no template exists for the given id (built-in
	// or user).
	ErrNotFound = errors.New("visual template not found")
	// ErrValidation wraps any semantic template validation failure -
	// see validation.go for the exact rules.
	ErrValidation = errors.New("visual template validation failed")
	// ErrImmutable means a caller tried to rename, delete, or replace a
	// built-in template - built-ins are application-owned and never
	// mutate (Stage 14A task Part 18/29).
	ErrImmutable = errors.New("visual template is immutable")
	// ErrTargetMismatch means a template's own Target does not match
	// the target it was asked to be used/compatible with.
	ErrTargetMismatch = errors.New("visual template target mismatch")
	// ErrUnsupportedTemplateVersion means a template file's own
	// top-level schemaVersion is not CurrentTemplateSchemaVersion.
	ErrUnsupportedTemplateVersion = errors.New("visual template schema version is not supported")
	// ErrUnsupportedDesignVersion means the embedded visual-design
	// document's own version could not be migrated to
	// visualdesign.CurrentVersion (unknown/future/malformed version).
	ErrUnsupportedDesignVersion = errors.New("embedded visual design version is not supported")
	// ErrTooLarge means a raw imported template file exceeded
	// MaxImportBytes.
	ErrTooLarge = errors.New("visual template import is too large")
)
