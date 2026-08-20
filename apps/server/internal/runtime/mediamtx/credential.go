package mediamtx

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// publisherSecretBytes is 256 bits (docs/remote-ingest.md §6) - a
// machine-generated capability, not a human-chosen password, so a
// one-way sha256: verifier (see PublisherPassVerifierFor) is an
// acceptable threat model without a slow KDF, unlike the D2B
// administrator password.
const publisherSecretBytes = 32

// NewPublisherSecret returns a fresh, random remote-ingest publisher
// secret (docs/remote-ingest.md §6), base64url-no-padding encoded -
// filesystem/URL-safe, since it ends up in the RTMPS query string a
// real OBS "Server"/"Stream Key" field carries
// (?user=...&pass=..., per MediaMTX's own documented RTMP credential
// mechanism).
func NewPublisherSecret() (string, error) {
	buf := make([]byte, publisherSecretBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate remote ingest publisher secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// PublisherPassVerifierFor returns the MediaMTX-native "sha256:
// <base64(sha256(secret))>" verifier for secret - verified directly
// against MediaMTX v1.19.3's own authentication documentation
// (docs/remote-ingest.md §1/§6). Never returns or logs the plaintext
// secret itself; the caller is responsible for that discipline on its
// own end (docs/remote-ingest.md §6/§8: return once, persist only
// this verifier).
func PublisherPassVerifierFor(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return "sha256:" + base64.StdEncoding.EncodeToString(sum[:])
}
