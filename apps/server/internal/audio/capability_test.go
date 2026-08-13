package audio

import (
	"testing"

	engagement "github.com/streaming-tree/server/internal/domain/engagement"
)

func TestCapabilityForKnownSupporterFamilyTypes(t *testing.T) {
	supporterFamily := []engagement.Type{
		engagement.TypeBits,
		engagement.TypeSubscription,
		engagement.TypeResubscription,
		engagement.TypeGiftedSubscription,
		engagement.TypeSubscriptionGiftBatch,
		engagement.TypeYouTubeMembership,
		engagement.TypeYouTubeMembershipMilestone,
		engagement.TypeYouTubeSuperChat,
		engagement.TypeYouTubeSuperSticker,
		engagement.TypeDonation,
	}
	for _, et := range supporterFamily {
		c := CapabilityFor(et)
		if !c.Speakable {
			t.Errorf("CapabilityFor(%s).Speakable = false, want true", et)
		}
		if !c.SupporterFamily {
			t.Errorf("CapabilityFor(%s).SupporterFamily = false, want true", et)
		}
	}
}

func TestCapabilityForChatMessageIsNotSupporterFamily(t *testing.T) {
	c := CapabilityFor(engagement.TypeChatMessage)
	if !c.Speakable {
		t.Error("CapabilityFor(chat.message).Speakable = false, want true")
	}
	if c.SupporterFamily {
		t.Error("CapabilityFor(chat.message).SupporterFamily = true, want false")
	}
}

func TestCapabilityForUnspeakableTypes(t *testing.T) {
	unspeakable := []engagement.Type{
		engagement.TypeChatMessageDeleted,
		engagement.TypeChatCleared,
		engagement.TypeModeration,
		engagement.TypeStreamOnline,
		engagement.TypeStreamOffline,
	}
	for _, et := range unspeakable {
		if c := CapabilityFor(et); c.Speakable {
			t.Errorf("CapabilityFor(%s).Speakable = true, want false", et)
		}
	}
}

func TestCapabilityForUnknownTypeReturnsZeroValue(t *testing.T) {
	c := CapabilityFor(engagement.Type("some.future.type"))
	if c != (Capability{}) {
		t.Errorf("CapabilityFor(unknown) = %+v, want zero value", c)
	}
}
