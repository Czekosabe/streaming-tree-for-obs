package youtube

import (
	"github.com/streaming-tree/server/internal/provider/youtube/streamlistpb"
)

// streamMessageType maps the gRPC streamList enum
// (LiveChatMessageSnippet_TypeWrapper_Type) back onto the exact same
// snippet.type string values the REST liveChatMessages.list JSON response
// uses ("textMessageEvent", ...) - see docs/provider-integrations/
// youtube-engagement.md §4b.1: both transports carry the same underlying
// schema, only the wire encoding differs. Keeping this mapping the only
// gRPC-aware code in this file lets livechat_normalize.go (and everything
// downstream of it) stay completely transport-independent: it already
// switches on these exact strings and never needs to know a message came
// from gRPC instead of REST.
func streamMessageType(t streamlistpb.LiveChatMessageSnippet_TypeWrapper_Type) string {
	switch t {
	case streamlistpb.LiveChatMessageSnippet_TypeWrapper_TEXT_MESSAGE_EVENT:
		return "textMessageEvent"
	case streamlistpb.LiveChatMessageSnippet_TypeWrapper_TOMBSTONE:
		return "tombstone"
	case streamlistpb.LiveChatMessageSnippet_TypeWrapper_FAN_FUNDING_EVENT:
		return "fanFundingEvent"
	case streamlistpb.LiveChatMessageSnippet_TypeWrapper_CHAT_ENDED_EVENT:
		return "chatEndedEvent"
	case streamlistpb.LiveChatMessageSnippet_TypeWrapper_SPONSOR_ONLY_MODE_STARTED_EVENT:
		return "sponsorOnlyModeStartedEvent"
	case streamlistpb.LiveChatMessageSnippet_TypeWrapper_SPONSOR_ONLY_MODE_ENDED_EVENT:
		return "sponsorOnlyModeEndedEvent"
	case streamlistpb.LiveChatMessageSnippet_TypeWrapper_NEW_SPONSOR_EVENT:
		return "newSponsorEvent"
	case streamlistpb.LiveChatMessageSnippet_TypeWrapper_MEMBER_MILESTONE_CHAT_EVENT:
		return "memberMilestoneChatEvent"
	case streamlistpb.LiveChatMessageSnippet_TypeWrapper_MEMBERSHIP_GIFTING_EVENT:
		return "membershipGiftingEvent"
	case streamlistpb.LiveChatMessageSnippet_TypeWrapper_GIFT_MEMBERSHIP_RECEIVED_EVENT:
		return "giftMembershipReceivedEvent"
	case streamlistpb.LiveChatMessageSnippet_TypeWrapper_USER_BANNED_EVENT:
		return "userBannedEvent"
	case streamlistpb.LiveChatMessageSnippet_TypeWrapper_SUPER_CHAT_EVENT:
		return "superChatEvent"
	case streamlistpb.LiveChatMessageSnippet_TypeWrapper_SUPER_STICKER_EVENT:
		return "superStickerEvent"
	case streamlistpb.LiveChatMessageSnippet_TypeWrapper_POLL_EVENT:
		return "pollEvent"
	case streamlistpb.LiveChatMessageSnippet_TypeWrapper_GIFT_EVENT:
		return "giftEvent"
	default:
		// INVALID_TYPE, or any future value this vendored proto does not
		// yet know about - falls through livechat_normalize.go's own
		// default case (an unsupported-type diagnostic), never a panic.
		return ""
	}
}

func streamBanType(t streamlistpb.LiveChatUserBannedMessageDetails_BanTypeWrapper_BanType) string {
	switch t {
	case streamlistpb.LiveChatUserBannedMessageDetails_BanTypeWrapper_PERMANENT:
		return "PERMANENT"
	case streamlistpb.LiveChatUserBannedMessageDetails_BanTypeWrapper_TEMPORARY:
		return "TEMPORARY"
	default:
		return ""
	}
}

// fromStreamMessage converts one gRPC-received streamlistpb.LiveChatMessage
// into this package's own LiveChatMessage - the exact same shape
// ListLiveChatMessages (the superseded REST path, kept only as historical
// context in comments elsewhere) already produced, so
// livechat_normalize.go needs no changes at all to normalize a message
// that arrived over gRPC instead of REST.
func fromStreamMessage(pb *streamlistpb.LiveChatMessage) LiveChatMessage {
	snippet := pb.GetSnippet()
	author := pb.GetAuthorDetails()

	resource := liveChatMessageResource{
		ID: pb.GetId(),
		Snippet: liveChatMessageSnippet{
			Type:              streamMessageType(snippet.GetType()),
			LiveChatID:        snippet.GetLiveChatId(),
			AuthorChannelID:   snippet.GetAuthorChannelId(),
			PublishedAt:       snippet.GetPublishedAt(),
			HasDisplayContent: snippet.GetHasDisplayContent(),
			DisplayMessage:    snippet.GetDisplayMessage(),
		},
		AuthorDetails: liveChatMessageAuthorDetails{
			ChannelID: author.GetChannelId(), ChannelURL: author.GetChannelUrl(),
			DisplayName: author.GetDisplayName(), ProfileImageURL: author.GetProfileImageUrl(),
			IsVerified: author.GetIsVerified(), IsChatOwner: author.GetIsChatOwner(),
			IsChatSponsor: author.GetIsChatSponsor(), IsChatModerator: author.GetIsChatModerator(),
		},
	}

	if d := snippet.GetTextMessageDetails(); d != nil {
		resource.Snippet.TextMessageDetails = &struct {
			MessageText string `json:"messageText"`
		}{MessageText: d.GetMessageText()}
	}
	if d := snippet.GetSuperChatDetails(); d != nil {
		resource.Snippet.SuperChatDetails = &struct {
			AmountMicros        flexibleInt64 `json:"amountMicros"`
			Currency            string        `json:"currency"`
			AmountDisplayString string        `json:"amountDisplayString"`
			UserComment         string        `json:"userComment"`
			Tier                int           `json:"tier"`
		}{
			AmountMicros: flexibleInt64(d.GetAmountMicros()), Currency: d.GetCurrency(),
			AmountDisplayString: d.GetAmountDisplayString(), UserComment: d.GetUserComment(),
			Tier: int(d.GetTier()),
		}
	}
	if d := snippet.GetSuperStickerDetails(); d != nil {
		meta := d.GetSuperStickerMetadata()
		resource.Snippet.SuperStickerDetails = &struct {
			AmountMicros         flexibleInt64 `json:"amountMicros"`
			Currency             string        `json:"currency"`
			AmountDisplayString  string        `json:"amountDisplayString"`
			Tier                 int           `json:"tier"`
			SuperStickerMetadata struct {
				StickerID string `json:"stickerId"`
				AltText   string `json:"altText"`
				Language  string `json:"language"`
			} `json:"superStickerMetadata"`
		}{
			AmountMicros: flexibleInt64(d.GetAmountMicros()), Currency: d.GetCurrency(),
			AmountDisplayString: d.GetAmountDisplayString(), Tier: int(d.GetTier()),
		}
		resource.Snippet.SuperStickerDetails.SuperStickerMetadata.StickerID = meta.GetStickerId()
		resource.Snippet.SuperStickerDetails.SuperStickerMetadata.AltText = meta.GetAltText()
		resource.Snippet.SuperStickerDetails.SuperStickerMetadata.Language = meta.GetAltTextLanguage()
	}
	if d := snippet.GetNewSponsorDetails(); d != nil {
		resource.Snippet.NewSponsorDetails = &struct {
			MemberLevelName string `json:"memberLevelName"`
			IsUpgrade       bool   `json:"isUpgrade"`
		}{MemberLevelName: d.GetMemberLevelName(), IsUpgrade: d.GetIsUpgrade()}
	}
	if d := snippet.GetMemberMilestoneChatDetails(); d != nil {
		resource.Snippet.MemberMilestoneChatDetails = &struct {
			UserComment     string `json:"userComment"`
			MemberMonth     int    `json:"memberMonth"`
			MemberLevelName string `json:"memberLevelName"`
		}{UserComment: d.GetUserComment(), MemberMonth: int(d.GetMemberMonth()), MemberLevelName: d.GetMemberLevelName()}
	}
	if d := snippet.GetMembershipGiftingDetails(); d != nil {
		resource.Snippet.MembershipGiftingDetails = &struct {
			GiftMembershipsCount     int    `json:"giftMembershipsCount"`
			GiftMembershipsLevelName string `json:"giftMembershipsLevelName"`
		}{GiftMembershipsCount: int(d.GetGiftMembershipsCount()), GiftMembershipsLevelName: d.GetGiftMembershipsLevelName()}
	}
	if d := snippet.GetGiftMembershipReceivedDetails(); d != nil {
		resource.Snippet.GiftMembershipReceivedDetails = &struct {
			MemberLevelName                      string `json:"memberLevelName"`
			GifterChannelID                      string `json:"gifterChannelId"`
			AssociatedMembershipGiftingMessageID string `json:"associatedMembershipGiftingMessageId"`
		}{
			MemberLevelName: d.GetMemberLevelName(), GifterChannelID: d.GetGifterChannelId(),
			AssociatedMembershipGiftingMessageID: d.GetAssociatedMembershipGiftingMessageId(),
		}
	}
	if d := snippet.GetUserBannedDetails(); d != nil {
		banned := d.GetBannedUserDetails()
		resource.Snippet.UserBannedDetails = &struct {
			BannedUserDetails struct {
				ChannelID       string `json:"channelId"`
				ChannelURL      string `json:"channelUrl"`
				DisplayName     string `json:"displayName"`
				ProfileImageURL string `json:"profileImageUrl"`
			} `json:"bannedUserDetails"`
			BanType            string        `json:"banType"`
			BanDurationSeconds flexibleInt64 `json:"banDurationSeconds"`
		}{
			BanType: streamBanType(d.GetBanType()), BanDurationSeconds: flexibleInt64(d.GetBanDurationSeconds()),
		}
		resource.Snippet.UserBannedDetails.BannedUserDetails.ChannelID = banned.GetChannelId()
		resource.Snippet.UserBannedDetails.BannedUserDetails.ChannelURL = banned.GetChannelUrl()
		resource.Snippet.UserBannedDetails.BannedUserDetails.DisplayName = banned.GetDisplayName()
		resource.Snippet.UserBannedDetails.BannedUserDetails.ProfileImageURL = banned.GetProfileImageUrl()
	}

	return LiveChatMessage{
		ID: resource.ID, Type: resource.Snippet.Type,
		AuthorChannelID: resource.Snippet.AuthorChannelID, PublishedAt: resource.Snippet.PublishedAt,
		DisplayMessage: resource.Snippet.DisplayMessage,
		Author: LiveChatMessageAuthor{
			ChannelID: resource.AuthorDetails.ChannelID, ChannelURL: resource.AuthorDetails.ChannelURL,
			DisplayName: resource.AuthorDetails.DisplayName, ProfileImageURL: resource.AuthorDetails.ProfileImageURL,
			IsVerified: resource.AuthorDetails.IsVerified, IsChatOwner: resource.AuthorDetails.IsChatOwner,
			IsChatSponsor: resource.AuthorDetails.IsChatSponsor, IsChatModerator: resource.AuthorDetails.IsChatModerator,
		},
		raw: resource,
	}
}

// fromStreamResponse converts one streamList server response into the same
// LiveChatMessagePage shape the superseded REST client used, so
// runtime/youtubeengagement's connector reads a page the same way
// regardless of transport. PollingIntervalMillis is always 0 here - the
// streaming RPC's response has no such field (docs/provider-integrations/
// youtube-engagement.md §4b.1); the connector no longer reads it once the
// gRPC transport is in use (see connector.go).
func fromStreamResponse(pb *streamlistpb.LiveChatMessageListResponse) LiveChatMessagePage {
	items := pb.GetItems()
	messages := make([]LiveChatMessage, 0, len(items))
	for _, item := range items {
		if item.GetId() == "" {
			continue // tolerate a malformed single entry rather than failing the whole page
		}
		messages = append(messages, fromStreamMessage(item))
	}
	return LiveChatMessagePage{
		NextPageToken: pb.GetNextPageToken(),
		Ended:         pb.GetOfflineAt() != "",
		Messages:      messages,
	}
}
