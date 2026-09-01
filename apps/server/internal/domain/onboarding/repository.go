package onboarding

import (
	"context"
	"time"
)

// Repository is the persistence port for the singleton onboarding state
// row.
type Repository interface {
	// GetState returns the singleton state row. found is false when no
	// row has ever been written - callers treat that identically to
	// Default(). In practice the Stage 21 migration always seeds a row
	// (docs/onboarding.md §4.3), so found is false only for a database
	// that predates that migration and has not yet been migrated - never
	// a state this Service itself needs to reason about differently.
	GetState(ctx context.Context) (State, bool, error)

	// SetStatus replaces the singleton row's status in full, preserving
	// CreatedAt if a row already exists.
	SetStatus(ctx context.Context, status Status, schemaVersion int, now time.Time) (State, error)
}
