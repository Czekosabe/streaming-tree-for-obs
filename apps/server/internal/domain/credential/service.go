package credential

import (
	"context"
	"errors"
	"fmt"

	"github.com/streaming-tree/server/internal/secrets"
)

// Service holds the credential use cases: validating and storing a
// destination stream key, reporting its status, deleting it, and cleaning up
// a platform's credentials when the platform itself is deleted.
type Service struct {
	store secrets.SecretStore
}

// NewService builds a Service around a secrets.SecretStore. Production code
// injects a *secrets.KeyringStore; tests inject secretstest.New().
func NewService(store secrets.SecretStore) *Service {
	return &Service{store: store}
}

func streamKeyKey(platformID string) string {
	return secrets.BuildKey(secrets.SecretTypeDestinationStreamKey, platformID)
}

// mapStoreError translates a secrets.SecretStore error into a credential
// domain error. It is the one place that boundary is crossed, so every
// caller in this file gets the same mapping.
func mapStoreError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, secrets.ErrUnavailable):
		return fmt.Errorf("%w: %s", ErrStoreUnavailable, err)
	case errors.Is(err, secrets.ErrNotFound):
		return ErrCredentialNotFound
	default:
		return fmt.Errorf("%w: %s", ErrStoreFailure, err)
	}
}

// Status reports the destination stream key's configured state for one
// platform, and whether the OS credential store could be reached to
// determine it.
//
// An unavailable store is not a Go error here: it is a legitimate, stable
// status the API must always be able to report. Only a genuine, unexpected
// store failure is returned as an error.
func (s *Service) Status(ctx context.Context, platformID string) (Status, StoreStatus, error) {
	exists, err := s.store.Exists(ctx, streamKeyKey(platformID))
	if err != nil {
		if errors.Is(err, secrets.ErrUnavailable) {
			return Status{Configured: false}, StoreStatus{Available: false}, nil
		}
		return Status{}, StoreStatus{}, mapStoreError(err)
	}
	return Status{Configured: exists}, StoreStatus{Available: true}, nil
}

// SetStreamKey validates and stores a new destination stream key for a
// platform, replacing any previous value.
func (s *Service) SetStreamKey(ctx context.Context, platformID, raw string) error {
	normalized, err := ValidateStreamKey(raw)
	if err != nil {
		return err
	}
	if err := s.store.Set(ctx, streamKeyKey(platformID), []byte(normalized)); err != nil {
		return mapStoreError(err)
	}
	return nil
}

// DeleteStreamKey removes a platform's destination stream key.
//
// DELETE is idempotent: deleting an absent key is defined as success, not
// ErrCredentialNotFound, so a client that retries a delete - or deletes
// before ever setting a key - gets the same result either time.
func (s *Service) DeleteStreamKey(ctx context.Context, platformID string) error {
	err := s.store.Delete(ctx, streamKeyKey(platformID))
	if err == nil || errors.Is(err, secrets.ErrNotFound) {
		return nil
	}
	return mapStoreError(err)
}

// DeletePlatformCredentials removes every credential belonging to one
// platform, as the first step of deleting the platform itself.
//
// The OS credential store and the SQLite database cannot share a
// transaction, so platform deletion is a strict two-step sequence: this
// method runs first, and the platform row is only removed once it succeeds.
// That ordering means a failure here leaves the platform (and its
// credential) exactly as they were, safe to retry, rather than ever leaving
// a platform row deleted while its credential silently survives.
//
// The one deliberate exception is a store that is unreachable rather than
// failing: see the ErrUnavailable case below.
func (s *Service) DeletePlatformCredentials(ctx context.Context, platformID string) error {
	err := s.store.Delete(ctx, streamKeyKey(platformID))
	switch {
	case err == nil, errors.Is(err, secrets.ErrNotFound):
		return nil

	case errors.Is(err, secrets.ErrUnavailable):
		// The store cannot be reached, so we cannot confirm whether a
		// credential still exists for this platform - it may genuinely
		// still be there. Blocking deletion of the platform row on a
		// transient credential-store outage would degrade ordinary platform
		// CRUD, which must keep working regardless of credential-store
		// availability. We accept the documented, low-impact risk of
		// leaving an inert entry behind under a platform ID that this
		// application will never generate again and will never look up
		// again, rather than degrade an unrelated feature.
		return nil

	default:
		return mapStoreError(err)
	}
}

// RetrieveForProcessStart returns the raw destination stream key for
// starting an outgoing stream process.
//
// This is the one method in this package that returns a secret value. It is
// reserved for the future FFmpeg destination-branch stage, is not part of
// CredentialService (the interface the HTTP layer is given in
// internal/httpapi), and must never be logged, formatted into an error, or
// held longer than starting the process requires.
func (s *Service) RetrieveForProcessStart(ctx context.Context, platformID string) (string, error) {
	value, err := s.store.Get(ctx, streamKeyKey(platformID))
	if err != nil {
		return "", mapStoreError(err)
	}
	return string(value), nil
}
