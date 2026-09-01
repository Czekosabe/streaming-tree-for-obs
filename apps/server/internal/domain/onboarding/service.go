package onboarding

import (
	"context"
	"fmt"
	"time"
)

// Clock returns the current time; injected so tests are deterministic.
type Clock func() time.Time

// Service holds the onboarding-state use cases.
type Service struct {
	repo Repository
	now  Clock
}

// NewService builds a Service.
func NewService(repo Repository, now Clock) *Service {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{repo: repo, now: now}
}

func mapRepoErr(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrStorage, err)
}

// State returns the current onboarding state, or the documented default
// if none has ever been saved (only possible on a database that predates
// the Stage 21 migration and has not yet applied it - see Repository.
// GetState's own doc comment).
func (s *Service) State(ctx context.Context) (State, error) {
	st, found, err := s.repo.GetState(ctx)
	if err != nil {
		return State{}, mapRepoErr(err)
	}
	if !found {
		return Default(), nil
	}
	return st, nil
}

// SetStatus records a new onboarding status, always at the current
// schema version - Stage 21 never writes an older version, since there is
// only one today. Returns ErrInvalidStatus for anything other than
// Status.Valid().
func (s *Service) SetStatus(ctx context.Context, status Status) (State, error) {
	if !status.Valid() {
		return State{}, fmt.Errorf("%w: %q", ErrInvalidStatus, status)
	}
	saved, err := s.repo.SetStatus(ctx, status, CurrentSchemaVersion, s.now())
	if err != nil {
		return State{}, mapRepoErr(err)
	}
	return saved, nil
}
