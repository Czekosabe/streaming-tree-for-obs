package goals

// Contribution declares, for one Type, how (if at all) it contributes to
// each of the four Stage 18A goal kinds - the one, single place
// contribution rules are decided (docs/goals-widgets.md §3-§7). An
// unknown/future Type has no map entry and therefore contributes
// nothing (the zero Contribution) until this table is explicitly
// extended.
type Contribution struct {
	// Followers: contributes exactly 1 to an enabled follower goal.
	Followers bool
	// Subscriptions: contributes exactly 1 to an enabled subscription
	// goal.
	Subscriptions bool
	// Money: contributes the event's own exact AmountMicros to an
	// enabled donation goal, only when the event's currency matches the
	// goal's own configured currency exactly (docs/goals-widgets.md §6).
	Money bool
	// Bits: contributes the event's own exact Quantity to an enabled
	// Bits goal.
	Bits bool
}

// Contributions maps every Type this package recognizes to its real
// contribution. Built directly from docs/goals-widgets.md §3's own
// table, which was itself built by reading the real Twitch/YouTube/
// StreamElements normalization code - never the aspirational event list
// in docs/engagement-architecture.md.
//
// TypeResubscription and TypeYouTubeMembershipMilestone are deliberately
// absent (docs/goals-widgets.md §5.1: a continuing subscription, not a
// new one). TypeSubscriptionGiftBatch is deliberately absent (§5.2: its
// own individual TypeGiftedSubscription events are counted instead, so
// the batch summary and its own recipients are never both counted -
// this is the safe, no-double-count rule this milestone's own audit of
// docs/engagement-architecture.md §5.4 required).
var Contributions = map[Type]Contribution{
	TypeFollow:              {Followers: true},
	TypeSubscription:        {Subscriptions: true},
	TypeGiftedSubscription:  {Subscriptions: true},
	TypeBits:                {Bits: true},
	TypeYouTubeMembership:   {Subscriptions: true},
	TypeYouTubeSuperChat:    {Money: true},
	TypeYouTubeSuperSticker: {Money: true},
	TypeDonation:            {Money: true},
}

// ContributionFor returns t's contribution, or the zero Contribution
// (nothing) if t is not a recognized Type.
func ContributionFor(t Type) Contribution {
	return Contributions[t]
}

// ContributesTo reports whether c contributes anything at all to k -
// used by the runtime manager to skip a goal kind entirely rather than
// compute a zero amount.
func (c Contribution) ContributesTo(k Kind) bool {
	switch k {
	case KindFollowers:
		return c.Followers
	case KindSubscriptions:
		return c.Subscriptions
	case KindDonations:
		return c.Money
	case KindBits:
		return c.Bits
	default:
		return false
	}
}
