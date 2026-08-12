package alerts

import (
	"time"

	domain "github.com/streaming-tree/server/internal/domain/alerts"
	"github.com/streaming-tree/server/internal/domain/engagement"
	"github.com/streaming-tree/server/internal/domain/visualdesign"
)

// mapEventType converts a normalized engagement.Type to this
// application's own closed domain.EventType, or ok=false when t is not
// one of the 8 real, currently alert-capable Twitch event types (see
// docs/progress.md's Stage 12A persistence entry for exactly how that
// list was derived from the real Twitch normalization code, not the
// aspirational planning-doc list).
func mapEventType(t engagement.Type) (domain.EventType, bool) {
	switch t {
	case engagement.TypeFollow:
		return domain.EventFollow, true
	case engagement.TypeSubscription:
		return domain.EventSubscription, true
	case engagement.TypeResubscription:
		return domain.EventResubscription, true
	case engagement.TypeGiftedSubscription:
		return domain.EventGiftedSubscription, true
	case engagement.TypeSubscriptionGiftBatch:
		return domain.EventSubscriptionGiftBatch, true
	case engagement.TypeBits:
		return domain.EventBits, true
	case engagement.TypeRaid:
		return domain.EventRaid, true
	case engagement.TypeChannelPointRedemption:
		return domain.EventChannelPointRedemption, true
	case engagement.TypeYouTubeMembership:
		return domain.EventYouTubeMembership, true
	case engagement.TypeYouTubeMembershipMilestone:
		return domain.EventYouTubeMembershipMilestone, true
	case engagement.TypeYouTubeSuperChat:
		return domain.EventYouTubeSuperChat, true
	case engagement.TypeYouTubeSuperSticker:
		return domain.EventYouTubeSuperSticker, true
	default:
		return "", false
	}
}

func containsProvider(list []domain.ProviderID, p domain.ProviderID) bool {
	for _, v := range list {
		if v == p {
			return true
		}
	}
	return false
}

func containsAccount(list []string, id string) bool {
	for _, v := range list {
		if v == id {
			return true
		}
	}
	return false
}

func quantityInRange(q int64, min, max *int64) bool {
	if min != nil && q < *min {
		return false
	}
	if max != nil && q > *max {
		return false
	}
	return true
}

// amountInRange evaluates a rule's monetary condition against evt's real
// Money value (Stage 15A task Part 38): an event with no Money can never
// satisfy a money-threshold rule; a currency mismatch never matches
// either, since this application performs no currency conversion - two
// different currencies are simply never comparable, not "0 vs
// converted".
func amountInRange(money *engagement.Money, currency string, min, max *int64) bool {
	if money == nil {
		return false
	}
	if money.Currency != currency {
		return false
	}
	if min != nil && money.AmountMicros < *min {
		return false
	}
	if max != nil && money.AmountMicros > *max {
		return false
	}
	return true
}

// MatchEvent evaluates every enabled rule in rules (already scoped to
// one profile by the caller) against evt and returns one Instance per
// matching rule - Part 8's "simpler policy": every matching rule
// enqueues independently, never first-match-wins, never dependent on
// rule slice order (the caller passes rules in the repository's own
// created_at order, but MatchEvent itself never relies on that).
//
// Pure and side-effect free (Part 32: "do not perform network I/O
// during matching") - now is injected so callers can build a
// deterministic QueuedAt. designs is Stage 13A's own already-resolved
// rule-id -> saved-design snapshot cache (see internal/alerts.Manager's
// own profileRuntime.designs), passed in rather than fetched here so
// this function stays pure - a nil/missing entry for a rule simply
// means that rule has no saved design (Instance.VisualDesign stays
// nil, the legacy renderer applies).
func MatchEvent(evt engagement.Event, rules []domain.Rule, designs map[string]*visualdesign.PublicDocument, now time.Time, lang domain.Language) []Instance {
	// Part 11/32 step 1: a synthetic event must never enter the real
	// alert rule path as if genuine.
	if evt.Synthetic {
		return nil
	}
	eventType, ok := mapEventType(evt.Type)
	if !ok {
		return nil
	}

	var out []Instance
	for _, rule := range rules {
		if !rule.Enabled || rule.EventType != eventType {
			continue
		}
		if len(rule.Providers) > 0 && !containsProvider(rule.Providers, domain.ProviderID(evt.ProviderID)) {
			continue
		}
		if len(rule.Accounts) > 0 && !containsAccount(rule.Accounts, evt.ConnectedAccountID) {
			continue
		}
		// Defensive only: no Stage 12A event type ever supplies role
		// data (see capability.go), and rule validation already rejects
		// saving a non-"everyone" RequiredRole for any of them - so a
		// rule reaching this point with a different role can never
		// actually be satisfied.
		if rule.RequiredRole != domain.RoleEveryone {
			continue
		}
		var quantity *int64
		if evt.Quantity != nil {
			quantity = evt.Quantity
		}
		if rule.MinimumQuantity != nil || rule.MaximumQuantity != nil {
			if quantity == nil || !quantityInRange(*quantity, rule.MinimumQuantity, rule.MaximumQuantity) {
				continue
			}
		}
		if rule.MinimumAmountMicros != nil || rule.MaximumAmountMicros != nil {
			if !amountInRange(evt.Money, rule.Currency, rule.MinimumAmountMicros, rule.MaximumAmountMicros) {
				continue
			}
		}
		out = append(out, buildInstance(rule, evt, eventType, now, lang, false, false, designs[rule.ID]))
	}
	return out
}

// buildInstance renders one Instance from rule's own snapshot and evt's
// data - Part 9's "Policy A" (a queued alert never observes a later
// rule edit). design is rule's own already-resolved visual-design
// snapshot (nil if none saved) - copied onto the Instance verbatim,
// never re-fetched, so a later design save/delete can never mutate an
// already-built Instance (Stage 13A task Part 22).
func buildInstance(rule domain.Rule, evt engagement.Event, eventType domain.EventType, now time.Time, lang domain.Language, synthetic, replayed bool, design *visualdesign.PublicDocument) Instance {
	capability := domain.CapabilityFor(eventType)

	inst := Instance{
		ProfileID: rule.ProfileID, RuleID: rule.ID,
		SourceEventID: evt.ID, ProviderID: domain.ProviderID(evt.ProviderID),
		ConnectedAccountID: evt.ConnectedAccountID, EventType: eventType,
		QueuedAt: now, Priority: rule.Priority, DurationMS: rule.DurationMS,
		PlatformLabel:  PlatformDisplayName(domain.ProviderID(evt.ProviderID)),
		EntryAnimation: rule.EntryAnimation, ExitAnimation: rule.ExitAnimation, AnimationDurationMS: rule.AnimationDurationMS,
		GroupCount: 1, Synthetic: synthetic, Replayed: replayed,
		TextTemplate: rule.TextTemplate, Language: lang, RuleUpdatedAt: rule.UpdatedAt,
		AllowGrouping: rule.AllowGrouping, GroupWindowMS: rule.GroupWindowMS,
		InterruptMode: rule.InterruptMode, Interruptible: rule.Interruptible,
		VisualDesign: design,
	}

	var username *string
	if capability.HasUser && evt.User != nil {
		inst.Anonymous = evt.User.Anonymous
		if !evt.User.Anonymous {
			name := evt.User.DisplayName
			if name == "" {
				name = evt.User.Login
			}
			if rule.ShowUsername {
				inst.Username = name
			}
			username = &name
			inst.ActorDisplayName = name
			inst.ActorProviderUserID = evt.User.ProviderUserID
			inst.AvatarURL = evt.User.AvatarURL
		}
	}

	var messagePtr *string
	if capability.HasMessage && evt.Message != nil {
		text := evt.Message.Text
		if rule.ShowMessage {
			inst.Message = text
		}
		messagePtr = &text
	}

	if capability.HasQuantity && evt.Quantity != nil {
		q := *evt.Quantity
		if rule.ShowQuantity {
			inst.Quantity = &q
		}
	}

	var rewardTitlePtr *string
	if capability.HasRewardTitle {
		if title, ok := evt.ProviderExtra["rewardTitle"]; ok && title != "" {
			inst.RewardTitle = title
			rewardTitlePtr = &title
		}
		if id, ok := evt.ProviderExtra["rewardId"]; ok {
			inst.RewardID = id
		}
	}

	var amountDisplayForTemplate *string
	var currencyForTemplate string
	if capability.HasAmount && evt.Money != nil {
		amount := evt.Money.AmountMicros
		display := evt.Money.DisplayAmount
		if display == "" {
			display = FormatAmountMicros(amount)
		}
		amountDisplayForTemplate = &display
		currencyForTemplate = evt.Money.Currency
		if rule.ShowAmount {
			inst.AmountMicros = &amount
			inst.Currency = evt.Money.Currency
		}
	}

	var membershipLevelForTemplate string
	if capability.HasMembershipLevel {
		if level, ok := evt.ProviderExtra["memberLevelName"]; ok && level != "" {
			inst.MembershipLevel = level
			membershipLevelForTemplate = level
		}
	}

	var quantityForTemplate *int64
	if capability.HasQuantity && evt.Quantity != nil {
		q := *evt.Quantity
		quantityForTemplate = &q
	}

	ctx := Context{
		Username: username, Platform: inst.PlatformLabel, EventType: EventTypeLabel(eventType, lang),
		Quantity: quantityForTemplate, Message: messagePtr, RewardTitle: rewardTitlePtr,
		AmountDisplay: amountDisplayForTemplate, Currency: currencyForTemplate, MembershipLevel: membershipLevelForTemplate,
		GroupCount: inst.GroupCount,
	}
	if result, err := Render(rule.TextTemplate, ctx); err == nil {
		inst.RenderedText = result.Text
	}

	return inst
}
