// Package chatoverlay is the Stage 10 public-overlay projection: a
// provider-independent boundary that consumes the Stage 9 operator-chat
// projection's own revision stream (internal/operatorchat) and produces a
// smaller, filtered, per-profile public item stream safe to serve to an
// OBS Browser Source.
//
// This package never imports internal/provider/twitch, and it never
// reconnects directly to the Engagement Event Bus - doing so would
// duplicate the lifecycle logic internal/operatorchat already implements
// correctly (message deletion, chat/user clearing, activity mapping).
// Asset resolution (badge images, emote URLs) is, exactly like Stage 9's
// own boundary, a presentation-layer concern handled by internal/httpapi
// at serialization time - see this package's own Badge/Fragment types,
// which carry only raw provider-stable identifiers.
//
// Every exported item here is in-memory only. Nothing in this package
// writes to SQLite - see internal/domain/chatoverlay for the persisted
// profile settings this package's Manager reads to configure each
// overlay's filtering and presentation.
package chatoverlay

import "time"

// CurrentVersion is the schema version of every Item this package
// produces. Bumped only on a breaking change to the public item shape.
const CurrentVersion = 1

// Operation is one revision's kind - see this package's own doc comment
// and docs/progress.md's Stage 10 entry for the full revision-protocol
// rationale.
type Operation string

const (
	// OpUpsert carries a complete current Item - a brand-new visible item
	// or an update to one already visible (its color, badges, or deleted
	// state changed).
	OpUpsert Operation = "upsert"
	// OpRemove carries only the id of an item that is no longer visible -
	// because Twitch deleted it, a clear affected it, capacity/lifetime
	// expiry evicted it, or a settings change hid it. Never carries the
	// item's own former content.
	OpRemove Operation = "remove"
	// OpReset carries a complete replacement of the entire visible set -
	// sent after a settings/account change that could affect many items
	// at once, and whenever a subscriber's gap cannot be satisfied by
	// replay.
	OpReset Operation = "reset"
)

// Kind is the item's category. The public overlay only ever shows two of
// operatorchat's four kinds - see this package's own filtering.go doc
// comment for why moderation/system rows never reach a viewer.
type Kind string

const (
	KindMessage  Kind = "message"
	KindActivity Kind = "activity"
)

// Badge is a reference to one badge a user carries - the raw,
// provider-stable identifiers only, exactly mirroring
// operatorchat.Badge's own reasoning: resolving these to an image URL is
// a presentation-layer concern performed by internal/httpapi, never here.
type Badge struct {
	SetID string
	ID    string
	Info  string
}

// FragmentType mirrors operatorchat.FragmentType, restricted to the
// fragment kinds this package ever renders publicly. A cheermote fragment
// arrives from operatorchat as FragmentCheermote but is folded into plain
// text here (see filtering.go) - Twitch's cheermote-tier image catalog is
// not resolved in this stage, exactly as documented in Stage 9's own
// research addendum.
type FragmentType string

const (
	FragmentText    FragmentType = "text"
	FragmentEmote   FragmentType = "emote"
	FragmentMention FragmentType = "mention"
)

// Fragment is one ordered piece of a public message - never carries a
// resolved image URL (see this package's own doc comment); EmoteID alone
// is enough for internal/httpapi to build one at serialization time,
// exactly like operatorchat's own Fragment.
type Fragment struct {
	Type    FragmentType
	Text    string
	EmoteID string
}

// Message is a public message item's content. Deliberately has no field
// for the original text of a deleted message - see Lifecycle-free design
// note on Item below.
type Message struct {
	PlainText string
	Fragments []Fragment
}

// Activity describes a public activity item.
type Activity struct {
	// ActivityType mirrors the source operatorchat.Activity.ActivityType
	// verbatim - never relabelled (bits stay "bits", never "donation").
	ActivityType string
	Amount       *float64
	Currency     string
	Quantity     *int64
}

// User is the public identity block attached to a message or activity
// item. Every field here is populated only when the owning overlay
// profile's own settings enable it - see buildItem in lifecycle.go -
// rather than always being present and left for the frontend to hide.
type User struct {
	ProviderUserID string
	DisplayName    string
	Color          string
	Badges         []Badge
	AvatarURL      string
	Anonymous      bool

	// Role flags derived from Twitch's own documented chat-badge set-id
	// vocabulary ("broadcaster", "moderator", "subscriber", "vip" - see
	// docs/provider-integrations/twitch-engagement.md's Stage 9
	// addendum), never inferred from a username. False when the
	// underlying badge is absent, never guessed.
	IsBroadcaster bool
	IsModerator   bool
	IsSubscriber  bool
	IsVIP         bool
}

// Item is the public, versioned overlay item - deliberately smaller than
// operatorchat.Item. A deleted message that is still shown (the owning
// profile's own show_deleted_placeholder setting) has Deleted=true and a
// nil Message - the original text is never carried in this struct at
// all, so there is no field a bug could accidentally serialize.
type Item struct {
	Version  int
	Sequence uint64
	ID       string

	Kind Kind

	ProviderID string
	// AccountLabel is a safe, non-empty label only when the owning
	// profile's show_account_label setting is on and a label was
	// resolvable - never a raw connected-account id, and never present
	// merely because more than one account exists if the setting is off.
	AccountLabel string
	// SourceAccountID is the raw connected-account id, kept only for
	// internal/httpapi's own presentation-time badge-image resolution
	// (Twitch badge image sets are channel-specific, so resolving one
	// needs the source account regardless of AccountLabel's own
	// visibility setting). Like every field on this type, it is never
	// serialized directly - see this package's own doc comment: JSON
	// shaping is internal/httpapi's job, building its own response DTO
	// that deliberately has no field for this one. Not a substitute for
	// AccountLabel and never rendered.
	SourceAccountID string

	OccurredAt time.Time

	// User is present for both kinds when the source event had identity
	// context; absent for an anonymous or identity-less event.
	User *User
	// Message is present only for KindMessage, and only when Deleted is
	// false or the profile shows a placeholder with content still
	// attached would defeat the point - see this type's own doc comment.
	Message *Message
	// Activity is present only for KindActivity.
	Activity *Activity

	Deleted   bool
	Synthetic bool
}

// Revision is one change to one overlay's public timeline.
type Revision struct {
	Sequence  uint64
	Operation Operation
	// Item is set only for OpUpsert.
	Item *Item
	// RemovedID is set only for OpRemove.
	RemovedID string
	// ResetItems is set only for OpReset - the complete new visible set,
	// so a client never needs a second round trip to recover from one.
	ResetItems []Item
}

// Status is one overlay's own runtime status - no message content.
type Status struct {
	SchemaVersion     int
	VisibleCount      int
	MaxVisibleItems   int
	RetainedRevisions int
	OldestSequence    uint64
	NewestSequence    uint64
	ActiveSubscribers int
	// UpstreamGap is true once this overlay's projection has ever
	// detected a gap between what it consumed from the operator-chat
	// projection and what that projection actually retained - a one-way
	// honest flag, mirroring operatorchat.Status.BusGap's own reasoning.
	UpstreamGap bool
}
