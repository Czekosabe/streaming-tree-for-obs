package platform

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// TimestampFormat is how every timestamp is serialized in the database.
//
// RFC 3339 with nanosecond precision in UTC: lexicographic ordering matches
// chronological ordering, which lets SQLite sort on the raw text column.
const TimestampFormat = time.RFC3339Nano

// FormatTimestamp renders a time for storage.
func FormatTimestamp(t time.Time) string {
	return t.UTC().Format(TimestampFormat)
}

// ParseTimestamp reads a stored timestamp.
func ParseTimestamp(s string) (time.Time, error) {
	return time.Parse(TimestampFormat, s)
}

// IDGenerator produces identifiers for new configured platforms.
type IDGenerator func() (string, error)

// Clock returns the current time; injected so tests are deterministic.
type Clock func() time.Time

// NewID returns a random, non-sequential identifier.
//
// Sequential integers are deliberately avoided as public identifiers: they leak
// how many destinations exist and invite enumeration.
func NewID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate platform id: %w", err)
	}
	return "pf_" + hex.EncodeToString(buf), nil
}

// Service holds the platform configuration use cases.
//
// It owns validation, ID and timestamp generation, and the mapping from
// repository failures to domain errors. Handlers stay free of business rules
// and SQL alike.
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
	s := &Service{
		repo:  repo,
		newID: NewID,
		now:   time.Now,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Definitions exposes the built-in provider registry.
func (s *Service) Definitions() []ProviderDefinition {
	return Definitions()
}

// List returns every configured platform.
func (s *Service) List(ctx context.Context) ([]Platform, error) {
	return s.repo.List(ctx)
}

// Get returns one configured platform.
func (s *Service) Get(ctx context.Context, id string) (Platform, error) {
	if id == "" {
		return Platform{}, ErrNotFound
	}
	return s.repo.Get(ctx, id)
}

// GetMany returns every named platform, keyed by ID. Returns
// ErrNotFound, naming the missing ID, if any of them does not exist -
// a caller applying a preset to a stale destination list should see a
// clear error rather than a silently partial result.
func (s *Service) GetMany(ctx context.Context, ids []string) (map[string]Platform, error) {
	out := make(map[string]Platform, len(ids))
	for _, id := range ids {
		p, err := s.repo.Get(ctx, id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return nil, fmt.Errorf("%w: platform %s", ErrNotFound, id)
			}
			return nil, err
		}
		out[id] = p
	}
	return out, nil
}

// Create configures a new destination platform.
//
// The initial metadata row is created empty in the same transaction, so every
// platform always has exactly one metadata record.
func (s *Service) Create(ctx context.Context, input CreateInput) (Platform, error) {
	validated, err := ValidateCreate(input)
	if err != nil {
		// An unknown provider is both a field violation and its own domain
		// error, so handlers can answer 422 with field details while still
		// being able to distinguish the case.
		if !KnownProvider(input.ProviderID) {
			return Platform{}, fmt.Errorf("%w: %s", ErrUnknownProvider, err)
		}
		return Platform{}, err
	}

	id, err := s.newID()
	if err != nil {
		return Platform{}, fmt.Errorf("%w: %s", ErrStorage, err)
	}

	sortOrder := 0
	if validated.SortOrder != nil {
		sortOrder = *validated.SortOrder
	} else {
		next, err := s.repo.NextSortOrder(ctx)
		if err != nil {
			return Platform{}, err
		}
		sortOrder = next
	}

	now := s.now().UTC()
	created := Platform{
		ID:          id,
		ProviderID:  validated.ProviderID,
		DisplayName: validated.DisplayName,
		Enabled:     validated.Enabled,
		SortOrder:   sortOrder,
		CreatedAt:   now,
		UpdatedAt:   now,
		Metadata: Metadata{
			Tags:      []string{},
			UpdatedAt: now,
		},
	}

	if err := s.repo.Create(ctx, created); err != nil {
		return Platform{}, err
	}

	return s.repo.Get(ctx, id)
}

// Update replaces the mutable configuration of a platform.
func (s *Service) Update(ctx context.Context, id string, input UpdateInput) (Platform, error) {
	validated, err := ValidateUpdate(input)
	if err != nil {
		return Platform{}, err
	}

	updatedAt := FormatTimestamp(s.now())
	if err := s.repo.Update(ctx, id, validated, updatedAt); err != nil {
		return Platform{}, err
	}

	return s.repo.Get(ctx, id)
}

// Delete removes a configured platform together with its metadata and tags.
func (s *Service) Delete(ctx context.Context, id string) error {
	if id == "" {
		return ErrNotFound
	}
	return s.repo.Delete(ctx, id)
}

// Metadata returns the stored metadata of one platform.
func (s *Service) Metadata(ctx context.Context, id string) (Metadata, error) {
	p, err := s.repo.Get(ctx, id)
	if err != nil {
		return Metadata{}, err
	}
	return p.Metadata, nil
}

// SaveMetadata validates metadata against the platform's provider definition
// and replaces the stored record and its tags atomically.
func (s *Service) SaveMetadata(ctx context.Context, id string, in Metadata) (Metadata, error) {
	existing, err := s.repo.Get(ctx, id)
	if err != nil {
		return Metadata{}, err
	}

	def, ok := Definition(existing.ProviderID)
	if !ok {
		// The row references a provider the binary no longer knows about. That
		// is a storage-consistency problem, not user error.
		return Metadata{}, fmt.Errorf("%w: platform %s references unknown provider %q",
			ErrStorage, id, existing.ProviderID)
	}

	normalized, err := ValidateMetadata(def, in)
	if err != nil {
		return Metadata{}, err
	}

	normalized.UpdatedAt = s.now().UTC()

	if err := s.repo.SaveMetadata(ctx, id, normalized); err != nil {
		return Metadata{}, err
	}

	updated, err := s.repo.Get(ctx, id)
	if err != nil {
		return Metadata{}, err
	}
	return updated.Metadata, nil
}

// SaveMetadataBatch validates and replaces the metadata of every named
// platform atomically - either all succeed or none are persisted.
// Provider publishing is never part of this: it only ever writes
// local metadata, same as the single-item SaveMetadata above.
//
// Every value in updates is expected to already be projected onto its
// destination's own provider capabilities (see metadatapreset's
// buildCandidate) - a caller handing this a value the destination's
// provider does not support still gets the same hard validation error
// SaveMetadata itself would give, never a silent drop.
func (s *Service) SaveMetadataBatch(ctx context.Context, updates map[string]Metadata) (map[string]Metadata, error) {
	if len(updates) == 0 {
		return map[string]Metadata{}, nil
	}

	normalized := make(map[string]Metadata, len(updates))
	for id, in := range updates {
		existing, err := s.repo.Get(ctx, id)
		if err != nil {
			return nil, err
		}

		def, ok := Definition(existing.ProviderID)
		if !ok {
			return nil, fmt.Errorf("%w: platform %s references unknown provider %q",
				ErrStorage, id, existing.ProviderID)
		}

		validated, err := ValidateMetadata(def, in)
		if err != nil {
			return nil, err
		}
		validated.UpdatedAt = s.now().UTC()
		normalized[id] = validated
	}

	if err := s.repo.SaveMetadataBatch(ctx, normalized); err != nil {
		return nil, err
	}

	out := make(map[string]Metadata, len(normalized))
	for id := range normalized {
		updated, err := s.repo.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		out[id] = updated.Metadata
	}
	return out, nil
}

// SetEnabledBatch replaces the Enabled flag of every named platform
// atomically - either every change lands, or none does. Unlike
// SaveMetadataBatch there is no per-item validation to run (Enabled is
// a plain bool, never provider-projected), so this delegates straight
// to the repository. Used by Stage 25's stream setup profiles to apply
// a whole destination set in one step (docs/stream-setup-profiles.md
// §5).
func (s *Service) SetEnabledBatch(ctx context.Context, updates map[string]bool) error {
	if len(updates) == 0 {
		return nil
	}
	return s.repo.SetEnabledBatch(ctx, updates)
}
