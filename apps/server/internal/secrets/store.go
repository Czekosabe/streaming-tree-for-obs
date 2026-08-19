// Package secrets is the credential-store foundation: a SecretStore port over
// the operating system's own credential store (Windows Credential Manager,
// macOS Keychain, Linux Secret Service), a centralized key-namespace format,
// and typed errors that never carry a secret value.
//
// Nothing in this package is specific to destination stream keys. It exists
// so future secret types - OAuth tokens for connected accounts, chief among
// them - can reuse the same storage abstraction with a different SecretType,
// rather than each feature inventing its own credential handling.
package secrets

import (
	"context"
	"errors"
)

// ServiceName scopes every credential this application stores, so its
// entries are never confused with another application's entries under the
// same OS credential store.
const ServiceName = "streaming-tree-for-obs"

// SecretType names a category of secret. It is the first namespace segment
// of every stored key, so different kinds of secret (a destination stream
// key today, an OAuth token later) can never collide even if they happened
// to share a subject identifier.
type SecretType string

const (
	// SecretTypeDestinationStreamKey is a configured destination platform's
	// outgoing RTMP stream key.
	SecretTypeDestinationStreamKey SecretType = "destination-stream-key"

	// SecretTypeOAuthTokenBundle is a connected account's complete OAuth
	// token set (access token, refresh token, token type, expiry), stored as
	// one atomically-replaced value - see internal/domain/account.TokenBundle.
	SecretTypeOAuthTokenBundle SecretType = "oauth-token-bundle"

	// SecretTypeDonationSourceToken is an external donation source's
	// credential (Stage 16A: a StreamElements personal JWT) - never an
	// OAuth token bundle, since a donation source has no refresh token or
	// expiry this application manages; see
	// internal/domain/donationsource.
	SecretTypeDonationSourceToken SecretType = "donation-source-token"

	// SecretTypeAdminPassword is the Stage 20D2B single-administrator
	// password verifier (docs/remote-management.md §9.1) - an
	// Argon2id-hashed string, never the plaintext password. There is
	// exactly one administrator identity, so this type has exactly one
	// stored key (see AdminPasswordSubjectID) rather than a
	// per-instance subject ID.
	SecretTypeAdminPassword SecretType = "admin-password"
)

// AdminPasswordSubjectID is the fixed BuildKey subject for
// SecretTypeAdminPassword - a single-administrator product has no
// per-instance identity to key by.
const AdminPasswordSubjectID = "default"

// BuildKey returns the namespaced key for one secret.
//
// subjectID must identify what the secret belongs to, never what it is
// currently displayed as. For a destination stream key, that is the
// configured platform's generated ID: stable for the platform's lifetime, so
// renaming the platform can never orphan its secret, and two destinations
// configured for the same provider always resolve to independent keys.
func BuildKey(secretType SecretType, subjectID string) string {
	return string(secretType) + ":" + subjectID
}

// Sentinel errors. A production SecretStore implementation must map every
// backend-specific failure onto one of these before returning; no
// driver-specific error, and no secret value, may cross this boundary.
var (
	// ErrUnavailable means the OS credential store could not be reached or
	// used for this operation - no Secret Service session, a locked
	// keychain, a permission failure, or an unsupported environment. It is
	// an expected, recoverable condition, not a bug: the backend must keep
	// running and answer a stable status when this happens.
	ErrUnavailable = errors.New("secret store unavailable")

	// ErrNotFound means the store was reachable but held no value for the
	// given key.
	ErrNotFound = errors.New("secret not found")

	// ErrFailure wraps an unexpected failure from a reachable store. The
	// underlying cause may be logged server-side but must never reach an API
	// response.
	//
	// The production KeyringStore does not currently distinguish "reachable
	// but refused" (locked keychain, permission denied) from "unreachable":
	// both map to ErrUnavailable, since the operator's fix is the same
	// either way and a coarse-but-safe answer beats a falsely precise one.
	// ErrFailure exists for a future backend that can make that distinction,
	// and is exercised today through the in-memory fake in secretstest, so
	// the credential_store_failure path is tested even though the real
	// store cannot yet trigger it.
	ErrFailure = errors.New("secret store failure")
)

// SecretStore is the port every credential-consuming service depends on.
//
// Implementations must be safe for concurrent use and must treat ctx
// cancellation as a normal failure path, not a reason to leave a partial
// write behind. No implementation may fall back to writing a secret in
// plaintext when the underlying store is unavailable: an unavailable store
// must surface as ErrUnavailable, never as a silent alternate storage
// location.
type SecretStore interface {
	// Set stores value under key, replacing any previous value.
	Set(ctx context.Context, key string, value []byte) error

	// Get returns the value stored under key, or ErrNotFound.
	Get(ctx context.Context, key string) ([]byte, error)

	// Delete removes the value stored under key, or ErrNotFound if there was
	// none.
	Delete(ctx context.Context, key string) error

	// Exists reports whether a value is stored under key, without returning
	// it. Unlike Get, a missing key is not an error: it is reported as
	// (false, nil). An error return means existence could not be
	// determined - the store was unavailable or failed.
	Exists(ctx context.Context, key string) (bool, error)
}
