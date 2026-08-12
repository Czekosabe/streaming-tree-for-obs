// Package donationsource holds the provider-independent external-donation
// source domain (Stage 16A): a local record of one operator-configured
// external donation service connection - StreamElements first - and its
// credential, kept entirely separate from internal/domain/account.
//
// This is a deliberate design decision, not an oversight: account.Account
// and account.Provider are OAuth-shaped throughout (Login, Scopes, a
// ValidateToken/RefreshToken/RevokeToken/GetIdentity provider interface, a
// StatusReconnectRequired meaning "repeat the OAuth authorization flow").
// A StreamElements personal JWT (see docs/provider-integrations/
// external-donations.md §3) has no login, no scopes, no refresh token, and
// no revoke endpoint this application calls - forcing it through
// account.Account's shape would mean faking several Provider interface
// methods to do nothing meaningful. A donation source is also not a
// streaming destination: it must never appear in platform destination
// CRUD, FFmpeg branch configuration, or stream-key configuration - it is
// an engagement source only.
//
// Deliberately excludes, and must never carry: any JWT/API key/OAuth
// token, any reconnect token, any donor identity, any donation event or
// donation history. The credential lives only in the OS credential store
// (internal/secrets), addressed by the source's ID - see credential.go.
// Runtime connection state (connecting/connected/reconnecting/error) is
// never persisted here either - see internal/runtime/
// streamelementsengagement's own Snapshot, mirroring internal/runtime/
// youtubeengagement's identical split between "what is configured" (this
// package) and "what is happening right now" (that runtime package).
package donationsource

import "time"

// ProviderID identifies which external donation service a source
// connects to. Deliberately its own type, never engagement.ProviderID/
// account.ProviderID/alerts.ProviderID - see those packages' own doc
// comments for the same reasoning repeated at every domain boundary in
// this project.
type ProviderID string

// ProviderStreamElements is Stage 16A's only supported donation-source
// provider. Streamlabs and Ko-fi are deliberately not implemented - see
// docs/provider-integrations/external-donations.md §2.
const ProviderStreamElements ProviderID = "streamelements"

// ValidProviders lists every accepted ProviderID.
var ValidProviders = []ProviderID{ProviderStreamElements}

func (p ProviderID) valid() bool {
	for _, v := range ValidProviders {
		if p == v {
			return true
		}
	}
	return false
}

// Source is one persisted external-donation source: safe metadata only.
// Its credential is never a field here - see credential.go.
type Source struct {
	ID         string
	ProviderID ProviderID

	// Label is the operator's own name for this source (e.g. "Main
	// channel donations") - shown in the UI, never used for matching or
	// identity.
	Label string

	// Enabled is the operator's explicit choice to receive donations
	// through this source. Mirrors connected_account_engagement_
	// settings.enabled exactly: the only persisted fact about whether
	// this source should run - never a runtime status.
	Enabled bool

	// RemoteChannelID is the donation service's own channel/account
	// identifier (StreamElements: the "Account ID" shown next to the
	// JWT on the operator's own dashboard) - safe to persist (the
	// provider's own docs call it public), never a secret, and never
	// sufficient on its own to authenticate as the operator. Required
	// for StreamElements, since the Astro subscribe request's own
	// `room` field needs it (docs/provider-integrations/
	// external-donations.md §5).
	RemoteChannelID string

	CreatedAt time.Time
	UpdatedAt time.Time
}

// CreateInput is the operator-supplied input to create a new donation
// source. Token is the sensitive credential (StreamElements JWT) -
// stored through SecretStore by the Service, never persisted in Source
// itself and never returned by any read path.
type CreateInput struct {
	ProviderID      ProviderID
	Label           string
	RemoteChannelID string
	Token           string
}

// UpdateInput is the operator-supplied input to update a source's safe
// metadata (never its credential - see Service.ReplaceCredential for
// that, a deliberately separate operation).
type UpdateInput struct {
	Label           string
	RemoteChannelID string
}
