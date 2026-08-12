package operatorchat

import engagement "github.com/streaming-tree/server/internal/domain/engagement"

// buildActivityItem converts a non-chat normalized engagement event
// (follow, subscription, resubscription, gifted subscription, subscription
// gift batch, bits, raid, channel-point redemption, or remote stream
// online/offline) into an activity item.
//
// The activity type is copied verbatim from the normalized event's own
// Type - never relabelled (Bits stay "bits", never "donation"; a gift
// batch stays "subscription_gift_batch", never collapsed into
// "gifted_subscription" - see internal/domain/engagement's own type
// vocabulary, which this function deliberately does not reinterpret).
func (p *Projection) buildActivityItem(evt engagement.Event) Item {
	activity := &Activity{
		ActivityType: string(evt.Type),
		Quantity:     evt.Quantity,
	}
	if evt.Money != nil {
		amount := evt.Money.AmountMicros
		activity.AmountMicros = &amount
		activity.Currency = evt.Money.Currency
		activity.DisplayAmount = evt.Money.DisplayAmount
	}
	item := Item{
		ID:                 newItemID("act"),
		SourceEventID:      evt.ID,
		ProviderID:         string(evt.ProviderID),
		ConnectedAccountID: evt.ConnectedAccountID,
		DestinationID:      p.resolveDestination(evt.ConnectedAccountID),
		Kind:               KindActivity,
		OccurredAt:         evt.PlatformTimestamp,
		ReceivedAt:         evt.ReceivedAt,
		Synthetic:          evt.Synthetic,
		Activity:           activity,
	}
	if evt.User != nil {
		item.User = toItemUser(evt.User)
	}
	return item
}
