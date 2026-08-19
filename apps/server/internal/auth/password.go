// Package auth is the Stage 20D2B single-administrator authentication
// foundation: Argon2id password verification, opaque server-side
// sessions with per-session CSRF tokens, and login rate limiting. It
// has no dependency on remote-management being enabled and no
// dependency on any transport - internal/httpapi wires it into the
// actual HTTP surface only when --remote-management is set (see
// docs/remote-management.md §3).
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters: RFC 9106 §4's second recommended option (for
// environments without dedicated authentication hardware - this
// application shares a general-purpose host with MediaMTX/FFmpeg and
// other services), used exactly as specified, not an invented value.
// docs/remote-management.md §9.1/§2 records the research this is
// drawn from.
const (
	argon2Time      = 3
	argon2MemoryKiB = 64 * 1024
	argon2Threads   = 4
	argon2KeyLen    = 32
	saltLen         = 16

	// argon2Version is Argon2's own version identifier (0x13 = 19
	// decimal, the current version both RFC 9106 and this package's
	// pinned golang.org/x/crypto/argon2 implement) - stored in the
	// verifier so a future library upgrade to a new version can detect
	// and reject (or deliberately support) an old-version verifier
	// rather than silently misinterpreting it.
	argon2Version = argon2.Version

	verifierAlgorithm = "argon2id"

	// Bounds a parsed verifier's own parameters must fall within before
	// they are ever passed to argon2.IDKey - a corrupted or malicious
	// verifier string can never itself trigger a resource-exhausting
	// derivation. Generous enough to comfortably contain the real
	// constants above plus headroom for a deliberate future parameter
	// upgrade, never so wide that a bad value is still practically
	// unbounded.
	maxParsedTime      = 32
	maxParsedMemoryKiB = 4 * 1024 * 1024 // 4 GiB
	maxParsedThreads   = 64
	maxParsedSaltLen   = 64
	maxParsedHashLen   = 128

	// MaxPasswordLength bounds the accepted password length before
	// hashing - Argon2's own cost is independent of password length,
	// but an unbounded input is still an easy memory/CPU amplification
	// vector at the HTTP layer, and no legitimate administrator
	// password is anywhere near this long.
	MaxPasswordLength = 256
)

// ErrEmptyPassword and ErrPasswordTooLong are returned by HashPassword
// before any Argon2id work happens - fast, deterministic input
// validation.
var (
	ErrEmptyPassword   = errors.New("password must not be empty")
	ErrPasswordTooLong = fmt.Errorf("password must not exceed %d bytes", MaxPasswordLength)

	// ErrMalformedVerifier covers every way a stored verifier string can
	// fail to parse: wrong field count, unknown algorithm, unsupported
	// version, non-numeric or out-of-bounds parameters, invalid
	// base64, or a salt/hash of the wrong length. Never distinguishes
	// which - a caller must not be able to use parse-failure detail to
	// probe the stored format.
	ErrMalformedVerifier = errors.New("malformed password verifier")
)

// HashPassword derives a new, self-describing Argon2id verifier for
// password, using a fresh crypto/rand salt - two calls for the same
// password never produce the same verifier.
func HashPassword(password string) (string, error) {
	if len(password) == 0 {
		return "", ErrEmptyPassword
	}
	if len(password) > MaxPasswordLength {
		return "", ErrPasswordTooLong
	}

	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generating salt: %w", err)
	}

	hash := argon2.IDKey([]byte(password), salt, argon2Time, argon2MemoryKiB, argon2Threads, argon2KeyLen)

	return formatVerifier(argon2Version, argon2Time, argon2MemoryKiB, argon2Threads, salt, hash), nil
}

// VerifyPassword reports whether password matches the given verifier
// string, using a constant-time comparison of the derived hash. A
// malformed verifier is treated as "does not match" (ErrMalformedVerifier
// is returned alongside false so a caller can distinguish "wrong
// password" from "corrupted stored state" for logging/diagnostics
// purposes only - never for a different HTTP response, per
// docs/remote-management.md §9's "wrong password and absent account
// state must not expose useful remote distinctions" contract).
func VerifyPassword(password, verifier string) (bool, error) {
	if len(password) == 0 || len(password) > MaxPasswordLength {
		return false, nil
	}

	version, time, memoryKiB, threads, salt, hash, err := parseVerifier(verifier)
	if err != nil {
		return false, err
	}

	// A future parameter/version upgrade may need to derive with
	// different values than today's constants describe - the parsed,
	// bounds-checked values from the verifier itself are always used
	// for the actual derivation, never the package's own current
	// constants, so an old verifier keeps verifying correctly across a
	// future upgrade (docs/remote-management.md §9.1).
	_ = version
	candidate := argon2.IDKey([]byte(password), salt, time, memoryKiB, threads, uint32(len(hash)))

	return subtle.ConstantTimeCompare(candidate, hash) == 1, nil
}

// formatVerifier renders the strict, versioned, self-describing format
// documented in docs/remote-management.md §9.1:
//
//	argon2id$v=19$m=65536,t=3,p=4$<base64 salt>$<base64 hash>
func formatVerifier(version int, time, memoryKiB uint32, threads uint8, salt, hash []byte) string {
	return fmt.Sprintf("%s$v=%d$m=%d,t=%d,p=%d$%s$%s",
		verifierAlgorithm, version, memoryKiB, time, threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)
}

// parseVerifier parses and bounds-checks a verifier string produced by
// formatVerifier, rejecting anything malformed, oversized, or using an
// unsupported algorithm/version before any value is used.
func parseVerifier(verifier string) (version int, time, memoryKiB uint32, threads uint8, salt, hash []byte, err error) {
	// A pathological input (e.g. a multi-megabyte string) is rejected by
	// length before any splitting/parsing work - bounded independent of
	// the individual field bounds below.
	if len(verifier) > 1024 {
		return 0, 0, 0, 0, nil, nil, ErrMalformedVerifier
	}

	parts := strings.Split(verifier, "$")
	if len(parts) != 5 {
		return 0, 0, 0, 0, nil, nil, ErrMalformedVerifier
	}
	if parts[0] != verifierAlgorithm {
		return 0, 0, 0, 0, nil, nil, ErrMalformedVerifier
	}

	v, ok := parseKeyValue(parts[1], "v")
	if !ok {
		return 0, 0, 0, 0, nil, nil, ErrMalformedVerifier
	}
	parsedVersion, err := strconv.Atoi(v)
	if err != nil || parsedVersion != argon2Version {
		return 0, 0, 0, 0, nil, nil, ErrMalformedVerifier
	}

	params := strings.Split(parts[2], ",")
	if len(params) != 3 {
		return 0, 0, 0, 0, nil, nil, ErrMalformedVerifier
	}
	mRaw, ok := parseKeyValue(params[0], "m")
	if !ok {
		return 0, 0, 0, 0, nil, nil, ErrMalformedVerifier
	}
	tRaw, ok := parseKeyValue(params[1], "t")
	if !ok {
		return 0, 0, 0, 0, nil, nil, ErrMalformedVerifier
	}
	pRaw, ok := parseKeyValue(params[2], "p")
	if !ok {
		return 0, 0, 0, 0, nil, nil, ErrMalformedVerifier
	}

	m, err := strconv.ParseUint(mRaw, 10, 32)
	if err != nil || m == 0 || m > maxParsedMemoryKiB {
		return 0, 0, 0, 0, nil, nil, ErrMalformedVerifier
	}
	t, err := strconv.ParseUint(tRaw, 10, 32)
	if err != nil || t == 0 || t > maxParsedTime {
		return 0, 0, 0, 0, nil, nil, ErrMalformedVerifier
	}
	p, err := strconv.ParseUint(pRaw, 10, 8)
	if err != nil || p == 0 || p > maxParsedThreads {
		return 0, 0, 0, 0, nil, nil, ErrMalformedVerifier
	}

	saltBytes, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(saltBytes) == 0 || len(saltBytes) > maxParsedSaltLen {
		return 0, 0, 0, 0, nil, nil, ErrMalformedVerifier
	}
	hashBytes, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(hashBytes) == 0 || len(hashBytes) > maxParsedHashLen {
		return 0, 0, 0, 0, nil, nil, ErrMalformedVerifier
	}

	return parsedVersion, uint32(t), uint32(m), uint8(p), saltBytes, hashBytes, nil
}

// parseKeyValue splits "key=value" and confirms the key matches want.
func parseKeyValue(field, want string) (string, bool) {
	key, value, found := strings.Cut(field, "=")
	if !found || key != want {
		return "", false
	}
	return value, true
}
