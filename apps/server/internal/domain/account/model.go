// Package account holds the provider-independent connected-account domain:
// linking a real provider identity (a Twitch login, later a YouTube or Kick
// one) to this application, tracking its health, and coordinating a
// destination platform's link to one.
//
// Three things are kept strictly separate, mirroring the platform package's
// own "three concepts" split (see internal/domain/platform):
//
//   - A connected Account is a real, external identity this application has
//     authorized against, independent of any configured destination.
//   - A platform.Platform (a configured destination) may link to at most one
//     Account, via a Link.
//   - The OAuth token bundle backing an Account is never in this package's
//     model at all - it lives only in the OS credential store, addressed by
//     the account's ID. See TokenBundle.
//
// Nothing in this package, and nothing it returns to the HTTP layer, ever
// carries an access token, a refresh token, a device code, or a client
// secret.
package account

import "time"

// ProviderID identifies which external provider an account belongs to.
//
// Deliberately its own type rather than platform.ProviderID: every provider
// that can have a configured destination is not necessarily one this
// application can connect an account for yet (and vice versa, in principle),
// so the two are kept distinct even though today's only value, "twitch",
// happens to be spelled the same as platform.ProviderTwitch.
type ProviderID string

const (
	// ProviderTwitch is the only provider with a connected-account adapter
	// implemented in this stage.
	ProviderTwitch ProviderID = "twitch"
)

// Status is the stable, machine-readable health of a connected account.
type Status string

const (
	// StatusConnected means the account's token was valid the last time it
	// was checked and every required scope is present.
	StatusConnected Status = "connected"
	// StatusReconnectRequired means the token could not be validated or
	// refreshed and the user must repeat the authorization flow. Provider
	// operations for this account stop until it is reconnected.
	StatusReconnectRequired Status = "reconnect_required"
)

// Account is the non-secret, storable record of one connected provider
// identity.
//
// Deliberately excludes: any token, any device code, any client secret, and
// the raw provider response the profile fields were read from.
type Account struct {
	ID              string
	ProviderID      ProviderID
	ProviderUserID  string
	Login           string
	DisplayName     string
	AvatarURL       string
	Status          Status
	LastValidatedAt *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
	// Scopes is the last set of OAuth scopes this application confirmed the
	// stored token actually carries, refreshed at validation time - never
	// what the account was originally asked for, so a scope Twitch silently
	// dropped is never mis-reported as granted.
	Scopes []string
}

// HasScope reports whether the account's last-known scope set contains s.
func (a Account) HasScope(s string) bool {
	for _, have := range a.Scopes {
		if have == s {
			return true
		}
	}
	return false
}

// ProfileUpdate is the subset of an account's fields a provider validation
// or finalization step may refresh. Identity (ID, ProviderID,
// ProviderUserID) is immutable after creation.
type ProfileUpdate struct {
	Login       string
	DisplayName string
	AvatarURL   string
	Scopes      []string
}

// Link is one configured destination platform's link to a connected account.
//
// PlatformID is the primary key: one destination links to at most one
// account. One account may back several destinations (Link is not unique on
// AccountID).
type Link struct {
	PlatformID string
	AccountID  string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// IntegrationSettings is the non-secret, provider-scoped application
// configuration needed to talk to a provider's OAuth/API at all - today,
// only a Client ID.
type IntegrationSettings struct {
	ProviderID ProviderID
	ClientID   string
	UpdatedAt  time.Time
}

// ClientIDSource reports where the effective Client ID came from, so the API
// (and the frontend) can explain why a field is or is not editable.
type ClientIDSource string

const (
	// SourceEnvironment means STREAMING_TREE_TWITCH_CLIENT_ID is set; it
	// always wins over any database row, and the frontend must not offer to
	// overwrite it.
	SourceEnvironment ClientIDSource = "environment"
	// SourceDatabase means the Client ID was saved by the operator through
	// the Settings page and is editable.
	SourceDatabase ClientIDSource = "database"
	// SourceMissing means neither an environment override nor a saved value
	// exists.
	SourceMissing ClientIDSource = "missing"
)

// IntegrationConfig is the resolved, effective configuration for one
// provider: whichever of the environment override or the database row
// currently applies.
type IntegrationConfig struct {
	ProviderID ProviderID
	ClientID   string
	Configured bool
	Source     ClientIDSource
}
