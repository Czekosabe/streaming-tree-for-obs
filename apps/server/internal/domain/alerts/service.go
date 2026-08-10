package alerts

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Clock returns the current time; injected so tests are deterministic.
type Clock func() time.Time

// AccountLookup resolves facts about a connected account needed to
// validate an explicit rule account-filter entry - never the account's
// token, never its full record. Deliberately a narrow, primitive-typed
// interface, mirroring internal/domain/chatautomation's own
// AccountLookup.
type AccountLookup interface {
	// AccountExists reports whether accountID names a real connected
	// account.
	AccountExists(ctx context.Context, accountID string) (bool, error)
}

// Service holds the alert-profile and alert-rule use cases: profile
// CRUD (including public-slug rotation) and rule CRUD, with the
// capability-driven condition validation and account-filter existence
// checks the Stage 12A task requires. Never matches an event or runs a
// queue itself - see internal/alerts's runtime Manager for that.
type Service struct {
	repo     Repository
	accounts AccountLookup
	now      Clock
	newID    func() (string, error)
	newSlug  func() (string, error)
}

// NewService builds a Service.
func NewService(repo Repository, accounts AccountLookup, now Clock) *Service {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{repo: repo, accounts: accounts, now: now, newID: NewProfileID, newSlug: NewPublicSlug}
}

func mapRepoErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrProfileNotFound) || errors.Is(err, ErrPublicSlugNotFound) ||
		errors.Is(err, ErrRuleNotFound) || errors.Is(err, ErrAccountNotFound) {
		return err
	}
	return fmt.Errorf("%w: %s", ErrStorage, err)
}

// --- profiles -----------------------------------------------------------

// CreateProfile creates a new alert profile with safe, validated
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
	p := DefaultProfile(name)
	p.ID = id
	p.PublicSlug = slug
	if err := ValidateProfileFields(p); err != nil {
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
		return Profile{}, ErrProfileNotFound
	}
	return p, nil
}

// GetProfileByPublicSlug returns one profile by its current public slug -
// used by the public alert endpoints.
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

// ListProfiles returns every alert profile.
func (s *Service) ListProfiles(ctx context.Context) ([]Profile, error) {
	list, err := s.repo.ListProfiles(ctx)
	if err != nil {
		return nil, mapRepoErr(err)
	}
	return list, nil
}

// ProfileInput carries a profile's editable fields.
type ProfileInput struct {
	Name                   string
	Enabled                bool
	Language               Language
	Theme                  Theme
	Position               Position
	TextAlign              TextAlign
	MaxQueueItems          int
	MaximumQueueAgeSeconds int
}

// ReplaceProfile validates and stores a full replacement of one
// profile's editable settings - id, public slug, and created_at are
// never changed by this call.
func (s *Service) ReplaceProfile(ctx context.Context, id string, in ProfileInput) (Profile, error) {
	existing, found, err := s.repo.GetProfile(ctx, id)
	if err != nil {
		return Profile{}, mapRepoErr(err)
	}
	if !found {
		return Profile{}, ErrProfileNotFound
	}
	p := Profile{
		ID: id, PublicSlug: existing.PublicSlug, CreatedAt: existing.CreatedAt,
		Name: in.Name, Enabled: in.Enabled, Language: in.Language,
		Theme: in.Theme, Position: in.Position, TextAlign: in.TextAlign,
		MaxQueueItems: in.MaxQueueItems, MaximumQueueAgeSeconds: in.MaximumQueueAgeSeconds,
	}
	if err := ValidateProfileFields(p); err != nil {
		return Profile{}, err
	}
	saved, err := s.repo.UpdateProfile(ctx, p)
	if err != nil {
		return Profile{}, mapRepoErr(err)
	}
	return saved, nil
}

// RotatePublicSlug replaces a profile's public slug with a freshly
// generated one, invalidating the previous Browser Source URL
// immediately.
func (s *Service) RotatePublicSlug(ctx context.Context, id string) (Profile, error) {
	if _, found, err := s.repo.GetProfile(ctx, id); err != nil {
		return Profile{}, mapRepoErr(err)
	} else if !found {
		return Profile{}, ErrProfileNotFound
	}
	slug, err := s.newSlug()
	if err != nil {
		return Profile{}, fmt.Errorf("%w: %s", ErrStorage, err)
	}
	saved, err := s.repo.RotatePublicSlug(ctx, id, slug)
	if err != nil {
		return Profile{}, mapRepoErr(err)
	}
	return saved, nil
}

// DeleteProfile removes a profile and every rule/filter belonging to it.
func (s *Service) DeleteProfile(ctx context.Context, id string) error {
	if err := s.repo.DeleteProfile(ctx, id); err != nil {
		return mapRepoErr(err)
	}
	return nil
}

// --- rules ----------------------------------------------------------------

// RuleInput carries a rule's editable fields, before persistence
// identifiers exist.
type RuleInput struct {
	Name      string
	Enabled   bool
	EventType EventType

	Priority   int
	DurationMS int

	MinimumQuantity *int64
	MaximumQuantity *int64

	RequiredRole Role

	ShowPlatform bool
	ShowUsername bool
	ShowMessage  bool
	ShowQuantity bool

	TextTemplate string

	EntryAnimation      Animation
	ExitAnimation       Animation
	AnimationDurationMS int

	Providers []ProviderID
	Accounts  []string
}

func (s *Service) validateRuleInput(ctx context.Context, profileID string, in RuleInput) error {
	r := Rule{
		ProfileID: profileID, Name: in.Name, EventType: in.EventType,
		Priority: in.Priority, DurationMS: in.DurationMS,
		MinimumQuantity: in.MinimumQuantity, MaximumQuantity: in.MaximumQuantity,
		RequiredRole: in.RequiredRole,
		ShowPlatform: in.ShowPlatform, ShowUsername: in.ShowUsername, ShowMessage: in.ShowMessage, ShowQuantity: in.ShowQuantity,
		TextTemplate: in.TextTemplate, EntryAnimation: in.EntryAnimation, ExitAnimation: in.ExitAnimation,
		AnimationDurationMS: in.AnimationDurationMS, Providers: in.Providers, Accounts: in.Accounts,
	}
	if err := ValidateRuleFields(r); err != nil {
		return err
	}
	for _, accountID := range in.Accounts {
		exists, err := s.accounts.AccountExists(ctx, accountID)
		if err != nil {
			return fmt.Errorf("%w: %s", ErrStorage, err)
		}
		if !exists {
			return ErrAccountNotFound
		}
	}
	return nil
}

// CreateRule validates and persists a new alert-rule definition,
// belonging to profileID.
func (s *Service) CreateRule(ctx context.Context, profileID string, in RuleInput) (Rule, error) {
	if _, found, err := s.repo.GetProfile(ctx, profileID); err != nil {
		return Rule{}, mapRepoErr(err)
	} else if !found {
		return Rule{}, ErrProfileNotFound
	}
	if err := s.validateRuleInput(ctx, profileID, in); err != nil {
		return Rule{}, err
	}
	id, err := NewRuleID()
	if err != nil {
		return Rule{}, fmt.Errorf("%w: %s", ErrStorage, err)
	}
	r := ruleFromInput(id, profileID, in)
	saved, err := s.repo.CreateRule(ctx, r)
	if err != nil {
		return Rule{}, mapRepoErr(err)
	}
	return saved, nil
}

func ruleFromInput(id, profileID string, in RuleInput) Rule {
	return Rule{
		ID: id, ProfileID: profileID, Name: in.Name, Enabled: in.Enabled, EventType: in.EventType,
		Priority: in.Priority, DurationMS: in.DurationMS,
		MinimumQuantity: in.MinimumQuantity, MaximumQuantity: in.MaximumQuantity,
		RequiredRole: in.RequiredRole,
		ShowPlatform: in.ShowPlatform, ShowUsername: in.ShowUsername, ShowMessage: in.ShowMessage, ShowQuantity: in.ShowQuantity,
		TextTemplate: in.TextTemplate, EntryAnimation: in.EntryAnimation, ExitAnimation: in.ExitAnimation,
		AnimationDurationMS: in.AnimationDurationMS, Providers: in.Providers, Accounts: in.Accounts,
	}
}

// GetRule returns one rule by id.
func (s *Service) GetRule(ctx context.Context, id string) (Rule, error) {
	r, found, err := s.repo.GetRule(ctx, id)
	if err != nil {
		return Rule{}, mapRepoErr(err)
	}
	if !found {
		return Rule{}, ErrRuleNotFound
	}
	return r, nil
}

// ListRules returns every rule belonging to profileID.
func (s *Service) ListRules(ctx context.Context, profileID string) ([]Rule, error) {
	list, err := s.repo.ListRules(ctx, profileID)
	if err != nil {
		return nil, mapRepoErr(err)
	}
	return list, nil
}

// ReplaceRule validates and stores a full replacement of one rule's
// editable fields and filter sets.
func (s *Service) ReplaceRule(ctx context.Context, id string, in RuleInput) (Rule, error) {
	existing, found, err := s.repo.GetRule(ctx, id)
	if err != nil {
		return Rule{}, mapRepoErr(err)
	}
	if !found {
		return Rule{}, ErrRuleNotFound
	}
	if err := s.validateRuleInput(ctx, existing.ProfileID, in); err != nil {
		return Rule{}, err
	}
	r := ruleFromInput(id, existing.ProfileID, in)
	saved, err := s.repo.UpdateRule(ctx, r)
	if err != nil {
		return Rule{}, mapRepoErr(err)
	}
	return saved, nil
}

// DeleteRule removes a rule and its filters.
func (s *Service) DeleteRule(ctx context.Context, id string) error {
	if err := s.repo.DeleteRule(ctx, id); err != nil {
		return mapRepoErr(err)
	}
	return nil
}

// OverlapWarning names two enabled rules, in the same profile and of
// the same event type, whose quantity ranges overlap - the Stage 12A
// task's own Part 8 "simpler policy" requires surfacing this as a
// management-only warning rather than silently suppressing one rule or
// defining a precedence order. Computed on read, never persisted.
type OverlapWarning struct {
	RuleID      string
	OtherRuleID string
	EventType   EventType
}

// OverlapWarnings returns every pair of enabled rules in profileID whose
// quantity ranges overlap for the same event type. Each unordered pair
// is reported once.
func (s *Service) OverlapWarnings(ctx context.Context, profileID string) ([]OverlapWarning, error) {
	rules, err := s.repo.ListRules(ctx, profileID)
	if err != nil {
		return nil, mapRepoErr(err)
	}
	var warnings []OverlapWarning
	for i := 0; i < len(rules); i++ {
		a := rules[i]
		if !a.Enabled {
			continue
		}
		for j := i + 1; j < len(rules); j++ {
			b := rules[j]
			if !b.Enabled || a.EventType != b.EventType {
				continue
			}
			if rangesOverlap(a.MinimumQuantity, a.MaximumQuantity, b.MinimumQuantity, b.MaximumQuantity) {
				warnings = append(warnings, OverlapWarning{RuleID: a.ID, OtherRuleID: b.ID, EventType: a.EventType})
			}
		}
	}
	return warnings, nil
}

// rangesOverlap reports whether two inclusive, possibly-unbounded
// [min,max] integer ranges intersect.
func rangesOverlap(aMin, aMax, bMin, bMax *int64) bool {
	// a starts after b ends -> no overlap.
	if aMin != nil && bMax != nil && *aMin > *bMax {
		return false
	}
	// b starts after a ends -> no overlap.
	if bMin != nil && aMax != nil && *bMin > *aMax {
		return false
	}
	return true
}
