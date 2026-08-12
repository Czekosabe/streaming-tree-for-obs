package donationsource

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/streaming-tree/server/internal/secrets"
)

// Clock returns the current time; injected so tests are deterministic.
type Clock func() time.Time

// Options constructs a Service.
type Options struct {
	Repository Repository
	Secrets    secrets.SecretStore
	Now        Clock

	// OnSourceRemoved is called after a source (and its credential) has
	// been fully deleted, so the runtime connector manager can cancel any
	// active connection - mirrors account.Options.OnAccountRemoved
	// exactly.
	OnSourceRemoved func(sourceID string)
}

// Service holds the donation-source CRUD use cases: safe-metadata
// persistence plus credential lifecycle coordination through SecretStore.
// Never opens a connection to a provider itself - see
// internal/runtime/streamelementsengagement for that.
type Service struct {
	repo    Repository
	secrets secrets.SecretStore
	now     Clock
	newID   func() (string, error)

	onSourceRemoved func(sourceID string)
}

// NewService builds a Service.
func NewService(opts Options) *Service {
	now := opts.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{
		repo: opts.Repository, secrets: opts.Secrets, now: now, newID: NewID,
		onSourceRemoved: opts.OnSourceRemoved,
	}
}

// Get returns one source, or found=false if it does not exist.
func (s *Service) Get(ctx context.Context, id string) (Source, bool, error) {
	src, err := s.repo.GetSource(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Source{}, false, nil
		}
		return Source{}, false, mapRepoErr(err)
	}
	return src, true, nil
}

// SourceExists reports whether id names a real donation source - the
// exact shape internal/alerts' combined account/source lookup adapter
// needs (mirrors account.Service's own existence-check usage from
// internal/alerts/wiring.go).
func (s *Service) SourceExists(ctx context.Context, id string) (bool, error) {
	_, found, err := s.Get(ctx, id)
	return found, err
}

// List returns every donation source.
func (s *Service) List(ctx context.Context) ([]Source, error) {
	sources, err := s.repo.ListSources(ctx)
	if err != nil {
		return nil, mapRepoErr(err)
	}
	return sources, nil
}

// Create validates and persists a new donation source, storing its
// credential through SecretStore before the source row becomes visible -
// a source is never listed without its credential already stored (see
// this file's own ordering below).
func (s *Service) Create(ctx context.Context, in CreateInput) (Source, error) {
	if err := validateProvider(in.ProviderID); err != nil {
		return Source{}, err
	}
	if err := validateLabel(in.Label); err != nil {
		return Source{}, err
	}
	if err := validateRemoteChannelID(in.RemoteChannelID); err != nil {
		return Source{}, err
	}
	if err := validateCredential(in.Token); err != nil {
		return Source{}, err
	}

	id, err := s.newID()
	if err != nil {
		return Source{}, fmt.Errorf("%w: %s", ErrStorage, err)
	}

	if err := StoreCredential(ctx, s.secrets, id, in.Token); err != nil {
		return Source{}, err
	}

	now := s.now()
	src := Source{
		ID: id, ProviderID: in.ProviderID, Label: in.Label,
		Enabled: false, RemoteChannelID: in.RemoteChannelID,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.repo.CreateSource(ctx, src); err != nil {
		// The row never became visible - remove the credential we just
		// stored rather than leaving an orphaned secret no row
		// references.
		_ = DeleteCredential(ctx, s.secrets, id)
		return Source{}, mapRepoErr(err)
	}
	return src, nil
}

// UpdateMetadata replaces a source's safe metadata only - never its
// credential (see ReplaceCredential) and never its Enabled flag (see
// SetEnabled), each a deliberately separate, narrower operation.
func (s *Service) UpdateMetadata(ctx context.Context, id string, in UpdateInput) (Source, error) {
	if err := validateLabel(in.Label); err != nil {
		return Source{}, err
	}
	if err := validateRemoteChannelID(in.RemoteChannelID); err != nil {
		return Source{}, err
	}
	src, err := s.repo.GetSource(ctx, id)
	if err != nil {
		return Source{}, mapRepoErr(err)
	}
	src.Label = in.Label
	src.RemoteChannelID = in.RemoteChannelID
	src.UpdatedAt = s.now()
	if err := s.repo.UpdateSource(ctx, src); err != nil {
		return Source{}, mapRepoErr(err)
	}
	return src, nil
}

// SetEnabled persists the operator's explicit enable/disable choice -
// the only fact this package persists about whether a source should run;
// the runtime connector manager reads it back the same way
// engagementsettings.Service's own enabled flag drives
// internal/runtime/youtubeengagement.
func (s *Service) SetEnabled(ctx context.Context, id string, enabled bool) (Source, error) {
	src, err := s.repo.GetSource(ctx, id)
	if err != nil {
		return Source{}, mapRepoErr(err)
	}
	src.Enabled = enabled
	src.UpdatedAt = s.now()
	if err := s.repo.UpdateSource(ctx, src); err != nil {
		return Source{}, mapRepoErr(err)
	}
	return src, nil
}

// ReplaceCredential atomically rotates a source's stored credential -
// deliberately separate from UpdateMetadata so a metadata-only edit can
// never accidentally touch the credential, and vice versa.
func (s *Service) ReplaceCredential(ctx context.Context, id, token string) error {
	if _, err := s.repo.GetSource(ctx, id); err != nil {
		return mapRepoErr(err)
	}
	return StoreCredential(ctx, s.secrets, id, token)
}

// CredentialConfigured reports whether id currently has a stored
// credential, without returning it.
func (s *Service) CredentialConfigured(ctx context.Context, id string) (bool, error) {
	return CredentialConfigured(ctx, s.secrets, id)
}

// Delete removes a source and its stored credential, then notifies
// OnSourceRemoved so the runtime connector manager cancels any active
// connection - mirrors account.Service.Disconnect's own ordering
// (persistence first, then the runtime callback).
func (s *Service) Delete(ctx context.Context, id string) error {
	if _, err := s.repo.GetSource(ctx, id); err != nil {
		return mapRepoErr(err)
	}
	if err := s.repo.DeleteSource(ctx, id); err != nil {
		return mapRepoErr(err)
	}
	if err := DeleteCredential(ctx, s.secrets, id); err != nil {
		return err
	}
	if s.onSourceRemoved != nil {
		s.onSourceRemoved(id)
	}
	return nil
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
