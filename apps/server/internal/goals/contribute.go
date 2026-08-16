package goals

import (
	engagement "github.com/streaming-tree/server/internal/domain/engagement"
	domain "github.com/streaming-tree/server/internal/domain/goals"
)

// typeMapping maps every real internal/domain/engagement.Type this
// package recognizes to its own internal/domain/goals.Type counterpart
// (docs/goals-widgets.md §3) - the ONE place that translation happens.
// An engagement.Type absent here has no goals.Type at all and is
// therefore skipped before ContributionFor is ever consulted.
var typeMapping = map[engagement.Type]domain.Type{
	engagement.TypeFollow:                     domain.TypeFollow,
	engagement.TypeSubscription:               domain.TypeSubscription,
	engagement.TypeResubscription:             domain.TypeResubscription,
	engagement.TypeGiftedSubscription:         domain.TypeGiftedSubscription,
	engagement.TypeSubscriptionGiftBatch:      domain.TypeSubscriptionGiftBatch,
	engagement.TypeBits:                       domain.TypeBits,
	engagement.TypeYouTubeMembership:          domain.TypeYouTubeMembership,
	engagement.TypeYouTubeMembershipMilestone: domain.TypeYouTubeMembershipMilestone,
	engagement.TypeYouTubeSuperChat:           domain.TypeYouTubeSuperChat,
	engagement.TypeYouTubeSuperSticker:        domain.TypeYouTubeSuperSticker,
	engagement.TypeDonation:                   domain.TypeDonation,
}

// handleEvent applies evt to every currently enabled, matching goal
// (docs/goals-widgets.md §3-§15). Synthetic events are rejected first,
// before any lookup - defense-in-depth against a future code path that
// publishes one to the real Bus (§16), even though none does today.
func (m *Manager) handleEvent(evt engagement.Event) {
	if evt.Synthetic {
		return
	}

	goalType, ok := typeMapping[evt.Type]
	if !ok {
		return
	}
	contribution := domain.ContributionFor(goalType)
	if contribution == (domain.Contribution{}) {
		return
	}

	key, hasKey := dedupeIdentity(evt)
	if !hasKey {
		return
	}

	list, err := m.domainSvc.ListGoals(m.ctx)
	if err != nil {
		return
	}
	for _, g := range list {
		if !g.Enabled {
			continue
		}
		if !contribution.ContributesTo(g.Kind) {
			continue
		}
		if !providerMatches(g.Providers, evt.ProviderID) {
			continue
		}
		if !accountMatches(g.Accounts, evt.ConnectedAccountID) {
			continue
		}
		amount, ok := contributionAmount(g.Kind, evt)
		if !ok {
			continue
		}
		if g.Kind == domain.KindDonations && evt.Money != nil && evt.Money.Currency != g.Currency {
			continue
		}
		_, _, _ = m.domainSvc.ApplyContribution(m.ctx, g.ID, key, amount)
	}
}

// dedupeIdentity returns evt's durable dedupe identity: evt's own
// ProviderEventID when set, evt's DedupeKey otherwise (docs/goals-
// widgets.md §11.1-§11.2 - Twitch's own goal-relevant events never set
// ProviderEventID, only YouTube/StreamElements do; DedupeKey, the
// EventSub delivery id, is always present as the fallback). ok is false
// only when neither is set, which no real connector produces today.
func dedupeIdentity(evt engagement.Event) (domain.AppliedEventKey, bool) {
	id := evt.ProviderEventID
	if id == "" {
		id = evt.DedupeKey
	}
	if id == "" {
		return domain.AppliedEventKey{}, false
	}
	return domain.AppliedEventKey{
		ProviderID:       domain.ProviderID(evt.ProviderID),
		AccountID:        evt.ConnectedAccountID,
		ProviderEventKey: id,
	}, true
}

func providerMatches(filters []domain.ProviderID, p engagement.ProviderID) bool {
	if len(filters) == 0 {
		return true
	}
	for _, f := range filters {
		if f == domain.ProviderID(p) {
			return true
		}
	}
	return false
}

func accountMatches(filters []string, accountID string) bool {
	if len(filters) == 0 {
		return true
	}
	for _, f := range filters {
		if f == accountID {
			return true
		}
	}
	return false
}

// contributionAmount computes exactly how much evt contributes to a
// goal of kind k, or ok=false when evt does not actually carry the data
// that kind needs (e.g. a Bits goal against an event with no Quantity -
// never reached for a real, well-formed provider event, but never
// assumed either).
func contributionAmount(k domain.Kind, evt engagement.Event) (amount int64, ok bool) {
	switch k {
	case domain.KindFollowers, domain.KindSubscriptions:
		return 1, true
	case domain.KindBits:
		if evt.Quantity == nil {
			return 0, false
		}
		return *evt.Quantity, true
	case domain.KindDonations:
		if evt.Money == nil {
			return 0, false
		}
		return evt.Money.AmountMicros, true
	default:
		return 0, false
	}
}
