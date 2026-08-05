package twitch

import (
	"encoding/json"
	"testing"
	"time"

	engagement "github.com/streaming-tree/server/internal/domain/engagement"
)

var testTimestamp = time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)

func normalize(t *testing.T, subType, raw string) engagement.Event {
	t.Helper()
	evt, err := NormalizeEventSubNotification("acct_1", subType, "msg_1", testTimestamp, json.RawMessage(raw))
	if err != nil {
		t.Fatalf("NormalizeEventSubNotification(%s) error = %v", subType, err)
	}
	if err := evt.Validate(); err != nil {
		t.Fatalf("normalized event failed Validate(): %v\nevent: %+v", err, evt)
	}
	return evt
}

func TestNormalizeChatMessageWithFragments(t *testing.T) {
	raw := `{
		"broadcaster_user_id": "b1", "chatter_user_id": "u1", "chatter_user_login": "viewer",
		"chatter_user_name": "Viewer", "message_id": "chatmsg1", "color": "#00FF7F",
		"badges": [{"set_id": "moderator", "id": "1", "info": ""}],
		"message": {"text": "hello Kappa @friend", "fragments": [
			{"type": "text", "text": "hello "},
			{"type": "emote", "text": "Kappa", "emote": {"id": "25"}},
			{"type": "text", "text": " "},
			{"type": "mention", "text": "@friend", "mention": {"user_id": "u2", "user_login": "friend", "user_name": "Friend"}}
		]}
	}`
	evt := normalize(t, "channel.chat.message", raw)

	if evt.Type != engagement.TypeChatMessage {
		t.Errorf("Type = %q, want chat.message", evt.Type)
	}
	if evt.User == nil || evt.User.ProviderUserID != "u1" || evt.User.Color != "#00FF7F" {
		t.Errorf("User = %+v, want chatter u1 with color", evt.User)
	}
	if len(evt.User.Badges) != 1 || evt.User.Badges[0].SetID != "moderator" {
		t.Errorf("Badges = %+v, want one moderator badge", evt.User.Badges)
	}
	if evt.Message == nil || len(evt.Message.Fragments) != 4 {
		t.Fatalf("Message.Fragments = %+v, want 4 fragments", evt.Message)
	}
	if evt.Message.Text != "hello Kappa @friend" {
		t.Errorf("Message.Text = %q, want deterministic concatenation", evt.Message.Text)
	}
	if evt.Message.Fragments[1].EmoteID != "25" {
		t.Errorf("emote fragment EmoteID = %q, want 25", evt.Message.Fragments[1].EmoteID)
	}
	if evt.Message.Fragments[3].MentionLogin != "friend" {
		t.Errorf("mention fragment MentionLogin = %q, want friend", evt.Message.Fragments[3].MentionLogin)
	}
	if evt.DedupeKey != "msg_1" {
		t.Errorf("DedupeKey = %q, want the EventSub delivery message id", evt.DedupeKey)
	}
}

func TestNormalizeChatMessageWithUnknownFragmentTypeDoesNotFail(t *testing.T) {
	raw := `{
		"broadcaster_user_id": "b1", "chatter_user_id": "u1", "chatter_user_login": "viewer",
		"chatter_user_name": "Viewer", "message_id": "chatmsg2",
		"message": {"text": "???", "fragments": [{"type": "some_future_type", "text": "???"}]}
	}`
	evt := normalize(t, "channel.chat.message", raw)
	if evt.Message.Fragments[0].Type != engagement.FragmentUnknown {
		t.Errorf("fragment type = %q, want unknown fallback", evt.Message.Fragments[0].Type)
	}
}

func TestNormalizeChatMessageDeleted(t *testing.T) {
	raw := `{"broadcaster_user_id": "b1", "target_user_id": "u1", "target_user_login": "viewer", "target_user_name": "Viewer", "message_id": "deleted_msg_1"}`
	evt := normalize(t, "channel.chat.message_delete", raw)
	if evt.Type != engagement.TypeChatMessageDeleted {
		t.Errorf("Type = %q, want chat.message_deleted", evt.Type)
	}
	if evt.ModerationRef != "deleted_msg_1" {
		t.Errorf("ModerationRef = %q, want deleted_msg_1", evt.ModerationRef)
	}
}

func TestNormalizeChatCleared(t *testing.T) {
	evt := normalize(t, "channel.chat.clear", `{"broadcaster_user_id": "b1"}`)
	if evt.Type != engagement.TypeChatCleared {
		t.Errorf("Type = %q, want chat.cleared", evt.Type)
	}
}

func TestNormalizeChatClearUserMessagesMapsToModerationWithStableAction(t *testing.T) {
	evt := normalize(t, "channel.chat.clear_user_messages", `{"broadcaster_user_id": "b1", "target_user_id": "u1"}`)
	if evt.Type != engagement.TypeModeration {
		t.Errorf("Type = %q, want moderation", evt.Type)
	}
	if evt.ModerationAction != "clear_user_messages" {
		t.Errorf("ModerationAction = %q, want clear_user_messages", evt.ModerationAction)
	}
}

func TestNormalizeFollow(t *testing.T) {
	evt := normalize(t, "channel.follow", `{"user_id": "u1", "user_login": "viewer", "user_name": "Viewer", "followed_at": "2026-08-05T12:00:00Z"}`)
	if evt.Type != engagement.TypeFollow {
		t.Errorf("Type = %q, want follow", evt.Type)
	}
	if evt.User.ProviderUserID != "u1" {
		t.Errorf("User.ProviderUserID = %q, want u1", evt.User.ProviderUserID)
	}
}

func TestNormalizeSubscribeNonGift(t *testing.T) {
	evt := normalize(t, "channel.subscribe", `{"user_id": "u1", "user_login": "viewer", "user_name": "Viewer", "tier": "1000", "is_gift": false}`)
	if evt.Type != engagement.TypeSubscription {
		t.Errorf("Type = %q, want subscription for a non-gift subscribe", evt.Type)
	}
}

func TestNormalizeSubscribeGiftedMapsToGiftedSubscription(t *testing.T) {
	evt := normalize(t, "channel.subscribe", `{"user_id": "u1", "user_login": "viewer", "user_name": "Viewer", "tier": "1000", "is_gift": true}`)
	if evt.Type != engagement.TypeGiftedSubscription {
		t.Errorf("Type = %q, want gifted_subscription for is_gift=true", evt.Type)
	}
}

func TestNormalizeSubscriptionGiftBatchStaysDistinctFromRecipientEvent(t *testing.T) {
	batch := normalize(t, "channel.subscription.gift", `{"user_id": "gifter1", "user_login": "gifter", "user_name": "Gifter", "total": 5, "tier": "1000", "cumulative_total": 20, "is_anonymous": false}`)
	if batch.Type != engagement.TypeSubscriptionGiftBatch {
		t.Errorf("Type = %q, want subscription_gift_batch", batch.Type)
	}
	if batch.Quantity == nil || *batch.Quantity != 5 {
		t.Errorf("Quantity = %v, want 5", batch.Quantity)
	}
	if batch.User == nil || batch.User.ProviderUserID != "gifter1" {
		t.Errorf("User = %+v, want the gifter's identity", batch.User)
	}
}

func TestNormalizeSubscriptionGiftAnonymousNeverFabricatesIdentity(t *testing.T) {
	evt := normalize(t, "channel.subscription.gift", `{"total": 3, "tier": "1000", "is_anonymous": true}`)
	if evt.User == nil || !evt.User.Anonymous {
		t.Fatalf("User = %+v, want Anonymous=true with no fabricated identity", evt.User)
	}
	if evt.User.ProviderUserID != "" || evt.User.Login != "" {
		t.Errorf("anonymous gift event carries an identity: %+v", evt.User)
	}
}

func TestNormalizeResubscriptionPreservesCumulativeAndStreakMonths(t *testing.T) {
	evt := normalize(t, "channel.subscription.message", `{
		"user_id": "u1", "user_login": "viewer", "user_name": "Viewer", "tier": "1000",
		"message": {"text": "Loving the content!"}, "cumulative_months": 10, "streak_months": 3, "duration_months": 1
	}`)
	if evt.Type != engagement.TypeResubscription {
		t.Errorf("Type = %q, want resubscription", evt.Type)
	}
	if evt.ProviderExtra["cumulativeMonths"] != "10" || evt.ProviderExtra["streakMonths"] != "3" {
		t.Errorf("ProviderExtra = %+v, want cumulative/streak months preserved", evt.ProviderExtra)
	}
	if evt.Message.Text != "Loving the content!" {
		t.Errorf("Message.Text = %q", evt.Message.Text)
	}
}

func TestNormalizeCheerNonAnonymous(t *testing.T) {
	evt := normalize(t, "channel.cheer", `{"is_anonymous": false, "user_id": "u1", "user_login": "viewer", "user_name": "Viewer", "message": "Cheer100 nice!", "bits": 100}`)
	if evt.Type != engagement.TypeBits {
		t.Errorf("Type = %q, want bits", evt.Type)
	}
	if evt.Quantity == nil || *evt.Quantity != 100 {
		t.Errorf("Quantity = %v, want 100", evt.Quantity)
	}
	if evt.User.Anonymous {
		t.Error("expected a non-anonymous cheer to carry a real identity")
	}
}

func TestNormalizeCheerAnonymous(t *testing.T) {
	evt := normalize(t, "channel.cheer", `{"is_anonymous": true, "message": "Cheer100", "bits": 100}`)
	if !evt.User.Anonymous {
		t.Error("expected anonymous cheer to be flagged Anonymous")
	}
}

func TestNormalizeIncomingRaidNeverInventsViewerCount(t *testing.T) {
	evt := normalize(t, "channel.raid", `{"from_broadcaster_user_id": "b2", "from_broadcaster_user_login": "raider", "from_broadcaster_user_name": "Raider", "viewers": 42}`)
	if evt.Type != engagement.TypeRaid {
		t.Errorf("Type = %q, want raid", evt.Type)
	}
	if evt.Quantity == nil || *evt.Quantity != 42 {
		t.Errorf("Quantity = %v, want the real reported viewer count 42", evt.Quantity)
	}
}

func TestNormalizeChannelPointRedemption(t *testing.T) {
	evt := normalize(t, "channel.channel_points_custom_reward_redemption.add", `{
		"id": "redemption1", "user_id": "u1", "user_login": "viewer", "user_name": "Viewer",
		"user_input": "pick me!", "reward": {"id": "reward1", "title": "Highlight message", "cost": 500}
	}`)
	if evt.Type != engagement.TypeChannelPointRedemption {
		t.Errorf("Type = %q, want channel_point_redemption", evt.Type)
	}
	if evt.ProviderEventID != "redemption1" {
		t.Errorf("ProviderEventID = %q, want redemption1", evt.ProviderEventID)
	}
	if evt.ProviderExtra["rewardTitle"] != "Highlight message" || evt.ProviderExtra["rewardCost"] != "500" {
		t.Errorf("ProviderExtra = %+v, want reward title/cost preserved", evt.ProviderExtra)
	}
	if evt.Message == nil || evt.Message.Text != "pick me!" {
		t.Errorf("Message = %+v, want the user's redemption input", evt.Message)
	}
}

func TestNormalizeStreamOnlineAndOffline(t *testing.T) {
	online := normalize(t, "stream.online", `{"id": "s1", "type": "live", "started_at": "2026-08-05T12:00:00Z"}`)
	if online.Type != engagement.TypeStreamOnline {
		t.Errorf("Type = %q, want stream.online", online.Type)
	}
	offline := normalize(t, "stream.offline", `{"broadcaster_user_id": "b1"}`)
	if offline.Type != engagement.TypeStreamOffline {
		t.Errorf("Type = %q, want stream.offline", offline.Type)
	}
}

func TestNormalizeRejectsUnrecognizedSubscriptionType(t *testing.T) {
	_, err := NormalizeEventSubNotification("acct_1", "channel.some.future.type", "msg_1", testTimestamp, json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected an error for an unrecognized subscription type")
	}
}

func TestNormalizeToleratesUnknownExtraFields(t *testing.T) {
	raw := `{"broadcaster_user_id": "b1", "user_id": "u1", "user_login": "viewer", "user_name": "Viewer", "tier": "1000", "is_gift": false, "a_field_added_by_twitch_later": "ignored"}`
	evt := normalize(t, "channel.subscribe", raw)
	if evt.Type != engagement.TypeSubscription {
		t.Errorf("Type = %q, want subscription despite an unknown extra field", evt.Type)
	}
}

func TestNormalizeRejectsMalformedJSON(t *testing.T) {
	_, err := NormalizeEventSubNotification("acct_1", "channel.follow", "msg_1", testTimestamp, json.RawMessage(`{not valid json`))
	if err == nil {
		t.Fatal("expected an error for malformed JSON")
	}
}
