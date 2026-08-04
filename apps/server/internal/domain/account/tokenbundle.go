package account

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/streaming-tree/server/internal/secrets"
)

// TokenBundle is the complete OAuth token set for one connected account,
// serialized as a single SecretStore value.
//
// It exists so one atomic SecretStore.Set replaces the access token and the
// refresh token together: Twitch refresh tokens rotate on every use (see
// docs/provider-integrations/twitch.md), and storing them as two
// independently-replaced secrets could leave a new access token paired with
// an already-spent refresh token if a process died between the two writes.
//
// This type, and everything that touches its fields, must never be logged,
// never serialized into an HTTP response, never placed in an error message,
// and never held longer than one provider operation requires.
type TokenBundle struct {
	// TokenType is always "bearer" for a supported bundle; anything else is
	// rejected at decode time rather than passed through unexamined.
	TokenType    string    `json:"tokenType"`
	AccessToken  string    `json:"accessToken"`
	RefreshToken string    `json:"refreshToken"`
	ExpiresAt    time.Time `json:"expiresAt"`
}

// maxTokenBundleBytes bounds decoding: a real bundle is a few hundred bytes,
// and this is a conservative ceiling against a corrupted or hostile value
// ever being fully parsed.
const maxTokenBundleBytes = 8 * 1024

// supportedTokenType is the only token_type this application's provider
// adapters currently know how to use.
const supportedTokenType = "bearer"

func tokenBundleKey(accountID string) string {
	return secrets.BuildKey(secrets.SecretTypeOAuthTokenBundle, accountID)
}

// encodeTokenBundle serializes a bundle for storage. The returned bytes are
// exactly what SecretStore.Set receives - never additionally wrapped, never
// logged by any caller.
func encodeTokenBundle(bundle TokenBundle) ([]byte, error) {
	if bundle.TokenType != supportedTokenType {
		return nil, fmt.Errorf("%w: unsupported token type", ErrStorage)
	}
	if bundle.AccessToken == "" || bundle.RefreshToken == "" {
		return nil, fmt.Errorf("%w: token bundle is missing a required field", ErrStorage)
	}
	return json.Marshal(bundle)
}

// decodeTokenBundle parses a stored bundle. Any failure - malformed JSON, an
// oversized value, an unsupported token type - maps to the same stable
// internal error, and the raw bytes are never included in it.
func decodeTokenBundle(raw []byte) (TokenBundle, error) {
	if len(raw) > maxTokenBundleBytes {
		return TokenBundle{}, fmt.Errorf("%w: token bundle exceeds the size limit", ErrStorage)
	}

	var bundle TokenBundle
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&bundle); err != nil {
		return TokenBundle{}, fmt.Errorf("%w: token bundle could not be decoded", ErrStorage)
	}
	if bundle.TokenType != supportedTokenType {
		return TokenBundle{}, fmt.Errorf("%w: unsupported token type", ErrStorage)
	}
	if bundle.AccessToken == "" || bundle.RefreshToken == "" {
		return TokenBundle{}, fmt.Errorf("%w: token bundle is missing a required field", ErrStorage)
	}

	return bundle, nil
}

// StoreTokenBundle atomically replaces the complete token bundle for one
// connected account. Called on initial connect, on reconnect, and on every
// successful refresh.
func StoreTokenBundle(ctx context.Context, store secrets.SecretStore, accountID string, bundle TokenBundle) error {
	encoded, err := encodeTokenBundle(bundle)
	if err != nil {
		return err
	}
	if err := store.Set(ctx, tokenBundleKey(accountID), encoded); err != nil {
		return mapSecretStoreError(err)
	}
	return nil
}

// LoadTokenBundle retrieves and decodes one account's token bundle.
func LoadTokenBundle(ctx context.Context, store secrets.SecretStore, accountID string) (TokenBundle, error) {
	raw, err := store.Get(ctx, tokenBundleKey(accountID))
	if err != nil {
		return TokenBundle{}, mapSecretStoreError(err)
	}
	return decodeTokenBundle(raw)
}

// DeleteTokenBundle removes one account's token bundle. Deleting an absent
// bundle is not an error - mirrors credential.Service.DeleteStreamKey's own
// idempotent-delete reasoning.
func DeleteTokenBundle(ctx context.Context, store secrets.SecretStore, accountID string) error {
	err := store.Delete(ctx, tokenBundleKey(accountID))
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
