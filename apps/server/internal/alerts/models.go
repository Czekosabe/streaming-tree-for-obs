package alerts

import (
	"time"

	domain "github.com/streaming-tree/server/internal/domain/alerts"
	"github.com/streaming-tree/server/internal/domain/visualdesign"
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

	// GroupCount was always 1 in Stage 12A (grouping was deferred to
	// Stage 12B). Stage 12B increments it in place when a later
	// compatible event merges into this still-queued instance - see
	// grouping.go. Never a list of members; only the bounded aggregate
	// count/quantity are ever retained (Stage 12B task Part 6).
	GroupCount int

	// Synthetic marks a locally generated test alert (Part 11/27) -
	// never sourced from a real Engagement Event Bus event.
	Synthetic bool
	// Replayed marks an alert re-shown via Replay Previous (Part 19) -
	// never a fresh match against the Event Bus.
	Replayed bool

	// --- Stage 12B: fields needed for grouping/preemption, all copied
	// from the rule's own snapshot or the source event at match time
	// (Policy A - never re-read live state later) and never exposed on
	// any public/management DTO directly. ---

	// ActorProviderUserID is the source event's stable provider user id
	// (never the display name, which is presentation, not identity) -
	// the true "same actor" comparison key for grouping. Empty for an
	// anonymous or user-less event, which makes it permanently
	// ineligible to start or join a group (see groupingEligible).
	ActorProviderUserID string
	// ActorDisplayName is the resolved display name used to re-render
	// {username} after a grouping merge, independent of the rule's own
	// ShowUsername toggle (matching buildInstance's existing "template
	// resolution ignores the show* toggle" behavior for the first
	// render).
	ActorDisplayName string
	// RewardID is the redemption event's own stable reward id (never
	// its user-editable title) - the grouping subject for
	// GroupingSameActorSameSubjectCount. Empty for every other event
	// type.
	RewardID string

	// TextTemplate and Language are the rule snapshot's own values,
	// stored so a grouping merge can deterministically re-render
	// RenderedText from this instance alone - never by re-reading a
	// possibly-since-edited domain.Rule.
	TextTemplate string
	Language     domain.Language

	// AllowGrouping/GroupWindowMS/RuleUpdatedAt are the rule snapshot's
	// own grouping configuration and version identity. RuleUpdatedAt is
	// part of the grouping key (see grouping.go's groupKey) so two
	// instances from two different saved versions of the same rule id
	// never merge, even mid-edit.
	AllowGrouping bool
	GroupWindowMS int
	RuleUpdatedAt time.Time

	// InterruptMode/Interruptible are the rule snapshot's own
	// preemption configuration - InterruptMode governs this instance
	// when it is the incoming candidate, Interruptible when it is the
	// one currently playing.
	InterruptMode domain.InterruptMode
	Interruptible bool

	// VisualDesign is Stage 13A's own immutable presentation snapshot:
	// the owning rule's currently-saved visual design, already reduced
	// to its safe PublicDocument form and copied in at buildInstance
	// time - nil means "no design saved, use the Stage 12 legacy fixed
	// renderer" (Part 18/19). Exactly like every other snapshot field on
	// this struct, editing or deleting the design later never mutates
	// an already-created Instance (current, queued, or the one replay
	// slot) - see internal/alerts.Manager's own rule/design cache and
	// Part 22's own "snapshot semantics for queued/current alerts".
	VisualDesign *visualdesign.PublicDocument
}
