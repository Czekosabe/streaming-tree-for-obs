package metadatapreset

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/streaming-tree/server/internal/domain/platform"
)

// IDGenerator produces identifiers for new presets.
type IDGenerator func() (string, error)

// Clock returns the current time; injected so tests are deterministic.
type Clock func() time.Time

// NewID returns a random, non-sequential preset identifier - matching
// platform.NewID's own reasoning (sequential integers would leak how
// many presets exist and invite enumeration).
func NewID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate metadata preset id: %w", err)
	}
	return "mp_" + hex.EncodeToString(buf), nil
}

// Service holds the metadata-preset CRUD use cases. Apply (docs/
// metadata-presets.md §6) is added in Stage 22C, depending on a
// separate narrow PlatformMetadataStore port - not part of this
// substage's scope.
type Service struct {
	repo  Repository
	newID IDGenerator
	now   Clock
}

// ServiceOption customises a Service, mainly for tests.
type ServiceOption func(*Service)

// WithIDGenerator overrides identifier generation.
func WithIDGenerator(gen IDGenerator) ServiceOption {
	return func(s *Service) { s.newID = gen }
}

// WithClock overrides the time source.
func WithClock(clock Clock) ServiceOption {
	return func(s *Service) { s.now = clock }
}

// NewService builds a Service around a repository.
func NewService(repo Repository, opts ...ServiceOption) *Service {
	s := &Service{repo: repo, newID: NewID, now: time.Now}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// List returns every preset.
func (s *Service) List(ctx context.Context) ([]Preset, error) {
	return s.repo.List(ctx)
}

// Get returns one preset.
func (s *Service) Get(ctx context.Context, id string) (Preset, error) {
	if id == "" {
		return Preset{}, ErrNotFound
	}
	return s.repo.Get(ctx, id)
}

// Create validates and persists a new preset.
func (s *Service) Create(ctx context.Context, input CreateInput) (Preset, error) {
	validated, err := ValidateCreate(input)
	if err != nil {
		return Preset{}, err
	}

	count, err := s.repo.Count(ctx)
	if err != nil {
		return Preset{}, err
	}
	if count >= MaxPresets {
		return Preset{}, fmt.Errorf("%w: at most %d presets are allowed", ErrTooMany, MaxPresets)
	}

	id, err := s.newID()
	if err != nil {
		return Preset{}, fmt.Errorf("%w: %s", ErrStorage, err)
	}

	now := s.now().UTC()
	created := Preset{
		ID:        id,
		Name:      validated.Name,
		Note:      validated.Note,
		Common:    validated.Common,
		Providers: validated.Providers,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.repo.Create(ctx, created); err != nil {
		return Preset{}, err
	}
	return s.repo.Get(ctx, id)
}

// Update replaces a preset's mutable fields in full.
func (s *Service) Update(ctx context.Context, id string, input UpdateInput) (Preset, error) {
	if id == "" {
		return Preset{}, ErrNotFound
	}

	validated, err := ValidateUpdate(input)
	if err != nil {
		return Preset{}, err
	}

	updatedAt := platform.FormatTimestamp(s.now())
	if err := s.repo.Update(ctx, id, validated, updatedAt); err != nil {
		return Preset{}, err
	}
	return s.repo.Get(ctx, id)
}

// Delete removes a preset. Never touches any destination's own
// metadata - deleting a preset and the destinations it was once
// applied to are independent operations (docs/metadata-presets.md §6).
func (s *Service) Delete(ctx context.Context, id string) error {
	if id == "" {
		return ErrNotFound
	}
	return s.repo.Delete(ctx, id)
}
