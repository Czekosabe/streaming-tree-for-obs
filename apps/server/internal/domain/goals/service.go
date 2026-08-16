package goals

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// AccountLookup resolves whether an id names a real connected account or
// donation source - never the account's token, never its full record.
// Deliberately a narrow, primitive-typed interface, mirroring
// alerts.AccountLookup exactly (docs/goals-widgets.md §14).
type AccountLookup interface {
	AccountExists(ctx context.Context, accountID string) (bool, error)
}

// Service holds the goal and widget-profile use cases: CRUD (including
// public-slug rotation), manual current/reset actions, and the
// capability-driven filter-existence checks docs/goals-widgets.md §14
// requires. Never matches an event or applies a contribution itself -
// see internal/goals's runtime Manager for that (it calls
// Repository.ApplyContribution directly through this Service, §26).
type Service struct {
	repo     Repository
	accounts AccountLookup
	now      Clock

	newGoalID   func() (string, error)
	newWidgetID func() (string, error)
	newSlug     func() (string, error)
}

// NewService builds a Service. accounts may be nil only in a test that
// never exercises a non-empty account filter.
func NewService(repo Repository, accounts AccountLookup, now Clock) *Service {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{repo: repo, accounts: accounts, now: now, newGoalID: NewGoalID, newWidgetID: NewWidgetProfileID, newSlug: NewPublicSlug}
}

func mapRepoErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrGoalNotFound) || errors.Is(err, ErrWidgetProfileNotFound) ||
		errors.Is(err, ErrPublicSlugNotFound) || errors.Is(err, ErrAccountNotFound) ||
		errors.Is(err, ErrGoalInUse) || errors.Is(err, ErrConfigConflict) {
		return err
	}
	return fmt.Errorf("%w: %s", ErrStorage, err)
}

func (s *Service) validateAccounts(ctx context.Context, accounts []string) error {
	for _, id := range accounts {
		if s.accounts == nil {
			return fmt.Errorf("%w: %s", ErrAccountNotFound, id)
		}
		ok, err := s.accounts.AccountExists(ctx, id)
		if err != nil {
			return fmt.Errorf("%w: %s", ErrStorage, err)
		}
		if !ok {
			return fmt.Errorf("%w: %s", ErrAccountNotFound, id)
		}
	}
	return nil
}

// --- goals ---------------------------------------------------------------

// CreateGoal creates a new goal with safe, validated defaults plus the
// caller's chosen name/kind/target. baseline sets both Baseline and
// Current (docs/goals-widgets.md §1's worked example: an operator-known
// starting point, never fabricated).
func (s *Service) CreateGoal(ctx context.Context, draft Goal) (Goal, error) {
	draft.Enabled = true
	draft.Current = draft.Baseline
	if err := ValidateGoalFields(draft); err != nil {
		return Goal{}, err
	}
	if err := s.validateAccounts(ctx, draft.Accounts); err != nil {
		return Goal{}, err
	}

	id, err := s.newGoalID()
	if err != nil {
		return Goal{}, fmt.Errorf("%w: %s", ErrStorage, err)
	}
	now := s.now()
	draft.ID = id
	draft.CreatedAt = now
	draft.UpdatedAt = now
	draft.StartedAt = now
	draft.ConfigRevision = 1

	created, err := s.repo.CreateGoal(ctx, draft)
	if err != nil {
		return Goal{}, mapRepoErr(err)
	}
	return created, nil
}

func (s *Service) GetGoal(ctx context.Context, id string) (Goal, error) {
	g, ok, err := s.repo.GetGoal(ctx, id)
	if err != nil {
		return Goal{}, mapRepoErr(err)
	}
	if !ok {
		return Goal{}, ErrGoalNotFound
	}
	return g, nil
}

func (s *Service) ListGoals(ctx context.Context) ([]Goal, error) {
	list, err := s.repo.ListGoals(ctx)
	if err != nil {
		return nil, mapRepoErr(err)
	}
	return list, nil
}

// UpdateGoal replaces every editable field of the goal named by
// draft.ID, checked against draft.ConfigRevision (optimistic
// concurrency, docs/goals-widgets.md §8.1). When draft's own
// Baseline (and, for a monetary goal, Currency) differs from the
// currently stored goal, Current is reset to the new Baseline in the
// same call (§9.3) - a goal can never end up with a Current value whose
// currency provenance is ambiguous. Otherwise Current is left
// untouched.
func (s *Service) UpdateGoal(ctx context.Context, draft Goal) (Goal, error) {
	existing, ok, err := s.repo.GetGoal(ctx, draft.ID)
	if err != nil {
		return Goal{}, mapRepoErr(err)
	}
	if !ok {
		return Goal{}, ErrGoalNotFound
	}

	baselineChanged := draft.Baseline != existing.Baseline
	currencyChanged := draft.Currency != existing.Currency
	if baselineChanged || currencyChanged {
		draft.Current = draft.Baseline
	} else {
		draft.Current = existing.Current
	}

	if err := ValidateGoalFields(draft); err != nil {
		return Goal{}, err
	}
	if err := s.validateAccounts(ctx, draft.Accounts); err != nil {
		return Goal{}, err
	}

	draft.CreatedAt = existing.CreatedAt
	draft.UpdatedAt = s.now()
	if baselineChanged || currencyChanged {
		draft.StartedAt = draft.UpdatedAt
	} else {
		draft.StartedAt = existing.StartedAt
	}

	updated, err := s.repo.UpdateGoal(ctx, draft)
	if err != nil {
		return Goal{}, mapRepoErr(err)
	}
	return updated, nil
}

func (s *Service) DeleteGoal(ctx context.Context, id string) error {
	if err := s.repo.DeleteGoal(ctx, id); err != nil {
		return mapRepoErr(err)
	}
	return nil
}

// SetCurrent persists a manual "set current value" action (docs/goals-
// widgets.md §9.1).
func (s *Service) SetCurrent(ctx context.Context, id string, current int64) (Goal, error) {
	existing, ok, err := s.repo.GetGoal(ctx, id)
	if err != nil {
		return Goal{}, mapRepoErr(err)
	}
	if !ok {
		return Goal{}, ErrGoalNotFound
	}
	max := maxValueFor(existing.Kind)
	if current < 0 || current > max {
		return Goal{}, validationErr("current must be 0-%d", max)
	}
	updated, err := s.repo.SetCurrent(ctx, id, current)
	if err != nil {
		return Goal{}, mapRepoErr(err)
	}
	return updated, nil
}

// ResetProgress persists a manual "reset to baseline" action (docs/
// goals-widgets.md §9.2).
func (s *Service) ResetProgress(ctx context.Context, id string) (Goal, error) {
	updated, err := s.repo.ResetProgress(ctx, id)
	if err != nil {
		return Goal{}, mapRepoErr(err)
	}
	return updated, nil
}

// ApplyContribution is the Service-level passthrough the runtime
// Manager calls for every accepted, matching event (docs/goals-
// widgets.md §12, §26) - kept on Service rather than exposing Repository
// directly to internal/goals, exactly mirroring how internal/alerts only
// ever talks to domain.Service.
func (s *Service) ApplyContribution(ctx context.Context, goalID string, key AppliedEventKey, amount int64) (applied bool, updated Goal, err error) {
	applied, updated, err = s.repo.ApplyContribution(ctx, goalID, key, amount)
	if err != nil {
		return false, Goal{}, mapRepoErr(err)
	}
	return applied, updated, nil
}

// PruneAppliedEvents removes dedupe-ledger rows older than the 30-day
// retention bound (docs/goals-widgets.md §11.5).
func (s *Service) PruneAppliedEvents(ctx context.Context, olderThan time.Time) (int64, error) {
	n, err := s.repo.PruneAppliedEvents(ctx, olderThan)
	if err != nil {
		return 0, mapRepoErr(err)
	}
	return n, nil
}

// --- widget profiles -------------------------------------------------------

// CreateWidgetProfile creates a new widget profile for an existing goal.
func (s *Service) CreateWidgetProfile(ctx context.Context, draft WidgetProfile) (WidgetProfile, error) {
	if _, ok, err := s.repo.GetGoal(ctx, draft.GoalID); err != nil {
		return WidgetProfile{}, mapRepoErr(err)
	} else if !ok {
		return WidgetProfile{}, ErrGoalNotFound
	}
	if err := ValidateWidgetProfileFields(draft); err != nil {
		return WidgetProfile{}, err
	}

	id, err := s.newWidgetID()
	if err != nil {
		return WidgetProfile{}, fmt.Errorf("%w: %s", ErrStorage, err)
	}
	slug, err := s.newSlug()
	if err != nil {
		return WidgetProfile{}, fmt.Errorf("%w: %s", ErrStorage, err)
	}
	now := s.now()
	draft.ID = id
	draft.PublicSlug = slug
	draft.CreatedAt = now
	draft.UpdatedAt = now

	created, err := s.repo.CreateWidgetProfile(ctx, draft)
	if err != nil {
		return WidgetProfile{}, mapRepoErr(err)
	}
	return created, nil
}

func (s *Service) GetWidgetProfile(ctx context.Context, id string) (WidgetProfile, error) {
	p, ok, err := s.repo.GetWidgetProfile(ctx, id)
	if err != nil {
		return WidgetProfile{}, mapRepoErr(err)
	}
	if !ok {
		return WidgetProfile{}, ErrWidgetProfileNotFound
	}
	return p, nil
}

func (s *Service) GetWidgetProfileByPublicSlug(ctx context.Context, slug string) (WidgetProfile, error) {
	p, ok, err := s.repo.GetWidgetProfileByPublicSlug(ctx, slug)
	if err != nil {
		return WidgetProfile{}, mapRepoErr(err)
	}
	if !ok {
		return WidgetProfile{}, ErrPublicSlugNotFound
	}
	return p, nil
}

// ListWidgetProfiles lists every widget profile, or only those for
// goalID when non-empty.
func (s *Service) ListWidgetProfiles(ctx context.Context, goalID string) ([]WidgetProfile, error) {
	list, err := s.repo.ListWidgetProfiles(ctx, goalID)
	if err != nil {
		return nil, mapRepoErr(err)
	}
	return list, nil
}

// UpdateWidgetProfile replaces every editable field except GoalID,
// PublicSlug, and CreatedAt - a widget profile is created for exactly
// one goal and never reassigned to another.
func (s *Service) UpdateWidgetProfile(ctx context.Context, draft WidgetProfile) (WidgetProfile, error) {
	existing, ok, err := s.repo.GetWidgetProfile(ctx, draft.ID)
	if err != nil {
		return WidgetProfile{}, mapRepoErr(err)
	}
	if !ok {
		return WidgetProfile{}, ErrWidgetProfileNotFound
	}
	if err := ValidateWidgetProfileFields(draft); err != nil {
		return WidgetProfile{}, err
	}
	draft.GoalID = existing.GoalID
	draft.PublicSlug = existing.PublicSlug
	draft.CreatedAt = existing.CreatedAt
	draft.UpdatedAt = s.now()

	updated, err := s.repo.UpdateWidgetProfile(ctx, draft)
	if err != nil {
		return WidgetProfile{}, mapRepoErr(err)
	}
	return updated, nil
}

// RotatePublicSlug replaces a widget profile's public slug - the
// previous URL stops resolving immediately (docs/goals-widgets.md §19).
func (s *Service) RotatePublicSlug(ctx context.Context, id string) (WidgetProfile, error) {
	slug, err := s.newSlug()
	if err != nil {
		return WidgetProfile{}, fmt.Errorf("%w: %s", ErrStorage, err)
	}
	updated, err := s.repo.RotatePublicSlug(ctx, id, slug)
	if err != nil {
		return WidgetProfile{}, mapRepoErr(err)
	}
	return updated, nil
}

func (s *Service) DeleteWidgetProfile(ctx context.Context, id string) error {
	if err := s.repo.DeleteWidgetProfile(ctx, id); err != nil {
		return mapRepoErr(err)
	}
	return nil
}
