// Package remoteoverlay is Stage 20D2C's shared remote-overlay
// capability mapping (docs/remote-ingest.md §12) - a single table and
// service shared by every overlay domain (chat overlay, alert
// profile, audio, supporter widget), rather than a duplicated
// remote-capability column/lifecycle in each domain's own schema. See
// docs/remote-ingest.md §12's own "implementation simplification" note
// for the reasoning.
package remoteoverlay

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"
)

// Domain names one of the four overlay surfaces a capability token can
// grant remote access to. A fixed, closed set - never an arbitrary
// caller-supplied string - because the repository validates every
// value against it.
type Domain string

const (
	DomainChatOverlay  Domain = "chat-overlay"
	DomainAlertProfile Domain = "alert-profile"
	DomainAudio        Domain = "audio"
	DomainWidget       Domain = "widget"
)

// ValidDomains lists every accepted Domain value, in a stable order -
// used for validation and for enumerating them in tests/documentation.
var ValidDomains = []Domain{DomainChatOverlay, DomainAlertProfile, DomainAudio, DomainWidget}

// IsValid reports whether d is one of ValidDomains.
func (d Domain) IsValid() bool {
	for _, valid := range ValidDomains {
		if d == valid {
			return true
		}
	}
	return false
}

// Capability is one issued remote-overlay capability: possession of
// Token grants remote read access to the overlay profile identified by
// (Domain, LocalSlug) - the same profile the existing local
// publicSlug already identifies for direct loopback access.
type Capability struct {
	Token     string
	Domain    Domain
	LocalSlug string
	CreatedAt time.Time
}

// tokenBytes is 256 bits (docs/remote-ingest.md §12), matching
// internal/domain/visualasset.NewPublicToken's own precedent for the
// codebase's other Internet-facing capability-shaped value - wider
// than the 160-bit local publicSlug, since this token is deliberately
// exposed to a broader audience (anyone the operator gives the remote
// overlay URL to, not just their own local OBS installation).
const tokenBytes = 32

// NewToken returns a fresh, random remote-overlay capability token,
// base64url-no-padding encoded - shorter in a URL than hex for the
// same entropy, and filesystem/URL-safe.
func NewToken() (string, error) {
	buf := make([]byte, tokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate remote overlay capability token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// ErrInvalidDomain is returned for a Domain that is not one of
// ValidDomains.
var ErrInvalidDomain = errors.New("invalid remote overlay domain")

// Repository is the storage port this package needs - implemented by
// internal/storage/sqlite.RemoteOverlayCapabilityRepository in
// production.
type Repository interface {
	// Issue generates a fresh token and atomically replaces any
	// previous capability for (domain, localSlug) - enable and rotate
	// share this one operation, exactly like
	// internal/remoteingest.Manager's own Provision/Rotate share
	// generateAndApply.
	Issue(ctx context.Context, domain Domain, localSlug string) (Capability, error)

	// Revoke removes any capability for (domain, localSlug).
	// Idempotent: revoking an already-absent capability is not an
	// error.
	Revoke(ctx context.Context, domain Domain, localSlug string) error

	// Get returns the current capability for (domain, localSlug), or
	// (Capability{}, false, nil) if none is issued.
	Get(ctx context.Context, domain Domain, localSlug string) (Capability, bool, error)

	// Resolve looks up a presented token and returns the real local
	// slug it grants access to, or ("", false, nil) if the token does
	// not currently match any issued capability for domain - a
	// revoked or rotated-away token simply fails to resolve, by
	// construction, not by a separate "disabled" flag.
	Resolve(ctx context.Context, domain Domain, token string) (localSlug string, ok bool, err error)
}
