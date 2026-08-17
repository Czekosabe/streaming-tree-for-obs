package supporterwidgets

import (
	"crypto/rand"
	"encoding/hex"

	engagement "github.com/streaming-tree/server/internal/domain/engagement"
	domain "github.com/streaming-tree/server/internal/domain/goals"
)

// newItemID generates a fresh, unpredictable, uncorrelated presentation
// item id - never derived from providerEventId or any other provider
// identifier (docs/supporter-widgets.md §12), mirroring
// internal/audio.NewItemID's own reasoning. crypto/rand.Read does not
// fail in practice on any platform this project targets; the caller
// never needs to abort event processing over it.
func newItemID() string {
	buf := make([]byte, 12)
	_, _ = rand.Read(buf)
	return "supitem_" + hex.EncodeToString(buf)
}

// isNewSubscriber is the "New" half of the subscription-family decision
// (docs/supporter-widgets.md §6) - reused by latest_subscriber,
// recent_supporters, and session_counter's new_subscriptions metric.
func isNewSubscriber(t engagement.Type) bool {
	switch t {
	case engagement.TypeSubscription, engagement.TypeGiftedSubscription, engagement.TypeYouTubeMembership:
		return true
	default:
		return false
	}
}

// isDonationFamily is the real, distinct monetary/paid-support event
// family (docs/supporter-widgets.md §9) - reused by latest_donation,
// largest_donation, and session_counter's support_event_count/
// support_amount metrics.
func isDonationFamily(t engagement.Type) bool {
	switch t {
	case engagement.TypeDonation, engagement.TypeYouTubeSuperChat, engagement.TypeYouTubeSuperSticker:
		return true
	default:
		return false
	}
}

// isSupporterFamily is the closed recent_supporters family (docs/
// supporter-widgets.md §7) - deliberately includes TypeResubscription
// (ongoing support is still support) and excludes follows, chat,
// moderation, raids, and TypeSubscriptionGiftBatch (its own individual
// TypeGiftedSubscription recipients are counted instead, so the batch
// itself never appears here - the same no-double-count discipline
// Stage 18A's own goal contribution table already applies).
func isSupporterFamily(t engagement.Type) bool {
	switch t {
	case engagement.TypeSubscription, engagement.TypeGiftedSubscription, engagement.TypeYouTubeMembership,
		engagement.TypeResubscription, engagement.TypeBits, engagement.TypeDonation,
		engagement.TypeYouTubeSuperChat, engagement.TypeYouTubeSuperSticker:
		return true
	default:
		return false
	}
}

// tickerTypeMapping maps every real engagement.Type the closed
// event_ticker allowlist recognizes to its own domain.SupporterEventType
// (docs/supporter-widgets.md §8) - an engagement.Type absent here can
// never appear in any ticker, no matter what a profile's own EventTypes
// selects.
var tickerTypeMapping = map[engagement.Type]domain.SupporterEventType{
	engagement.TypeFollow:                     domain.EventTypeFollow,
	engagement.TypeSubscription:               domain.EventTypeSubscription,
	engagement.TypeResubscription:             domain.EventTypeResubscription,
	engagement.TypeGiftedSubscription:         domain.EventTypeGiftedSubscription,
	engagement.TypeSubscriptionGiftBatch:      domain.EventTypeSubscriptionGiftBatch,
	engagement.TypeBits:                       domain.EventTypeBits,
	engagement.TypeRaid:                       domain.EventTypeRaid,
	engagement.TypeDonation:                   domain.EventTypeDonation,
	engagement.TypeYouTubeSuperChat:           domain.EventTypeYouTubeSuperChat,
	engagement.TypeYouTubeSuperSticker:        domain.EventTypeYouTubeSuperSticker,
	engagement.TypeYouTubeMembership:          domain.EventTypeYouTubeMembership,
	engagement.TypeYouTubeMembershipMilestone: domain.EventTypeYouTubeMembershipMilestone,
}

// providerMatches/accountMatches mirror internal/goals's own identical,
// unexported helpers exactly (empty filter means "any") - duplicated
// rather than imported since they are unexported there, and this
// project's own convention (see every scripts/verify-*.mjs's duplicated
// fake-provider harness) already accepts small, stable duplication over
// a cross-package dependency for a five-line helper.
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

func containsEventType(list []domain.SupporterEventType, t domain.SupporterEventType) bool {
	for _, v := range list {
		if v == t {
			return true
		}
	}
	return false
}

// buildItem builds one presentation-safe SupporterItem from evt (docs/
// supporter-widgets.md §11) - never fabricates a placeholder for an
// unavailable field. showMessage gates whether evt's own message text
// (a donation/cheer comment - user-authored content) is ever included;
// every call site not explicitly passing true for a profile's own
// ShowMessage passes false, minimizing unnecessary public disclosure by
// default (docs/supporter-widgets.md §9, §12).
func buildItem(evt engagement.Event, showMessage bool) SupporterItem {
	item := SupporterItem{ItemID: newItemID(), Provider: string(evt.ProviderID), ObservedAt: evt.ReceivedAt}
	if evt.User != nil && !evt.User.Anonymous && evt.User.DisplayName != "" {
		item.DisplayName = evt.User.DisplayName
	}
	if evt.Money != nil {
		item.HasAmount = true
		item.AmountMicros = evt.Money.AmountMicros
		item.Currency = evt.Money.Currency
	}
	if evt.Quantity != nil {
		item.HasQuantity = true
		item.Quantity = *evt.Quantity
	}
	if showMessage && evt.Message != nil {
		item.Message = evt.Message.Text
	}
	return item
}

func prependSupporter(list []SupporterItem, item SupporterItem, maxItems int) []SupporterItem {
	list = append([]SupporterItem{item}, list...)
	if maxItems > 0 && len(list) > maxItems {
		list = list[:maxItems]
	}
	return list
}

func prependTicker(list []TickerItem, item TickerItem, maxItems int) []TickerItem {
	list = append([]TickerItem{item}, list...)
	if maxItems > 0 && len(list) > maxItems {
		list = list[:maxItems]
	}
	return list
}

// counterDelta computes how much evt contributes to a session_counter
// widget configured with metric/currency (docs/supporter-widgets.md
// §13), or ok=false when evt is not eligible for that exact metric.
func counterDelta(metric domain.SessionMetric, currency string, evt engagement.Event) (delta int64, ok bool) {
	switch metric {
	case domain.MetricFollows:
		if evt.Type == engagement.TypeFollow {
			return 1, true
		}
	case domain.MetricNewSubscriptions:
		if isNewSubscriber(evt.Type) {
			return 1, true
		}
	case domain.MetricResubscriptions:
		if evt.Type == engagement.TypeResubscription {
			return 1, true
		}
	case domain.MetricGiftedSubscriptions:
		if evt.Type == engagement.TypeGiftedSubscription {
			return 1, true
		}
	case domain.MetricRaids:
		if evt.Type == engagement.TypeRaid {
			return 1, true
		}
	case domain.MetricBitsQuantity:
		if evt.Type == engagement.TypeBits && evt.Quantity != nil {
			return *evt.Quantity, true
		}
	case domain.MetricSupportEventCount:
		if isDonationFamily(evt.Type) {
			return 1, true
		}
	case domain.MetricSupportAmount:
		if isDonationFamily(evt.Type) && evt.Money != nil && evt.Money.Currency == currency {
			return evt.Money.AmountMicros, true
		}
	}
	return 0, false
}

// applyEventToProjection updates proj in place for one widget profile p
// and one matching-filtered evt, dispatched entirely by p.Kind (docs/
// supporter-widgets.md §9) - reports whether proj actually changed, so
// the caller only bumps Revision/UpdatedAt on a genuine change.
func applyEventToProjection(p domain.WidgetProfile, evt engagement.Event, proj *Projection) bool {
	switch p.Kind {
	case domain.WidgetProfileKindLatestFollower:
		if evt.Type != engagement.TypeFollow {
			return false
		}
		item := buildItem(evt, false)
		proj.Latest = &item
		return true

	case domain.WidgetProfileKindLatestSubscriber:
		if !isNewSubscriber(evt.Type) {
			return false
		}
		item := buildItem(evt, false)
		proj.Latest = &item
		return true

	case domain.WidgetProfileKindLatestDonation:
		if !isDonationFamily(evt.Type) {
			return false
		}
		item := buildItem(evt, p.ShowMessage)
		proj.Latest = &item
		return true

	case domain.WidgetProfileKindLargestDonation:
		if !isDonationFamily(evt.Type) || evt.Money == nil || evt.Money.Currency != p.Currency {
			return false
		}
		if proj.Largest != nil && evt.Money.AmountMicros <= proj.Largest.AmountMicros {
			return false // a strictly larger amount replaces; equal does not (docs/supporter-widgets.md §9).
		}
		item := buildItem(evt, false) // largest_donation never shows a message (docs/supporter-widgets.md §9).
		proj.Largest = &item
		return true

	case domain.WidgetProfileKindRecentSupporters:
		if !isSupporterFamily(evt.Type) {
			return false
		}
		proj.Recent = prependSupporter(proj.Recent, buildItem(evt, p.ShowMessage), p.MaxItems)
		return true

	case domain.WidgetProfileKindEventTicker:
		eventType, ok := tickerTypeMapping[evt.Type]
		if !ok || !containsEventType(p.EventTypes, eventType) {
			return false
		}
		proj.Ticker = prependTicker(proj.Ticker, TickerItem{SupporterItem: buildItem(evt, p.ShowMessage), EventType: string(eventType)}, p.MaxItems)
		return true

	case domain.WidgetProfileKindSessionCounter:
		delta, ok := counterDelta(p.Metric, p.Currency, evt)
		if !ok {
			return false
		}
		proj.Counter += delta
		return true

	default:
		return false
	}
}
