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

// Provider is the narrow, provider-specific contract account.Service and
// internal/runtime/deviceflow depend on. Exactly one adapter implements it
// today (internal/provider/twitch); a future YouTube or Kick adapter
// implements the same shape without any change here.
//
// Every method is given whatever credentials it needs explicitly (an access
// token, a Client ID) rather than reaching into shared state, so this
// interface carries no notion of "the current account" and is safe to call
// concurrently for many accounts.
type Provider interface {
	ProviderID() ProviderID

	// StartDeviceFlow begins a new device-authorization attempt.
	StartDeviceFlow(ctx context.Context, clientID string, scopes []string) (DeviceFlowStart, error)

	// PollDeviceFlow performs one token-exchange attempt. The caller is
	// responsible for honoring the returned (or previously-returned)
	// Interval and for stopping once a terminal PollStatus is reached.
	PollDeviceFlow(ctx context.Context, clientID, deviceCode string) (PollOutcome, error)

	// ValidateToken checks an access token against the provider's
	// validation endpoint.
	ValidateToken(ctx context.Context, accessToken string) (ValidationResult, error)

	// RefreshToken exchanges a refresh token for a brand new bundle. The
	// caller must persist the returned bundle before considering the
	// refresh complete - see TokenBundle's own doc comment on rotation.
	RefreshToken(ctx context.Context, clientID, refreshToken string) (TokenBundle, error)

	// RevokeToken best-effort revokes an access token. A provider reporting
	// the token as already invalid must be treated as success by the
	// caller - see account.Service.Disconnect.
	RevokeToken(ctx context.Context, clientID, accessToken string) error

	// GetIdentity resolves the stable identity and display profile behind
	// an access token.
	GetIdentity(ctx context.Context, accessToken, clientID string) (Identity, error)
}
