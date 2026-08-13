package audio

import (
	"testing"

	engagement "github.com/streaming-tree/server/internal/domain/engagement"
)

func user(name string) *engagement.User {
	return &engagement.User{ProviderUserID: "u1", DisplayName: name}
}

func anonUser() *engagement.User {
	return &engagement.User{Anonymous: true}
}

func msg(text string) *engagement.Message {
	return &engagement.Message{Text: text}
}

func money(display string, micros int64, currency string) *engagement.Money {
	return &engagement.Money{AmountMicros: micros, Currency: currency, DisplayAmount: display}
}

func TestBuildUtteranceChatMessage(t *testing.T) {
	evt := engagement.Event{Type: engagement.TypeChatMessage, User: user("Ada"), Message: msg("hello there")}
	text, ok := BuildUtterance(evt)
	if !ok || text != "Ada says hello there" {
		t.Errorf("BuildUtterance() = %q, %v, want %q, true", text, ok, "Ada says hello there")
	}
}

func TestBuildUtteranceChatMessageMissingNameOrMessage(t *testing.T) {
	if _, ok := BuildUtterance(engagement.Event{Type: engagement.TypeChatMessage, User: anonUser(), Message: msg("hi")}); ok {
		t.Error("BuildUtterance() ok = true for a chat message with no resolvable name, want false")
	}
	if _, ok := BuildUtterance(engagement.Event{Type: engagement.TypeChatMessage, User: user("Ada"), Message: msg("")}); ok {
		t.Error("BuildUtterance() ok = true for a chat message with empty text, want false")
	}
}

func TestBuildUtteranceFollow(t *testing.T) {
	text, ok := BuildUtterance(engagement.Event{Type: engagement.TypeFollow, User: user("Grace")})
	if !ok || text != "Grace followed" {
		t.Errorf("BuildUtterance() = %q, %v, want %q, true", text, ok, "Grace followed")
	}
}

func TestBuildUtteranceResubscriptionWithMessage(t *testing.T) {
	text, ok := BuildUtterance(engagement.Event{Type: engagement.TypeResubscription, User: user("Linus"), Message: msg("still here")})
	want := "Linus resubscribed: still here"
	if !ok || text != want {
		t.Errorf("BuildUtterance() = %q, %v, want %q, true", text, ok, want)
	}
}

func TestBuildUtteranceGiftedSubscriptionGiftBatchAnonymous(t *testing.T) {
	qty := int64(5)
	text, ok := BuildUtterance(engagement.Event{Type: engagement.TypeSubscriptionGiftBatch, User: anonUser(), Quantity: &qty})
	want := "An anonymous gifter gifted 5 subscriptions"
	if !ok || text != want {
		t.Errorf("BuildUtterance() = %q, %v, want %q, true", text, ok, want)
	}
}

func TestBuildUtteranceGiftedSubscriptionGiftBatchNamed(t *testing.T) {
	qty := int64(3)
	text, ok := BuildUtterance(engagement.Event{Type: engagement.TypeSubscriptionGiftBatch, User: user("Ada"), Quantity: &qty})
	want := "Ada gifted 3 subscriptions"
	if !ok || text != want {
		t.Errorf("BuildUtterance() = %q, %v, want %q, true", text, ok, want)
	}
}

func TestBuildUtteranceBitsWithMessageAndAnonymous(t *testing.T) {
	qty := int64(100)
	text, ok := BuildUtterance(engagement.Event{Type: engagement.TypeBits, User: anonUser(), Quantity: &qty, Message: msg("nice stream")})
	want := "Someone cheered 100 bits: nice stream"
	if !ok || text != want {
		t.Errorf("BuildUtterance() = %q, %v, want %q, true", text, ok, want)
	}
}

func TestBuildUtteranceRaidWithAndWithoutQuantity(t *testing.T) {
	qty := int64(42)
	text, ok := BuildUtterance(engagement.Event{Type: engagement.TypeRaid, User: user("Ada"), Quantity: &qty})
	if !ok || text != "Ada raided with 42 viewers" {
		t.Errorf("BuildUtterance() = %q, %v, want raid with viewers", text, ok)
	}

	text, ok = BuildUtterance(engagement.Event{Type: engagement.TypeRaid, User: user("Ada")})
	if !ok || text != "Ada raided" {
		t.Errorf("BuildUtterance() = %q, %v, want plain raid", text, ok)
	}
}

func TestBuildUtteranceChannelPointRedemption(t *testing.T) {
	text, ok := BuildUtterance(engagement.Event{Type: engagement.TypeChannelPointRedemption, User: user("Ada"), Message: msg("hydrate!")})
	want := "Ada redeemed a reward: hydrate!"
	if !ok || text != want {
		t.Errorf("BuildUtterance() = %q, %v, want %q, true", text, ok, want)
	}
}

func TestBuildUtteranceYouTubeMembershipWithAndWithoutLevel(t *testing.T) {
	evt := engagement.Event{Type: engagement.TypeYouTubeMembership, User: user("Ada"), ProviderExtra: map[string]string{"memberLevelName": "Gold"}}
	text, ok := BuildUtterance(evt)
	if !ok || text != "Ada became a Gold member" {
		t.Errorf("BuildUtterance() = %q, %v, want membership with level", text, ok)
	}

	text, ok = BuildUtterance(engagement.Event{Type: engagement.TypeYouTubeMembership, User: user("Ada")})
	if !ok || text != "Ada became a member" {
		t.Errorf("BuildUtterance() = %q, %v, want membership without level", text, ok)
	}
}

func TestBuildUtteranceYouTubeMembershipMilestone(t *testing.T) {
	months := int64(12)
	evt := engagement.Event{
		Type:          engagement.TypeYouTubeMembershipMilestone,
		User:          user("Ada"),
		Quantity:      &months,
		Message:       msg("been a great year"),
		ProviderExtra: map[string]string{"memberLevelName": "Gold"},
	}
	text, ok := BuildUtterance(evt)
	want := "Ada (Gold) celebrated 12 months as a member: been a great year"
	if !ok || text != want {
		t.Errorf("BuildUtterance() = %q, %v, want %q, true", text, ok, want)
	}
}

func TestBuildUtteranceYouTubeSuperChatRequiresAmount(t *testing.T) {
	evt := engagement.Event{Type: engagement.TypeYouTubeSuperChat, User: user("Ada"), Message: msg("great stream"), Money: money("$5.00", 5_000_000, "USD")}
	text, ok := BuildUtterance(evt)
	want := "Ada sent a super chat of $5.00: great stream"
	if !ok || text != want {
		t.Errorf("BuildUtterance() = %q, %v, want %q, true", text, ok, want)
	}

	if _, ok := BuildUtterance(engagement.Event{Type: engagement.TypeYouTubeSuperChat, User: user("Ada")}); ok {
		t.Error("BuildUtterance() ok = true for a super chat with no Money, want false")
	}
}

func TestBuildUtteranceYouTubeSuperSticker(t *testing.T) {
	evt := engagement.Event{Type: engagement.TypeYouTubeSuperSticker, User: user("Ada"), Money: money("$2.00", 2_000_000, "USD")}
	text, ok := BuildUtterance(evt)
	want := "Ada sent a super sticker of $2.00"
	if !ok || text != want {
		t.Errorf("BuildUtterance() = %q, %v, want %q, true", text, ok, want)
	}
}

func TestBuildUtteranceDonationAnonymousAndNamed(t *testing.T) {
	evt := engagement.Event{Type: engagement.TypeDonation, User: anonUser(), Money: money("$10.00", 10_000_000, "USD"), Message: msg("keep it up")}
	text, ok := BuildUtterance(evt)
	want := "An anonymous donor donated $10.00: keep it up"
	if !ok || text != want {
		t.Errorf("BuildUtterance() = %q, %v, want %q, true", text, ok, want)
	}

	evt2 := engagement.Event{Type: engagement.TypeDonation, User: user("Ada"), Money: money("$10.00", 10_000_000, "USD")}
	text, ok = BuildUtterance(evt2)
	want2 := "Ada donated $10.00"
	if !ok || text != want2 {
		t.Errorf("BuildUtterance() = %q, %v, want %q, true", text, ok, want2)
	}
}

func TestBuildUtteranceDonationRequiresAmount(t *testing.T) {
	if _, ok := BuildUtterance(engagement.Event{Type: engagement.TypeDonation, User: user("Ada")}); ok {
		t.Error("BuildUtterance() ok = true for a donation with no Money, want false")
	}
}

func TestBuildUtteranceMoneyFallsBackToFormattedMicrosWhenDisplayAmountEmpty(t *testing.T) {
	evt := engagement.Event{Type: engagement.TypeDonation, User: user("Ada"), Money: &engagement.Money{AmountMicros: 1_500_000, Currency: "USD"}}
	text, ok := BuildUtterance(evt)
	want := "Ada donated 1.50 USD"
	if !ok || text != want {
		t.Errorf("BuildUtterance() = %q, %v, want %q, true", text, ok, want)
	}
}

func TestBuildUtteranceUnspeakableTypeReturnsFalse(t *testing.T) {
	if _, ok := BuildUtterance(engagement.Event{Type: engagement.TypeModeration, User: user("Ada")}); ok {
		t.Error("BuildUtterance() ok = true for a moderation event, want false")
	}
}
