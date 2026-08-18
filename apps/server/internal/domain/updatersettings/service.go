package updatersettings

import (
	"context"
	"fmt"
	"time"
)

// Clock returns the current time; injected so tests are deterministic.
type Clock func() time.Time

// Service holds the updater-preferences use cases.
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

// Preferences returns the current preferences, or the documented
// default if none have ever been saved.
func (s *Service) Preferences(ctx context.Context) (Preferences, error) {
	p, found, err := s.repo.GetPreferences(ctx)
	if err != nil {
		return Preferences{}, mapRepoErr(err)
	}
	if !found {
		return Default(), nil
	}
	return p, nil
}

// ReplacePreferences writes a full replacement of the preferences row.
func (s *Service) ReplacePreferences(ctx context.Context, p Preferences) (Preferences, error) {
	saved, err := s.repo.SetPreferences(ctx, p, s.now())
	if err != nil {
		return Preferences{}, mapRepoErr(err)
	}
	return saved, nil
}
