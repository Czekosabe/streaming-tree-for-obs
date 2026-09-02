// Package streaminsights is Stage 27's read-only aggregation domain
// (docs/stream-insights.md): "how has my streaming actually gone?",
// computed entirely from Stage 24's already-recorded, already-
// authorized operational history. It persists nothing of its own -
// every Insights value is derived at request time from
// streamsession.Repository.ListSessions, and structurally cannot
// carry engagement content (see content_exclusion_test.go).
package streaminsights

import "time"

// SessionSummary is a compact pointer at one specific session, used
// for LongestSession.
type SessionSummary struct {
	SessionID string
	StartedAt time.Time
	Duration  time.Duration
}

// DestinationInsights groups every session-destination row across
// history that snapshot-matches one real destination identity.
// PlatformID is nil when every row grouped here came from a
// destination that has since been deleted - grouped by its own
// ProviderID/DisplayName snapshot instead, never dropped and never
// merged into an unrelated currently-existing destination that
// happens to share a provider.
type DestinationInsights struct {
	PlatformID    *string
	ProviderID    string
	DisplayName   string
	SessionCount  int
	TotalDuration time.Duration
	// OutcomeCounts maps a streamsession.Outcome string to how many
	// times this destination ended that way - "" is a real bucket,
	// meaning this destination's participation was still open when
	// computed.
	OutcomeCounts map[string]int
}

// Insights is the full computed result of one Compute call.
type Insights struct {
	TotalSessions          int
	TotalStreamingDuration time.Duration
	// AverageSessionDuration is zero when TotalSessions == 0.
	AverageSessionDuration time.Duration
	// LongestSession is nil when TotalSessions == 0.
	LongestSession *SessionSummary
	// SessionsByEndReason maps a streamsession.EndReason string to a
	// count - "" is a real bucket, meaning that session is still open.
	SessionsByEndReason map[string]int
	Destinations        []DestinationInsights
}
