package twitch

import (
	"encoding/json"
	"fmt"
	"time"

	engagement "github.com/streaming-tree/server/internal/domain/engagement"
)

// Wire shapes for the "event" body of each selected notification type - see
// docs/provider-integrations/twitch-engagement.md. Deliberately unexported:
// nothing outside this package ever sees a raw Twitch event payload (the
// stage task's "provider payload boundary" requirement).

type wireChatFragment struct {
	Type  string `json:"type"`
	Text  string `json:"text"`
	Emote *struct {
		ID string `json:"id"`
	} `json:"emote"`
	Cheermote *struct {
		Prefix string `json:"prefix"`
		Bits   int    `json:"bits"`
	} `json:"cheermote"`
	Mention *struct {
		UserID    string `json:"user_id"`
		UserLogin string `json:"user_login"`
		UserName  string `json:"user_name"`
	} `json:"mention"`
}

type wireChatBadge struct {
	SetID string `json:"set_id"`
	ID    string `json:"id"`
	Info  string `json:"info"`
}

type wireChatMessageEvent struct {
	BroadcasterUserID  string `json:"broadcaster_user_id"`
	ChatterUserID      string `json:"chatter_user_id"`
	ChatterUserLogin   string `json:"chatter_user_login"`
	ChatterUserName    string `json:"chatter_user_name"`
	ChatterIsAnonymous bool   `json:"chatter_is_anonymous"`
	MessageID          string `json:"message_id"`
	Message            struct {
		Text      string             `json:"text"`
		Fragments []wireChatFragment `json:"fragments"`
	} `json:"message"`
	Color                       string          `json:"color"`
	Badges                      []wireChatBadge `json:"badges"`
	ChannelPointsCustomRewardID string          `json:"channel_points_custom_reward_id"`
}

type wireChatMessageDeleteEvent struct {
	BroadcasterUserID string `json:"broadcaster_user_id"`
	TargetUserID      string `json:"target_user_id"`
	TargetUserLogin   string `json:"target_user_login"`
	TargetUserName    string `json:"target_user_name"`
	MessageID         string `json:"message_id"`
}

type wireChatClearEvent struct {
	BroadcasterUserID string `json:"broadcaster_user_id"`
}

type wireChatClearUserMessagesEvent struct {
	BroadcasterUserID string `json:"broadcaster_user_id"`
	TargetUserID      string `json:"target_user_id"`
}

type wireFollowEvent struct {
	UserID     string `json:"user_id"`
	UserLogin  string `json:"user_login"`
	UserName   string `json:"user_name"`
	FollowedAt string `json:"followed_at"`
}

type wireSubscribeEvent struct {
	UserID    string `json:"user_id"`
	UserLogin string `json:"user_login"`
	UserName  string `json:"user_name"`
	Tier      string `json:"tier"`
	IsGift    bool   `json:"is_gift"`
}

type wireSubscriptionGiftEvent struct {
	UserID          string `json:"user_id"`
	UserLogin       string `json:"user_login"`
	UserName        string `json:"user_name"`
	Total           int    `json:"total"`
	Tier            string `json:"tier"`
	CumulativeTotal *int   `json:"cumulative_total"`
	IsAnonymous     bool   `json:"is_anonymous"`
}

type wireSubscriptionMessageEvent struct {
	UserID    string `json:"user_id"`
	UserLogin string `json:"user_login"`
	UserName  string `json:"user_name"`
	Tier      string `json:"tier"`
	Message   struct {
		Text string `json:"text"`
	} `json:"message"`
	CumulativeMonths int  `json:"cumulative_months"`
	StreakMonths     *int `json:"streak_months"`
	DurationMonths   int  `json:"duration_months"`
}

type wireCheerEvent struct {
	IsAnonymous bool   `json:"is_anonymous"`
	UserID      string `json:"user_id"`
	UserLogin   string `json:"user_login"`
	UserName    string `json:"user_name"`
	Message     string `json:"message"`
	Bits        int    `json:"bits"`
}

type wireRaidEvent struct {
	FromBroadcasterUserID    string `json:"from_broadcaster_user_id"`
	FromBroadcasterUserLogin string `json:"from_broadcaster_user_login"`
	FromBroadcasterUserName  string `json:"from_broadcaster_user_name"`
	Viewers                  int    `json:"viewers"`
}

type wireChannelPointsRedemptionEvent struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	UserLogin string `json:"user_login"`
	UserName  string `json:"user_name"`
	UserInput string `json:"user_input"`
	Reward    struct {
		ID    string `json:"id"`
		Title string `json:"title"`
		Cost  int    `json:"cost"`
	} `json:"reward"`
	RedeemedAt string `json:"redeemed_at"`
}

type wireStreamOnlineEvent struct {
	Type      string `json:"type"`
	StartedAt string `json:"started_at"`
}

// base builds the fields every normalized event shares, before a specific
// mapping function fills in the rest.
func base(accountID, subType, messageID string, timestamp time.Time) engagement.Event {
	return engagement.Event{
		SchemaVersion:      engagement.CurrentSchemaVersion,
		ProviderID:         engagement.ProviderTwitch,
		ConnectedAccountID: accountID,
		ProviderEventType:  subType,
		PlatformTimestamp:  timestamp,
		DedupeKey:          messageID,
	}
}

func mapFragments(in []wireChatFragment) []engagement.Fragment {
	out := make([]engagement.Fragment, 0, len(in))
	for _, f := range in {
		switch f.Type {
		case "text":
			out = append(out, engagement.Fragment{Type: engagement.FragmentText, Text: f.Text})
		case "emote":
			id := ""
			if f.Emote != nil {
				id = f.Emote.ID
			}
			out = append(out, engagement.Fragment{Type: engagement.FragmentEmote, Text: f.Text, EmoteID: id})
		case "cheermote":
			prefix, bits := "", 0
			if f.Cheermote != nil {
				prefix, bits = f.Cheermote.Prefix, f.Cheermote.Bits
			}
			out = append(out, engagement.Fragment{Type: engagement.FragmentCheermote, Text: f.Text, CheermotePrefix: prefix, CheermoteBits: bits})
		case "mention":
			var userID, login, name string
			if f.Mention != nil {
				userID, login, name = f.Mention.UserID, f.Mention.UserLogin, f.Mention.UserName
			}
			out = append(out, engagement.Fragment{
				Type: engagement.FragmentMention, Text: f.Text,
				MentionUserID: userID, MentionLogin: login, MentionDisplayName: name,
			})
		default:
			out = append(out, engagement.Fragment{Type: engagement.FragmentUnknown, Text: f.Text})
		}
	}
	return out
}

func mapBadges(in []wireChatBadge) []engagement.Badge {
	out := make([]engagement.Badge, 0, len(in))
	for _, b := range in {
		out = append(out, engagement.Badge{SetID: b.SetID, ID: b.ID, Info: b.Info})
	}
	return out
}

// NormalizeEventSubNotification maps one EventSub notification's raw event
// body to the Stage 8A normalized model. subType is the subscription's own
// type string (EventSubEnvelope.Metadata.SubscriptionType); messageID is
// the delivery's metadata.message_id, used verbatim as the event's
// deduplication key (see docs/provider-integrations/twitch-engagement.md).
func NormalizeEventSubNotification(accountID, subType, messageID string, timestamp time.Time, raw json.RawMessage) (engagement.Event, error) {
	switch subType {
	case "channel.chat.message":
		return normalizeChatMessage(accountID, subType, messageID, timestamp, raw)
	case "channel.chat.message_delete":
		return normalizeChatMessageDelete(accountID, subType, messageID, timestamp, raw)
	case "channel.chat.clear":
		return normalizeChatClear(accountID, subType, messageID, timestamp, raw)
	case "channel.chat.clear_user_messages":
		return normalizeChatClearUserMessages(accountID, subType, messageID, timestamp, raw)
	case "channel.follow":
		return normalizeFollow(accountID, subType, messageID, timestamp, raw)
	case "channel.subscribe":
		return normalizeSubscribe(accountID, subType, messageID, timestamp, raw)
	case "channel.subscription.gift":
		return normalizeSubscriptionGift(accountID, subType, messageID, timestamp, raw)
	case "channel.subscription.message":
		return normalizeSubscriptionMessage(accountID, subType, messageID, timestamp, raw)
	case "channel.cheer":
		return normalizeCheer(accountID, subType, messageID, timestamp, raw)
	case "channel.raid":
		return normalizeRaid(accountID, subType, messageID, timestamp, raw)
	case "channel.channel_points_custom_reward_redemption.add":
		return normalizeChannelPointsRedemption(accountID, subType, messageID, timestamp, raw)
	case "stream.online":
		return normalizeStreamOnline(accountID, subType, messageID, timestamp, raw)
	case "stream.offline":
		return normalizeStreamOffline(accountID, subType, messageID, timestamp, raw)
	default:
		return engagement.Event{}, fmt.Errorf("%w: unrecognized subscription type %q", ErrInvalidResponse, subType)
	}
}

func normalizeChatMessage(accountID, subType, messageID string, ts time.Time, raw json.RawMessage) (engagement.Event, error) {
	var w wireChatMessageEvent
	if err := json.Unmarshal(raw, &w); err != nil {
		return engagement.Event{}, fmt.Errorf("%w: channel.chat.message: %s", ErrInvalidResponse, err)
	}
	msg := engagement.NewMessage(mapFragments(w.Message.Fragments))
	evt := base(accountID, subType, messageID, ts)
	evt.Type = engagement.TypeChatMessage
	evt.ProviderEventID = w.MessageID
	evt.Message = &msg
	evt.User = &engagement.User{
		ProviderUserID: w.ChatterUserID, Login: w.ChatterUserLogin, DisplayName: w.ChatterUserName,
		Color: w.Color, Badges: mapBadges(w.Badges), Anonymous: w.ChatterIsAnonymous,
	}
	if w.ChannelPointsCustomRewardID != "" {
		evt.ProviderExtra = map[string]string{"channelPointsCustomRewardId": w.ChannelPointsCustomRewardID}
	}
	return evt, nil
}

func normalizeChatMessageDelete(accountID, subType, messageID string, ts time.Time, raw json.RawMessage) (engagement.Event, error) {
	var w wireChatMessageDeleteEvent
	if err := json.Unmarshal(raw, &w); err != nil {
		return engagement.Event{}, fmt.Errorf("%w: channel.chat.message_delete: %s", ErrInvalidResponse, err)
	}
	evt := base(accountID, subType, messageID, ts)
	evt.Type = engagement.TypeChatMessageDeleted
	evt.ModerationRef = w.MessageID
	evt.User = &engagement.User{ProviderUserID: w.TargetUserID, Login: w.TargetUserLogin, DisplayName: w.TargetUserName}
	return evt, nil
}

func normalizeChatClear(accountID, subType, messageID string, ts time.Time, raw json.RawMessage) (engagement.Event, error) {
	var w wireChatClearEvent
	if err := json.Unmarshal(raw, &w); err != nil {
		return engagement.Event{}, fmt.Errorf("%w: channel.chat.clear: %s", ErrInvalidResponse, err)
	}
	evt := base(accountID, subType, messageID, ts)
	evt.Type = engagement.TypeChatCleared
	return evt, nil
}

func normalizeChatClearUserMessages(accountID, subType, messageID string, ts time.Time, raw json.RawMessage) (engagement.Event, error) {
	var w wireChatClearUserMessagesEvent
	if err := json.Unmarshal(raw, &w); err != nil {
		return engagement.Event{}, fmt.Errorf("%w: channel.chat.clear_user_messages: %s", ErrInvalidResponse, err)
	}
	evt := base(accountID, subType, messageID, ts)
	evt.Type = engagement.TypeModeration
	evt.ModerationAction = "clear_user_messages"
	evt.User = &engagement.User{ProviderUserID: w.TargetUserID}
	return evt, nil
}

func normalizeFollow(accountID, subType, messageID string, ts time.Time, raw json.RawMessage) (engagement.Event, error) {
	var w wireFollowEvent
	if err := json.Unmarshal(raw, &w); err != nil {
		return engagement.Event{}, fmt.Errorf("%w: channel.follow: %s", ErrInvalidResponse, err)
	}
	evt := base(accountID, subType, messageID, ts)
	evt.Type = engagement.TypeFollow
	evt.User = &engagement.User{ProviderUserID: w.UserID, Login: w.UserLogin, DisplayName: w.UserName}
	return evt, nil
}

func normalizeSubscribe(accountID, subType, messageID string, ts time.Time, raw json.RawMessage) (engagement.Event, error) {
	var w wireSubscribeEvent
	if err := json.Unmarshal(raw, &w); err != nil {
		return engagement.Event{}, fmt.Errorf("%w: channel.subscribe: %s", ErrInvalidResponse, err)
	}
	evt := base(accountID, subType, messageID, ts)
	// A gifted individual subscription and a normal new subscription arrive
	// on the same subscription type, distinguished only by is_gift - see
	// docs/provider-integrations/twitch-engagement.md.
	if w.IsGift {
		evt.Type = engagement.TypeGiftedSubscription
	} else {
		evt.Type = engagement.TypeSubscription
	}
	evt.User = &engagement.User{ProviderUserID: w.UserID, Login: w.UserLogin, DisplayName: w.UserName}
	evt.ProviderExtra = map[string]string{"tier": w.Tier}
	return evt, nil
}

func normalizeSubscriptionGift(accountID, subType, messageID string, ts time.Time, raw json.RawMessage) (engagement.Event, error) {
	var w wireSubscriptionGiftEvent
	if err := json.Unmarshal(raw, &w); err != nil {
		return engagement.Event{}, fmt.Errorf("%w: channel.subscription.gift: %s", ErrInvalidResponse, err)
	}
	evt := base(accountID, subType, messageID, ts)
	evt.Type = engagement.TypeSubscriptionGiftBatch
	qty := int64(w.Total)
	evt.Quantity = &qty
	evt.ProviderExtra = map[string]string{"tier": w.Tier}
	if !w.IsAnonymous && w.UserID != "" {
		evt.User = &engagement.User{ProviderUserID: w.UserID, Login: w.UserLogin, DisplayName: w.UserName}
	} else {
		// Anonymous gifter: never fabricate an identity - see
		// docs/provider-integrations/twitch-engagement.md and the stage
		// task's explicit "preserve anonymous behavior" requirement.
		evt.User = &engagement.User{Anonymous: true}
	}
	return evt, nil
}

func normalizeSubscriptionMessage(accountID, subType, messageID string, ts time.Time, raw json.RawMessage) (engagement.Event, error) {
	var w wireSubscriptionMessageEvent
	if err := json.Unmarshal(raw, &w); err != nil {
		return engagement.Event{}, fmt.Errorf("%w: channel.subscription.message: %s", ErrInvalidResponse, err)
	}
	evt := base(accountID, subType, messageID, ts)
	evt.Type = engagement.TypeResubscription
	msg := engagement.NewMessage([]engagement.Fragment{{Type: engagement.FragmentText, Text: w.Message.Text}})
	evt.Message = &msg
	evt.User = &engagement.User{ProviderUserID: w.UserID, Login: w.UserLogin, DisplayName: w.UserName}
	extra := map[string]string{
		"tier":             w.Tier,
		"cumulativeMonths": fmt.Sprintf("%d", w.CumulativeMonths),
		"durationMonths":   fmt.Sprintf("%d", w.DurationMonths),
	}
	if w.StreakMonths != nil {
		extra["streakMonths"] = fmt.Sprintf("%d", *w.StreakMonths)
	}
	evt.ProviderExtra = extra
	return evt, nil
}

func normalizeCheer(accountID, subType, messageID string, ts time.Time, raw json.RawMessage) (engagement.Event, error) {
	var w wireCheerEvent
	if err := json.Unmarshal(raw, &w); err != nil {
		return engagement.Event{}, fmt.Errorf("%w: channel.cheer: %s", ErrInvalidResponse, err)
	}
	evt := base(accountID, subType, messageID, ts)
	evt.Type = engagement.TypeBits
	bits := int64(w.Bits)
	evt.Quantity = &bits
	if w.Message != "" {
		msg := engagement.NewMessage([]engagement.Fragment{{Type: engagement.FragmentText, Text: w.Message}})
		evt.Message = &msg
	}
	if !w.IsAnonymous && w.UserID != "" {
		evt.User = &engagement.User{ProviderUserID: w.UserID, Login: w.UserLogin, DisplayName: w.UserName}
	} else {
		evt.User = &engagement.User{Anonymous: true}
	}
	return evt, nil
}

func normalizeRaid(accountID, subType, messageID string, ts time.Time, raw json.RawMessage) (engagement.Event, error) {
	var w wireRaidEvent
	if err := json.Unmarshal(raw, &w); err != nil {
		return engagement.Event{}, fmt.Errorf("%w: channel.raid: %s", ErrInvalidResponse, err)
	}
	evt := base(accountID, subType, messageID, ts)
	evt.Type = engagement.TypeRaid
	qty := int64(w.Viewers)
	evt.Quantity = &qty
	evt.User = &engagement.User{ProviderUserID: w.FromBroadcasterUserID, Login: w.FromBroadcasterUserLogin, DisplayName: w.FromBroadcasterUserName}
	return evt, nil
}

func normalizeChannelPointsRedemption(accountID, subType, messageID string, ts time.Time, raw json.RawMessage) (engagement.Event, error) {
	var w wireChannelPointsRedemptionEvent
	if err := json.Unmarshal(raw, &w); err != nil {
		return engagement.Event{}, fmt.Errorf("%w: channel.channel_points_custom_reward_redemption.add: %s", ErrInvalidResponse, err)
	}
	evt := base(accountID, subType, messageID, ts)
	evt.Type = engagement.TypeChannelPointRedemption
	evt.ProviderEventID = w.ID
	evt.User = &engagement.User{ProviderUserID: w.UserID, Login: w.UserLogin, DisplayName: w.UserName}
	evt.ProviderExtra = map[string]string{
		"rewardId":    w.Reward.ID,
		"rewardTitle": w.Reward.Title,
		"rewardCost":  fmt.Sprintf("%d", w.Reward.Cost),
	}
	if w.UserInput != "" {
		msg := engagement.NewMessage([]engagement.Fragment{{Type: engagement.FragmentText, Text: w.UserInput}})
		evt.Message = &msg
	}
	return evt, nil
}

func normalizeStreamOnline(accountID, subType, messageID string, ts time.Time, raw json.RawMessage) (engagement.Event, error) {
	var w wireStreamOnlineEvent
	if err := json.Unmarshal(raw, &w); err != nil {
		return engagement.Event{}, fmt.Errorf("%w: stream.online: %s", ErrInvalidResponse, err)
	}
	evt := base(accountID, subType, messageID, ts)
	evt.Type = engagement.TypeStreamOnline
	if w.Type != "" {
		evt.ProviderExtra = map[string]string{"streamType": w.Type}
	}
	return evt, nil
}

func normalizeStreamOffline(accountID, subType, messageID string, ts time.Time, raw json.RawMessage) (engagement.Event, error) {
	evt := base(accountID, subType, messageID, ts)
	evt.Type = engagement.TypeStreamOffline
	return evt, nil
}
