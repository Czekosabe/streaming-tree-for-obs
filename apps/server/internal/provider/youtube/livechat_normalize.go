package youtube

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	engagement "github.com/streaming-tree/server/internal/domain/engagement"
)

// errMalformedMessage means a liveChatMessage claimed a type but its own
// matching "*Details" sub-object was missing or unusable - a real,
// unexpected shape mismatch, never silently ignored the way a genuinely
// unsupported *type* is (see NormalizeResult.Supported below).
var errMalformedMessage = errors.New("malformed youtube live chat message")

// NormalizeResult is the outcome of normalizing one raw YouTube
// liveChatMessage. Exactly one of three things is true:
//
//   - Supported is true: Event is a real, valid engagement.Event ready to
//     publish.
//   - Lifecycle is non-empty: this message is a connector state-machine
//     signal (chatEndedEvent), never published to the Event Bus itself -
//     see internal/runtime/youtubeengagement.
//   - Neither: this message's type is intentionally not normalized in
//     Stage 15A (tombstone, sponsor-only-mode events, pollEvent, giftEvent,
//     fanFundingEvent, or any future/unknown type) - see
//     docs/provider-integrations/youtube-engagement.md §5 for the mapping
//     table and the reasoning behind each deliberate omission. The caller
//     is expected to count this as a bounded, rate-limited diagnostic,
//     never log the raw message.
type NormalizeResult struct {
	Event     engagement.Event
	Supported bool
	Lifecycle string
}

// Lifecycle signal constants - never published to the Event Bus.
const (
	LifecycleChatEnded = "chat_ended"
)

// NormalizeLiveChatMessage converts one raw YouTube liveChatMessage into
// the provider-independent Engagement Event shape, or reports that this
// message's type is a connector lifecycle signal or an intentionally
// unsupported type. See docs/provider-integrations/youtube-engagement.md
// §5 for the full mapping table.
func NormalizeLiveChatMessage(accountID string, msg LiveChatMessage) (NormalizeResult, error) {
	switch msg.Type {
	case "textMessageEvent":
		return normalizeTextMessage(accountID, msg)
	case "newSponsorEvent":
		return normalizeNewSponsor(accountID, msg)
	case "memberMilestoneChatEvent":
		return normalizeMemberMilestone(accountID, msg)
	case "membershipGiftingEvent":
		return normalizeMembershipGifting(accountID, msg)
	case "giftMembershipReceivedEvent":
		return normalizeGiftMembershipReceived(accountID, msg)
	case "superChatEvent":
		return normalizeSuperChat(accountID, msg)
	case "superStickerEvent":
		return normalizeSuperSticker(accountID, msg)
	case "userBannedEvent":
		return normalizeUserBanned(accountID, msg)
	case "chatEndedEvent":
		return NormalizeResult{Lifecycle: LifecycleChatEnded}, nil
	case "tombstone":
		// Explicitly not a deletion notification - see
		// docs/provider-integrations/youtube-engagement.md §5.
		return NormalizeResult{}, nil
	case "sponsorOnlyModeStartedEvent", "sponsorOnlyModeEndedEvent":
		// Channel-configuration signal, not audience engagement - never
		// published (§5/§8 of the research document).
		return NormalizeResult{}, nil
	case "pollEvent", "giftEvent", "fanFundingEvent":
		// Deliberately unsupported in Stage 15A - see §5's reasoning for
		// each (no safe normalized concept, or ID-reuse semantics that
		// make ordinary dedup unsafe, or a superseded legacy type).
		return NormalizeResult{}, nil
	default:
		return NormalizeResult{}, nil
	}
}

func base(accountID string, msg LiveChatMessage) (engagement.Event, error) {
	ts, err := time.Parse(time.RFC3339, msg.PublishedAt)
	if err != nil {
		return engagement.Event{}, fmt.Errorf("%w: %s: invalid publishedAt: %s", errMalformedMessage, msg.Type, err)
	}
	return engagement.Event{
		SchemaVersion:      engagement.CurrentSchemaVersion,
		ProviderID:         engagement.ProviderYouTube,
		ConnectedAccountID: accountID,
		ProviderEventID:    msg.ID,
		ProviderEventType:  msg.Type,
		PlatformTimestamp:  ts,
		DedupeKey:          msg.ID,
	}, nil
}

// authorUser builds the safe User block for the message's own author
// (msg.Author) - used by every event type where the author is the subject
// (everything except userBannedEvent, whose subject is the banned user, not
// the moderator that took the action - see normalizeUserBanned).
func authorUser(msg LiveChatMessage) *engagement.User {
	a := msg.Author
	if a.ChannelID == "" && a.DisplayName == "" {
		return nil
	}
	var roles []engagement.Role
	if a.IsChatOwner {
		roles = append(roles, engagement.RoleBroadcaster)
	}
	if a.IsChatModerator {
		roles = append(roles, engagement.RoleModerator)
	}
	if a.IsChatSponsor {
		// YouTube's "sponsor" is the current channel-membership concept
		// ("member" in the UI) - the closest existing provider-
		// independent role is Subscriber. IsVerified is deliberately
		// never mapped to any role (verified identity is not moderation
		// standing).
		roles = append(roles, engagement.RoleSubscriber)
	}
	return &engagement.User{
		ProviderUserID: a.ChannelID, DisplayName: a.DisplayName, AvatarURL: a.ProfileImageURL, Roles: roles,
	}
}

func normalizeTextMessage(accountID string, msg LiveChatMessage) (NormalizeResult, error) {
	evt, err := base(accountID, msg)
	if err != nil {
		return NormalizeResult{}, err
	}
	details := msg.raw.Snippet.TextMessageDetails
	if details == nil {
		return NormalizeResult{}, fmt.Errorf("%w: textMessageEvent missing textMessageDetails", errMalformedMessage)
	}
	evt.Type = engagement.TypeChatMessage
	evt.User = authorUser(msg)
	if evt.User == nil {
		return NormalizeResult{}, fmt.Errorf("%w: textMessageEvent missing author identity", errMalformedMessage)
	}
	text := details.MessageText
	message := engagement.NewMessage([]engagement.Fragment{{Type: engagement.FragmentText, Text: text}})
	evt.Message = &message
	return NormalizeResult{Event: evt, Supported: true}, nil
}

func normalizeNewSponsor(accountID string, msg LiveChatMessage) (NormalizeResult, error) {
	evt, err := base(accountID, msg)
	if err != nil {
		return NormalizeResult{}, err
	}
	details := msg.raw.Snippet.NewSponsorDetails
	if details == nil {
		return NormalizeResult{}, fmt.Errorf("%w: newSponsorEvent missing newSponsorDetails", errMalformedMessage)
	}
	evt.Type = engagement.TypeYouTubeMembership
	evt.User = authorUser(msg)
	if evt.User == nil {
		return NormalizeResult{}, fmt.Errorf("%w: newSponsorEvent missing author identity", errMalformedMessage)
	}
	evt.ProviderExtra = boundedExtra(map[string]string{
		"memberLevelName": details.MemberLevelName,
		"isUpgrade":       strconv.FormatBool(details.IsUpgrade),
	})
	return NormalizeResult{Event: evt, Supported: true}, nil
}

func normalizeMemberMilestone(accountID string, msg LiveChatMessage) (NormalizeResult, error) {
	evt, err := base(accountID, msg)
	if err != nil {
		return NormalizeResult{}, err
	}
	details := msg.raw.Snippet.MemberMilestoneChatDetails
	if details == nil {
		return NormalizeResult{}, fmt.Errorf("%w: memberMilestoneChatEvent missing memberMilestoneChatDetails", errMalformedMessage)
	}
	evt.Type = engagement.TypeYouTubeMembershipMilestone
	evt.User = authorUser(msg)
	if evt.User == nil {
		return NormalizeResult{}, fmt.Errorf("%w: memberMilestoneChatEvent missing author identity", errMalformedMessage)
	}
	month := int64(details.MemberMonth)
	evt.Quantity = &month
	if details.UserComment != "" {
		message := engagement.NewMessage([]engagement.Fragment{{Type: engagement.FragmentText, Text: details.UserComment}})
		evt.Message = &message
	}
	evt.ProviderExtra = boundedExtra(map[string]string{"memberLevelName": details.MemberLevelName})
	return NormalizeResult{Event: evt, Supported: true}, nil
}

func normalizeMembershipGifting(accountID string, msg LiveChatMessage) (NormalizeResult, error) {
	evt, err := base(accountID, msg)
	if err != nil {
		return NormalizeResult{}, err
	}
	details := msg.raw.Snippet.MembershipGiftingDetails
	if details == nil {
		return NormalizeResult{}, fmt.Errorf("%w: membershipGiftingEvent missing membershipGiftingDetails", errMalformedMessage)
	}
	// Reused, not duplicated - semantically identical to Twitch's own
	// gift-batch concept (docs/provider-integrations/
	// youtube-engagement.md §5).
	evt.Type = engagement.TypeSubscriptionGiftBatch
	evt.User = authorUser(msg)
	if evt.User == nil {
		return NormalizeResult{}, fmt.Errorf("%w: membershipGiftingEvent missing author identity", errMalformedMessage)
	}
	count := int64(details.GiftMembershipsCount)
	evt.Quantity = &count
	evt.ProviderExtra = boundedExtra(map[string]string{"giftMembershipsLevelName": details.GiftMembershipsLevelName})
	return NormalizeResult{Event: evt, Supported: true}, nil
}

func normalizeGiftMembershipReceived(accountID string, msg LiveChatMessage) (NormalizeResult, error) {
	evt, err := base(accountID, msg)
	if err != nil {
		return NormalizeResult{}, err
	}
	details := msg.raw.Snippet.GiftMembershipReceivedDetails
	if details == nil {
		return NormalizeResult{}, fmt.Errorf("%w: giftMembershipReceivedEvent missing giftMembershipReceivedDetails", errMalformedMessage)
	}
	// Reused, not duplicated - semantically identical to Twitch's own
	// "you received a gifted sub" concept.
	evt.Type = engagement.TypeGiftedSubscription
	// The message's own author is the recipient (proto doc comment:
	// "giftMembershipReceivedEvent - the user that received the gift
	// membership").
	evt.User = authorUser(msg)
	if evt.User == nil {
		return NormalizeResult{}, fmt.Errorf("%w: giftMembershipReceivedEvent missing author identity", errMalformedMessage)
	}
	evt.ProviderExtra = boundedExtra(map[string]string{
		"memberLevelName": details.MemberLevelName,
		"gifterChannelId": details.GifterChannelID,
	})
	return NormalizeResult{Event: evt, Supported: true}, nil
}

func normalizeSuperChat(accountID string, msg LiveChatMessage) (NormalizeResult, error) {
	evt, err := base(accountID, msg)
	if err != nil {
		return NormalizeResult{}, err
	}
	details := msg.raw.Snippet.SuperChatDetails
	if details == nil {
		return NormalizeResult{}, fmt.Errorf("%w: superChatEvent missing superChatDetails", errMalformedMessage)
	}
	money, err := engagement.NewMoney(int64(details.AmountMicros), details.Currency, details.AmountDisplayString)
	if err != nil {
		return NormalizeResult{}, fmt.Errorf("%w: superChatEvent: %s", errMalformedMessage, err)
	}
	evt.Type = engagement.TypeYouTubeSuperChat
	evt.User = authorUser(msg)
	if evt.User == nil {
		return NormalizeResult{}, fmt.Errorf("%w: superChatEvent missing author identity", errMalformedMessage)
	}
	evt.Money = &money
	if details.UserComment != "" {
		message := engagement.NewMessage([]engagement.Fragment{{Type: engagement.FragmentText, Text: details.UserComment}})
		evt.Message = &message
	}
	evt.ProviderExtra = boundedExtra(map[string]string{"tier": strconv.Itoa(details.Tier)})
	return NormalizeResult{Event: evt, Supported: true}, nil
}

func normalizeSuperSticker(accountID string, msg LiveChatMessage) (NormalizeResult, error) {
	evt, err := base(accountID, msg)
	if err != nil {
		return NormalizeResult{}, err
	}
	details := msg.raw.Snippet.SuperStickerDetails
	if details == nil {
		return NormalizeResult{}, fmt.Errorf("%w: superStickerEvent missing superStickerDetails", errMalformedMessage)
	}
	money, err := engagement.NewMoney(int64(details.AmountMicros), details.Currency, details.AmountDisplayString)
	if err != nil {
		return NormalizeResult{}, fmt.Errorf("%w: superStickerEvent: %s", errMalformedMessage, err)
	}
	evt.Type = engagement.TypeYouTubeSuperSticker
	evt.User = authorUser(msg)
	if evt.User == nil {
		return NormalizeResult{}, fmt.Errorf("%w: superStickerEvent missing author identity", errMalformedMessage)
	}
	evt.Money = &money
	// No sticker image URL exists anywhere in the API (confirmed,
	// docs/provider-integrations/youtube-engagement.md §3.3) - only safe
	// text/alt metadata is ever carried forward, never invented.
	evt.ProviderExtra = boundedExtra(map[string]string{
		"tier":      strconv.Itoa(details.Tier),
		"stickerId": details.SuperStickerMetadata.StickerID,
		"altText":   details.SuperStickerMetadata.AltText,
	})
	return NormalizeResult{Event: evt, Supported: true}, nil
}

func normalizeUserBanned(accountID string, msg LiveChatMessage) (NormalizeResult, error) {
	evt, err := base(accountID, msg)
	if err != nil {
		return NormalizeResult{}, err
	}
	details := msg.raw.Snippet.UserBannedDetails
	if details == nil {
		return NormalizeResult{}, fmt.Errorf("%w: userBannedEvent missing userBannedDetails", errMalformedMessage)
	}
	// Reused, not duplicated - a ban is "a moderation action happened,
	// described by a stable action identifier, not tied to one specific
	// prior message," exactly like Twitch's own clear_user_messages
	// (docs/provider-integrations/youtube-engagement.md §5, refined
	// during implementation - see docs/progress.md).
	evt.Type = engagement.TypeModeration
	action := "timeout"
	if details.BanType == "PERMANENT" {
		action = "ban"
	}
	evt.ModerationAction = action
	// The subject of a ban is the banned user, not the moderator that
	// issued it - the banned user's identity is carried in
	// bannedUserDetails, deliberately not msg.Author here.
	b := details.BannedUserDetails
	if b.ChannelID != "" || b.DisplayName != "" {
		evt.User = &engagement.User{ProviderUserID: b.ChannelID, DisplayName: b.DisplayName, AvatarURL: b.ProfileImageURL}
	}
	extra := map[string]string{}
	if details.BanType == "TEMPORARY" && details.BanDurationSeconds > 0 {
		extra["banDurationSeconds"] = strconv.FormatInt(int64(details.BanDurationSeconds), 10)
	}
	evt.ProviderExtra = boundedExtra(extra)
	return NormalizeResult{Event: evt, Supported: true}, nil
}

// boundedExtra drops empty values and returns nil (never an empty non-nil
// map) when nothing remains, keeping ProviderExtra's own bounds (see
// internal/domain/engagement/validation.go) trivially satisfied - every
// call site here passes at most 2-3 entries, far under the 8-entry cap.
func boundedExtra(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		if v != "" {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
