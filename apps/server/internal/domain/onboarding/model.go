// Package onboarding holds the one persisted Stage 21 first-run
// onboarding preference: whether the operator has completed, explicitly
// skipped, or never engaged the first-run setup assistant
// (docs/onboarding.md §4). Deliberately minimal, mirroring
// internal/domain/updatersettings's own singleton-row reasoning - this is
// a UI-flow preference only, never a secret, never a machine/installation
// id, never a step-by-step history.
package onboarding

import "time"

// Status is the onboarding flow's own state - one value, not independent
// "completed"/"dismissed" booleans, matching this codebase's repeated
// pattern (mediamtx.ProcessState, branch.State, mediamtx.IngestState) of
// making an impossible combination unrepresentable.
type Status string

const (
	// StatusPending means the operator has never completed or dismissed
	// the flow - the frontend auto-shows it once.
	StatusPending Status = "pending"
	// StatusCompleted means the operator finished the flow.
	StatusCompleted Status = "completed"
	// StatusDismissed means the operator explicitly skipped the flow, or
	// this row was seeded for an existing (pre-Stage-21) installation by
	// the migration's own existing-user rule (docs/onboarding.md §4.3) -
	// both cases mean the same thing going forward: available on request,
	// never auto-shown.
	StatusDismissed Status = "dismissed"
)

// Valid reports whether s is one of the three known status values.
func (s Status) Valid() bool {
	switch s {
	case StatusPending, StatusCompleted, StatusDismissed:
		return true
	default:
		return false
	}
}

// CurrentSchemaVersion is the current onboarding flow's own revision. A
// future major revision that deliberately wants to re-offer itself to
// already-onboarded operators (without nagging on an ordinary feature
// addition) would migrate rows with an older SchemaVersion back to
// StatusPending - not exercised today, since there is only one version.
const CurrentSchemaVersion = 1

// State is the singleton onboarding state row.
type State struct {
	Status        Status
	SchemaVersion int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Default returns the out-of-the-box state for a database where the
// migration's own existing-user rule found no prior real usage: pending,
// at the current schema version.
func Default() State {
	return State{Status: StatusPending, SchemaVersion: CurrentSchemaVersion}
}
