package alerts

import "testing"

func TestCapabilityForYouTubeEventTypes(t *testing.T) {
	cases := []struct {
		eventType EventType
		want      Capability
	}{
		{EventYouTubeMembership, Capability{HasUser: true, HasMembershipLevel: true}},
		{EventYouTubeMembershipMilestone, Capability{HasUser: true, HasMessage: true, HasQuantity: true, HasMembershipLevel: true}},
		{EventYouTubeSuperChat, Capability{HasUser: true, HasMessage: true, HasAmount: true}},
		{EventYouTubeSuperSticker, Capability{HasUser: true, HasAmount: true}},
	}
	for _, c := range cases {
		if got := CapabilityFor(c.eventType); got != c.want {
			t.Errorf("CapabilityFor(%s) = %+v, want %+v", c.eventType, got, c.want)
		}
	}
}

func TestCapabilityNoTwitchEventTypeHasAmountOrMembership(t *testing.T) {
	twitchTypes := []EventType{
		EventFollow, EventSubscription, EventResubscription, EventGiftedSubscription,
		EventSubscriptionGiftBatch, EventBits, EventRaid, EventChannelPointRedemption,
	}
	for _, et := range twitchTypes {
		c := CapabilityFor(et)
		if c.HasAmount {
			t.Errorf("CapabilityFor(%s).HasAmount = true, want false (no Twitch normalization path populates Money)", et)
		}
		if c.HasMembershipLevel {
			t.Errorf("CapabilityFor(%s).HasMembershipLevel = true, want false", et)
		}
	}
}

func TestGroupingCapabilityYouTubeEventTypesNeverGroupable(t *testing.T) {
	youtubeTypes := []EventType{
		EventYouTubeMembership, EventYouTubeMembershipMilestone,
		EventYouTubeSuperChat, EventYouTubeSuperSticker,
	}
	for _, et := range youtubeTypes {
		if got := GroupingCapabilityFor(et); got.Groupable {
			t.Errorf("GroupingCapabilityFor(%s).Groupable = true, want false (monetary/membership events are never auto-grouped)", et)
		}
	}
}

func TestValidEventTypesIncludesAllThirteenTypes(t *testing.T) {
	if len(ValidEventTypes) != 13 {
		t.Fatalf("len(ValidEventTypes) = %d, want 13 (8 Twitch + 4 YouTube + 1 donation)", len(ValidEventTypes))
	}
	for _, et := range ValidEventTypes {
		if _, ok := Capabilities[et]; !ok {
			t.Errorf("ValidEventTypes contains %s with no Capabilities entry", et)
		}
		if _, ok := GroupingCapabilities[et]; !ok {
			t.Errorf("ValidEventTypes contains %s with no GroupingCapabilities entry", et)
		}
	}
}
