package account

import (
	"context"
	"time"
)

// DeviceFlowStart is a provider's response to beginning a device-flow
// attempt, including the device code the caller must keep private - callers
// are internal/runtime/deviceflow only; a device code must never reach an
// HTTP response (see deviceflow.Snapshot for the public shape it is
// deliberately excluded from).
type DeviceFlowStart struct {
	DeviceCode      string
	UserCode        string
	VerificationURI string
	ExpiresIn       time.Duration
	Interval        time.Duration
}

// PollStatus is the outcome of one device-flow poll attempt.
type PollStatus string

const (
	PollPending  PollStatus = "pending"
	PollSlowDown PollStatus = "slow_down"
	PollDenied   PollStatus = "denied"
	PollExpired  PollStatus = "expired"
	PollComplete PollStatus = "complete"
)

// PollOutcome is one device-flow poll attempt's result. Bundle and Scopes
// are populated only when Status is PollComplete.
type PollOutcome struct {
	Status PollStatus
	Bundle TokenBundle
	Scopes []string
}

// Identity is the minimal, stable provider-user identity and display profile
// this application needs after a successful authorization.
type Identity struct {
	ProviderUserID string
	Login          string
	DisplayName    string
	AvatarURL      string
}

// ValidationResult is the outcome of checking a token against the provider's
// validation endpoint.
type ValidationResult struct {
	Valid          bool
	ClientID       string
	ProviderUserID string
	Scopes         []string
	ExpiresIn      time.Duration
}

// Provider is the narrow, provider-specific contract account.Service itself
// depends on: validating, refreshing and revoking a token, and resolving the
// identity behind one. Every provider adapter implements this, regardless of
// which OAuth flow it uses to obtain a token in the first place.
//
// Every method is given whatever credentials it needs explicitly (an access
// token, a Client ID) rather than reaching into shared state, so this
// interface carries no notion of "the current account" and is safe to call
// concurrently for many accounts.
type Provider interface {
	ProviderID() ProviderID

	// ValidateToken checks an access token against the provider's
	// validation endpoint.
	ValidateToken(ctx context.Context, accessToken string) (ValidationResult, error)

	// RefreshToken exchanges a refresh token for a brand new bundle. The
	// caller must persist the returned bundle before considering the
	// refresh complete - see TokenBundle's own doc comment on rotation.
	//
	// A provider whose refresh response can omit a new refresh token (see
	// internal/provider/youtube) must itself preserve the previous
	// refreshToken argument in the returned TokenBundle rather than ever
	// returning an empty one - account.Service always persists exactly what
	// this method returns.
	RefreshToken(ctx context.Context, clientID, refreshToken string) (TokenBundle, error)

	// RevokeToken best-effort revokes a token. A provider reporting the
	// token as already invalid must be treated as success by the caller -
	// see account.Service.Disconnect.
	RevokeToken(ctx context.Context, clientID, accessToken string) error

	// GetIdentity resolves the stable identity and display profile behind
	// an access token. For a provider whose authorization can require an
	// explicit disambiguation step among several owned identities (see
	// internal/runtime/youtubeauth's channel selection), this method is
	// meaningful only once that step is already resolved; it is not used
	// during that step itself.
	GetIdentity(ctx context.Context, accessToken, clientID string) (Identity, error)
}

// DeviceFlowProvider extends Provider with the two methods specific to
// Twitch's Device Code Grant Flow (RFC 8628-shaped polling). Only
// internal/runtime/deviceflow.Manager depends on this narrower interface;
// account.Service depends on Provider alone.
//
// Deliberately not part of Provider itself: a provider using a different
// OAuth flow (YouTube's Authorization Code + PKCE + loopback callback, via
// internal/runtime/youtubeauth) has no device code and no poll loop, and
// forcing it to implement these two methods meaninglessly would be exactly
// the kind of framework-shaped-around-one-provider mistake the connected-
// account foundation was designed to avoid.
type DeviceFlowProvider interface {
	Provider

	// StartDeviceFlow begins a new device-authorization attempt.
	StartDeviceFlow(ctx context.Context, clientID string, scopes []string) (DeviceFlowStart, error)

	// PollDeviceFlow performs one token-exchange attempt. The caller is
	// responsible for honoring the returned (or previously-returned)
	// Interval and for stopping once a terminal PollStatus is reached.
	PollDeviceFlow(ctx context.Context, clientID, deviceCode string) (PollOutcome, error)
}
