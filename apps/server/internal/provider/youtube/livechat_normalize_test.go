package youtube

import (
	"testing"
	"time"

	engagement "github.com/streaming-tree/server/internal/domain/engagement"
)

const testPublishedAt = "2026-08-12T06:00:00Z"

func msgWithSnippet(id, msgType string, snippet liveChatMessageSnippet, author liveChatMessageAuthorDetails) LiveChatMessage {
	snippet.Type = msgType
	snippet.PublishedAt = testPublishedAt
	resource := liveChatMessageResource{ID: id, Snippet: snippet, AuthorDetails: author}
	return LiveChatMessage{
		ID: id, Type: msgType, PublishedAt: testPublishedAt,
		AuthorChannelID: author.ChannelID,
		Author: LiveChatMessageAuthor{
			ChannelID: author.ChannelID, DisplayName: author.DisplayName, ProfileImageURL: author.ProfileImageURL,
			IsVerified: author.IsVerified, IsChatOwner: author.IsChatOwner,
			IsChatSponsor: author.IsChatSponsor, IsChatModerator: author.IsChatModerator,
		},
		raw: resource,
	}
}

func normalAuthor() liveChatMessageAuthorDetails {
	return liveChatMessageAuthorDetails{ChannelID: "UC_123", DisplayName: "Some Viewer", ProfileImageURL: "https://example.com/a.png"}
}

func TestNormalizeTextMessage(t *testing.T) {
	msg := msgWithSnippet("msg_1", "textMessageEvent", liveChatMessageSnippet{
		TextMessageDetails: &struct {
			MessageText string `json:"messageText"`
		}{MessageText: "hello chat"},
	}, normalAuthor())

	res, err := NormalizeLiveChatMessage("acct_1", msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Supported {
		t.Fatal("expected Supported=true")
	}
	if res.Event.Type != engagement.TypeChatMessage {
		t.Fatalf("expected TypeChatMessage, got %s", res.Event.Type)
	}
	if res.Event.ProviderID != engagement.ProviderYouTube {
		t.Fatalf("expected ProviderYouTube, got %s", res.Event.ProviderID)
	}
	if res.Event.DedupeKey != "msg_1" || res.Event.ProviderEventID != "msg_1" {
		t.Fatalf("expected dedupe key/provider event id = msg_1, got %q/%q", res.Event.DedupeKey, res.Event.ProviderEventID)
	}
	if res.Event.Message == nil || res.Event.Message.Text != "hello chat" {
		t.Fatalf("expected message text 'hello chat', got %+v", res.Event.Message)
	}
	if res.Event.User == nil || res.Event.User.ProviderUserID != "UC_123" {
		t.Fatalf("expected user UC_123, got %+v", res.Event.User)
	}
	if err := res.Event.Validate(); err != nil {
		t.Fatalf("normalized event failed Validate: %v", err)
	}
}

func TestNormalizeTextMessageMissingDetailsIsMalformed(t *testing.T) {
	msg := msgWithSnippet("msg_2", "textMessageEvent", liveChatMessageSnippet{}, normalAuthor())
	_, err := NormalizeLiveChatMessage("acct_1", msg)
	if err == nil {
		t.Fatal("expected malformed-message error")
	}
}

func TestNormalizeNewSponsorEvent(t *testing.T) {
	msg := msgWithSnippet("msg_3", "newSponsorEvent", liveChatMessageSnippet{
		NewSponsorDetails: &struct {
			MemberLevelName string `json:"memberLevelName"`
			IsUpgrade       bool   `json:"isUpgrade"`
		}{MemberLevelName: "Level 2", IsUpgrade: false},
	}, normalAuthor())

	res, err := NormalizeLiveChatMessage("acct_1", msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Event.Type != engagement.TypeYouTubeMembership {
		t.Fatalf("expected TypeYouTubeMembership, got %s", res.Event.Type)
	}
	if res.Event.ProviderExtra["memberLevelName"] != "Level 2" {
		t.Fatalf("expected memberLevelName in ProviderExtra, got %+v", res.Event.ProviderExtra)
	}
	if err := res.Event.Validate(); err != nil {
		t.Fatalf("normalized event failed Validate: %v", err)
	}
}

func TestNormalizeMemberMilestoneChatEvent(t *testing.T) {
	msg := msgWithSnippet("msg_4", "memberMilestoneChatEvent", liveChatMessageSnippet{
		MemberMilestoneChatDetails: &struct {
			UserComment     string `json:"userComment"`
			MemberMonth     int    `json:"memberMonth"`
			MemberLevelName string `json:"memberLevelName"`
		}{UserComment: "6 months!", MemberMonth: 6, MemberLevelName: "Level 1"},
	}, normalAuthor())

	res, err := NormalizeLiveChatMessage("acct_1", msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Event.Type != engagement.TypeYouTubeMembershipMilestone {
		t.Fatalf("expected TypeYouTubeMembershipMilestone, got %s", res.Event.Type)
	}
	if res.Event.Quantity == nil || *res.Event.Quantity != 6 {
		t.Fatalf("expected quantity=6, got %+v", res.Event.Quantity)
	}
	if res.Event.Message == nil || res.Event.Message.Text != "6 months!" {
		t.Fatalf("expected comment preserved, got %+v", res.Event.Message)
	}
}

func TestNormalizeMemberMilestoneChatEventWithNoComment(t *testing.T) {
	msg := msgWithSnippet("msg_4b", "memberMilestoneChatEvent", liveChatMessageSnippet{
		MemberMilestoneChatDetails: &struct {
			UserComment     string `json:"userComment"`
			MemberMonth     int    `json:"memberMonth"`
			MemberLevelName string `json:"memberLevelName"`
		}{MemberMonth: 12},
	}, normalAuthor())

	res, err := NormalizeLiveChatMessage("acct_1", msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Event.Message != nil {
		t.Fatalf("expected no message for an empty comment, got %+v", res.Event.Message)
	}
}

func TestNormalizeMembershipGiftingEventReusesGiftBatchType(t *testing.T) {
	msg := msgWithSnippet("msg_5", "membershipGiftingEvent", liveChatMessageSnippet{
		MembershipGiftingDetails: &struct {
			GiftMembershipsCount     int    `json:"giftMembershipsCount"`
			GiftMembershipsLevelName string `json:"giftMembershipsLevelName"`
		}{GiftMembershipsCount: 5, GiftMembershipsLevelName: "Level 1"},
	}, normalAuthor())

	res, err := NormalizeLiveChatMessage("acct_1", msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Event.Type != engagement.TypeSubscriptionGiftBatch {
		t.Fatalf("expected reused TypeSubscriptionGiftBatch, got %s", res.Event.Type)
	}
	if res.Event.Quantity == nil || *res.Event.Quantity != 5 {
		t.Fatalf("expected quantity=5, got %+v", res.Event.Quantity)
	}
}

func TestNormalizeGiftMembershipReceivedEventReusesGiftedSubscriptionType(t *testing.T) {
	msg := msgWithSnippet("msg_6", "giftMembershipReceivedEvent", liveChatMessageSnippet{
		GiftMembershipReceivedDetails: &struct {
			MemberLevelName                      string `json:"memberLevelName"`
			GifterChannelID                      string `json:"gifterChannelId"`
			AssociatedMembershipGiftingMessageID string `json:"associatedMembershipGiftingMessageId"`
		}{MemberLevelName: "Level 1", GifterChannelID: "UC_gifter"},
	}, normalAuthor())

	res, err := NormalizeLiveChatMessage("acct_1", msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Event.Type != engagement.TypeGiftedSubscription {
		t.Fatalf("expected reused TypeGiftedSubscription, got %s", res.Event.Type)
	}
	if res.Event.ProviderExtra["gifterChannelId"] != "UC_gifter" {
		t.Fatalf("expected gifterChannelId in ProviderExtra, got %+v", res.Event.ProviderExtra)
	}
}

func TestNormalizeSuperChatEvent(t *testing.T) {
	msg := msgWithSnippet("msg_7", "superChatEvent", liveChatMessageSnippet{
		SuperChatDetails: &struct {
			AmountMicros        flexibleInt64 `json:"amountMicros"`
			Currency            string        `json:"currency"`
			AmountDisplayString string        `json:"amountDisplayString"`
			UserComment         string        `json:"userComment"`
			Tier                int           `json:"tier"`
		}{AmountMicros: 5_000_000, Currency: "usd", AmountDisplayString: "$5.00", UserComment: "great stream!", Tier: 2},
	}, normalAuthor())

	res, err := NormalizeLiveChatMessage("acct_1", msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Event.Type != engagement.TypeYouTubeSuperChat {
		t.Fatalf("expected TypeYouTubeSuperChat, got %s", res.Event.Type)
	}
	if res.Event.Money == nil {
		t.Fatal("expected Money to be set")
	}
	if res.Event.Money.AmountMicros != 5_000_000 || res.Event.Money.Currency != "USD" {
		t.Fatalf("expected 5000000 USD, got %+v", res.Event.Money)
	}
	if res.Event.Message == nil || res.Event.Message.Text != "great stream!" {
		t.Fatalf("expected comment preserved, got %+v", res.Event.Message)
	}
	if err := res.Event.Validate(); err != nil {
		t.Fatalf("normalized event failed Validate: %v", err)
	}
}

func TestNormalizeSuperStickerEventHasNoImageURL(t *testing.T) {
	msg := msgWithSnippet("msg_8", "superStickerEvent", liveChatMessageSnippet{
		SuperStickerDetails: &struct {
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
			AmountMicros: 2_000_000, Currency: "USD", AmountDisplayString: "$2.00", Tier: 1,
			SuperStickerMetadata: struct {
				StickerID string `json:"stickerId"`
				AltText   string `json:"altText"`
				Language  string `json:"language"`
			}{StickerID: "sticker_1", AltText: "excited cat"},
		},
	}, normalAuthor())

	res, err := NormalizeLiveChatMessage("acct_1", msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Event.Type != engagement.TypeYouTubeSuperSticker {
		t.Fatalf("expected TypeYouTubeSuperSticker, got %s", res.Event.Type)
	}
	if res.Event.Money == nil || res.Event.Money.AmountMicros != 2_000_000 {
		t.Fatalf("expected money 2000000, got %+v", res.Event.Money)
	}
	if res.Event.ProviderExtra["altText"] != "excited cat" {
		t.Fatalf("expected altText preserved, got %+v", res.Event.ProviderExtra)
	}
	for k := range res.Event.ProviderExtra {
		if k == "imageUrl" || k == "stickerUrl" {
			t.Fatalf("no image URL should ever be present, found key %q", k)
		}
	}
}

func TestNormalizeUserBannedEventTargetsBannedUserNotModerator(t *testing.T) {
	msg := msgWithSnippet("msg_9", "userBannedEvent", liveChatMessageSnippet{
		UserBannedDetails: &struct {
			BannedUserDetails struct {
				ChannelID       string `json:"channelId"`
				ChannelURL      string `json:"channelUrl"`
				DisplayName     string `json:"displayName"`
				ProfileImageURL string `json:"profileImageUrl"`
			} `json:"bannedUserDetails"`
			BanType            string        `json:"banType"`
			BanDurationSeconds flexibleInt64 `json:"banDurationSeconds"`
		}{
			BannedUserDetails: struct {
				ChannelID       string `json:"channelId"`
				ChannelURL      string `json:"channelUrl"`
				DisplayName     string `json:"displayName"`
				ProfileImageURL string `json:"profileImageUrl"`
			}{ChannelID: "UC_banned", DisplayName: "Rude Viewer"},
			BanType: "TEMPORARY", BanDurationSeconds: 300,
		},
	}, liveChatMessageAuthorDetails{ChannelID: "UC_moderator", DisplayName: "Mod Person", IsChatModerator: true})

	res, err := NormalizeLiveChatMessage("acct_1", msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Event.Type != engagement.TypeModeration {
		t.Fatalf("expected reused TypeModeration, got %s", res.Event.Type)
	}
	if res.Event.ModerationAction != "timeout" {
		t.Fatalf("expected 'timeout' action for TEMPORARY ban, got %q", res.Event.ModerationAction)
	}
	if res.Event.User == nil || res.Event.User.ProviderUserID != "UC_banned" {
		t.Fatalf("expected banned user as subject, got %+v", res.Event.User)
	}
	if res.Event.ProviderExtra["banDurationSeconds"] != "300" {
		t.Fatalf("expected banDurationSeconds=300, got %+v", res.Event.ProviderExtra)
	}
}

func TestNormalizeUserBannedEventPermanentBan(t *testing.T) {
	msg := msgWithSnippet("msg_9b", "userBannedEvent", liveChatMessageSnippet{
		UserBannedDetails: &struct {
			BannedUserDetails struct {
				ChannelID       string `json:"channelId"`
				ChannelURL      string `json:"channelUrl"`
				DisplayName     string `json:"displayName"`
				ProfileImageURL string `json:"profileImageUrl"`
			} `json:"bannedUserDetails"`
			BanType            string        `json:"banType"`
			BanDurationSeconds flexibleInt64 `json:"banDurationSeconds"`
		}{
			BannedUserDetails: struct {
				ChannelID       string `json:"channelId"`
				ChannelURL      string `json:"channelUrl"`
				DisplayName     string `json:"displayName"`
				ProfileImageURL string `json:"profileImageUrl"`
			}{ChannelID: "UC_banned2"},
			BanType: "PERMANENT",
		},
	}, normalAuthor())

	res, err := NormalizeLiveChatMessage("acct_1", msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Event.ModerationAction != "ban" {
		t.Fatalf("expected 'ban' action for PERMANENT ban, got %q", res.Event.ModerationAction)
	}
}

func TestNormalizeChatEndedEventIsLifecycleNotAnEvent(t *testing.T) {
	msg := msgWithSnippet("msg_10", "chatEndedEvent", liveChatMessageSnippet{}, liveChatMessageAuthorDetails{})
	res, err := NormalizeLiveChatMessage("acct_1", msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Supported {
		t.Fatal("chatEndedEvent must never be Supported (never published to the Event Bus)")
	}
	if res.Lifecycle != LifecycleChatEnded {
		t.Fatalf("expected LifecycleChatEnded, got %q", res.Lifecycle)
	}
}

func TestNormalizeTombstoneIsDroppedNotDeletion(t *testing.T) {
	msg := msgWithSnippet("msg_11", "tombstone", liveChatMessageSnippet{}, liveChatMessageAuthorDetails{})
	res, err := NormalizeLiveChatMessage("acct_1", msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Supported || res.Lifecycle != "" {
		t.Fatalf("expected tombstone to be dropped entirely, got %+v", res)
	}
}

func TestNormalizeUnsupportedTypesAreDroppedNotErrored(t *testing.T) {
	for _, ty := range []string{
		"sponsorOnlyModeStartedEvent", "sponsorOnlyModeEndedEvent",
		"pollEvent", "giftEvent", "fanFundingEvent", "someBrandNewFutureEventType",
	} {
		msg := msgWithSnippet("msg_x", ty, liveChatMessageSnippet{}, liveChatMessageAuthorDetails{})
		res, err := NormalizeLiveChatMessage("acct_1", msg)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", ty, err)
		}
		if res.Supported || res.Lifecycle != "" {
			t.Fatalf("%s: expected an unsupported, non-lifecycle result, got %+v", ty, res)
		}
	}
}

func TestNormalizeGiftEventNeverBecomesSuperChat(t *testing.T) {
	// giftEvent (the newer virtual "Jewels" gift type) must never be
	// mistaken for real currency - it carries no Money at all in Stage
	// 15A since it is left entirely unsupported.
	msg := msgWithSnippet("msg_12", "giftEvent", liveChatMessageSnippet{}, normalAuthor())
	res, err := NormalizeLiveChatMessage("acct_1", msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Supported {
		t.Fatal("giftEvent must not be normalized as a supported event")
	}
	if res.Event.Money != nil {
		t.Fatal("giftEvent must never produce a Money value")
	}
}

func TestNormalizeInvalidPublishedAtIsMalformed(t *testing.T) {
	msg := msgWithSnippet("msg_13", "textMessageEvent", liveChatMessageSnippet{
		TextMessageDetails: &struct {
			MessageText string `json:"messageText"`
		}{MessageText: "hi"},
	}, normalAuthor())
	msg.PublishedAt = "not-a-real-timestamp"
	msg.raw.Snippet.PublishedAt = "not-a-real-timestamp"
	_, err := NormalizeLiveChatMessage("acct_1", msg)
	if err == nil {
		t.Fatal("expected malformed-message error for an unparseable publishedAt")
	}
}

func TestNormalizeSuperChatWithNegativeAmountIsRejected(t *testing.T) {
	// Not realistically producible via the flexibleInt64 unmarshal path
	// (negative amountMicros should never occur), but the normalizer must
	// still refuse to construct an invalid Money rather than trust the
	// wire value blindly.
	msg := msgWithSnippet("msg_14", "superChatEvent", liveChatMessageSnippet{
		SuperChatDetails: &struct {
			AmountMicros        flexibleInt64 `json:"amountMicros"`
			Currency            string        `json:"currency"`
			AmountDisplayString string        `json:"amountDisplayString"`
			UserComment         string        `json:"userComment"`
			Tier                int           `json:"tier"`
		}{AmountMicros: -1, Currency: "USD"},
	}, normalAuthor())
	_, err := NormalizeLiveChatMessage("acct_1", msg)
	if err == nil {
		t.Fatal("expected error for negative Super Chat amount")
	}
}

func TestFlexibleInt64UnmarshalsStringAndNumber(t *testing.T) {
	var fromString flexibleInt64
	if err := fromString.UnmarshalJSON([]byte(`"1750000"`)); err != nil {
		t.Fatalf("unexpected error parsing string-encoded int: %v", err)
	}
	if fromString != 1_750_000 {
		t.Fatalf("expected 1750000, got %d", fromString)
	}

	var fromNumber flexibleInt64
	if err := fromNumber.UnmarshalJSON([]byte(`1750000`)); err != nil {
		t.Fatalf("unexpected error parsing number-encoded int: %v", err)
	}
	if fromNumber != 1_750_000 {
		t.Fatalf("expected 1750000, got %d", fromNumber)
	}
}

// Sanity check that testPublishedAt itself parses under time.RFC3339, since
// every fixture above relies on it.
func TestFixturePublishedAtIsValidRFC3339(t *testing.T) {
	if _, err := time.Parse(time.RFC3339, testPublishedAt); err != nil {
		t.Fatalf("fixture timestamp is not valid RFC3339: %v", err)
	}
}
