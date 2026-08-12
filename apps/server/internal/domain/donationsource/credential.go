package donationsource

import (
	"context"
	"errors"
	"fmt"

	"github.com/streaming-tree/server/internal/secrets"
)

// credentialKey returns the namespaced SecretStore key for one donation
// source's credential - mirrors internal/domain/account's own
// tokenBundleKey exactly, using the dedicated SecretTypeDonationSourceToken
// namespace so a donation source's credential can never collide with an
// OAuth token bundle or a destination stream key even if the ids happened
// to coincide.
func credentialKey(sourceID string) string {
	return secrets.BuildKey(secrets.SecretTypeDonationSourceToken, sourceID)
}

// StoreCredential atomically replaces one donation source's credential
// (a StreamElements personal JWT, in full, as pasted by the operator -
// never decoded, never parsed, never validated locally; see
// docs/provider-integrations/external-donations.md §8). Called on
// creation and on every explicit credential replacement.
func StoreCredential(ctx context.Context, store secrets.SecretStore, sourceID, token string) error {
	if err := validateCredential(token); err != nil {
		return err
	}
	if err := store.Set(ctx, credentialKey(sourceID), []byte(token)); err != nil {
		return mapSecretStoreError(err)
	}
	return nil
}

// LoadCredential retrieves one donation source's stored credential. Only
// ever called by the runtime connector immediately before opening a
// connection - never cached longer than that, never logged, never
// returned by any HTTP handler.
func LoadCredential(ctx context.Context, store secrets.SecretStore, sourceID string) (string, error) {
	raw, err := store.Get(ctx, credentialKey(sourceID))
	if err != nil {
		return "", mapSecretStoreError(err)
	}
	return string(raw), nil
}

// CredentialConfigured reports whether a credential is currently stored
// for sourceID, without returning its value - the same "Stored"/"Missing"
// distinction internal/domain/credential's own StreamKey status uses,
// never a claim that the stored value is still valid (only the provider
// itself can confirm that, when a connection is actually attempted).
func CredentialConfigured(ctx context.Context, store secrets.SecretStore, sourceID string) (bool, error) {
	exists, err := store.Exists(ctx, credentialKey(sourceID))
	if err != nil {
		return false, mapSecretStoreError(err)
	}
	return exists, nil
}

// DeleteCredential removes one donation source's stored credential.
// Deleting an absent credential is not an error - mirrors
// account.DeleteTokenBundle's own idempotent-delete reasoning.
func DeleteCredential(ctx context.Context, store secrets.SecretStore, sourceID string) error {
	err := store.Delete(ctx, credentialKey(sourceID))
	if err == nil || errors.Is(err, secrets.ErrNotFound) {
		return nil
	}
	return mapSecretStoreError(err)
}

func mapSecretStoreError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, secrets.ErrUnavailable):
		return fmt.Errorf("%w: %s", ErrSecretStoreUnavailable, err)
	case errors.Is(err, secrets.ErrNotFound):
		return ErrNotFound
	default:
		return fmt.Errorf("%w: %s", ErrSecretStore, err)
	}
}
