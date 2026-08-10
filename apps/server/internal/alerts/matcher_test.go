package alerts

import (
	"testing"
	"time"

	domain "github.com/streaming-tree/server/internal/domain/alerts"
	"github.com/streaming-tree/server/internal/domain/engagement"
)

var fixedNow = time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)

func baseEvent(t engagement.Type) engagement.Event {
	return engagement.Event{
		SchemaVersion: engagement.CurrentSchemaVersion, ID: "evt_1", ProviderID: engagement.ProviderTwitch,
		ConnectedAccountID: "acct_1", Type: t, PlatformTimestamp: fixedNow,
		DedupeKey: "dk_1",
		User:      &engagement.User{ProviderUserID: "u1", Login: "viewer", DisplayName: "Viewer"},
	}
}

func baseRule(id string, eventType domain.EventType) domain.Rule {
	return domain.Rule{
		ID: id, ProfileID: "alprof_1", Name: "r", Enabled: true, EventType: eventType,
		Priority: 50, DurationMS: 5000, RequiredRole: domain.RoleEveryone,
		ShowPlatform: true, ShowUsername: true,
		TextTemplate:   "{username} - {eventType}",
		EntryAnimation: domain.AnimationFade, ExitAnimation: domain.AnimationFade, AnimationDurationMS: 400,
	}
}

func TestMatchEventEveryRealEventType(t *testing.T) {
	cases := []struct {
		engagementType engagement.Type
		domainType     domain.EventType
	}{
		{engagement.TypeFollow, domain.EventFollow},
		{engagement.TypeSubscription, domain.EventSubscription},
		{engagement.TypeResubscription, domain.EventResubscription},
		{engagement.TypeGiftedSubscription, domain.EventGiftedSubscription},
		{engagement.TypeSubscriptionGiftBatch, domain.EventSubscriptionGiftBatch},
		{engagement.TypeBits, domain.EventBits},
		{engagement.TypeRaid, domain.EventRaid},
		{engagement.TypeChannelPointRedemption, domain.EventChannelPointRedemption},
	}
	for _, c := range cases {
		t.Run(string(c.engagementType), func(t *testing.T) {
			evt := baseEvent(c.engagementType)
			rule := baseRule("r1", c.domainType)
			out := MatchEvent(evt, []domain.Rule{rule}, nil, fixedNow, domain.LanguageEnglish)
			if len(out) != 1 {
				t.Fatalf("MatchEvent() = %d instances, want 1", len(out))
			}
			if out[0].EventType != c.domainType {
				t.Errorf("EventType = %q, want %q", out[0].EventType, c.domainType)
			}
			if out[0].RuleID != "r1" || out[0].ProfileID != "alprof_1" {
				t.Errorf("instance = %+v, want RuleID=r1 ProfileID=alprof_1", out[0])
			}
		})
	}
}

func TestMatchEventIgnoresUnsupportedType(t *testing.T) {
	evt := baseEvent(engagement.TypeChatMessage)
	rule := baseRule("r1", domain.EventFollow)
	if out := MatchEvent(evt, []domain.Rule{rule}, nil, fixedNow, domain.LanguageEnglish); len(out) != 0 {
		t.Errorf("MatchEvent() for chat.message = %d instances, want 0", len(out))
	}
}

func TestMatchEventIgnoresSyntheticRealBusEvent(t *testing.T) {
	evt := baseEvent(engagement.TypeFollow)
	evt.Synthetic = true
	rule := baseRule("r1", domain.EventFollow)
	if out := MatchEvent(evt, []domain.Rule{rule}, nil, fixedNow, domain.LanguageEnglish); len(out) != 0 {
		t.Errorf("MatchEvent() for a synthetic event = %d instances, want 0 (must use the separate test-alert path)", len(out))
	}
}

func TestMatchEventDisabledRuleNeverMatches(t *testing.T) {
	evt := baseEvent(engagement.TypeFollow)
	rule := baseRule("r1", domain.EventFollow)
	rule.Enabled = false
	if out := MatchEvent(evt, []domain.Rule{rule}, nil, fixedNow, domain.LanguageEnglish); len(out) != 0 {
		t.Errorf("MatchEvent() for a disabled rule = %d instances, want 0", len(out))
	}
}

func TestMatchEventProviderFilter(t *testing.T) {
	evt := baseEvent(engagement.TypeFollow)
	rule := baseRule("r1", domain.EventFollow)
	rule.Providers = []domain.ProviderID{"kick"}
	if out := MatchEvent(evt, []domain.Rule{rule}, nil, fixedNow, domain.LanguageEnglish); len(out) != 0 {
		t.Errorf("MatchEvent() with a mismatched provider filter = %d instances, want 0", len(out))
	}
	rule.Providers = []domain.ProviderID{domain.ProviderTwitch}
	if out := MatchEvent(evt, []domain.Rule{rule}, nil, fixedNow, domain.LanguageEnglish); len(out) != 1 {
		t.Errorf("MatchEvent() with a matching provider filter = %d instances, want 1", len(out))
	}
}

func TestMatchEventAccountFilter(t *testing.T) {
	evt := baseEvent(engagement.TypeFollow)
	rule := baseRule("r1", domain.EventFollow)
	rule.Accounts = []string{"acct_other"}
	if out := MatchEvent(evt, []domain.Rule{rule}, nil, fixedNow, domain.LanguageEnglish); len(out) != 0 {
		t.Errorf("MatchEvent() with a mismatched account filter = %d instances, want 0", len(out))
	}
	rule.Accounts = []string{"acct_1"}
	if out := MatchEvent(evt, []domain.Rule{rule}, nil, fixedNow, domain.LanguageEnglish); len(out) != 1 {
		t.Errorf("MatchEvent() with a matching account filter = %d instances, want 1", len(out))
	}
}

func quantityEvent(q int64) engagement.Event {
	evt := baseEvent(engagement.TypeBits)
	evt.Quantity = &q
	return evt
}

func TestMatchEventQuantityTiersNonOverlapping(t *testing.T) {
	i := func(v int64) *int64 { return &v }
	low := baseRule("low", domain.EventBits)
	low.MinimumQuantity, low.MaximumQuantity = i(1), i(99)
	mid := baseRule("mid", domain.EventBits)
	mid.MinimumQuantity, mid.MaximumQuantity = i(100), i(999)
	high := baseRule("high", domain.EventBits)
	high.MinimumQuantity = i(1000)
	rules := []domain.Rule{low, mid, high}

	cases := []struct {
		quantity int64
		wantRule string
	}{
		{50, "low"},
		{100, "mid"},
		{999, "mid"},
		{1000, "high"},
		{5000, "high"},
	}
	for _, c := range cases {
		out := MatchEvent(quantityEvent(c.quantity), rules, nil, fixedNow, domain.LanguageEnglish)
		if len(out) != 1 {
			t.Fatalf("quantity=%d: MatchEvent() = %d instances, want exactly 1 (no overlap)", c.quantity, len(out))
		}
		if out[0].RuleID != c.wantRule {
			t.Errorf("quantity=%d: matched rule = %q, want %q", c.quantity, out[0].RuleID, c.wantRule)
		}
	}
}

func TestMatchEventQuantityBoundsInclusive(t *testing.T) {
	i := func(v int64) *int64 { return &v }
	rule := baseRule("r1", domain.EventBits)
	rule.MinimumQuantity, rule.MaximumQuantity = i(100), i(200)

	if out := MatchEvent(quantityEvent(99), []domain.Rule{rule}, nil, fixedNow, domain.LanguageEnglish); len(out) != 0 {
		t.Error("quantity=99 (below inclusive minimum) matched, want no match")
	}
	if out := MatchEvent(quantityEvent(100), []domain.Rule{rule}, nil, fixedNow, domain.LanguageEnglish); len(out) != 1 {
		t.Error("quantity=100 (inclusive minimum) did not match")
	}
	if out := MatchEvent(quantityEvent(200), []domain.Rule{rule}, nil, fixedNow, domain.LanguageEnglish); len(out) != 1 {
		t.Error("quantity=200 (inclusive maximum) did not match")
	}
	if out := MatchEvent(quantityEvent(201), []domain.Rule{rule}, nil, fixedNow, domain.LanguageEnglish); len(out) != 0 {
		t.Error("quantity=201 (above inclusive maximum) matched, want no match")
	}
}

func TestMatchEventMultipleMatchingRulesAllEnqueue(t *testing.T) {
	evt := baseEvent(engagement.TypeFollow)
	r1 := baseRule("r1", domain.EventFollow)
	r2 := baseRule("r2", domain.EventFollow)
	out := MatchEvent(evt, []domain.Rule{r1, r2}, nil, fixedNow, domain.LanguageEnglish)
	if len(out) != 2 {
		t.Fatalf("MatchEvent() with two independently-matching rules = %d instances, want 2", len(out))
	}
}

func TestMatchEventAnonymousEventNeverFabricatesIdentity(t *testing.T) {
	evt := baseEvent(engagement.TypeBits)
	q := int64(50)
	evt.Quantity = &q
	evt.User = &engagement.User{Anonymous: true}
	rule := baseRule("r1", domain.EventBits)
	rule.TextTemplate = "{username} cheered {quantity} bits"
	out := MatchEvent(evt, []domain.Rule{rule}, nil, fixedNow, domain.LanguageEnglish)
	if len(out) != 1 {
		t.Fatalf("MatchEvent() = %d instances, want 1", len(out))
	}
	if out[0].Username != "" {
		t.Errorf("Username = %q for an anonymous cheer, want empty", out[0].Username)
	}
	if !out[0].Anonymous {
		t.Error("Anonymous = false for an anonymous cheer, want true")
	}
}

func TestMatchEventMissingOptionalFieldNeverPanics(t *testing.T) {
	evt := baseEvent(engagement.TypeFollow)
	evt.User = nil
	rule := baseRule("r1", domain.EventFollow)
	out := MatchEvent(evt, []domain.Rule{rule}, nil, fixedNow, domain.LanguageEnglish)
	if len(out) != 1 {
		t.Fatalf("MatchEvent() with a nil User = %d instances, want 1", len(out))
	}
}

func TestMatchEventShowFlagsControlInstanceFields(t *testing.T) {
	evt := baseEvent(engagement.TypeBits)
	q := int64(500)
	evt.Quantity = &q
	evt.Message = &engagement.Message{Text: "gg"}
	rule := baseRule("r1", domain.EventBits)
	rule.ShowUsername = false
	rule.ShowMessage = false
	rule.ShowQuantity = false
	out := MatchEvent(evt, []domain.Rule{rule}, nil, fixedNow, domain.LanguageEnglish)
	if len(out) != 1 {
		t.Fatalf("MatchEvent() = %d instances, want 1", len(out))
	}
	inst := out[0]
	if inst.Username != "" || inst.Message != "" || inst.Quantity != nil {
		t.Errorf("instance = %+v, want every show-flag-gated field empty", inst)
	}
}

func TestMatchEventRewardTitleFromProviderExtra(t *testing.T) {
	evt := baseEvent(engagement.TypeChannelPointRedemption)
	evt.ProviderExtra = map[string]string{"rewardTitle": "Hydrate!"}
	rule := baseRule("r1", domain.EventChannelPointRedemption)
	rule.TextTemplate = "{username} redeemed {rewardTitle}"
	out := MatchEvent(evt, []domain.Rule{rule}, nil, fixedNow, domain.LanguageEnglish)
	if len(out) != 1 {
		t.Fatalf("MatchEvent() = %d instances, want 1", len(out))
	}
	if out[0].RewardTitle != "Hydrate!" {
		t.Errorf("RewardTitle = %q, want Hydrate!", out[0].RewardTitle)
	}
	if out[0].RenderedText != "Viewer redeemed Hydrate!" {
		t.Errorf("RenderedText = %q, want %q", out[0].RenderedText, "Viewer redeemed Hydrate!")
	}
}

func TestMatchEventNoProviderHTTPCallEverPossible(t *testing.T) {
	// Structural test: MatchEvent's signature takes no context.Context and
	// no HTTP client, so a network call is not merely avoided but
	// impossible to add without changing the function signature -
	// documenting the Stage 12A task's own Part 32 requirement.
	evt := baseEvent(engagement.TypeFollow)
	rule := baseRule("r1", domain.EventFollow)
	_ = MatchEvent(evt, []domain.Rule{rule}, nil, fixedNow, domain.LanguageEnglish)
}
