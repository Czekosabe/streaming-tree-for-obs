package secrets

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"testing"
)

// TestKeyringStoreAgainstTheRealOSCredentialStore is an explicit opt-in
// smoke test against whatever real credential store is available on the
// machine running it (Windows Credential Manager, macOS Keychain, or Linux
// Secret Service).
//
// It is skipped unless STREAMING_TREE_CREDENTIAL_SMOKE_TEST=1 is already set
// in the environment - this test never sets that flag itself, and nothing in
// the normal build, lint or test commands sets it either. It uses a random,
// disposable service name and a random, meaningless secret value: never the
// production ServiceName, never a real platform ID, never a real stream key.
// It removes what it wrote as its last step, and also attempts cleanup
// immediately via t.Cleanup so a failing assertion does not leave residue
// behind.
func TestKeyringStoreAgainstTheRealOSCredentialStore(t *testing.T) {
	if os.Getenv("STREAMING_TREE_CREDENTIAL_SMOKE_TEST") != "1" {
		t.Skip("skipping: set STREAMING_TREE_CREDENTIAL_SMOKE_TEST=1 to run against the real OS credential store")
	}

	serviceName := "streaming-tree-smoketest-" + randomHex(t, 8)
	key := "smoketest-" + randomHex(t, 8)
	value := []byte("smoketest-value-" + randomHex(t, 8))

	store := newKeyringStoreForTesting(serviceName)
	ctx := context.Background()

	t.Cleanup(func() {
		_ = store.Delete(ctx, key)
	})

	if err := store.Set(ctx, key, value); err != nil {
		if errors.Is(err, ErrUnavailable) {
			t.Skipf("no usable OS credential store on this system: %v", err)
		}
		t.Fatalf("Set() error = %v", err)
	}

	exists, err := store.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if !exists {
		t.Fatal("Exists() = false immediately after Set()")
	}

	got, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if string(got) != string(value) {
		t.Fatalf("Get() = %q, want %q", got, value)
	}

	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	exists, err = store.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists() after Delete() error = %v", err)
	}
	if exists {
		t.Fatal("Exists() = true after Delete()")
	}
}

func randomHex(t *testing.T, n int) string {
	t.Helper()
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("generating random bytes failed: %v", err)
	}
	return hex.EncodeToString(buf)
}
