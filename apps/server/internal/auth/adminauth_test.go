package auth

import (
	"context"
	"testing"

	"github.com/streaming-tree/server/internal/secrets/secretstest"
)

func TestAdminAuthenticatorVerifyPasswordCorrect(t *testing.T) {
	store := secretstest.New()
	ctx := context.Background()

	if err := SetAdminPassword(ctx, store, "correct-password"); err != nil {
		t.Fatalf("SetAdminPassword() error = %v", err)
	}

	authn := AdminAuthenticator{Store: store}
	ok, err := authn.VerifyPassword(ctx, "correct-password")
	if err != nil {
		t.Fatalf("VerifyPassword() error = %v", err)
	}
	if !ok {
		t.Error("VerifyPassword() = false for the correct password, want true")
	}
}

func TestAdminAuthenticatorVerifyPasswordWrong(t *testing.T) {
	store := secretstest.New()
	ctx := context.Background()

	if err := SetAdminPassword(ctx, store, "correct-password"); err != nil {
		t.Fatalf("SetAdminPassword() error = %v", err)
	}

	authn := AdminAuthenticator{Store: store}
	ok, err := authn.VerifyPassword(ctx, "wrong-password")
	if err != nil {
		t.Fatalf("VerifyPassword() error = %v", err)
	}
	if ok {
		t.Error("VerifyPassword() = true for the wrong password, want false")
	}
}

func TestAdminAuthenticatorVerifyPasswordNoneProvisioned(t *testing.T) {
	store := secretstest.New()
	authn := AdminAuthenticator{Store: store}

	ok, err := authn.VerifyPassword(context.Background(), "any-password")
	if err != nil {
		t.Fatalf("VerifyPassword() error = %v, want nil (treated as no match, not an error)", err)
	}
	if ok {
		t.Error("VerifyPassword() = true with no verifier ever provisioned, want false")
	}
}

func TestSetAdminPasswordOverwritesPrevious(t *testing.T) {
	store := secretstest.New()
	ctx := context.Background()

	if err := SetAdminPassword(ctx, store, "first-password"); err != nil {
		t.Fatalf("SetAdminPassword() error = %v", err)
	}
	if err := SetAdminPassword(ctx, store, "second-password"); err != nil {
		t.Fatalf("SetAdminPassword() error = %v", err)
	}

	authn := AdminAuthenticator{Store: store}
	if ok, _ := authn.VerifyPassword(ctx, "first-password"); ok {
		t.Error("the old password still verifies after a reset, want false")
	}
	if ok, _ := authn.VerifyPassword(ctx, "second-password"); !ok {
		t.Error("the new password does not verify after a reset, want true")
	}
}

func TestAdminPasswordProvisionedReflectsState(t *testing.T) {
	store := secretstest.New()
	ctx := context.Background()

	provisioned, err := AdminPasswordProvisioned(ctx, store)
	if err != nil {
		t.Fatalf("AdminPasswordProvisioned() error = %v", err)
	}
	if provisioned {
		t.Error("AdminPasswordProvisioned() = true before any password was ever set, want false")
	}

	if err := SetAdminPassword(ctx, store, "a-password"); err != nil {
		t.Fatalf("SetAdminPassword() error = %v", err)
	}

	provisioned, err = AdminPasswordProvisioned(ctx, store)
	if err != nil {
		t.Fatalf("AdminPasswordProvisioned() error = %v", err)
	}
	if !provisioned {
		t.Error("AdminPasswordProvisioned() = false after SetAdminPassword, want true")
	}
}

func TestAdminPasswordNeverStoredAsPlaintext(t *testing.T) {
	store := secretstest.New()
	ctx := context.Background()
	const password = "a very identifiable plaintext password 12345"

	if err := SetAdminPassword(ctx, store, password); err != nil {
		t.Fatalf("SetAdminPassword() error = %v", err)
	}

	raw, err := store.Get(ctx, adminPasswordKey())
	if err != nil {
		t.Fatalf("store.Get() error = %v", err)
	}
	if string(raw) == password {
		t.Fatal("the stored verifier equals the plaintext password")
	}
	if containsSubstring(string(raw), password) {
		t.Fatal("the stored verifier contains the plaintext password as a substring")
	}
}

func containsSubstring(haystack, needle string) bool {
	return len(needle) > 0 && (len(haystack) >= len(needle)) &&
		(func() bool {
			for i := 0; i+len(needle) <= len(haystack); i++ {
				if haystack[i:i+len(needle)] == needle {
					return true
				}
			}
			return false
		})()
}
