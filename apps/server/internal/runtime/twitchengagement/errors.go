package twitchengagement

import "errors"

var (
	// ErrUnsupportedProvider means the target account is not a Twitch
	// account - only Twitch accounts may enable engagement in Stage 8A.
	ErrUnsupportedProvider = errors.New("only twitch accounts support engagement in this stage")

	// ErrNotFound means no connector (running or blocked) exists for the
	// given account - it was never enabled.
	ErrNotFound = errors.New("engagement connector not found")

	// ErrConflict means a restart was requested for a connector already
	// mid-(re)connect in a way that would race a second attempt.
	ErrConflict = errors.New("engagement connector busy")
)

// Blocker codes - stable, English-message-free, attached to Snapshot.
// internal/httpapi maps these to localized frontend copy.
const (
	BlockerScopeUpgradeRequired       = "engagement_scope_upgrade_required"
	BlockerIntegrationNotConfigured   = "engagement_not_configured"
	BlockerAccountUnhealthy           = "engagement_account_unhealthy"
	BlockerCredentialStoreUnavailable = "credential_store_unavailable"
)
