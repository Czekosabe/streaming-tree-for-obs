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
	// OpRemove carries only the id of an item that is no longer visible,
	// plus a stable RemoveReason - never the item's own former content.
	// A settings/privacy change that hides a previously-visible item
	// (a newly blocked term, a newly hidden user, a filter toggle, a
	// narrowed account selection) never produces an OpRemove in this
	// design at all - it always produces a full OpReset instead (see
	// Projection.Configure), which the frontend already applies
	// immediately and in full. OpRemove is therefore reserved for the
	// two genuinely cosmetic reasons (RemoveReasonExpired,
	// RemoveReasonCapacityEvicted, safe to animate) and the individual
	// moderation-lifecycle reasons operator-chat's own revision stream
	// reports for a single already-visible item (RemoveReasonMessageDeleted,
	// RemoveReasonChatCleared, RemoveReasonUserMessagesCleared, all
	// immediate, never animated) - see RemoveReason's own doc comment.
	OpRemove Operation = "remove"
	// OpReset carries a complete replacement of the entire visible set -
	// sent after a settings/account change that could affect many items
	// at once, and whenever a subscriber's gap cannot be satisfied by
	// replay.
	OpReset Operation = "reset"
	// OpPresentationChanged carries no item content at all - just a
	// sequence number, participating in the same monotonic revision
	// stream/replay/gap mechanism every other Operation already uses
	// (Stage 13B, docs/visual-designs.md §25). Emitted exactly once per
	// successful chat visual-design Save or Delete
	// (Manager.NotifyPresentationChanged); tells an already-connected
	// client "your GET .../config response is now stale - refetch it
	// before trusting it further." Never a substitute for an item
	// revision, and the item reducer's own state is completely
	// unaffected by it.
	OpPresentationChanged Operation = "presentation_changed"
)

// RemoveReason is the stable, closed reason an OpRemove revision
// carries - never a raw operator-chat deletion detail, never which
// blocked term or hidden user was involved, and never enough
// information to reconstruct the removed item's own content. Exactly
// two reasons are "cosmetic" (see IsCosmetic) and safe for the
// frontend to animate; every other reason is an immediate removal a
// client must apply without delay and without retaining the item's
// content for a "leaving" transition - see this package's own doc
// comment and docs/project-overview.md's Stage 10 corrective-pass
// entry for the full safety rationale.
type RemoveReason string

const (
	// RemoveReasonExpired: the item's own configured message lifetime
	// elapsed. Cosmetic - safe to animate.
	RemoveReasonExpired RemoveReason = "expired"
	// RemoveReasonCapacityEvicted: the oldest visible item was evicted
	// because a newer one exceeded the profile's own MaxVisibleItems.
	// Cosmetic - safe to animate.
	RemoveReasonCapacityEvicted RemoveReason = "capacity_evicted"
	// RemoveReasonMessageDeleted: a moderator deleted this one message
	// on Twitch (operatorchat.DeletionReasonModeratorDeleted). Immediate.
	RemoveReasonMessageDeleted RemoveReason = "message_deleted"
	// RemoveReasonChatCleared: a whole-chat clear affected this item
	// (operatorchat.DeletionReasonChatCleared). Immediate.
	RemoveReasonChatCleared RemoveReason = "chat_cleared"
	// RemoveReasonUserMessagesCleared: a clear-this-user's-messages
	// action affected this item (operatorchat.DeletionReasonUserMessagesCleared).
	// Immediate.
	RemoveReasonUserMessagesCleared RemoveReason = "user_messages_cleared"
	// RemoveReasonUnknown is the safe fallback for a lifecycle deletion
	// this package does not recognize a specific reason for - treated
	// as immediate, never cosmetic, so an unrecognized case can never
	// accidentally retain hidden content on screen for an animation.
	RemoveReasonUnknown RemoveReason = "unknown"
)

// IsCosmetic reports whether r is safe for the frontend to animate as a
// "leaving" transition rather than removing immediately. Only natural
// expiry and capacity eviction qualify - every moderation, clear, or
// settings-driven removal must be immediate (see this type's own doc
// comment for why settings/privacy changes never even reach this
// function: they travel as a full OpReset, not an OpRemove).
func (r RemoveReason) IsCosmetic() bool {
	return r == RemoveReasonExpired || r == RemoveReasonCapacityEvicted
}

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
	// AmountMicros/Currency/DisplayAmount mirror
	// operatorchat.Activity.AmountMicros/Currency/DisplayAmount - always
	// integer micros, never a float. See internal/domain/engagement.Money.
	AmountMicros  *int64
	Currency      string
	DisplayAmount string
	Quantity      *int64
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
	// Reason is set only for OpRemove - see RemoveReason's own doc
	// comment.
	Reason RemoveReason
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
