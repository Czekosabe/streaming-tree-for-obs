package chatoverlay

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Clock returns the current time; injected so tests are deterministic.
type Clock func() time.Time

// Service holds the chat-overlay-profile use cases: create, read, update,
// delete, slug rotation, account selection, hidden users, blocked terms,
// and activity-type selection.
type Service struct {
	repo    Repository
	now     Clock
	newID   func() (string, error)
	newSlug func() (string, error)
}

// NewService builds a Service.
func NewService(repo Repository, now Clock) *Service {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{repo: repo, now: now, newID: NewID, newSlug: NewPublicSlug}
}

// mapRepoErr passes known domain sentinels through unchanged, so callers
// can still errors.Is match them - only a truly unexpected persistence
// failure is wrapped as ErrStorage. A repository never itself returns a
// *platform.ValidationError (all validation happens before the
// repository is ever called - see ValidateProfile/ValidateBlockedTerm's
// own call sites below), so there is nothing to special-case for that
// here.
func mapRepoErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrPublicSlugNotFound) ||
		errors.Is(err, ErrAccountNotFound) || errors.Is(err, ErrUserNotFound) || errors.Is(err, ErrTermNotFound) {
		return err
	}
	return fmt.Errorf("%w: %s", ErrStorage, err)
}

// CreateProfile creates a new overlay profile with safe, validated
// defaults plus the caller's chosen name.
func (s *Service) CreateProfile(ctx context.Context, name string) (Profile, error) {
	id, err := s.newID()
	if err != nil {
		return Profile{}, fmt.Errorf("%w: %s", ErrStorage, err)
	}
	slug, err := s.newSlug()
	if err != nil {
		return Profile{}, fmt.Errorf("%w: %s", ErrStorage, err)
	}

	p := Default(name)
	p.ID = id
	p.PublicSlug = slug

	if err := ValidateProfile(p); err != nil {
		return Profile{}, err
	}

	saved, err := s.repo.CreateProfile(ctx, p)
	if err != nil {
		return Profile{}, mapRepoErr(err)
	}
	return saved, nil
}

// GetProfile returns one profile by its management id.
func (s *Service) GetProfile(ctx context.Context, id string) (Profile, error) {
	p, found, err := s.repo.GetProfile(ctx, id)
	if err != nil {
		return Profile{}, mapRepoErr(err)
	}
	if !found {
		return Profile{}, ErrNotFound
	}
	return p, nil
}

// GetProfileByPublicSlug returns one profile by its current public slug -
// used by the public overlay endpoints.
func (s *Service) GetProfileByPublicSlug(ctx context.Context, slug string) (Profile, error) {
	p, found, err := s.repo.GetProfileByPublicSlug(ctx, slug)
	if err != nil {
		return Profile{}, mapRepoErr(err)
	}
	if !found {
		return Profile{}, ErrPublicSlugNotFound
	}
	return p, nil
}

// ListProfiles returns every overlay profile.
func (s *Service) ListProfiles(ctx context.Context) ([]Profile, error) {
	list, err := s.repo.ListProfiles(ctx)
	if err != nil {
		return nil, mapRepoErr(err)
	}
	return list, nil
}

// ReplaceProfile validates and stores a full replacement of one profile's
// editable settings - id, public slug, and created_at are never changed
// by this call.
func (s *Service) ReplaceProfile(ctx context.Context, p Profile) (Profile, error) {
	if err := ValidateProfile(p); err != nil {
		return Profile{}, err
	}
	saved, err := s.repo.UpdateProfile(ctx, p)
	if err != nil {
		return Profile{}, mapRepoErr(err)
	}
	return saved, nil
}

// DeleteProfile removes a profile and every related row.
func (s *Service) DeleteProfile(ctx context.Context, id string) error {
	if err := s.repo.DeleteProfile(ctx, id); err != nil {
		return mapRepoErr(err)
	}
	return nil
}

// RotatePublicSlug replaces one profile's public slug, invalidating its
// previous Browser Source URL immediately.
func (s *Service) RotatePublicSlug(ctx context.Context, id string) (Profile, error) {
	slug, err := s.newSlug()
	if err != nil {
		return Profile{}, fmt.Errorf("%w: %s", ErrStorage, err)
	}
	saved, err := s.repo.RotatePublicSlug(ctx, id, slug, s.now())
	if err != nil {
		return Profile{}, mapRepoErr(err)
	}
	return saved, nil
}

// Accounts returns the connected-account ids explicitly selected for one
// overlay.
func (s *Service) Accounts(ctx context.Context, overlayID string) ([]string, error) {
	list, err := s.repo.ListAccounts(ctx, overlayID)
	if err != nil {
		return nil, mapRepoErr(err)
	}
	return list, nil
}

// SetAccounts replaces the full selected-account set for one overlay.
func (s *Service) SetAccounts(ctx context.Context, overlayID string, accountIDs []string) error {
	if err := s.repo.SetAccounts(ctx, overlayID, accountIDs); err != nil {
		return mapRepoErr(err)
	}
	return nil
}

// HiddenUsers returns every user hidden from one overlay's public output.
func (s *Service) HiddenUsers(ctx context.Context, overlayID string) ([]HiddenUser, error) {
	list, err := s.repo.ListHiddenUsers(ctx, overlayID)
	if err != nil {
		return nil, mapRepoErr(err)
	}
	return list, nil
}

// HideUser adds one user to one overlay's hidden list, idempotently.
func (s *Service) HideUser(ctx context.Context, overlayID string, providerID ProviderID, connectedAccountID, providerUserID, label string) (HiddenUser, error) {
	saved, err := s.repo.AddHiddenUser(ctx, HiddenUser{
		OverlayID: overlayID, ProviderID: providerID, ConnectedAccountID: connectedAccountID,
		ProviderUserID: providerUserID, Label: label,
	}, s.now())
	if err != nil {
		return HiddenUser{}, mapRepoErr(err)
	}
	return saved, nil
}

// UnhideUser removes one hidden-user entry.
func (s *Service) UnhideUser(ctx context.Context, overlayID string, providerID ProviderID, connectedAccountID, providerUserID string) error {
	if err := s.repo.RemoveHiddenUser(ctx, overlayID, providerID, connectedAccountID, providerUserID); err != nil {
		return mapRepoErr(err)
	}
	return nil
}

// BlockedTerms returns every blocked term for one overlay.
func (s *Service) BlockedTerms(ctx context.Context, overlayID string) ([]BlockedTerm, error) {
	list, err := s.repo.ListBlockedTerms(ctx, overlayID)
	if err != nil {
		return nil, mapRepoErr(err)
	}
	return list, nil
}

// AddBlockedTerm validates and adds one term to one overlay, idempotently.
func (s *Service) AddBlockedTerm(ctx context.Context, overlayID, value string, mode MatchMode) (BlockedTerm, error) {
	if err := ValidateBlockedTerm(value, mode); err != nil {
		return BlockedTerm{}, err
	}

	existing, err := s.repo.ListBlockedTerms(ctx, overlayID)
	if err != nil {
		return BlockedTerm{}, mapRepoErr(err)
	}
	if len(existing) >= MaxBlockedTermsPerOverlay {
		already := false
		normalized := NormalizeTerm(value)
		for _, t := range existing {
			if NormalizeTerm(t.Value) == normalized {
				already = true
				break
			}
		}
		if !already {
			return BlockedTerm{}, fmt.Errorf("%w: this overlay already has the maximum of %d blocked terms",
				ErrStorage, MaxBlockedTermsPerOverlay)
		}
	}

	id, err := NewTermID()
	if err != nil {
		return BlockedTerm{}, fmt.Errorf("%w: %s", ErrStorage, err)
	}

	saved, err := s.repo.AddBlockedTerm(ctx, BlockedTerm{
		ID: id, OverlayID: overlayID, Value: value, MatchMode: mode,
	}, s.now())
	if err != nil {
		return BlockedTerm{}, mapRepoErr(err)
	}
	return saved, nil
}

// RemoveBlockedTerm removes one blocked-term entry.
func (s *Service) RemoveBlockedTerm(ctx context.Context, overlayID, id string) error {
	if err := s.repo.RemoveBlockedTerm(ctx, overlayID, id); err != nil {
		return mapRepoErr(err)
	}
	return nil
}

// ActivityTypes returns the activity types explicitly selected for one
// overlay.
func (s *Service) ActivityTypes(ctx context.Context, overlayID string) ([]string, error) {
	list, err := s.repo.ListActivityTypes(ctx, overlayID)
	if err != nil {
		return nil, mapRepoErr(err)
	}
	return list, nil
}

// SetActivityTypes replaces the full activity-type selection for one
// overlay.
func (s *Service) SetActivityTypes(ctx context.Context, overlayID string, activityTypes []string) error {
	if err := s.repo.SetActivityTypes(ctx, overlayID, activityTypes); err != nil {
		return mapRepoErr(err)
	}
	return nil
}
