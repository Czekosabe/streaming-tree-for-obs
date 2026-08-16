package goals

import (
	"context"
	"time"
)

// AppliedEventKey identifies one durable dedupe-ledger entry (docs/
// goals-widgets.md §11.2) - scoped per goal, never globally (§11.4).
type AppliedEventKey struct {
	ProviderID       ProviderID
	AccountID        string
	ProviderEventKey string
}

// Repository is the persistence port for goals, their provider/account
// filters, the durable contribution dedupe ledger, and widget profiles.
// Every write that touches more than one table is atomic - a caller
// never observes a goal with a provider filter row but no canonical
// row, and never observes a dedupe-ledger row without its matching
// Current increment or vice versa (docs/goals-widgets.md §12).
type Repository interface {
	CreateGoal(ctx context.Context, g Goal) (Goal, error)
	GetGoal(ctx context.Context, id string) (Goal, bool, error)
	ListGoals(ctx context.Context) ([]Goal, error)
	// UpdateGoal replaces every editable field and the full provider/
	// account filter sets - never a partial patch. id, CreatedAt, and
	// Current are unchanged by this call unless the caller (see
	// service.go's reconfigure logic) explicitly sets a new Current
	// itself. Fails with ErrConfigConflict if g.ConfigRevision does not
	// match the currently stored value; the returned Goal carries the
	// freshly bumped revision on success.
	UpdateGoal(ctx context.Context, g Goal) (Goal, error)
	// DeleteGoal removes a goal and its filters. Fails with
	// ErrGoalInUse if one or more widget profiles still reference it
	// (docs/goals-widgets.md §18 - explicit rejection, never cascade).
	DeleteGoal(ctx context.Context, id string) error

	// SetCurrent persists a manual "set current value" action (docs/
	// goals-widgets.md §9.1). Updates UpdatedAt only - never
	// ConfigRevision.
	SetCurrent(ctx context.Context, id string, current int64) (Goal, error)
	// ResetProgress sets Current back to the goal's own Baseline and
	// refreshes StartedAt (docs/goals-widgets.md §9.2).
	ResetProgress(ctx context.Context, id string) (Goal, error)

	// ApplyContribution atomically claims key's dedupe identity for
	// goalID and increments Current by amount in one transaction
	// (docs/goals-widgets.md §12). applied is false, with no error and
	// an unchanged Goal, when key was already applied for this exact
	// goal - not an error condition, the normal outcome of a duplicate
	// delivery.
	ApplyContribution(ctx context.Context, goalID string, key AppliedEventKey, amount int64) (applied bool, updated Goal, err error)
	// PruneAppliedEvents deletes ledger rows older than olderThan
	// (docs/goals-widgets.md §11.5) and reports how many were removed.
	PruneAppliedEvents(ctx context.Context, olderThan time.Time) (int64, error)

	CreateWidgetProfile(ctx context.Context, p WidgetProfile) (WidgetProfile, error)
	GetWidgetProfile(ctx context.Context, id string) (WidgetProfile, bool, error)
	GetWidgetProfileByPublicSlug(ctx context.Context, slug string) (WidgetProfile, bool, error)
	// ListWidgetProfiles lists every widget profile, or only those for
	// goalID when non-empty.
	ListWidgetProfiles(ctx context.Context, goalID string) ([]WidgetProfile, error)
	// UpdateWidgetProfile replaces every editable field except GoalID,
	// PublicSlug, and CreatedAt - see RotatePublicSlug for the only way
	// PublicSlug ever changes, and see service.go for why GoalID never
	// changes after creation (a widget profile is created for exactly
	// one goal).
	UpdateWidgetProfile(ctx context.Context, p WidgetProfile) (WidgetProfile, error)
	RotatePublicSlug(ctx context.Context, id, newSlug string) (WidgetProfile, error)
	DeleteWidgetProfile(ctx context.Context, id string) error
}
