package donationsource

import "errors"

// Sentinel domain errors. The HTTP layer maps these to stable API error
// codes; no repository- or SecretStore-level error, and no credential
// value, ever reaches it directly.
var (
	// ErrNotFound means no donation source exists with the given ID.
	ErrNotFound = errors.New("donation source not found")

	// ErrInvalidProvider means the requested ProviderID is not one Stage
	// 16A supports.
	ErrInvalidProvider = errors.New("unsupported donation source provider")

	// ErrInvalidLabel means the label is empty or exceeds the bounded
	// length.
	ErrInvalidLabel = errors.New("invalid donation source label")

	// ErrInvalidRemoteChannelID means the remote channel id is empty or
	// exceeds the bounded length.
	ErrInvalidRemoteChannelID = errors.New("invalid donation source remote channel id")

	// ErrCredentialRequired means an operation required a credential
	// (creating a source, replacing its token) but none was supplied.
	ErrCredentialRequired = errors.New("donation source credential is required")

	// ErrCredentialTooLong means the supplied credential exceeds the
	// bounded size this application accepts - defensive only; never
	// logs or echoes the value itself.
	ErrCredentialTooLong = errors.New("donation source credential is too long")

	// ErrConflict is returned when a write cannot be applied because it
	// would violate a uniqueness or consistency rule.
	ErrConflict = errors.New("conflict")

	// ErrStorage wraps any unexpected persistence failure. The
	// underlying driver error is kept for the logs but must never reach
	// a client.
	ErrStorage = errors.New("storage failure")

	// ErrSecretStore wraps any unexpected SecretStore failure, mirroring
	// internal/domain/account's own boundary.
	ErrSecretStore = errors.New("secret store failure")

	// ErrSecretStoreUnavailable means the OS credential store could not
	// be reached - expected to happen sometimes, never a bug.
	ErrSecretStoreUnavailable = errors.New("secret store unavailable")
)
