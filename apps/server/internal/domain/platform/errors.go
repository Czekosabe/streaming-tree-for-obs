package platform

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Sentinel domain errors. Handlers map these to HTTP status codes; raw storage
// errors (including SQLite messages) never reach the API surface.
var (
	// ErrNotFound is returned when a configured platform does not exist.
	ErrNotFound = errors.New("platform not found")

	// ErrUnknownProvider is returned for a provider identifier that is not in
	// the built-in registry.
	ErrUnknownProvider = errors.New("unknown provider")

	// ErrConflict is returned when a write cannot be applied because it would
	// violate a uniqueness or consistency rule.
	ErrConflict = errors.New("conflict")

	// ErrStorage wraps any unexpected persistence failure. The underlying
	// driver error is kept for the logs but must not be returned to a client.
	ErrStorage = errors.New("storage failure")
)

// Validation rule identifiers. These are stable and are what the frontend
// localizes; the English message travelling next to them is only a fallback.
const (
	RuleRequired     = "required"
	RuleTooLong      = "too_long"
	RuleTooShort     = "too_short"
	RuleTooMany      = "too_many"
	RuleUnsupported  = "unsupported"
	RuleNotSupported = "not_supported_by_provider"
	RuleDuplicate    = "duplicate"
	RuleInvalid      = "invalid"
)

// FieldViolation is one failed validation rule on one field.
type FieldViolation struct {
	// Field is the API field path, e.g. "title" or "tags".
	Field string
	// Rule is a stable identifier the frontend can localize.
	Rule string
	// Message is an English fallback for clients without a mapping.
	Message string
	// Params carries the numbers a localized message needs, e.g. {"max": 140}.
	Params map[string]any
}

// ValidationError collects every violation found in one request, so the client
// can show all of them at once instead of discovering them one save at a time.
type ValidationError struct {
	Violations []FieldViolation
}

func (e *ValidationError) Error() string {
	if len(e.Violations) == 0 {
		return "validation failed"
	}
	parts := make([]string, 0, len(e.Violations))
	for _, v := range e.Violations {
		parts = append(parts, v.Field+": "+v.Message)
	}
	sort.Strings(parts)
	return "validation failed - " + strings.Join(parts, "; ")
}

// Add appends a violation.
func (e *ValidationError) Add(field, rule, message string, params map[string]any) {
	e.Violations = append(e.Violations, FieldViolation{
		Field:   field,
		Rule:    rule,
		Message: message,
		Params:  params,
	})
}

// Addf appends a violation with a formatted English message.
func (e *ValidationError) Addf(field, rule string, params map[string]any, format string, args ...any) {
	e.Add(field, rule, fmt.Sprintf(format, args...), params)
}

// HasViolations reports whether anything failed.
func (e *ValidationError) HasViolations() bool {
	return len(e.Violations) > 0
}

// OrNil returns the error only when something actually failed, so callers can
// write `return v.OrNil()` without checking first.
func (e *ValidationError) OrNil() error {
	if e.HasViolations() {
		return e
	}
	return nil
}

// AsValidationError extracts a *ValidationError from an error chain.
func AsValidationError(err error) (*ValidationError, bool) {
	var target *ValidationError
	if errors.As(err, &target) {
		return target, true
	}
	return nil, false
}
