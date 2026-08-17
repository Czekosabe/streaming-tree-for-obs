package goals

// WidgetProfileKind is the closed set of widget-profile kinds Stage 18B
// supports (docs/supporter-widgets.md §5). WidgetProfileKindGoal is the
// only kind Stage 18A ever produced - every existing row defaults to it,
// unchanged in behavior.
type WidgetProfileKind string

const (
	WidgetProfileKindGoal             WidgetProfileKind = "goal"
	WidgetProfileKindLatestFollower   WidgetProfileKind = "latest_follower"
	WidgetProfileKindLatestSubscriber WidgetProfileKind = "latest_subscriber"
	WidgetProfileKindLatestDonation   WidgetProfileKind = "latest_donation"
	WidgetProfileKindLargestDonation  WidgetProfileKind = "largest_donation"
	WidgetProfileKindRecentSupporters WidgetProfileKind = "recent_supporters"
	WidgetProfileKindEventTicker      WidgetProfileKind = "event_ticker"
	WidgetProfileKindSessionCounter   WidgetProfileKind = "session_counter"
	WidgetProfileKindDashboard        WidgetProfileKind = "dashboard"
)

// ValidWidgetProfileKinds lists every accepted WidgetProfileKind.
var ValidWidgetProfileKinds = []WidgetProfileKind{
	WidgetProfileKindGoal, WidgetProfileKindLatestFollower, WidgetProfileKindLatestSubscriber,
	WidgetProfileKindLatestDonation, WidgetProfileKindLargestDonation, WidgetProfileKindRecentSupporters,
	WidgetProfileKindEventTicker, WidgetProfileKindSessionCounter, WidgetProfileKindDashboard,
}

func (k WidgetProfileKind) valid() bool {
	for _, v := range ValidWidgetProfileKinds {
		if v == k {
			return true
		}
	}
	return false
}

// RequiresGoal reports whether k presents exactly one persisted goal
// (docs/supporter-widgets.md §5) - the only kind Stage 18A ever had.
func (k WidgetProfileKind) RequiresGoal() bool { return k == WidgetProfileKindGoal }

// IsDashboard reports whether k is the composition kind (docs/
// supporter-widgets.md §9) - a dashboard has no filters, no goal, and no
// runtime projection of its own.
func (k WidgetProfileKind) IsDashboard() bool { return k == WidgetProfileKindDashboard }

// HasOwnFilters reports whether k reads Providers/Accounts directly off
// the WidgetProfile itself, rather than (for KindGoal) deferring to the
// referenced goal's own filters, or (for KindDashboard) having none.
func (k WidgetProfileKind) HasOwnFilters() bool {
	return !k.RequiresGoal() && !k.IsDashboard()
}

// RequiresCurrency reports whether k needs exactly one configured
// currency: largest_donation (comparison requires it) and
// session_counter when its own Metric is MetricSupportAmount (checked
// separately - see ValidateWidgetProfileFields).
func (k WidgetProfileKind) RequiresCurrency() bool {
	return k == WidgetProfileKindLargestDonation
}

// RequiresMaxItems reports whether k is a bounded list kind.
func (k WidgetProfileKind) RequiresMaxItems() bool {
	return k == WidgetProfileKindRecentSupporters || k == WidgetProfileKindEventTicker
}

// SessionMetric is the closed set of session_counter metrics (docs/
// supporter-widgets.md §13) - never an arbitrary formula/expression.
type SessionMetric string

const (
	MetricFollows             SessionMetric = "follows"
	MetricNewSubscriptions    SessionMetric = "new_subscriptions"
	MetricResubscriptions     SessionMetric = "resubscriptions"
	MetricGiftedSubscriptions SessionMetric = "gifted_subscriptions"
	MetricRaids               SessionMetric = "raids"
	MetricBitsQuantity        SessionMetric = "bits_quantity"
	MetricSupportEventCount   SessionMetric = "support_event_count"
	MetricSupportAmount       SessionMetric = "support_amount"
)

// ValidSessionMetrics lists every accepted SessionMetric.
var ValidSessionMetrics = []SessionMetric{
	MetricFollows, MetricNewSubscriptions, MetricResubscriptions, MetricGiftedSubscriptions,
	MetricRaids, MetricBitsQuantity, MetricSupportEventCount, MetricSupportAmount,
}

func (m SessionMetric) valid() bool {
	for _, v := range ValidSessionMetrics {
		if v == m {
			return true
		}
	}
	return false
}

// RequiresCurrency reports whether m needs a configured currency
// (docs/supporter-widgets.md §13 - only the exact-money metric does).
func (m SessionMetric) RequiresCurrency() bool { return m == MetricSupportAmount }

// SupporterEventType is event_ticker's own closed presentation-type
// allowlist (docs/supporter-widgets.md §8) - deliberately its own type,
// never engagement.Type directly, mirroring domain/goals.Type's own
// "no domain package imports engagement" rule.
type SupporterEventType string

const (
	EventTypeFollow                     SupporterEventType = "follow"
	EventTypeSubscription               SupporterEventType = "subscription"
	EventTypeResubscription             SupporterEventType = "resubscription"
	EventTypeGiftedSubscription         SupporterEventType = "gifted_subscription"
	EventTypeSubscriptionGiftBatch      SupporterEventType = "subscription_gift_batch"
	EventTypeBits                       SupporterEventType = "bits"
	EventTypeRaid                       SupporterEventType = "raid"
	EventTypeDonation                   SupporterEventType = "donation"
	EventTypeYouTubeSuperChat           SupporterEventType = "youtube_super_chat"
	EventTypeYouTubeSuperSticker        SupporterEventType = "youtube_super_sticker"
	EventTypeYouTubeMembership          SupporterEventType = "youtube_membership"
	EventTypeYouTubeMembershipMilestone SupporterEventType = "youtube_membership_milestone"
)

// ValidEventTicketTypes lists every SupporterEventType the event_ticker
// kind may allowlist (docs/supporter-widgets.md §8) - a type absent here
// can never appear in a ticker, no matter what a future engagement.Type
// exists.
var ValidEventTicketTypes = []SupporterEventType{
	EventTypeFollow, EventTypeSubscription, EventTypeResubscription, EventTypeGiftedSubscription,
	EventTypeSubscriptionGiftBatch, EventTypeBits, EventTypeRaid, EventTypeDonation,
	EventTypeYouTubeSuperChat, EventTypeYouTubeSuperSticker, EventTypeYouTubeMembership,
	EventTypeYouTubeMembershipMilestone,
}

func (t SupporterEventType) valid() bool {
	for _, v := range ValidEventTicketTypes {
		if v == t {
			return true
		}
	}
	return false
}

// DashboardChild is one bounded grid placement of an existing widget
// profile inside a dashboard profile (docs/supporter-widgets.md §9).
// References WidgetProfileID by id only - never copies the child's own
// state.
type DashboardChild struct {
	WidgetProfileID string
	Column          int
	ColumnSpan      int
	Row             int
	RowSpan         int
}

// Dashboard bounds (docs/supporter-widgets.md §9, governing task §25).
const (
	MinDashboardChildren = 1
	MaxDashboardChildren = 8
	MinDashboardColumns  = 1
	MaxDashboardColumns  = 4
)
