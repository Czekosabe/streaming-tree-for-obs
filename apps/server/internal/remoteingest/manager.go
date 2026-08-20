// Package remoteingest coordinates the Stage 20D2C remote-ingest
// publisher credential's lifecycle across the two systems a
// provision/rotate/revoke request must keep in sync: the secret store
// (internal/auth's credential functions, docs/remote-ingest.md §6) and
// the MediaMTX supervisor (docs/remote-ingest.md §9). Neither of those
// packages depends on the other for this purpose - this package is the
// coordination point, kept separate from cmd/server/main.go so it can
// be unit-tested without constructing a whole server.
package remoteingest

import (
	"context"
	"errors"

	"github.com/streaming-tree/server/internal/auth"
	"github.com/streaming-tree/server/internal/runtime/mediamtx"
	"github.com/streaming-tree/server/internal/secrets"
)

// ErrStreamingActive is returned by Provision/Rotate/Revoke while
// MediaMTX currently reports the canonical ingest path receiving a
// stream (docs/remote-ingest.md §9) - changing the credential during
// an active remote publish would silently drop it. The caller must
// not retry blindly; the operator needs to stop the current publish
// first or wait for it to end.
var ErrStreamingActive = errors.New("remote ingest credential cannot change while streaming is active")

// ErrAlreadyProvisioned is returned by Provision when a credential
// already exists (docs/remote-ingest.md §8) - use Rotate instead.
var ErrAlreadyProvisioned = errors.New("a remote ingest credential is already provisioned")

// Supervisor is the subset of *mediamtx.Supervisor this package needs
// - declared as an interface so tests can drive a stub instead of a
// real MediaMTX process.
type Supervisor interface {
	Snapshot() mediamtx.Snapshot
	UpdateRemoteIngestCredential(verifier string)
	RequestRestart(ctx context.Context) error
}

// Manager implements httpapi.RemoteIngestService (duck-typed, no
// import needed in either direction) in production.
type Manager struct {
	Store      secrets.SecretStore
	Supervisor Supervisor

	// RTMPSAddress and IngestPath are fixed at construction from the
	// already-validated config (docs/remote-ingest.md §3/§4) - static
	// deployment facts the status endpoint reports, never mutated
	// through any HTTP route.
	RTMPSAddress string
	IngestPath   string
}

// Status reports whether a remote-ingest publisher credential is
// currently provisioned.
func (m *Manager) Status(ctx context.Context) (bool, error) {
	return auth.RemoteIngestPublisherProvisioned(ctx, m.Store)
}

// IngestReceiving reports whether MediaMTX currently reports the
// canonical ingest path receiving a stream - the streaming-safety
// gate's own predicate (docs/remote-ingest.md §9).
func (m *Manager) IngestReceiving() bool {
	return m.Supervisor.Snapshot().Ingest.State == mediamtx.IngestReceiving
}

// Provision generates a fresh credential and returns its plaintext
// secret exactly once. Refuses with ErrAlreadyProvisioned if a
// credential already exists (use Rotate), and with ErrStreamingActive
// while the canonical path is currently receiving.
func (m *Manager) Provision(ctx context.Context) (string, error) {
	provisioned, err := m.Status(ctx)
	if err != nil {
		return "", err
	}
	if provisioned {
		return "", ErrAlreadyProvisioned
	}
	return m.generateAndApply(ctx)
}

// Rotate generates a fresh credential, invalidating the previous one
// immediately, and returns the new plaintext secret exactly once.
// Refuses with ErrStreamingActive while the canonical path is
// currently receiving. Unlike Provision, a missing prior credential is
// not an error - rotating with nothing provisioned yet behaves exactly
// like Provision, since there is nothing meaningfully different about
// "generate the first credential" versus "generate a replacement
// credential" from the store's or MediaMTX's point of view - only the
// HTTP layer's own already-configured 409 differs between the two
// endpoints.
func (m *Manager) Rotate(ctx context.Context) (string, error) {
	return m.generateAndApply(ctx)
}

func (m *Manager) generateAndApply(ctx context.Context) (string, error) {
	if m.IngestReceiving() {
		return "", ErrStreamingActive
	}
	secret, err := auth.ProvisionRemoteIngestPublisherCredential(ctx, m.Store)
	if err != nil {
		return "", err
	}
	if err := m.applyStoredVerifier(ctx); err != nil {
		return "", err
	}
	return secret, nil
}

// Revoke removes the current credential - after this call, nothing
// can authenticate as the remote publisher until Provision/Rotate is
// called again. Refuses with ErrStreamingActive while the canonical
// path is currently receiving. Idempotent: revoking with nothing
// provisioned is not an error.
func (m *Manager) Revoke(ctx context.Context) error {
	if m.IngestReceiving() {
		return ErrStreamingActive
	}
	if err := auth.RevokeRemoteIngestPublisherCredential(ctx, m.Store); err != nil {
		return err
	}
	return m.applyStoredVerifier(ctx)
}

// applyStoredVerifier re-reads the just-written verifier (or its
// absence, which reads back as an empty string) and pushes it into the
// running supervisor, then requests a restart so MediaMTX picks up the
// regenerated configuration - a controlled, bounded restart
// (docs/remote-ingest.md §9), never a silent one, and never reached
// while streaming is active (both callers above check that before any
// store mutation happens, so this restart is never racing an actual
// live publish).
func (m *Manager) applyStoredVerifier(ctx context.Context) error {
	verifier, _, err := auth.RemoteIngestPublisherVerifier(ctx, m.Store)
	if err != nil {
		return err
	}
	m.Supervisor.UpdateRemoteIngestCredential(verifier)
	return m.Supervisor.RequestRestart(ctx)
}
