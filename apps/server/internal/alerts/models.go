package alerts

import (
	"time"

	domain "github.com/streaming-tree/server/internal/domain/alerts"
)

// clock returns the current time; injected everywhere in this package so
// tests are deterministic (a fake clock, never a real timer, drives
// every queue/expiration/duration decision - see queue.go).
type clock func() time.Time

// Instance is one self-contained, in-memory alert - a normalized event
// that matched a rule, holding a snapshot of that rule's presentation
// settings at match time (Part 9's "Policy A": queued alerts keep their
// creation-time rule snapshot, never rebuilt on a later config change,
// so queue behavior stays deterministic and a currently-playing alert
// is never mutated by an editor save).
//
// Deliberately excludes anything the Stage 12A task's own Part 9
// forbids: no OAuth token, stream key, EventSub session id, reconnect
// URL, raw provider payload, or arbitrary ProviderExtra passthrough.
type Instance struct {
	ID        string
	ProfileID string
	RuleID    string

	SourceEventID      string
	ProviderID         domain.ProviderID
	ConnectedAccountID string
	EventType          domain.EventType

	QueuedAt   time.Time
	Priority   int
	DurationMS int

	// Username/Message/Quantity/RewardTitle are nil/zero when the
	// underlying event has none (anonymous actor, no message) or when
	// the rule's own visibility toggle is off - never fabricated.
	Username    string
	Anonymous   bool
	Message     string
	Quantity    *int64
	RewardTitle string

	PlatformLabel string
	RenderedText  string

	EntryAnimation      domain.Animation
	ExitAnimation       domain.Animation
	AnimationDurationMS int

	// GroupCount is always 1 in Stage 12A (grouping is deferred to Stage
	// 12B - see docs/progress.md's Stage 12A persistence entry). Kept as
	// a field now so Stage 12B's grouping does not need a new field on
	// this struct, only new logic that increments it.
	GroupCount int

	// Synthetic marks a locally generated test alert (Part 11/27) -
	// never sourced from a real Engagement Event Bus event.
	Synthetic bool
	// Replayed marks an alert re-shown via Replay Previous (Part 19) -
	// never a fresh match against the Event Bus.
	Replayed bool
}
