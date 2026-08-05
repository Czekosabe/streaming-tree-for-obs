package remotetarget

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Clock returns the current time; injected so tests are deterministic.
type Clock func() time.Time

// Service holds the remote-target use cases: get, set (with a provider-
// match check), and delete.
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
	if errors.Is(err, ErrNotFound) {
		return err
	}
	return fmt.Errorf("%w: %s", ErrStorage, err)
}

// GetTarget returns the target set for a platform, if any.
func (s *Service) GetTarget(ctx context.Context, platformID string) (Target, bool, error) {
	target, found, err := s.repo.Get(ctx, platformID)
	if err != nil {
		return Target{}, false, mapRepoErr(err)
	}
	return target, found, nil
}

// SetTarget creates or replaces a platform's remote target.
//
// platformProviderID is the destination's own provider identifier, resolved
// by the caller (internal/httpapi, which also rejects any provider other
// than YouTube before ever reaching here) - this package deliberately does
// not depend on internal/domain/platform, mirroring internal/domain/
// account.Service.LinkPlatform's own reasoning.
func (s *Service) SetTarget(ctx context.Context, platformID, platformProviderID, resourceType, resourceID, displayName string) (Target, error) {
	target := Target{
		PlatformID: platformID, ProviderID: platformProviderID,
		ResourceType: resourceType, ResourceID: resourceID, DisplayName: displayName,
	}
	saved, err := s.repo.Set(ctx, target, s.now())
	if err != nil {
		return Target{}, mapRepoErr(err)
	}
	return saved, nil
}

// DeleteTarget removes a platform's remote target.
func (s *Service) DeleteTarget(ctx context.Context, platformID string) error {
	if err := s.repo.Delete(ctx, platformID); err != nil {
		return mapRepoErr(err)
	}
	return nil
}
