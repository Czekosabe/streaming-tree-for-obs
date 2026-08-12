// Package operatorchat is the Stage 9 unified-operator-chat projection: a
// provider-independent boundary that subscribes to the Engagement Event Bus
// (internal/engagement) and converts normalized events
// (internal/domain/engagement) into chat-shaped, lifecycle-aware items.
//
// This package never imports internal/provider/twitch or any other
// provider adapter. Twitch is a source of normalized events like any future
// provider would be; nothing here branches on ProviderID except to copy it
// through to the public item unchanged. Asset resolution (badge images,
// emote URLs) is a presentation-layer concern handled by the HTTP layer
// (internal/httpapi), which has access to both this projection and a
// provider-specific asset resolver - see docs/progress.md's Stage 9 entry
// for why that boundary was drawn there rather than here.
//
// The projection is in-memory only. Nothing in this package writes to
// SQLite, a file, or any other persistent store - see buffer.go and
// projection.go's own doc comments.
package operatorchat

import "time"

// CurrentVersion is the schema version of every Item this package produces.
// Bumped only on a breaking change to the public item shape.
const CurrentVersion = 1

// Kind is the item's category. Every kind has clear, validated per-kind
// field expectations (see Item's own doc comment) rather than one
// optional-field blob every consumer must guess at.
type Kind string

const (
	// KindMessage is a real chat message, with lifecycle state (it may
	// later become deleted).
	KindMessage Kind = "message"
	// KindActivity is a non-chat engagement event presented inline: a
	// follow, subscription, gift, cheer, raid, redemption, or remote
	// stream online/offline notice.
	KindActivity Kind = "activity"
	// KindModeration is a moderation action that could not be expressed as
	// an update to a single retained message: a user-messages clear
	// referencing more messages than are individually shown, or a
	// reference to a message no longer retained.
	KindModeration Kind = "moderation"
	// KindSystem is a projection-generated informational row not tied to
	// one specific provider event (for example, "chat was cleared").
	KindSystem Kind = "system"
)

// DeletionReason is a stable, English-message-free identifier for why a
// message item's lifecycle was marked deleted.
type DeletionReason string

const (
	DeletionReasonModeratorDeleted    DeletionReason = "moderator_deleted"
	DeletionReasonChatCleared         DeletionReason = "chat_cleared"
	DeletionReasonUserMessagesCleared DeletionReason = "user_messages_cleared"
)

// Badge is a reference to one badge a user carries - the raw, provider-
// stable identifiers only. Resolving these to an image URL is a
// presentation-layer concern (internal/httpapi + internal/provider/twitch/
// chatassets), deliberately not performed inside this package.
type Badge struct {
	SetID string
	ID    string
	Info  string
}

// User is the identity block attached to a message or activity item -
// mirrors internal/domain/engagement.User field-for-field, copied rather
// than embedded so this package's public shape never accidentally changes
// just because the normalized event model does.
type User struct {
	ProviderUserID string
	Login          string
	DisplayName    string
	AvatarURL      string
	Color          string
	Badges         []Badge
	Anonymous      bool
}

// FragmentType mirrors internal/domain/engagement.FragmentType.
type FragmentType string

const (
	FragmentText      FragmentType = "text"
	FragmentEmote     FragmentType = "emote"
	FragmentCheermote FragmentType = "cheermote"
	FragmentMention   FragmentType = "mention"
	FragmentUnknown   FragmentType = "unknown"
)

// Fragment is one ordered piece of a chat message - the raw identifiers
// only, mirroring internal/domain/engagement.Fragment. An emote fragment's
// image URL is resolved at presentation time from EmoteID alone (see
// docs/provider-integrations/twitch-engagement.md's Stage 9 addendum for
// why that needs no cache or provider request at all).
type Fragment struct {
	Type               FragmentType
	Text               string
	EmoteID            string
	CheermotePrefix    string
	CheermoteBits      int
	MentionUserID      string
	MentionLogin       string
	MentionDisplayName string
}

// Message is a chat-shaped item's content.
type Message struct {
	PlainText string
	Fragments []Fragment
}

// Activity describes a non-chat engagement event presented inline as an
// activity item.
type Activity struct {
	// ActivityType mirrors the source engagement.Type (e.g. "follow",
	// "bits", "raid") - never a made-up second vocabulary.
	ActivityType string
	// AmountMicros/Currency/DisplayAmount mirror engagement.Event.Money
	// when the source event carried one (Stage 15A: YouTube Super
	// Chat/Super Sticker) - always integer micros, never a float. All
	// three are the zero value together when the event carried no money.
	AmountMicros  *int64
	Currency      string
	DisplayAmount string
	Quantity      *int64
}

// ModerationInfo describes a moderation or system item's meaning.
type ModerationInfo struct {
	// Action is a stable identifier: "message_deleted", "chat_cleared",
	// "user_messages_cleared", or "message_deleted_not_retained".
	Action string
	// TargetUserID is set when the action targeted one specific user
	// (e.g. clear_user_messages) - empty for a whole-chat clear.
	TargetUserID string
	// TargetMessageRef is the provider event ID of the message the action
	// refers to, when known - never invented when the original message
	// was never retained.
	TargetMessageRef string
}

// Lifecycle is a message item's mutable state. Every other field on Item is
// fixed at creation; Lifecycle is the one part a later revision may change.
type Lifecycle struct {
	Deleted        bool
	DeletedAt      *time.Time
	DeletionReason DeletionReason
}

// Item is the public, versioned operator-chat item - see this package's
// doc comment and docs/progress.md's Stage 9 entry for the full design
// rationale. Never carries a raw provider payload, a token, a WebSocket
// session id, or a reconnect URL - it is built only from
// internal/domain/engagement.Event's own already-sanitized fields.
type Item struct {
	Version int
	// Sequence is assigned by the projection at revision time - it
	// increases on every new item AND on every lifecycle update to an
	// existing item (a revision), never reused. It is NOT the same
	// sequence space as the Engagement Event Bus's own sequence numbers.
	Sequence uint64
	// ID is stable across lifecycle changes - a message becoming deleted
	// updates the item with this same ID, never creates a second row with
	// a new one (see lifecycle.go).
	ID string
	// SourceEventID is the engagement.Event.ID that most recently produced
	// or updated this item (the original message's bus event ID for an
	// unmodified message; the deletion event's ID after a deletion).
	SourceEventID string
	// ProviderMessageID is the provider's own raw chat-message identifier
	// (Twitch's channel.chat.message message_id), present only for
	// KindMessage. Not a credential, but deliberately never added to the
	// public OBS overlay DTO (internal/httpapi's
	// publicChatOverlayItemResponse) - it exists here only so the private
	// operator Chat page can populate Stage 11A's reply_parent_message_id
	// when the operator replies to an existing Twitch message. Distinct
	// from ID (this package's own composite key) and from SourceEventID
	// (the Engagement Event Bus's internal id) - see messageItemID's own
	// doc comment for why those two are not usable as a Twitch reply
	// target.
	ProviderMessageID string

	ProviderID         string
	ConnectedAccountID string
	// DestinationID is set only when the connected account is linked to
	// exactly one configured destination - never guessed when there is
	// more than one, mirroring engagement.Event.DestinationID's own rule.
	DestinationID string

	Kind Kind

	OccurredAt time.Time
	ReceivedAt time.Time

	// User is present for KindMessage and most KindActivity items; absent
	// for KindSystem and any activity with no user context.
	User *User
	// Message is present only for KindMessage.
	Message *Message
	// Activity is present only for KindActivity.
	Activity *Activity
	// Moderation is present only for KindModeration and KindSystem.
	Moderation *ModerationInfo

	Lifecycle Lifecycle

	Synthetic bool
}
