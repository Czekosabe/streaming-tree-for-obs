package goals

import "errors"

var (
	// ErrStorage wraps any unexpected persistence failure.
	ErrStorage = errors.New("storage failure")

	// ErrGoalNotFound means the referenced goal does not exist.
	ErrGoalNotFound = errors.New("goal not found")
	// ErrWidgetProfileNotFound means the referenced widget profile does
	// not exist.
	ErrWidgetProfileNotFound = errors.New("goal widget profile not found")
	// ErrPublicSlugNotFound means no widget profile currently has the
	// given public slug.
	ErrPublicSlugNotFound = errors.New("goal widget profile public slug not found")

	// ErrAccountNotFound means a goal's account/source filter names an
	// id that is neither a connected account nor a donation source.
	ErrAccountNotFound = errors.New("connected account or donation source not found")

	// ErrValidation wraps a semantic validation failure (bounds,
	// required fields, enum values) - see validation.go for the exact
	// rules.
	ErrValidation = errors.New("goal validation failed")

	// ErrCurrencyMismatch means a manual set-current request's implied
	// currency does not match the goal's own configured currency (docs/
	// goals-widgets.md §9.1).
	ErrCurrencyMismatch = errors.New("goal currency mismatch")

	// ErrGoalInUse means a goal cannot be deleted because one or more
	// widget profiles still reference it (docs/goals-widgets.md §18 -
	// explicit rejection, never a silent cascade).
	ErrGoalInUse = errors.New("goal is referenced by one or more widget profiles")

	// ErrConfigConflict means a PUT /api/goals/{id} request's
	// ConfigRevision did not match the currently stored value (docs/
	// goals-widgets.md §8.1 - optimistic concurrency for configuration
	// edits only, never for contribution application).
	ErrConfigConflict = errors.New("goal configuration was changed by someone else")
)
