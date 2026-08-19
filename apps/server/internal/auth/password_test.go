package auth

import (
	"strings"
	"testing"
)

func TestHashAndVerifyPasswordRoundTrip(t *testing.T) {
	verifier, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	ok, err := VerifyPassword("correct horse battery staple", verifier)
	if err != nil {
		t.Fatalf("VerifyPassword() error = %v", err)
	}
	if !ok {
		t.Error("VerifyPassword() = false for the correct password, want true")
	}
}

func TestVerifyPasswordWrongPassword(t *testing.T) {
	verifier, err := HashPassword("the real password")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	ok, err := VerifyPassword("not the real password", verifier)
	if err != nil {
		t.Fatalf("VerifyPassword() error = %v", err)
	}
	if ok {
		t.Error("VerifyPassword() = true for the wrong password, want false")
	}
}

func TestHashPasswordUsesRandomSalt(t *testing.T) {
	v1, err := HashPassword("same password")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	v2, err := HashPassword("same password")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if v1 == v2 {
		t.Error("two hashes of the same password produced an identical verifier - salt is not random")
	}

	// Both must still independently verify.
	if ok, _ := VerifyPassword("same password", v1); !ok {
		t.Error("first verifier does not verify its own password")
	}
	if ok, _ := VerifyPassword("same password", v2); !ok {
		t.Error("second verifier does not verify its own password")
	}
}

func TestHashPasswordEmptyRejected(t *testing.T) {
	if _, err := HashPassword(""); err != ErrEmptyPassword {
		t.Errorf("HashPassword(\"\") error = %v, want ErrEmptyPassword", err)
	}
}

func TestHashPasswordTooLongRejected(t *testing.T) {
	tooLong := strings.Repeat("x", MaxPasswordLength+1)
	if _, err := HashPassword(tooLong); err != ErrPasswordTooLong {
		t.Errorf("HashPassword(too long) error = %v, want ErrPasswordTooLong", err)
	}
}

func TestHashPasswordMaxLengthAccepted(t *testing.T) {
	maxLen := strings.Repeat("x", MaxPasswordLength)
	verifier, err := HashPassword(maxLen)
	if err != nil {
		t.Fatalf("HashPassword(max length) error = %v", err)
	}
	ok, err := VerifyPassword(maxLen, verifier)
	if err != nil || !ok {
		t.Errorf("VerifyPassword(max length) = %v, %v, want true, nil", ok, err)
	}
}

func TestVerifyPasswordUnicodeSupported(t *testing.T) {
	password := "pąssw🔒rd-日本語-Zażółć"
	verifier, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword(unicode) error = %v", err)
	}
	ok, err := VerifyPassword(password, verifier)
	if err != nil || !ok {
		t.Errorf("VerifyPassword(unicode) = %v, %v, want true, nil", ok, err)
	}
}

func TestVerifyPasswordEmptyCandidateRejected(t *testing.T) {
	verifier, err := HashPassword("some password")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	ok, err := VerifyPassword("", verifier)
	if err != nil {
		t.Fatalf("VerifyPassword(\"\") error = %v, want nil", err)
	}
	if ok {
		t.Error("VerifyPassword(\"\") = true, want false")
	}
}

func TestVerifyPasswordMalformedVerifierFormats(t *testing.T) {
	valid, err := HashPassword("password")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	parts := strings.Split(valid, "$")
	if len(parts) != 5 {
		t.Fatalf("unexpected verifier shape: %d parts", len(parts))
	}

	cases := map[string]string{
		"empty string":         "",
		"wrong field count":    "argon2id$v=19$m=65536,t=3,p=4$onlyonefield",
		"unknown algorithm":    "bcrypt$v=19$m=65536,t=3,p=4$" + parts[3] + "$" + parts[4],
		"unsupported version":  "argon2id$v=1$m=65536,t=3,p=4$" + parts[3] + "$" + parts[4],
		"non-numeric version":  "argon2id$v=abc$m=65536,t=3,p=4$" + parts[3] + "$" + parts[4],
		"unsupported algo m=0": "argon2id$v=19$m=0,t=3,p=4$" + parts[3] + "$" + parts[4],
		"absurd memory":        "argon2id$v=19$m=999999999999,t=3,p=4$" + parts[3] + "$" + parts[4],
		"absurd time":          "argon2id$v=19$m=65536,t=999999999,p=4$" + parts[3] + "$" + parts[4],
		"absurd threads":       "argon2id$v=19$m=65536,t=3,p=999999$" + parts[3] + "$" + parts[4],
		"zero threads":         "argon2id$v=19$m=65536,t=3,p=0$" + parts[3] + "$" + parts[4],
		"invalid base64 salt":  "argon2id$v=19$m=65536,t=3,p=4$not-valid-base64!!!$" + parts[4],
		"invalid base64 hash":  "argon2id$v=19$m=65536,t=3,p=4$" + parts[3] + "$not-valid-base64!!!",
		"empty salt":           "argon2id$v=19$m=65536,t=3,p=4$$" + parts[4],
		"empty hash":           "argon2id$v=19$m=65536,t=3,p=4$" + parts[3] + "$",
		"missing m key":        "argon2id$v=19$x=65536,t=3,p=4$" + parts[3] + "$" + parts[4],
		"wrong param count":    "argon2id$v=19$m=65536,t=3$" + parts[3] + "$" + parts[4],
	}

	for name, verifier := range cases {
		t.Run(name, func(t *testing.T) {
			ok, err := VerifyPassword("password", verifier)
			if ok {
				t.Errorf("VerifyPassword(%q) = true, want false", verifier)
			}
			if err != ErrMalformedVerifier {
				t.Errorf("VerifyPassword(%q) error = %v, want ErrMalformedVerifier", verifier, err)
			}
		})
	}
}

func TestVerifyPasswordOversizedVerifierRejectedBeforeParsing(t *testing.T) {
	huge := "argon2id$" + strings.Repeat("x", 2000)
	ok, err := VerifyPassword("password", huge)
	if ok {
		t.Error("VerifyPassword(oversized) = true, want false")
	}
	if err != ErrMalformedVerifier {
		t.Errorf("VerifyPassword(oversized) error = %v, want ErrMalformedVerifier", err)
	}
}

func TestVerifyPasswordTruncatedHashRejected(t *testing.T) {
	verifier, err := HashPassword("password")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	// Truncate the final (hash) segment.
	idx := strings.LastIndex(verifier, "$")
	truncated := verifier[:idx+1] + verifier[idx+2:]
	ok, err := VerifyPassword("password", truncated)
	if ok {
		t.Error("VerifyPassword(truncated hash) = true, want false")
	}
	// A truncated base64 string may or may not still decode depending on
	// padding - either a malformed-verifier error or a clean false
	// (constant-time mismatch against a shorter hash) is acceptable;
	// what must never happen is a true result.
	_ = err
}

func TestVerifyPasswordModifiedHashRejected(t *testing.T) {
	verifier, err := HashPassword("password")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	// Flip the verifier's last character - still valid base64 shape
	// (same length), but a different byte value, so this exercises the
	// constant-time comparison's mismatch path specifically rather than
	// a parse failure.
	mutated := []rune(verifier)
	last := len(mutated) - 1
	if mutated[last] == 'A' {
		mutated[last] = 'B'
	} else {
		mutated[last] = 'A'
	}
	ok, err := VerifyPassword("password", string(mutated))
	if err != nil {
		// A mutated final character may occasionally still fail base64
		// decoding depending on padding boundaries - accept either a
		// clean false or ErrMalformedVerifier, never a true result.
		if err != ErrMalformedVerifier {
			t.Fatalf("VerifyPassword(mutated) unexpected error = %v", err)
		}
		return
	}
	if ok {
		t.Error("VerifyPassword(mutated hash) = true, want false")
	}
}

func TestFormatVerifierMatchesDocumentedShape(t *testing.T) {
	verifier, err := HashPassword("password")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	// docs/remote-management.md §9.1:
	// argon2id$v=19$m=65536,t=3,p=4$<salt>$<hash>
	want := "argon2id$v=19$m=65536,t=3,p=4$"
	if !strings.HasPrefix(verifier, want) {
		t.Errorf("verifier = %q, want prefix %q", verifier, want)
	}
}
