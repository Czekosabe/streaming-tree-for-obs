// Package streamsession is Stage 24's stream session / operational
// history domain (docs/stream-session-history.md). It records ONLY the
// application's own operational timeline - when a session started and
// ended, and which destinations participated with what coarse outcome.
// It never stores chat messages, chatter names, donation messages,
// donor names/amounts, membership/Super Chat content, alert payload
// content, TTS text, or any other viewer/engagement content -
// structurally, not merely by convention: no type in this package
// carries any of that.
package streamsession

import "time"

// EndReason is a closed, bounded classification of why a session
// closed - never a raw error message (docs/stream-session-history.md
// §5).
type EndReason string

const (
	// EndReasonIngestStopped is the normal case: ingest genuinely
	// stopped receiving and the grace window elapsed with nothing
	// reconnecting.
	EndReasonIngestStopped EndReason = "ingest_stopped"
	// EndReasonUncleanShutdown means this session was left open across
	// a process restart (a crash, or the operator quitting Streaming
	// Tree without stopping OBS first) and was recovered at the next
	// startup using its own last heartbeat, never a fabricated time.
	EndReasonUncleanShutdown EndReason = "unclean_shutdown"
)

// Outcome is a closed, bounded classification of how one destination's
// participation within a session ended - never a raw FFmpeg stderr
// line or a provider HTTP response body (docs/stream-session-
// history.md §4).
type Outcome string

const (
	// OutcomeCompleted means the destination was live and then
	// stopped being live for a reason other than an error (the
	// operator stopped that one destination, or it finished cleanly)
	// while the session itself kept going.
	OutcomeCompleted Outcome = "completed"
	// OutcomeError means the destination's branch was in StateError
	// the moment it stopped being live.
	OutcomeError Outcome = "error"
	// OutcomeSessionEnded means the destination was still live when
	// the whole session ended - a graceful end, not an error, but
	// distinct from OutcomeCompleted since nothing about that
	// destination specifically signaled it was done.
	OutcomeSessionEnded Outcome = "session_ended"
)

// Session is one contiguous period during which local MediaMTX ingest
// was actually receiving a publish from OBS (docs/stream-session-
// history.md §1).
type Session struct {
	ID           string
	StartedAt    time.Time
	EndedAt      *time.Time // nil while the session is still open
	LastSeenAt   time.Time
	EndReason    EndReason // "" while open
	Destinations []Destination
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Open reports whether the session has not yet ended.
func (s Session) Open() bool {
	return s.EndedAt == nil
}

// Destination is one destination's own participation record within a
// Session. ProviderID/DisplayName are a snapshot taken when the row
// was created, never re-resolved from the live platform row, so a
// later rename or deletion of the destination never rewrites what
// history already says happened (docs/stream-session-history.md §3).
type Destination struct {
	ID          string
	SessionID   string
	PlatformID  *string // nil if the destination has since been deleted
	ProviderID  string
	DisplayName string
	StartedAt   time.Time
	EndedAt     *time.Time // nil while this participation is still open
	Outcome     Outcome    // "" while open
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Open reports whether this destination's participation has not yet
// ended.
func (d Destination) Open() bool {
	return d.EndedAt == nil
}
