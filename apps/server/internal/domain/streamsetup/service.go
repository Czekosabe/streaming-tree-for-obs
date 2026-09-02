package streamsetup

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/streaming-tree/server/internal/domain/metadatapreset"
	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/runtime/branch"
)

// NewID returns a random, non-sequential profile identifier - matching
// platform.NewID's own reasoning.
func NewID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate stream setup profile id: %w", err)
	}
	return "setup_" + hex.EncodeToString(buf), nil
}

// Clock returns the current time; injected so tests are deterministic.
type Clock func() time.Time

// PlatformPort is the narrow slice of platform.Service this domain
// depends on - reused directly, never re-implemented.
type PlatformPort interface {
	List(ctx context.Context) ([]platform.Platform, error)
	Get(ctx context.Context, id string) (platform.Platform, error)
	SetEnabledBatch(ctx context.Context, updates map[string]bool) error
}

// MetadataPresetPort is the narrow slice of metadatapreset.Service this
// domain depends on - the exact same preview/apply semantics Stage 22
// already established, never a second implementation of them.
type MetadataPresetPort interface {
	Get(ctx context.Context, id string) (metadatapreset.Preset, error)
	ApplyPreview(ctx context.Context, presetID string, platformIDs []string) ([]metadatapreset.DestinationPreview, error)
	Apply(ctx context.Context, presetID string, platformIDs []string) (map[string]platform.Metadata, error)
}

// BranchSnapshotter is the narrow read-only port onto branch.Manager
// this domain needs for active-stream safety (docs/stream-setup-
// profiles.md §6).
type BranchSnapshotter interface {
	Snapshot(ctx context.Context) ([]branch.Snapshot, error)
}

// activeBranchStates mirrors updater.StreamingActive's own exact set -
// never a second definition of "is a broadcast active".
var activeBranchStates = map[branch.State]bool{
	branch.StateStarting:         true,
	branch.StateLive:             true,
	branch.StateRestarting:       true,
	branch.StateWaitingForIngest: true,
	branch.StateStopping:         true,
}

// Service holds the Stage 25 CRUD/duplicate/save-current/preview/apply
// use cases.
type Service struct {
	repo      Repository
	platforms PlatformPort
	presets   MetadataPresetPort
	branches  BranchSnapshotter
	newID     func() (string, error)
	now       Clock
}

// NewService builds a Service.
func NewService(repo Repository, platforms PlatformPort, presets MetadataPresetPort, branches BranchSnapshotter) *Service {
	return &Service{
		repo: repo, platforms: platforms, presets: presets, branches: branches,
		newID: NewID, now: time.Now,
	}
}

func (s *Service) List(ctx context.Context) ([]Profile, error) {
	return s.repo.List(ctx)
}

func (s *Service) Get(ctx context.Context, id string) (Profile, error) {
	return s.repo.Get(ctx, id)
}

// resolveDestinations snapshots provider/display name for each real
// platform id - a caller-supplied id that does not currently exist is
// rejected outright (creating/editing a profile is not the place to
// silently accept a dangling reference; only a LATER deletion of an
// already-referenced destination is preserved as "missing" - docs/
// stream-setup-profiles.md §4).
func (s *Service) resolveDestinations(ctx context.Context, platformIDs []string) ([]Destination, error) {
	dests := make([]Destination, 0, len(platformIDs))
	for _, id := range platformIDs {
		p, err := s.platforms.Get(ctx, id)
		if err != nil {
			if errors.Is(err, platform.ErrNotFound) {
				return nil, fmt.Errorf("%w: destination %s", platform.ErrNotFound, id)
			}
			return nil, err
		}
		pid := id
		dests = append(dests, Destination{PlatformID: &pid, ProviderID: string(p.ProviderID), DisplayName: p.DisplayName})
	}
	return dests, nil
}

func (s *Service) Create(ctx context.Context, input CreateInput) (Profile, error) {
	input, err := ValidateCreate(input)
	if err != nil {
		return Profile{}, err
	}
	count, err := s.repo.Count(ctx)
	if err != nil {
		return Profile{}, err
	}
	if count >= MaxProfiles {
		return Profile{}, fmt.Errorf("%w: at most %d stream setup profiles are allowed", ErrTooMany, MaxProfiles)
	}

	dests, err := s.resolveDestinations(ctx, input.DestinationIDs)
	if err != nil {
		return Profile{}, err
	}
	presetName, err := s.resolvePresetName(ctx, input.MetadataPresetID)
	if err != nil {
		return Profile{}, err
	}
	id, err := s.newID()
	if err != nil {
		return Profile{}, err
	}
	now := s.now()
	p := Profile{
		ID: id, Name: input.Name, Note: input.Note, Destinations: dests,
		MetadataPresetID: input.MetadataPresetID, MetadataPresetName: presetName,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.repo.Create(ctx, p); err != nil {
		return Profile{}, err
	}
	return p, nil
}

func (s *Service) Update(ctx context.Context, id string, input UpdateInput) (Profile, error) {
	input, err := ValidateUpdate(input)
	if err != nil {
		return Profile{}, err
	}
	existing, err := s.repo.Get(ctx, id)
	if err != nil {
		return Profile{}, err
	}
	dests, err := s.resolveDestinations(ctx, input.DestinationIDs)
	if err != nil {
		return Profile{}, err
	}
	presetName, err := s.resolvePresetName(ctx, input.MetadataPresetID)
	if err != nil {
		return Profile{}, err
	}
	existing.Name, existing.Note = input.Name, input.Note
	existing.Destinations = dests
	existing.MetadataPresetID = input.MetadataPresetID
	existing.MetadataPresetName = presetName
	existing.UpdatedAt = s.now()
	if err := s.repo.Update(ctx, existing); err != nil {
		return Profile{}, err
	}
	return existing, nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

// Duplicate copies an existing profile under a new name - a genuinely
// useful workflow for iterating on a setup without losing the
// original.
func (s *Service) Duplicate(ctx context.Context, id, newName string) (Profile, error) {
	newName = NormalizeName(newName)
	v := &platform.ValidationError{}
	validateName(newName, v)
	if err := v.OrNil(); err != nil {
		return Profile{}, err
	}

	existing, err := s.repo.Get(ctx, id)
	if err != nil {
		return Profile{}, err
	}

	count, err := s.repo.Count(ctx)
	if err != nil {
		return Profile{}, err
	}
	if count >= MaxProfiles {
		return Profile{}, fmt.Errorf("%w: at most %d stream setup profiles are allowed", ErrTooMany, MaxProfiles)
	}

	newID, err := s.newID()
	if err != nil {
		return Profile{}, err
	}
	now := s.now()
	copyProfile := Profile{
		ID: newID, Name: newName, Note: existing.Note, Destinations: existing.Destinations,
		MetadataPresetID: existing.MetadataPresetID, MetadataPresetName: existing.MetadataPresetName,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.repo.Create(ctx, copyProfile); err != nil {
		return Profile{}, err
	}
	return copyProfile, nil
}

// SaveCurrent captures the currently-enabled destination set into a new
// named profile. The metadata-preset reference is never auto-detected
// (nothing tracks which preset current metadata came from) - it is
// whatever the caller explicitly passes, exactly like an ordinary
// Create.
func (s *Service) SaveCurrent(ctx context.Context, name, note string, metadataPresetID *string) (Profile, error) {
	all, err := s.platforms.List(ctx)
	if err != nil {
		return Profile{}, err
	}
	ids := make([]string, 0, len(all))
	for _, p := range all {
		if p.Enabled {
			ids = append(ids, p.ID)
		}
	}
	return s.Create(ctx, CreateInput{Name: name, Note: note, DestinationIDs: ids, MetadataPresetID: metadataPresetID})
}

// resolvePresetName validates that presetID (if any) refers to a real,
// currently-existing preset and returns its own name to snapshot -
// creating/editing a profile never accepts a dangling reference; only
// a LATER deletion of an already-referenced preset is preserved as
// "missing" (docs/stream-setup-profiles.md §4).
func (s *Service) resolvePresetName(ctx context.Context, presetID *string) (string, error) {
	if presetID == nil {
		return "", nil
	}
	preset, err := s.presets.Get(ctx, *presetID)
	if err != nil {
		if errors.Is(err, metadatapreset.ErrNotFound) {
			return "", fmt.Errorf("%w: metadata preset %s", metadatapreset.ErrNotFound, *presetID)
		}
		return "", err
	}
	return preset.Name, nil
}
