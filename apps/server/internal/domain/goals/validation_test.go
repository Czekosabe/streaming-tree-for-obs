package goals

import (
	"fmt"
	"testing"
)

func TestValidateGoalFieldsAcceptsEachKind(t *testing.T) {
	cases := []Goal{
		DefaultGoal("Followers", KindFollowers, 100),
		DefaultGoal("Subs", KindSubscriptions, 50),
		{Name: "Donations", Kind: KindDonations, Enabled: true, Target: 1000000, Currency: "USD"},
		DefaultGoal("Bits", KindBits, 10000),
	}
	for _, g := range cases {
		if err := ValidateGoalFields(g); err != nil {
			t.Errorf("kind %s: unexpected error: %v", g.Kind, err)
		}
	}
}

func TestValidateGoalFieldsRejectsUnknownKind(t *testing.T) {
	g := DefaultGoal("Mystery", Kind("mystery"), 10)
	if err := ValidateGoalFields(g); err == nil {
		t.Fatal("expected an error for an unknown kind")
	}
}

func TestValidateGoalFieldsRejectsZeroTarget(t *testing.T) {
	g := DefaultGoal("Followers", KindFollowers, 0)
	if err := ValidateGoalFields(g); err == nil {
		t.Fatal("expected an error for a zero target")
	}
}

func TestValidateGoalFieldsRejectsNegativeTarget(t *testing.T) {
	g := DefaultGoal("Followers", KindFollowers, -5)
	if err := ValidateGoalFields(g); err == nil {
		t.Fatal("expected an error for a negative target")
	}
}

func TestValidateGoalFieldsRejectsTargetAboveMax(t *testing.T) {
	g := DefaultGoal("Followers", KindFollowers, MaxGoalCountValue+1)
	if err := ValidateGoalFields(g); err == nil {
		t.Fatal("expected an error for a target above the max")
	}
}

func TestValidateGoalFieldsRejectsNegativeCurrent(t *testing.T) {
	g := DefaultGoal("Followers", KindFollowers, 100)
	g.Current = -1
	if err := ValidateGoalFields(g); err == nil {
		t.Fatal("expected an error for a negative current")
	}
}

func TestValidateGoalFieldsAllowsCurrentAboveTarget(t *testing.T) {
	g := DefaultGoal("Followers", KindFollowers, 100)
	g.Current = 150
	if err := ValidateGoalFields(g); err != nil {
		t.Errorf("current > target must be allowed (over-target retention): %v", err)
	}
}

func TestValidateGoalFieldsRequiresCurrencyForDonations(t *testing.T) {
	g := Goal{Name: "Fund", Kind: KindDonations, Enabled: true, Target: 100}
	if err := ValidateGoalFields(g); err == nil {
		t.Fatal("expected an error for a donation goal with no currency")
	}
}

func TestValidateGoalFieldsRejectsInvalidCurrencyCode(t *testing.T) {
	g := Goal{Name: "Fund", Kind: KindDonations, Enabled: true, Target: 100, Currency: "usd1"}
	if err := ValidateGoalFields(g); err == nil {
		t.Fatal("expected an error for a lowercase/malformed currency code")
	}
}

func TestValidateGoalFieldsRejectsCurrencyForNonMonetaryKind(t *testing.T) {
	g := DefaultGoal("Followers", KindFollowers, 100)
	g.Currency = "USD"
	if err := ValidateGoalFields(g); err == nil {
		t.Fatal("expected an error for a non-monetary goal carrying a currency")
	}
}

func TestValidateGoalFieldsRejectsEmptyName(t *testing.T) {
	g := DefaultGoal("", KindFollowers, 100)
	if err := ValidateGoalFields(g); err == nil {
		t.Fatal("expected an error for an empty name")
	}
}

func TestValidateGoalFieldsRejectsTooLongName(t *testing.T) {
	long := make([]rune, MaxNameCodePoints+1)
	for i := range long {
		long[i] = 'a'
	}
	g := DefaultGoal(string(long), KindFollowers, 100)
	if err := ValidateGoalFields(g); err == nil {
		t.Fatal("expected an error for a name over the bound")
	}
}

func TestValidateProvidersRejectsUnknownProvider(t *testing.T) {
	if err := ValidateProviders([]ProviderID{"kick"}); err == nil {
		t.Fatal("expected an error for an unsupported provider")
	}
}

func TestValidateProvidersRejectsDuplicateProvider(t *testing.T) {
	if err := ValidateProviders([]ProviderID{ProviderTwitch, ProviderTwitch}); err == nil {
		t.Fatal("expected an error for a duplicate provider entry")
	}
}

func TestValidateGoalFieldsRejectsDuplicateAccountFilter(t *testing.T) {
	g := DefaultGoal("Followers", KindFollowers, 100)
	g.Accounts = []string{"acc_1", "acc_1"}
	if err := ValidateGoalFields(g); err == nil {
		t.Fatal("expected an error for a duplicate account filter entry")
	}
}

func TestValidateWidgetProfileFieldsAcceptsDefaults(t *testing.T) {
	p := DefaultWidgetProfile("goal_1", "My Widget")
	if err := ValidateWidgetProfileFields(p); err != nil {
		t.Errorf("unexpected error for default widget profile: %v", err)
	}
}

func TestValidateWidgetProfileFieldsRejectsBadHexColor(t *testing.T) {
	p := DefaultWidgetProfile("goal_1", "My Widget")
	p.BackgroundColor = "purple"
	if err := ValidateWidgetProfileFields(p); err == nil {
		t.Fatal("expected an error for a non-hex color")
	}
}

func TestValidateWidgetProfileFieldsRejectsOutOfRangeOpacity(t *testing.T) {
	p := DefaultWidgetProfile("goal_1", "My Widget")
	p.Opacity = 1.5
	if err := ValidateWidgetProfileFields(p); err == nil {
		t.Fatal("expected an error for opacity above 1.0")
	}
}

func TestValidateWidgetProfileFieldsRejectsUnknownOrientation(t *testing.T) {
	p := DefaultWidgetProfile("goal_1", "My Widget")
	p.Orientation = "diagonal"
	if err := ValidateWidgetProfileFields(p); err == nil {
		t.Fatal("expected an error for an unsupported orientation")
	}
}

// --- Stage 18B kind-specific validation (docs/supporter-widgets.md §5) ---

func TestValidateWidgetProfileFieldsAcceptsEveryKindsOwnDefaults(t *testing.T) {
	for _, kind := range ValidWidgetProfileKinds {
		p := DefaultWidgetProfileOfKind(kind, "goal_1", "My Widget")
		if kind == WidgetProfileKindLargestDonation {
			p.Currency = "USD"
		}
		if kind == WidgetProfileKindSessionCounter {
			p.Metric = MetricFollows
		}
		if kind == WidgetProfileKindDashboard {
			p.Children = []DashboardChild{{WidgetProfileID: "widget_1", Column: 1, ColumnSpan: 1, Row: 1, RowSpan: 1}}
		}
		if err := ValidateWidgetProfileFields(p); err != nil {
			t.Errorf("kind %q: unexpected error: %v", kind, err)
		}
	}
}

func TestValidateWidgetProfileFieldsRejectsUnknownKind(t *testing.T) {
	p := DefaultWidgetProfile("goal_1", "My Widget")
	p.Kind = "nonsense"
	if err := ValidateWidgetProfileFields(p); err == nil {
		t.Fatal("expected an error for an unsupported kind")
	}
}

func TestValidateWidgetProfileFieldsRequiresGoalIDForGoalKind(t *testing.T) {
	p := DefaultWidgetProfile("", "My Widget")
	if err := ValidateWidgetProfileFields(p); err == nil {
		t.Fatal("expected an error for an empty goalId on a goal widget")
	}
}

func TestValidateWidgetProfileFieldsRejectsGoalIDForNonGoalKind(t *testing.T) {
	p := DefaultWidgetProfileOfKind(WidgetProfileKindLatestFollower, "", "My Widget")
	p.GoalID = "goal_1"
	if err := ValidateWidgetProfileFields(p); err == nil {
		t.Fatal("expected an error for a goalId on a non-goal widget")
	}
}

func TestValidateWidgetProfileFieldsRejectsFiltersForGoalKind(t *testing.T) {
	p := DefaultWidgetProfile("goal_1", "My Widget")
	p.Providers = []ProviderID{ProviderTwitch}
	if err := ValidateWidgetProfileFields(p); err == nil {
		t.Fatal("expected an error for provider filters on a goal widget")
	}
}

func TestValidateWidgetProfileFieldsAcceptsFiltersForLatestFollower(t *testing.T) {
	p := DefaultWidgetProfileOfKind(WidgetProfileKindLatestFollower, "", "My Widget")
	p.Providers = []ProviderID{ProviderTwitch}
	p.Accounts = []string{"acct_1"}
	if err := ValidateWidgetProfileFields(p); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateWidgetProfileFieldsRejectsMaxItemsOutOfRange(t *testing.T) {
	p := DefaultWidgetProfileOfKind(WidgetProfileKindRecentSupporters, "", "My Widget")
	p.MaxItems = MaxRecentSupporters + 1
	if err := ValidateWidgetProfileFields(p); err == nil {
		t.Fatal("expected an error for maxItems above the recent_supporters bound")
	}
}

func TestValidateWidgetProfileFieldsEventTickerAllowsUpToItsOwnHigherBound(t *testing.T) {
	p := DefaultWidgetProfileOfKind(WidgetProfileKindEventTicker, "", "My Widget")
	p.MaxItems = MaxEventTickerItems
	if err := ValidateWidgetProfileFields(p); err != nil {
		t.Errorf("unexpected error at the event_ticker bound: %v", err)
	}
	p.MaxItems = MaxRecentSupporters + 1
	if err := ValidateWidgetProfileFields(p); err != nil {
		t.Errorf("event_ticker's own bound (50) must allow more than recent_supporters' bound (20): %v", err)
	}
}

func TestValidateWidgetProfileFieldsRejectsMaxItemsForKindWithoutOne(t *testing.T) {
	p := DefaultWidgetProfileOfKind(WidgetProfileKindLatestFollower, "", "My Widget")
	p.MaxItems = 5
	if err := ValidateWidgetProfileFields(p); err == nil {
		t.Fatal("expected an error for maxItems on a kind with no bounded list")
	}
}

func TestValidateWidgetProfileFieldsLargestDonationRequiresCurrency(t *testing.T) {
	p := DefaultWidgetProfileOfKind(WidgetProfileKindLargestDonation, "", "My Widget")
	if err := ValidateWidgetProfileFields(p); err == nil {
		t.Fatal("expected an error for largest_donation with no currency")
	}
	p.Currency = "USD"
	if err := ValidateWidgetProfileFields(p); err != nil {
		t.Errorf("unexpected error once currency is set: %v", err)
	}
}

func TestValidateWidgetProfileFieldsRejectsCurrencyForKindThatDoesNotNeedIt(t *testing.T) {
	p := DefaultWidgetProfileOfKind(WidgetProfileKindLatestDonation, "", "My Widget")
	p.Currency = "USD"
	if err := ValidateWidgetProfileFields(p); err == nil {
		t.Fatal("expected an error for a currency on latest_donation (any currency is shown as-is)")
	}
}

func TestValidateWidgetProfileFieldsSessionCounterRequiresMetric(t *testing.T) {
	p := DefaultWidgetProfileOfKind(WidgetProfileKindSessionCounter, "", "My Widget")
	if err := ValidateWidgetProfileFields(p); err == nil {
		t.Fatal("expected an error for session_counter with no metric")
	}
}

func TestValidateWidgetProfileFieldsSupportAmountMetricRequiresCurrency(t *testing.T) {
	p := DefaultWidgetProfileOfKind(WidgetProfileKindSessionCounter, "", "My Widget")
	p.Metric = MetricSupportAmount
	if err := ValidateWidgetProfileFields(p); err == nil {
		t.Fatal("expected an error for support_amount with no currency")
	}
	p.Currency = "EUR"
	if err := ValidateWidgetProfileFields(p); err != nil {
		t.Errorf("unexpected error once currency is set: %v", err)
	}
}

func TestValidateWidgetProfileFieldsNonMoneyMetricRejectsCurrency(t *testing.T) {
	p := DefaultWidgetProfileOfKind(WidgetProfileKindSessionCounter, "", "My Widget")
	p.Metric = MetricFollows
	p.Currency = "USD"
	if err := ValidateWidgetProfileFields(p); err == nil {
		t.Fatal("expected an error for a currency on the follows metric")
	}
}

func TestValidateWidgetProfileFieldsEventTickerRejectsUnknownEventType(t *testing.T) {
	p := DefaultWidgetProfileOfKind(WidgetProfileKindEventTicker, "", "My Widget")
	p.EventTypes = []SupporterEventType{"chat_message"}
	if err := ValidateWidgetProfileFields(p); err == nil {
		t.Fatal("expected an error for an event type outside the closed allowlist")
	}
}

func TestValidateWidgetProfileFieldsEventTickerRejectsDuplicateEventType(t *testing.T) {
	p := DefaultWidgetProfileOfKind(WidgetProfileKindEventTicker, "", "My Widget")
	p.EventTypes = []SupporterEventType{EventTypeFollow, EventTypeFollow}
	if err := ValidateWidgetProfileFields(p); err == nil {
		t.Fatal("expected an error for a duplicated event type")
	}
}

func TestValidateWidgetProfileFieldsDashboardRequiresAtLeastOneChild(t *testing.T) {
	p := DefaultWidgetProfileOfKind(WidgetProfileKindDashboard, "", "My Dashboard")
	if err := ValidateWidgetProfileFields(p); err == nil {
		t.Fatal("expected an error for a dashboard with zero children")
	}
}

func TestValidateWidgetProfileFieldsDashboardRejectsTooManyChildren(t *testing.T) {
	p := DefaultWidgetProfileOfKind(WidgetProfileKindDashboard, "", "My Dashboard")
	for i := 0; i < MaxDashboardChildren+1; i++ {
		p.Children = append(p.Children, DashboardChild{WidgetProfileID: fmt.Sprintf("widget_%d", i), Column: 1, ColumnSpan: 1, Row: 1, RowSpan: 1})
	}
	if err := ValidateWidgetProfileFields(p); err == nil {
		t.Fatal("expected an error for more than the max dashboard children")
	}
}

func TestValidateWidgetProfileFieldsDashboardRejectsDuplicateChild(t *testing.T) {
	p := DefaultWidgetProfileOfKind(WidgetProfileKindDashboard, "", "My Dashboard")
	p.Children = []DashboardChild{
		{WidgetProfileID: "widget_1", Column: 1, ColumnSpan: 1, Row: 1, RowSpan: 1},
		{WidgetProfileID: "widget_1", Column: 2, ColumnSpan: 1, Row: 1, RowSpan: 1},
	}
	if err := ValidateWidgetProfileFields(p); err == nil {
		t.Fatal("expected an error for the same widget listed twice in one dashboard")
	}
}

func TestValidateWidgetProfileFieldsDashboardRejectsColumnSpanBeyondColumns(t *testing.T) {
	p := DefaultWidgetProfileOfKind(WidgetProfileKindDashboard, "", "My Dashboard")
	p.Columns = 2
	p.Children = []DashboardChild{{WidgetProfileID: "widget_1", Column: 2, ColumnSpan: 2, Row: 1, RowSpan: 1}}
	if err := ValidateWidgetProfileFields(p); err == nil {
		t.Fatal("expected an error for a child spanning past the dashboard's own column count")
	}
}

func TestValidateWidgetProfileFieldsRejectsColumnsForNonDashboard(t *testing.T) {
	p := DefaultWidgetProfileOfKind(WidgetProfileKindLatestFollower, "", "My Widget")
	p.Columns = 2
	if err := ValidateWidgetProfileFields(p); err == nil {
		t.Fatal("expected an error for columns on a non-dashboard widget")
	}
}

func TestProgressBasisPointsClampFreeAboveTarget(t *testing.T) {
	g := DefaultGoal("Followers", KindFollowers, 100)
	g.Current = 150
	if got := g.ProgressBasisPoints(); got != 15000 {
		t.Errorf("ProgressBasisPoints() = %d, want 15000 (over-target, unclamped)", got)
	}
}

func TestProgressBasisPointsHalfway(t *testing.T) {
	g := DefaultGoal("Followers", KindFollowers, 100)
	g.Current = 50
	if got := g.ProgressBasisPoints(); got != 5000 {
		t.Errorf("ProgressBasisPoints() = %d, want 5000", got)
	}
}

func TestGoalCompleted(t *testing.T) {
	g := DefaultGoal("Followers", KindFollowers, 100)
	g.Current = 100
	if !g.Completed() {
		t.Error("Completed() = false, want true when current == target")
	}
	g.Current = 99
	if g.Completed() {
		t.Error("Completed() = true, want false when current < target")
	}
}

func TestContributionForUnknownTypeIsZero(t *testing.T) {
	c := ContributionFor(Type("chat.message"))
	if c.Followers || c.Subscriptions || c.Money || c.Bits {
		t.Errorf("ContributionFor(unknown) = %+v, want the zero Contribution", c)
	}
}

func TestContributionForExcludesResubscriptionAndMilestone(t *testing.T) {
	if ContributionFor(TypeResubscription).Subscriptions {
		t.Error("resubscription must not contribute to a subscription goal (docs/goals-widgets.md §5.1)")
	}
	if ContributionFor(TypeYouTubeMembershipMilestone).Subscriptions {
		t.Error("membership milestone must not contribute to a subscription goal (docs/goals-widgets.md §5.1)")
	}
}

func TestContributionForExcludesGiftBatchButIncludesIndividualGift(t *testing.T) {
	if ContributionFor(TypeSubscriptionGiftBatch).Subscriptions {
		t.Error("subscription_gift_batch must not contribute - would double-count with individual gifted_subscription events (docs/goals-widgets.md §5.2)")
	}
	if !ContributionFor(TypeGiftedSubscription).Subscriptions {
		t.Error("gifted_subscription must contribute exactly 1 per recipient (docs/goals-widgets.md §5.2)")
	}
}

func TestContributesToMapsKindCorrectly(t *testing.T) {
	c := Contribution{Money: true}
	if !c.ContributesTo(KindDonations) {
		t.Error("Money contribution must map to KindDonations")
	}
	if c.ContributesTo(KindBits) {
		t.Error("Money contribution must not map to KindBits")
	}
}
