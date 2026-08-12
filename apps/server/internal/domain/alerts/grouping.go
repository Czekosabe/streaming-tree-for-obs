package alerts

// GroupingStrategy is the closed, internal set of algorithms Stage 12B may
// use to merge several queued alerts into one - never exposed to the
// frontend/API as a free-form code (Stage 12B task Part 8: "Do not expose
// arbitrary strategy code to the user"), only as the derived
// GroupingCapability.Groupable boolean the rule editor uses to show or
// hide the "Group similar alerts" control.
type GroupingStrategy int

const (
	// GroupingNone: this event type has no safe grouping strategy at all.
	GroupingNone GroupingStrategy = iota
	// GroupingSameActorQuantitySum: members must share the same actor
	// identity; the group's Quantity is the true, additive sum of every
	// member's own quantity (Bits amount, gift-sub count) - never a
	// fabricated total, never a currency conversion.
	GroupingSameActorQuantitySum
	// GroupingSameActorSameSubjectCount: members must share the same
	// actor identity AND the same stable subject (a channel-point
	// reward's own id, never its display title) - only GroupCount is
	// meaningful, there is no quantity to sum.
	GroupingSameActorSameSubjectCount
)

// GroupingCapability declares, for one EventType, whether and how Stage
// 12B may safely group several queued alerts of that type together. Built
// by reading this package's own real Capability table (capability.go,
// itself derived from the real Twitch normalization code) and the Stage
// 12B task's own explicit worked examples - never a generic "group
// everything" default. See docs/progress.md's Stage 12B grouping entry
// for the full per-type reasoning this table encodes.
type GroupingCapability struct {
	// Groupable: false means the rule editor hides/disables grouping for
	// this event type entirely, and the backend rejects AllowGrouping=true
	// on a rule of this type (ValidateRuleConditions).
	Groupable bool
	Strategy  GroupingStrategy
	// QuantitySummable: true only for GroupingSameActorQuantitySum - a
	// grouped instance's Quantity is a truthful running sum.
	QuantitySummable bool
	// RequiresNoMessage: this event type's Capability.HasMessage is true,
	// so a truthful grouped presentation requires ShowMessage=false AND a
	// template that never references {message} (Stage 12B task Part 11:
	// "never concatenate arbitrary messages, never choose one
	// arbitrarily") - enforced by ValidateRuleConditions plus
	// internal/alerts.ValidateGroupingTemplate for the template half.
	RequiresNoMessage bool
	// SubjectFromRewardID: true only for
	// GroupingSameActorSameSubjectCount - members must also share the
	// same stable reward id, sourced internally from the normalized
	// event's own ProviderExtra["rewardId"], never the (renamable)
	// reward title and never exposed publicly.
	SubjectFromRewardID bool
}

// GroupingCapabilities maps every valid EventType to its real grouping
// capability. Every entry is a deliberate, documented decision - not a
// default derived mechanically from Capabilities.
//
//   - EventFollow, EventSubscription: a real Twitch user can follow or
//     start a brand-new subscription at most once - there is no genuine
//     "the same actor did this again within the window" scenario for
//     either type to group, same-actor or otherwise.
//   - EventResubscription: HasMessage is true (the resub message), and a
//     real resubscription recurs at most monthly - never a real burst
//     within a bounded grouping window - so there is no safe subject to
//     aggregate and the message-bearing rule would forbid it anyway.
//   - EventGiftedSubscription: this is the *recipient* event - every
//     member would necessarily be a *different* person (the gifter is a
//     separate, distinct EventSubscriptionGiftBatch event entirely, see
//     SubjectFromRewardID's sibling rule below and
//     docs/progress.md), so grouping recipients together would present
//     several unrelated actors as one - exactly what Part 8's safety rule
//     forbids.
//   - EventSubscriptionGiftBatch: the Stage 12B task's own explicit
//     example - the *gifter* may genuinely purchase more than one gift
//     batch in quick succession; the group's Quantity (total subs
//     gifted) stays truthful regardless of tier mix, since "gifted N
//     subs" remains accurate whatever the tier breakdown was.
//   - EventBits: the Stage 12B task's own explicit example - repeated
//     cheers from the same actor may sum truthfully, conditioned on
//     RequiresNoMessage since Bits carries a real per-event message.
//   - EventRaid: the Stage 12B task's own explicit example - a real
//     raid is one broadcaster raiding once; summing viewer counts across
//     what would necessarily be *different* raiding broadcasters would
//     misrepresent several separate raids as one.
//   - EventChannelPointRedemption: a genuinely reusable reward can be
//     redeemed repeatedly by the same viewer in a burst - the Stage 12B
//     task's own explicit "any additional stable grouping subject such
//     as reward ID" case - conditioned on RequiresNoMessage since a
//     redemption carries real user-input text.
//   - EventYouTubeMembership, EventYouTubeMembershipMilestone: no
//     documented benefit to merging distinct membership announcements,
//     and a milestone carries a real per-event comment.
//   - EventYouTubeSuperChat, EventYouTubeSuperSticker: Stage 15A task's
//     own explicit conservative policy - a monetary event is never
//     automatically grouped. Summing amounts across a group would risk
//     silently merging different currencies or hiding an individually
//     urgent paid message inside an older queued group; not attempted
//     without a proven-safe design.
//   - EventDonation: Stage 16A task's own explicit instruction - each
//     real donation is individually meaningful and is never
//     auto-grouped, for the exact same reasoning as the two YouTube
//     monetary types above. A large donation can still jump the queue
//     through the existing, unrelated priority/interrupt mechanism -
//     no separate "big donation interrupt" feature was built.
var GroupingCapabilities = map[EventType]GroupingCapability{
	EventFollow:                 {Groupable: false},
	EventSubscription:           {Groupable: false},
	EventResubscription:         {Groupable: false},
	EventGiftedSubscription:     {Groupable: false},
	EventSubscriptionGiftBatch:  {Groupable: true, Strategy: GroupingSameActorQuantitySum, QuantitySummable: true},
	EventBits:                   {Groupable: true, Strategy: GroupingSameActorQuantitySum, QuantitySummable: true, RequiresNoMessage: true},
	EventRaid:                   {Groupable: false},
	EventChannelPointRedemption: {Groupable: true, Strategy: GroupingSameActorSameSubjectCount, RequiresNoMessage: true, SubjectFromRewardID: true},

	EventYouTubeMembership:          {Groupable: false},
	EventYouTubeMembershipMilestone: {Groupable: false},
	EventYouTubeSuperChat:           {Groupable: false},
	EventYouTubeSuperSticker:        {Groupable: false},

	EventDonation: {Groupable: false},
}

// GroupingCapabilityFor returns t's grouping capability, or the zero
// GroupingCapability (not groupable) for an unrecognized type.
func GroupingCapabilityFor(t EventType) GroupingCapability {
	return GroupingCapabilities[t]
}

// Bounds for a rule's grouping window (Stage 12B task Part 5: "use a
// bounded grouping window... a later member must NOT extend the window
// indefinitely"). Always validated regardless of AllowGrouping - see
// Rule's own doc comment for why a single unconditional bound was chosen
// over a cross-column SQLite CHECK.
const (
	MinGroupWindowMS     = 1000
	MaxGroupWindowMS     = 30000
	DefaultGroupWindowMS = 5000
)

// MaxGroupMembers bounds how many source events one grouped queued alert
// may ever represent (Stage 12B task Part 6). Once reached, a further
// compatible event starts a new candidate instead of continuing to
// increment this one.
const MaxGroupMembers = 1000
