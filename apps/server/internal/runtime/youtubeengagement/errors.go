package youtubeengagement

import "errors"

var (
	// ErrUnsupportedProvider means the target account is not a YouTube
	// account.
	ErrUnsupportedProvider = errors.New("only youtube accounts support this engagement connector")

	// ErrNotFound means no connector (running or blocked) exists for the
	// given account - it was never enabled.
	ErrNotFound = errors.New("engagement connector not found")
)

// Blocker codes - stable, English-message-free, attached to Snapshot.
// internal/httpapi maps these to localized frontend copy.
const (
	BlockerAccountUnhealthy = "engagement_account_unhealthy"
)

// Stable error codes surfaced as Snapshot.LastError.
const (
	ErrorReconnectRequired   = "reconnect_required"
	ErrorProviderUnavailable = "youtube_unavailable"
	ErrorRateLimited         = "youtube_rate_limited"
	ErrorQuotaExceeded       = "youtube_quota_exceeded"
)
