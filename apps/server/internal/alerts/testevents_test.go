package alerts

import (
	"testing"

	domain "github.com/streaming-tree/server/internal/domain/alerts"
	"github.com/streaming-tree/server/internal/domain/engagement"
)

func TestReverseMapEventTypeYouTube(t *testing.T) {
	cases := []struct {
		domainType domain.EventType
		want       engagement.Type
	}{
		{domain.EventYouTubeMembership, engagement.TypeYouTubeMembership},
		{domain.EventYouTubeMembershipMilestone, engagement.TypeYouTubeMembershipMilestone},
		{domain.EventYouTubeSuperChat, engagement.TypeYouTubeSuperChat},
		{domain.EventYouTubeSuperSticker, engagement.TypeYouTubeSuperSticker},
	}
	for _, c := range cases {
		if got := reverseMapEventType(c.domainType); got != c.want {
			t.Errorf("reverseMapEventType(%s) = %q, want %q", c.domainType, got, c.want)
		}
	}
}

func TestBuildFixtureEventYouTubeProviderID(t *testing.T) {
	for _, et := range []domain.EventType{
		domain.EventYouTubeMembership, domain.EventYouTubeMembershipMilestone,
		domain.EventYouTubeSuperChat, domain.EventYouTubeSuperSticker,
	} {
		evt := buildFixtureEvent(et, "", fixedNow)
		if evt.ProviderID != engagement.ProviderYouTube {
			t.Errorf("buildFixtureEvent(%s).ProviderID = %q, want youtube", et, evt.ProviderID)
		}
	}
	// Every pre-Stage-15A fixture must still default to Twitch.
	evt := buildFixtureEvent(domain.EventFollow, "", fixedNow)
	if evt.ProviderID != engagement.ProviderTwitch {
		t.Errorf("buildFixtureEvent(follow).ProviderID = %q, want twitch", evt.ProviderID)
	}
}

func TestBuildFixtureEventSuperChatHasMoney(t *testing.T) {
	evt := buildFixtureEvent(domain.EventYouTubeSuperChat, "", fixedNow)
	if evt.Money == nil {
		t.Fatal("buildFixtureEvent(youtube_super_chat).Money = nil, want a populated Money value")
	}
	if evt.Money.Currency != "USD" {
		t.Errorf("Money.Currency = %q, want USD (default fixture currency)", evt.Money.Currency)
	}
	if evt.Money.AmountMicros <= 0 {
		t.Errorf("Money.AmountMicros = %d, want > 0", evt.Money.AmountMicros)
	}
}

func TestBuildFixtureEventAlternateCurrencyScenario(t *testing.T) {
	evt := buildFixtureEvent(domain.EventYouTubeSuperChat, ScenarioAlternateCurrency, fixedNow)
	if evt.Money == nil {
		t.Fatal("Money = nil, want populated")
	}
	if evt.Money.Currency != "EUR" {
		t.Errorf("Money.Currency under ScenarioAlternateCurrency = %q, want EUR", evt.Money.Currency)
	}
}

func TestBuildFixtureEventNoCommentScenarioSuppressesMessage(t *testing.T) {
	withComment := buildFixtureEvent(domain.EventYouTubeSuperChat, "", fixedNow)
	if withComment.Message == nil {
		t.Fatal("default Super Chat fixture has no Message, want one (capability.HasMessage=true)")
	}
	noComment := buildFixtureEvent(domain.EventYouTubeSuperChat, ScenarioNoComment, fixedNow)
	if noComment.Message != nil {
		t.Errorf("Message under ScenarioNoComment = %+v, want nil", noComment.Message)
	}
	// Money must still be present - only the comment is suppressed.
	if noComment.Money == nil {
		t.Error("Money under ScenarioNoComment = nil, want still populated")
	}
}

func TestBuildFixtureEventMembershipLevel(t *testing.T) {
	evt := buildFixtureEvent(domain.EventYouTubeMembership, "", fixedNow)
	if evt.ProviderExtra["memberLevelName"] == "" {
		t.Error("buildFixtureEvent(youtube_membership).ProviderExtra[memberLevelName] is empty, want a fixture level name")
	}
	if evt.Money != nil {
		t.Error("buildFixtureEvent(youtube_membership).Money is set, want nil (membership has no HasAmount capability)")
	}
}

func TestBuildFixtureEventMembershipMilestoneHasQuantity(t *testing.T) {
	evt := buildFixtureEvent(domain.EventYouTubeMembershipMilestone, "", fixedNow)
	if evt.Quantity == nil {
		t.Fatal("buildFixtureEvent(youtube_membership_milestone).Quantity = nil, want a representative month count")
	}
	if *evt.Quantity <= 0 {
		t.Errorf("Quantity = %d, want > 0", *evt.Quantity)
	}
	if evt.ProviderExtra["memberLevelName"] == "" {
		t.Error("milestone fixture missing memberLevelName")
	}
}

func TestBuildTestInstanceYouTubeEventTypes(t *testing.T) {
	for _, et := range []domain.EventType{
		domain.EventYouTubeMembership, domain.EventYouTubeMembershipMilestone,
		domain.EventYouTubeSuperChat, domain.EventYouTubeSuperSticker,
	} {
		rule := baseRule("r1", et)
		rule.ShowAmount = true
		inst := BuildTestInstance(rule, "", fixedNow, domain.LanguageEnglish, nil)
		if !inst.Synthetic {
			t.Errorf("BuildTestInstance(%s).Synthetic = false, want true", et)
		}
		if inst.EventType != et {
			t.Errorf("BuildTestInstance(%s).EventType = %q, want %q", et, inst.EventType, et)
		}
	}
}

func TestBuildTestInstanceSuperChatAmountScenarios(t *testing.T) {
	rule := baseRule("r1", domain.EventYouTubeSuperChat)
	rule.ShowAmount = true
	rule.TextTemplate = "{username} sent {amount} {currency}"

	usd := BuildTestInstance(rule, "", fixedNow, domain.LanguageEnglish, nil)
	if usd.Currency != "USD" {
		t.Errorf("default scenario Currency = %q, want USD", usd.Currency)
	}

	eur := BuildTestInstance(rule, ScenarioAlternateCurrency, fixedNow, domain.LanguageEnglish, nil)
	if eur.Currency != "EUR" {
		t.Errorf("ScenarioAlternateCurrency Currency = %q, want EUR", eur.Currency)
	}
	if usd.RenderedText == eur.RenderedText {
		t.Error("USD and EUR preview scenarios rendered identical text, want the currency to differ")
	}
}
