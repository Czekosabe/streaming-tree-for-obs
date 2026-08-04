package credential

import "errors"

// Sentinel domain errors. The HTTP layer maps these to stable API error
// codes; no secrets.SecretStore-level or OS-level error ever reaches it
// directly - see mapStoreError in service.go.
var (
	// ErrCredentialNotFound is reserved for a strict "read this credential"
	// path. The current API never needs to return it: status reporting
	// treats an absent credential as a normal, successful "not configured"
	// answer, and deleting an absent credential is defined as a successful
	// no-op (see Service.DeleteStreamKey). It exists so a future endpoint
	// that must distinguish "absent" from "present" has a stable error to
	// map without inventing one under time pressure.
	ErrCredentialNotFound = errors.New("credential not found")

	// ErrStoreUnavailable means the OS credential store could not be reached
	// or used right now. It is expected to happen (no desktop session, a
	// locked keychain, an unsupported environment) and must never be treated
	// as an application bug.
	ErrStoreUnavailable = errors.New("credential store unavailable")

	// ErrStoreFailure wraps an unexpected failure from a reachable store.
	ErrStoreFailure = errors.New("credential store failure")
)
