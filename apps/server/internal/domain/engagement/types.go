// Package engagement holds the provider-independent normalized engagement
// event model: a chat message, a follow, a subscription, and so on,
// expressed the same way no matter which provider produced it.
//
// This package defines the shape only. Nothing here talks to a provider,
// stores an event, or distributes one to a consumer - see
// internal/engagement for the in-process Event Bus that does, and
// internal/provider/twitch for the Twitch connector that produces events in
// this shape from real EventSub notifications.
package engagement

// SchemaVersion identifies the shape of the normalized event model itself,
// so a future breaking change to the model can be told apart from a mere
// new event type. Every event this package produces carries the current
// version; API responses (see internal/httpapi) surface it too, so a
// frontend or overlay consumer can detect a version it does not understand
// instead of silently misreading a changed field.
type SchemaVersion int

// CurrentSchemaVersion is the version every Event this package constructs
// carries today.
const CurrentSchemaVersion SchemaVersion = 1

// ProviderID identifies which connector produced an event.
//
// Deliberately its own type rather than reusing account.ProviderID or
// platform.ProviderID: this package must not import either domain (an event
// is a fact about something that happened, not about an account or a
// destination), and Go's type system does not let three independent string
// types be silently substituted for one another.
type ProviderID string

// ProviderTwitch and ProviderYouTube are the providers this application's
// connectors produce events for (Stage 8A and Stage 15A respectively).
const (
	ProviderTwitch  ProviderID = "twitch"
	ProviderYouTube ProviderID = "youtube"
)

// Type is the normalized, provider-independent event type. See
// docs/engagement-architecture.md §5.4 for the full planned vocabulary;
// this stage implements the subset below.
type Type string

const (
	TypeChatMessage            Type = "chat.message"
	TypeChatMessageDeleted     Type = "chat.message_deleted"
	TypeChatCleared            Type = "chat.cleared"
	TypeModeration             Type = "moderation"
	TypeFollow                 Type = "follow"
	TypeSubscription           Type = "subscription"
	TypeResubscription         Type = "resubscription"
	TypeGiftedSubscription     Type = "gifted_subscription"
	TypeSubscriptionGiftBatch  Type = "subscription_gift_batch"
	TypeBits                   Type = "bits"
	TypeRaid                   Type = "raid"
	TypeChannelPointRedemption Type = "channel_point_redemption"
	TypeStreamOnline           Type = "stream.online"
	TypeStreamOffline          Type = "stream.offline"

	// Stage 15A (YouTube). See docs/provider-integrations/
	// youtube-engagement.md §5 for the full mapping table and the
	// reasoning behind reusing TypeSubscriptionGiftBatch/
	// TypeGiftedSubscription/TypeModeration for the YouTube events that
	// are genuinely semantically equivalent, and adding these four new
	// types only for the ones that are not.
	//
	// TypeYouTubeMembership is a brand-new channel membership
	// (YouTube's newSponsorEvent) - never reused for an ongoing member's
	// milestone chat, which is a different fact (see below).
	TypeYouTubeMembership Type = "youtube.membership"
	// TypeYouTubeMembershipMilestone is an existing member's milestone
	// chat (YouTube's memberMilestoneChatEvent) - deliberately not
	// relabelled as a fresh membership.
	TypeYouTubeMembershipMilestone Type = "youtube.membership_milestone"
	// TypeYouTubeSuperChat and TypeYouTubeSuperSticker are real,
	// distinct monetary/paid-support events - never collapsed into
	// TypeBits or into each other.
	TypeYouTubeSuperChat    Type = "youtube.super_chat"
	TypeYouTubeSuperSticker Type = "youtube.super_sticker"
)

// KnownTypes lists every Type this stage's model and connectors recognize,
// in the stable order used for validation and for any place that needs to
// enumerate the whole set (e.g. a test fixture or an OpenAPI-style contract
// document).
var KnownTypes = []Type{
	TypeChatMessage,
	TypeChatMessageDeleted,
	TypeChatCleared,
	TypeModeration,
	TypeFollow,
	TypeSubscription,
	TypeResubscription,
	TypeGiftedSubscription,
	TypeSubscriptionGiftBatch,
	TypeBits,
	TypeRaid,
	TypeChannelPointRedemption,
	TypeStreamOnline,
	TypeStreamOffline,
	TypeYouTubeMembership,
	TypeYouTubeMembershipMilestone,
	TypeYouTubeSuperChat,
	TypeYouTubeSuperSticker,
}

// Known reports whether t is one of KnownTypes.
func (t Type) Known() bool {
	for _, known := range KnownTypes {
		if known == t {
			return true
		}
	}
	return false
}

// Role is a user's standing in the channel an event happened in, as the
// provider reported it - not this application's own notion of a role.
type Role string

const (
	RoleBroadcaster Role = "broadcaster"
	RoleModerator   Role = "moderator"
	RoleVIP         Role = "vip"
	RoleSubscriber  Role = "subscriber"
)
