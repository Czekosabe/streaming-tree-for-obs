package account

import "errors"

// Sentinel domain errors. The HTTP layer maps these to stable API error
// codes; no repository- or provider-level error, and no secret value, ever
// reaches it directly.
var (
	// ErrNotFound means no connected account exists with the given ID.
	ErrNotFound = errors.New("connected account not found")

	// ErrProviderMismatch means a platform and an account (or an account and
	// a newly authorized identity) belong to different providers - a Twitch
	// destination can never link to a non-Twitch account, and a reconnect
	// can never silently swap which real identity an account represents.
	ErrProviderMismatch = errors.New("provider mismatch")

	// ErrIdentityMismatch means a reconnect attempt authorized a different
	// provider user than the account being reconnected.
	ErrIdentityMismatch = errors.New("authorized identity does not match the account being reconnected")

	// ErrAlreadyLinked is reserved for a future stricter linking policy; the
	// current policy allows replacing a platform's link explicitly (see
	// Service.LinkPlatform), so this is not returned by that path today.
	ErrAlreadyLinked = errors.New("platform already linked to a different account")

	// ErrLinkNotFound means the platform has no connected-account link.
	ErrLinkNotFound = errors.New("no connected account is linked to this platform")

	// ErrReconnectRequired means the account's token could not be validated
	// or refreshed, so no provider operation can proceed until the user
	// reconnects.
	ErrReconnectRequired = errors.New("account requires reconnection")

	// ErrMissingScope means the account's last-known scopes do not include
	// one this operation requires.
	ErrMissingScope = errors.New("required scope not granted")

	// ErrIntegrationNotConfigured means no Client ID is configured for the
	// provider (neither environment nor database).
	ErrIntegrationNotConfigured = errors.New("provider integration not configured")

	// ErrIntegrationLocked means the Client ID cannot be changed right now
	// because connected accounts for that provider still exist - see
	// Service.SetIntegrationClientID.
	ErrIntegrationLocked = errors.New("client id is locked while connected accounts exist")

	// ErrConflict is returned when a write cannot be applied because it
	// would violate a uniqueness or consistency rule.
	ErrConflict = errors.New("conflict")

	// ErrStorage wraps any unexpected persistence failure. The underlying
	// driver error is kept for the logs but must never reach a client.
	ErrStorage = errors.New("storage failure")

	// ErrSecretStore wraps any unexpected SecretStore failure, mirroring
	// internal/domain/credential's own boundary.
	ErrSecretStore = errors.New("secret store failure")

	// ErrSecretStoreUnavailable means the OS credential store could not be
	// reached - expected to happen sometimes, never a bug.
	ErrSecretStoreUnavailable = errors.New("secret store unavailable")
)
