package streamelementsengagement

import "errors"

var (
	// ErrUnsupportedProvider means the target source is not a
	// StreamElements donation source.
	ErrUnsupportedProvider = errors.New("only streamelements donation sources support this engagement connector")

	// ErrNotFound means no connector (running or blocked) exists for the
	// given source id - it was never enabled.
	ErrNotFound = errors.New("engagement connector not found")
)

// Stable error codes surfaced as Snapshot.LastError. internal/httpapi maps
// these to localized frontend copy - never a raw provider error string.
const (
	ErrorCredentialMissing   = "streamelements_credential_missing"
	ErrorAuthFailed          = "streamelements_auth_failed"
	ErrorProviderUnavailable = "streamelements_unavailable"
)
