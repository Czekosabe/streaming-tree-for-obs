package account

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/secrets/secretstest"
)

func testBundle() TokenBundle {
	return TokenBundle{
		TokenType: "bearer", AccessToken: "fake-access-token", RefreshToken: "fake-refresh-token",
		ExpiresAt: time.Now().Add(time.Hour),
	}
}

func TestStoreAndLoadTokenBundleRoundTrips(t *testing.T) {
	store := secretstest.New()
	ctx := context.Background()

	if err := StoreTokenBundle(ctx, store, "acct_1", testBundle()); err != nil {
		t.Fatalf("StoreTokenBundle() error = %v", err)
	}

	got, err := LoadTokenBundle(ctx, store, "acct_1")
	if err != nil {
		t.Fatalf("LoadTokenBundle() error = %v", err)
	}
	if got.AccessToken != "fake-access-token" || got.RefreshToken != "fake-refresh-token" {
		t.Errorf("LoadTokenBundle() = %+v, want the stored tokens back", got)
	}
}

func TestStoreTokenBundleRejectsAnUnsupportedTokenType(t *testing.T) {
	store := secretstest.New()
	bundle := testBundle()
	bundle.TokenType = "mac"

	err := StoreTokenBundle(context.Background(), store, "acct_1", bundle)
	if !errors.Is(err, ErrStorage) {
		t.Errorf("StoreTokenBundle() error = %v, want ErrStorage", err)
	}
	if store.Len() != 0 {
		t.Error("an unsupported bundle was written to the store")
	}
}

func TestStoreTokenBundleRejectsAMissingField(t *testing.T) {
	store := secretstest.New()
	bundle := testBundle()
	bundle.RefreshToken = ""

	if err := StoreTokenBundle(context.Background(), store, "acct_1", bundle); !errors.Is(err, ErrStorage) {
		t.Errorf("StoreTokenBundle() error = %v, want ErrStorage", err)
	}
}

func TestLoadTokenBundleRejectsAnOversizedValue(t *testing.T) {
	store := secretstest.New()
	huge := make([]byte, maxTokenBundleBytes+1)
	for i := range huge {
		huge[i] = 'x'
	}
	if err := store.Set(context.Background(), tokenBundleKey("acct_1"), huge); err != nil {
		t.Fatalf("seeding the store failed: %v", err)
	}

	_, err := LoadTokenBundle(context.Background(), store, "acct_1")
	if !errors.Is(err, ErrStorage) {
		t.Errorf("LoadTokenBundle() error = %v, want ErrStorage", err)
	}
}

func TestLoadTokenBundleRejectsMalformedJSON(t *testing.T) {
	store := secretstest.New()
	if err := store.Set(context.Background(), tokenBundleKey("acct_1"), []byte("not json")); err != nil {
		t.Fatalf("seeding the store failed: %v", err)
	}

	_, err := LoadTokenBundle(context.Background(), store, "acct_1")
	if !errors.Is(err, ErrStorage) {
		t.Errorf("LoadTokenBundle() error = %v, want ErrStorage", err)
	}
}

func TestLoadTokenBundleRejectsUnknownFields(t *testing.T) {
	store := secretstest.New()
	payload := `{"tokenType":"bearer","accessToken":"a","refreshToken":"r","expiresAt":"2030-01-01T00:00:00Z","clientSecret":"leak"}`
	if err := store.Set(context.Background(), tokenBundleKey("acct_1"), []byte(payload)); err != nil {
		t.Fatalf("seeding the store failed: %v", err)
	}

	_, err := LoadTokenBundle(context.Background(), store, "acct_1")
	if !errors.Is(err, ErrStorage) {
		t.Errorf("LoadTokenBundle() error = %v, want ErrStorage - an unexpected field must not be silently accepted", err)
	}
}

func TestDeleteTokenBundleIsIdempotentForAnAbsentBundle(t *testing.T) {
	store := secretstest.New()
	if err := DeleteTokenBundle(context.Background(), store, "acct_never_stored"); err != nil {
		t.Errorf("DeleteTokenBundle() on an absent bundle error = %v, want nil", err)
	}
}

func TestTokenBundleKeyNamespaceIsStable(t *testing.T) {
	key := tokenBundleKey("acct_1")
	if !strings.HasPrefix(key, "oauth-token-bundle:") {
		t.Errorf("tokenBundleKey() = %q, want the oauth-token-bundle: prefix", key)
	}
	if key != tokenBundleKey("acct_1") {
		t.Error("tokenBundleKey() is not deterministic for the same account id")
	}
	if tokenBundleKey("acct_1") == tokenBundleKey("acct_2") {
		t.Error("two different accounts produced the same secret key")
	}
}

func TestEncodeTokenBundleErrorNeverContainsTheTokenValue(t *testing.T) {
	bundle := testBundle()
	bundle.TokenType = "mac"

	_, err := encodeTokenBundle(bundle)
	if err == nil {
		t.Fatal("expected an error for an unsupported token type")
	}
	if strings.Contains(err.Error(), bundle.AccessToken) || strings.Contains(err.Error(), bundle.RefreshToken) {
		t.Errorf("error message leaked a token value: %v", err)
	}
}
