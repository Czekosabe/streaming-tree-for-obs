package engagementsettings

import (
	"context"
	"fmt"
	"time"
)

// Clock returns the current time; injected so tests are deterministic.
type Clock func() time.Time

// Service holds the engagement-settings use cases: read, set enabled/
// disabled, delete, and list every enabled account for startup.
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

// Get returns one account's settings. A never-configured account reports
// found=false, which callers treat identically to an explicit Enabled:
// false row - the default is disabled either way.
func (s *Service) Get(ctx context.Context, accountID string) (Settings, bool, error) {
	settings, found, err := s.repo.Get(ctx, accountID)
	if err != nil {
		return Settings{}, false, mapRepoErr(err)
	}
	return settings, found, nil
}

// SetEnabled creates or updates one account's enabled preference.
//
// Whether accountID is actually a Twitch account (the only provider allowed
// to enable engagement in Stage 8A) is validated by the caller
// (internal/httpapi), which already has the account record in hand - this
// package deliberately does not import internal/domain/account, mirroring
// internal/domain/remotetarget's own reasoning.
func (s *Service) SetEnabled(ctx context.Context, accountID string, enabled bool) (Settings, error) {
	saved, err := s.repo.Set(ctx, Settings{AccountID: accountID, Enabled: enabled}, s.now())
	if err != nil {
		return Settings{}, mapRepoErr(err)
	}
	return saved, nil
}

// Delete removes one account's settings explicitly.
func (s *Service) Delete(ctx context.Context, accountID string) error {
	if err := s.repo.Delete(ctx, accountID); err != nil {
		return mapRepoErr(err)
	}
	return nil
}

// ListEnabled returns every account currently configured enabled.
func (s *Service) ListEnabled(ctx context.Context) ([]Settings, error) {
	settings, err := s.repo.ListEnabled(ctx)
	if err != nil {
		return nil, mapRepoErr(err)
	}
	return settings, nil
}
