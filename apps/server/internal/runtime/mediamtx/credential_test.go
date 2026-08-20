package mediamtx

import (
	"strings"
	"testing"
)

func TestNewPublisherSecretHas256BitsOfEntropy(t *testing.T) {
	secret, err := NewPublisherSecret()
	if err != nil {
		t.Fatalf("NewPublisherSecret() returned an error: %v", err)
	}
	// base64url-no-padding of 32 bytes is 43 characters (ceil(32*8/6),
	// no trailing padding).
	if len(secret) != 43 {
		t.Errorf("len(secret) = %d, want 43 (32 bytes, base64url no padding)", len(secret))
	}
	for _, forbidden := range []string{"+", "/", "="} {
		if strings.Contains(secret, forbidden) {
			t.Errorf("secret contains %q, not filesystem/URL-safe", forbidden)
		}
	}
}

func TestNewPublisherSecretIsRandomEachCall(t *testing.T) {
	first, err := NewPublisherSecret()
	if err != nil {
		t.Fatalf("NewPublisherSecret() returned an error: %v", err)
	}
	second, err := NewPublisherSecret()
	if err != nil {
		t.Fatalf("NewPublisherSecret() returned an error: %v", err)
	}
	if first == second {
		t.Error("two consecutive calls returned the same secret")
	}
}

func TestPublisherPassVerifierForMatchesMediaMTXsOwnDocumentedRecipe(t *testing.T) {
	// docs/2-features/06-authentication.md (MediaMTX v1.19.3, cited in
	// docs/remote-ingest.md §1): "echo -n "mypass" | openssl dgst
	// -binary -sha256 | openssl base64. Then store with sha256: prefix
	// in the config." Independently verified via the real openssl CLI:
	// printf '%s' "mypass" | openssl dgst -binary -sha256 | openssl
	// base64 -> 6nHCWnpgIka0w5gkuFVniJSpb0O7m3ExnDlwCh4EUiI=
	got := PublisherPassVerifierFor("mypass")
	want := "sha256:6nHCWnpgIka0w5gkuFVniJSpb0O7m3ExnDlwCh4EUiI="
	if got != want {
		t.Errorf("PublisherPassVerifierFor(%q) = %q, want %q (cross-checked against the real openssl CLI)", "mypass", got, want)
	}
}

func TestPublisherPassVerifierForNeverEmbedsThePlaintextSecret(t *testing.T) {
	secret := "super-secret-plaintext-value"
	verifier := PublisherPassVerifierFor(secret)
	if strings.Contains(verifier, secret) {
		t.Error("the verifier contains the plaintext secret")
	}
	if !strings.HasPrefix(verifier, "sha256:") {
		t.Errorf("verifier = %q, want the sha256: prefix", verifier)
	}
}

func TestPublisherPassVerifierForIsDeterministic(t *testing.T) {
	secret := "the-same-secret"
	if PublisherPassVerifierFor(secret) != PublisherPassVerifierFor(secret) {
		t.Error("PublisherPassVerifierFor produced different output for the same input")
	}
}

func TestPublisherPassVerifierForDiffersForDifferentSecrets(t *testing.T) {
	if PublisherPassVerifierFor("secret-one") == PublisherPassVerifierFor("secret-two") {
		t.Error("two different secrets produced the same verifier")
	}
}
