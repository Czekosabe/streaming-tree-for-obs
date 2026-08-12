package alerts

import (
	"strings"
	"time"

	domain "github.com/streaming-tree/server/internal/domain/alerts"
	"github.com/streaming-tree/server/internal/domain/engagement"
	"github.com/streaming-tree/server/internal/domain/visualdesign"
)

// Scenario is a synthetic test-alert fixture name (Part 27). The 8
// event-shaped scenarios exactly name a real, currently-supported
// EventType; the remaining four are presentation-edge fixtures that
// apply on top of whichever event type the request is actually testing
// (a rule's own event type, or an explicit profile-level scenario).
const (
	ScenarioVeryLongUsername = "very_long_username"
	ScenarioVeryLongMessage  = "very_long_message"
	ScenarioAnonymous        = "anonymous_bits"
	ScenarioMissingAvatar    = "missing_avatar"
	// ScenarioNoComment previews a monetary event with no user comment -
	// Super Chat/Super Sticker both allow an empty comment for real
	// (Stage 15A task Part 44: "optional comment absent").
	ScenarioNoComment = "no_comment"
	// ScenarioAlternateCurrency previews a monetary event in a second
	// currency (EUR instead of the default USD fixture), so a template
	// author can see {currency} resolve to something other than the
	// default before ever receiving a real non-USD Super Chat.
	ScenarioAlternateCurrency = "alternate_currency"
)

// ScenarioEventType returns the EventType a bare scenario name (one of
// the 8 real types, given as its own string) maps to, or ok=false for a
// presentation-edge fixture name that has no single fixed event type
// (it applies on top of a rule's own type instead).
func ScenarioEventType(scenario string) (domain.EventType, bool) {
	t := domain.EventType(scenario)
	for _, v := range domain.ValidEventTypes {
		if v == t {
			return t, true
		}
	}
	return "", false
}

// buildFixtureEvent constructs one synthetic, in-memory
// engagement.Event for eventType, never published to the real
// Engagement Event Bus and never contacting Twitch - Part 27's own
// "generated locally... never published through the Twitch connector."
// edgeScenario, when non-empty and applicable to eventType's own
// capability, tweaks the fixture (a very long name, an anonymous actor,
// etc.) so an operator can preview how the fixed renderer handles that
// specific edge case.
func buildFixtureEvent(eventType domain.EventType, edgeScenario string, now time.Time) engagement.Event {
	engagementType := reverseMapEventType(eventType)
	evt := engagement.Event{
		SchemaVersion: engagement.CurrentSchemaVersion, ID: "synthetic",
		ProviderID: fixtureProviderID(eventType), ConnectedAccountID: "synthetic",
		Type: engagementType, PlatformTimestamp: now, DedupeKey: "synthetic",
		Synthetic: true,
	}

	capability := domain.CapabilityFor(eventType)
	username := "TestViewer"
	if edgeScenario == ScenarioVeryLongUsername {
		username = strings.Repeat("VeryLongTestViewerName", 12) // ~264 characters
	}
	if capability.HasUser {
		evt.User = &engagement.User{ProviderUserID: "synthetic_user", Login: strings.ToLower(username), DisplayName: username}
	}
	if capability.HasAnonymity && edgeScenario == ScenarioAnonymous {
		evt.User = &engagement.User{Anonymous: true}
	}

	if capability.HasMessage && edgeScenario != ScenarioNoComment {
		text := "This is a test alert message."
		if edgeScenario == ScenarioVeryLongMessage {
			text = strings.Repeat("This is a very long test alert message. ", 15) // well over the render limit
		}
		evt.Message = &engagement.Message{Text: text}
	}

	if capability.HasQuantity {
		q := fixtureQuantity(eventType)
		evt.Quantity = &q
	}

	if capability.HasRewardTitle {
		evt.ProviderExtra = map[string]string{"rewardTitle": "Test Reward"}
	}

	if capability.HasAmount {
		money := fixtureMoney(eventType, edgeScenario)
		evt.Money = &money
	}

	if capability.HasMembershipLevel {
		if evt.ProviderExtra == nil {
			evt.ProviderExtra = map[string]string{}
		}
		evt.ProviderExtra["memberLevelName"] = "Level 2"
	}

	return evt
}

// fixtureProviderID reports the realistic provider for a preview fixture -
// YouTube for the four YouTube-only event types (membership/milestone/
// Super Chat/Super Sticker), StreamElements for the generic donation
// event (Stage 16A - the only donation-source provider implemented),
// Twitch for everything else, including the two event types Stage 15A's
// own YouTube gift events reuse (a real gift from either provider
// previews identically, since the event type itself carries no provider -
// this default keeps every pre-Stage-15A preview fixture byte-for-byte
// unchanged).
func fixtureProviderID(eventType domain.EventType) engagement.ProviderID {
	switch eventType {
	case domain.EventYouTubeMembership, domain.EventYouTubeMembershipMilestone,
		domain.EventYouTubeSuperChat, domain.EventYouTubeSuperSticker:
		return engagement.ProviderYouTube
	case domain.EventDonation:
		return engagement.ProviderStreamElements
	default:
		return engagement.ProviderTwitch
	}
}

// fixtureMoney builds a representative Money value for a monetary preview
// - a plausible mid-tier amount, in USD by default or EUR under
// ScenarioAlternateCurrency (Stage 15A task Part 44's own "different
// currency examples").
func fixtureMoney(eventType domain.EventType, edgeScenario string) engagement.Money {
	currency := "USD"
	amountMicros := int64(5_000_000) // $5.00
	display := "$5.00"
	if edgeScenario == ScenarioAlternateCurrency {
		currency = "EUR"
		amountMicros = 10_000_000 // €10.00
		display = "€10.00"
	}
	if eventType == domain.EventYouTubeSuperSticker {
		amountMicros = 2_000_000 // a smaller, plausible sticker tier
		display = "$2.00"
		if edgeScenario == ScenarioAlternateCurrency {
			amountMicros = 4_000_000
			display = "€4.00"
		}
	}
	money, err := engagement.NewMoney(amountMicros, currency, display)
	if err != nil {
		// Unreachable for these fixed, valid fixture values - NewMoney
		// only rejects a negative/overflow amount or an empty currency.
		return engagement.Money{AmountMicros: amountMicros, Currency: currency, DisplayAmount: display}
	}
	return money
}

func fixtureQuantity(eventType domain.EventType) int64 {
	switch eventType {
	case domain.EventBits:
		return 250
	case domain.EventSubscriptionGiftBatch:
		return 5
	case domain.EventRaid:
		return 42
	case domain.EventYouTubeMembershipMilestone:
		return 6 // a representative member-month count
	default:
		return 1
	}
}

func reverseMapEventType(t domain.EventType) engagement.Type {
	switch t {
	case domain.EventFollow:
		return engagement.TypeFollow
	case domain.EventSubscription:
		return engagement.TypeSubscription
	case domain.EventResubscription:
		return engagement.TypeResubscription
	case domain.EventGiftedSubscription:
		return engagement.TypeGiftedSubscription
	case domain.EventSubscriptionGiftBatch:
		return engagement.TypeSubscriptionGiftBatch
	case domain.EventBits:
		return engagement.TypeBits
	case domain.EventRaid:
		return engagement.TypeRaid
	case domain.EventChannelPointRedemption:
		return engagement.TypeChannelPointRedemption
	case domain.EventYouTubeMembership:
		return engagement.TypeYouTubeMembership
	case domain.EventYouTubeMembershipMilestone:
		return engagement.TypeYouTubeMembershipMilestone
	case domain.EventYouTubeSuperChat:
		return engagement.TypeYouTubeSuperChat
	case domain.EventYouTubeSuperSticker:
		return engagement.TypeYouTubeSuperSticker
	case domain.EventDonation:
		return engagement.TypeDonation
	default:
		return ""
	}
}

// BuildTestInstance builds one synthetic Instance for rule, using
// edgeScenario (may be empty for the plain representative fixture of
// rule's own event type). Always marked Synthetic - never mistaken for
// a real match, and never itself published to the Engagement Event Bus
// (Part 27: "prefer a direct Alert Manager test path instead"). design
// is rule's own already-resolved visual-design snapshot (nil if none
// saved) - Test Rule always uses the rule's last SAVED design, never an
// unsaved frontend draft (Stage 13A task Part 40).
func BuildTestInstance(rule domain.Rule, edgeScenario string, now time.Time, lang domain.Language, design *visualdesign.PublicDocument) Instance {
	evt := buildFixtureEvent(rule.EventType, edgeScenario, now)
	return buildInstance(rule, evt, rule.EventType, now, lang, true, false, design)
}
