package audio

import (
	"context"
	"fmt"
	"time"
)

// Clock returns the current time; injected so tests are deterministic.
type Clock func() time.Time

// Options constructs a Service.
type Options struct {
	Repository Repository
	Now        Clock
}

// Service holds the Stage 17A settings use cases: read-with-defaults,
// full-replacement update, and public-slug rotation. Never touches
// runtime queue/cooldown/provider state - see internal/audio for that.
type Service struct {
	repo    Repository
	now     Clock
	newSlug func() (string, error)
}

// NewService builds a Service.
func NewService(opts Options) *Service {
	now := opts.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{repo: opts.Repository, now: now, newSlug: NewPublicSlug}
}

// Get returns the current settings, seeding the singleton row (with a
// freshly generated public slug) on the very first call if none exists
// yet - mirrors operatorchatprefs's own "absent row means Default()"
// convention, except the public slug specifically must be persisted
// once assigned rather than regenerated on every read.
func (s *Service) Get(ctx context.Context) (Settings, error) {
	existing, found, err := s.repo.GetSettings(ctx)
	if err != nil {
		return Settings{}, mapRepoErr(err)
	}
	if found {
		return existing, nil
	}

	def := Default()
	slug, err := s.newSlug()
	if err != nil {
		return Settings{}, fmt.Errorf("%w: %s", ErrStorage, err)
	}
	def.PublicSlug = slug
	now := s.now()
	def.CreatedAt = now
	def.UpdatedAt = now
	saved, err := s.repo.SetSettings(ctx, def, now)
	if err != nil {
		return Settings{}, mapRepoErr(err)
	}
	return saved, nil
}

// Update validates and replaces every configuration field in full. The
// public slug and creation time are never touched by this method - use
// RotatePublicSlug for the slug, and CreatedAt is otherwise immutable.
func (s *Service) Update(ctx context.Context, input Settings) (Settings, error) {
	if err := ValidateSettings(input); err != nil {
		return Settings{}, err
	}
	current, err := s.Get(ctx)
	if err != nil {
		return Settings{}, err
	}
	input.PublicSlug = current.PublicSlug
	input.CreatedAt = current.CreatedAt
	now := s.now()
	saved, err := s.repo.SetSettings(ctx, input, now)
	if err != nil {
		return Settings{}, mapRepoErr(err)
	}
	return saved, nil
}

// RotatePublicSlug atomically replaces the public audio output URL's own
// locator - every previously issued URL stops working immediately.
// Never changes any other configuration field.
func (s *Service) RotatePublicSlug(ctx context.Context) (Settings, error) {
	current, err := s.Get(ctx)
	if err != nil {
		return Settings{}, err
	}
	slug, err := s.newSlug()
	if err != nil {
		return Settings{}, fmt.Errorf("%w: %s", ErrStorage, err)
	}
	current.PublicSlug = slug
	now := s.now()
	saved, err := s.repo.SetSettings(ctx, current, now)
	if err != nil {
		return Settings{}, mapRepoErr(err)
	}
	return saved, nil
}

func mapRepoErr(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrStorage, err)
}
