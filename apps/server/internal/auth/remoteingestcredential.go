package auth

import (
	"context"
	"errors"

	"github.com/streaming-tree/server/internal/runtime/mediamtx"
	"github.com/streaming-tree/server/internal/secrets"
)

// remoteIngestPublisherKey is the fixed SecretStore key the single
// remote-ingest publisher verifier is stored under (docs/remote-
// ingest.md §6) - reuses the existing SecretStore abstraction,
// exactly mirroring adminPasswordKey's own pattern. Not SQLite: this
// project's established precedent for a security-sensitive verifier
// is the SecretStore (OS keyring on desktop, the AES-256-GCM
// HeadlessStore in headless mode), the same store the D2B
// administrator verifier already uses.
func remoteIngestPublisherKey() string {
	return secrets.BuildKey(secrets.SecretTypeRemoteIngestPublisherPassword, secrets.RemoteIngestPublisherSubjectID)
}

// ProvisionRemoteIngestPublisherCredential generates a fresh 256-bit
// secret, stores only its MediaMTX-native verifier, and returns the
// plaintext secret exactly once (docs/remote-ingest.md §6) - the
// caller (the HTTP handler) must return it to the operator in that one
// response and never persist, log, or re-derive it afterward.
// Overwrites any previously provisioned credential, exactly like
// rotation - the caller is responsible for the streaming-safety gate
// (docs/remote-ingest.md §9) before calling this during rotation.
func ProvisionRemoteIngestPublisherCredential(ctx context.Context, store secrets.SecretStore) (string, error) {
	secret, err := mediamtx.NewPublisherSecret()
	if err != nil {
		return "", err
	}
	verifier := mediamtx.PublisherPassVerifierFor(secret)
	if err := store.Set(ctx, remoteIngestPublisherKey(), []byte(verifier)); err != nil {
		return "", err
	}
	return secret, nil
}

// RemoteIngestPublisherVerifier returns the stored MediaMTX-native
// verifier, or ("", false, nil) if none is provisioned yet. This is
// the value main.go embeds directly into RemoteIngestOptions -
// mediamtx.RenderConfig never sees, receives, or could leak the
// plaintext secret, only this already-hashed verifier.
func RemoteIngestPublisherVerifier(ctx context.Context, store secrets.SecretStore) (string, bool, error) {
	raw, err := store.Get(ctx, remoteIngestPublisherKey())
	if err != nil {
		if errors.Is(err, secrets.ErrNotFound) {
			return "", false, nil
		}
		return "", false, err
	}
	return string(raw), true, nil
}

// RemoteIngestPublisherProvisioned reports whether a remote-ingest
// publisher credential currently exists - used by the status endpoint
// and by provision's own "already configured, use rotate instead"
// 409 check (docs/remote-ingest.md §8).
func RemoteIngestPublisherProvisioned(ctx context.Context, store secrets.SecretStore) (bool, error) {
	return store.Exists(ctx, remoteIngestPublisherKey())
}

// RevokeRemoteIngestPublisherCredential deletes the stored verifier.
// After this call, mediamtx.RenderConfig's own documented behavior
// (an empty PublisherPassVerifier omits the publisher entry entirely)
// means no credential can authenticate - default-deny enforced by
// MediaMTX itself, not by a separate "disabled" flag this application
// would need to interpret correctly on every code path.
func RevokeRemoteIngestPublisherCredential(ctx context.Context, store secrets.SecretStore) error {
	err := store.Delete(ctx, remoteIngestPublisherKey())
	if errors.Is(err, secrets.ErrNotFound) {
		// Revoking an already-absent credential is not an error - the
		// end state (no credential) is exactly what the caller wanted.
		return nil
	}
	return err
}
