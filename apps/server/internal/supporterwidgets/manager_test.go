package supporterwidgets

import (
	"context"
	"sync"
	"testing"
	"time"

	engagement "github.com/streaming-tree/server/internal/domain/engagement"
	domain "github.com/streaming-tree/server/internal/domain/goals"
	bus "github.com/streaming-tree/server/internal/engagement"
)

// fakeClock is a minimal, mutex-protected fake clock - mirrors
// internal/goals's own identical test helper.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}
func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// fakeLister is a minimal in-memory WidgetProfileLister - Manager only
// ever reads through this narrow interface, so a full domain.Repository
// double is unnecessary here (unlike internal/goals's own manager_test.go,
// which needs CreateGoal/ApplyContribution too).
type fakeLister struct {
	mu       sync.Mutex
	profiles map[string]domain.WidgetProfile
}

func newFakeLister() *fakeLister { return &fakeLister{profiles: map[string]domain.WidgetProfile{}} }

func (l *fakeLister) add(p domain.WidgetProfile) domain.WidgetProfile {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.profiles[p.ID] = p
	return p
}

func (l *fakeLister) ListWidgetProfiles(_ context.Context, _ string) ([]domain.WidgetProfile, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]domain.WidgetProfile, 0, len(l.profiles))
	for _, p := range l.profiles {
		out = append(out, p)
	}
	return out, nil
}

func newTestManager(t *testing.T, fc *fakeClock) (*Manager, *bus.Bus, *fakeLister) {
	t.Helper()
	lister := newFakeLister()
	b := bus.New(bus.Options{Now: fc.Now})
	mgr := NewManager(ManagerOptions{Profiles: lister, Bus: b, Now: fc.Now})
	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	waitUntil(t, time.Second, mgr.Subscribed)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = mgr.Shutdown(ctx)
		b.Shutdown()
	})
	return mgr, b, lister
}

func waitUntil(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition never became true within timeout")
}

func publish(t *testing.T, b *bus.Bus, evt engagement.Event) {
	t.Helper()
	if _, _, err := b.Publish(evt); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
}

func waitForRevision(t *testing.T, mgr *Manager, profileID string, minRevision uint64) Projection {
	t.Helper()
	var got Projection
	waitUntil(t, 5*time.Second, func() bool {
		got = mgr.Snapshot(profileID)
		return got.Revision >= minRevision
	})
	return got
}

func newProfile(id string, kind domain.WidgetProfileKind) domain.WidgetProfile {
	p := domain.DefaultWidgetProfileOfKind(kind, "", "Widget "+id)
	p.ID = id
	p.PublicSlug = "slug_" + id
	now := time.Now().UTC()
	p.CreatedAt, p.UpdatedAt = now, now
	return p
}

func followEvent(providerID engagement.ProviderID, accountID, dedupeKey, login string) engagement.Event {
	return engagement.Event{
		SchemaVersion: engagement.CurrentSchemaVersion, ProviderID: providerID,
		ConnectedAccountID: accountID, Type: engagement.TypeFollow, PlatformTimestamp: time.Now().UTC(),
		DedupeKey: dedupeKey, User: &engagement.User{ProviderUserID: "u_" + login, Login: login, DisplayName: login},
	}
}

func donationEvent(dedupeKey string, amountMicros int64, currency, displayName, message string) engagement.Event {
	evt := engagement.Event{
		SchemaVersion: engagement.CurrentSchemaVersion, ProviderID: engagement.ProviderStreamElements,
		ConnectedAccountID: "src_se", Type: engagement.TypeDonation, PlatformTimestamp: time.Now().UTC(), DedupeKey: dedupeKey,
		Money: &engagement.Money{AmountMicros: amountMicros, Currency: currency},
	}
	if displayName != "" {
		evt.User = &engagement.User{DisplayName: displayName}
	} else {
		evt.User = &engagement.User{Anonymous: true}
	}
	if message != "" {
		msg := engagement.NewMessage([]engagement.Fragment{{Type: engagement.FragmentText, Text: message}})
		evt.Message = &msg
	}
	return evt
}

// --- current-position subscription (docs/supporter-widgets.md §4) -------

func TestManagerNeverReplaysRetainedEvents(t *testing.T) {
	fc := newFakeClock()
	lister := newFakeLister()
	b := bus.New(bus.Options{Now: fc.Now})
	t.Cleanup(b.Shutdown)

	// Publish BEFORE the manager ever starts - retained in the ring.
	publish(t, b, followEvent(engagement.ProviderTwitch, "acct_1", "dk_before", "early"))

	p := lister.add(newProfile("widget_1", domain.WidgetProfileKindLatestFollower))
	mgr := NewManager(ManagerOptions{Profiles: lister, Bus: b, Now: fc.Now})
	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	waitUntil(t, time.Second, mgr.Subscribed)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = mgr.Shutdown(ctx)
	})

	// A genuinely new event after Start must be applied...
	publish(t, b, followEvent(engagement.ProviderTwitch, "acct_1", "dk_after", "late"))
	got := waitForRevision(t, mgr, p.ID, 1)
	if got.Latest == nil || got.Latest.DisplayName != "late" {
		t.Fatalf("Latest = %+v, want the post-Start follower only", got.Latest)
	}
}

// --- latest_follower (docs/supporter-widgets.md §9) ----------------------

func TestLatestFollowerUpdatesOnFollow(t *testing.T) {
	fc := newFakeClock()
	mgr, b, lister := newTestManager(t, fc)
	p := lister.add(newProfile("widget_1", domain.WidgetProfileKindLatestFollower))

	publish(t, b, followEvent(engagement.ProviderTwitch, "acct_1", "dk_1", "ada"))
	got := waitForRevision(t, mgr, p.ID, 1)
	if got.Latest == nil || got.Latest.DisplayName != "ada" || got.Latest.Provider != "twitch" {
		t.Fatalf("Latest = %+v, want ada/twitch", got.Latest)
	}
}

func TestLatestFollowerIgnoresNonFollowAndSynthetic(t *testing.T) {
	fc := newFakeClock()
	mgr, b, lister := newTestManager(t, fc)
	p := lister.add(newProfile("widget_1", domain.WidgetProfileKindLatestFollower))

	publish(t, b, donationEvent("dk_1", 1_000_000, "USD", "Ada", ""))
	synth := followEvent(engagement.ProviderTwitch, "acct_1", "dk_synth", "synthetic")
	synth.Synthetic = true
	publish(t, b, synth)
	real := followEvent(engagement.ProviderTwitch, "acct_1", "dk_real", "real")
	publish(t, b, real)

	got := waitForRevision(t, mgr, p.ID, 1)
	if got.Latest == nil || got.Latest.DisplayName != "real" {
		t.Fatalf("Latest = %+v, want only the real follow to have been applied", got.Latest)
	}
}

func TestLatestFollowerRespectsProviderFilter(t *testing.T) {
	fc := newFakeClock()
	mgr, b, lister := newTestManager(t, fc)
	p := newProfile("widget_1", domain.WidgetProfileKindLatestFollower)
	p.Providers = []domain.ProviderID{domain.ProviderYouTube}
	lister.add(p)

	publish(t, b, followEvent(engagement.ProviderTwitch, "acct_1", "dk_tw", "twitcher"))
	time.Sleep(50 * time.Millisecond)
	if got := mgr.Snapshot(p.ID); got.Latest != nil {
		t.Fatalf("Latest = %+v, want nil (Twitch filtered out)", got.Latest)
	}
}

// --- latest_subscriber: new only (docs/supporter-widgets.md §6) ----------

func TestLatestSubscriberAcceptsNewIgnoresContinuing(t *testing.T) {
	fc := newFakeClock()
	mgr, b, lister := newTestManager(t, fc)
	p := lister.add(newProfile("widget_1", domain.WidgetProfileKindLatestSubscriber))

	resub := engagement.Event{
		SchemaVersion: engagement.CurrentSchemaVersion, ProviderID: engagement.ProviderTwitch,
		ConnectedAccountID: "acct_1", Type: engagement.TypeResubscription, PlatformTimestamp: time.Now().UTC(), DedupeKey: "dk_resub",
		User: &engagement.User{DisplayName: "OldTimer"},
	}
	publish(t, b, resub)
	time.Sleep(50 * time.Millisecond)
	if got := mgr.Snapshot(p.ID); got.Latest != nil {
		t.Fatalf("Latest = %+v, want nil (resubscription is not a new subscriber)", got.Latest)
	}

	newSub := engagement.Event{
		SchemaVersion: engagement.CurrentSchemaVersion, ProviderID: engagement.ProviderTwitch,
		ConnectedAccountID: "acct_1", Type: engagement.TypeSubscription, PlatformTimestamp: time.Now().UTC(), DedupeKey: "dk_new",
		User: &engagement.User{DisplayName: "Fresh"},
	}
	publish(t, b, newSub)
	got := waitForRevision(t, mgr, p.ID, 1)
	if got.Latest == nil || got.Latest.DisplayName != "Fresh" {
		t.Fatalf("Latest = %+v, want Fresh", got.Latest)
	}
}

func TestLatestSubscriberAcceptsGiftedRecipientAndYouTubeMembership(t *testing.T) {
	fc := newFakeClock()
	mgr, b, lister := newTestManager(t, fc)
	p := lister.add(newProfile("widget_1", domain.WidgetProfileKindLatestSubscriber))

	gift := engagement.Event{
		SchemaVersion: engagement.CurrentSchemaVersion, ProviderID: engagement.ProviderTwitch,
		ConnectedAccountID: "acct_1", Type: engagement.TypeGiftedSubscription, PlatformTimestamp: time.Now().UTC(), DedupeKey: "dk_gift",
		User: &engagement.User{DisplayName: "Recipient"},
	}
	publish(t, b, gift)
	got := waitForRevision(t, mgr, p.ID, 1)
	if got.Latest == nil || got.Latest.DisplayName != "Recipient" {
		t.Fatalf("Latest = %+v, want Recipient", got.Latest)
	}

	membership := engagement.Event{
		SchemaVersion: engagement.CurrentSchemaVersion, ProviderID: engagement.ProviderYouTube,
		ConnectedAccountID: "acct_yt", Type: engagement.TypeYouTubeMembership, PlatformTimestamp: time.Now().UTC(), DedupeKey: "dk_member",
		User: &engagement.User{DisplayName: "NewMember"},
	}
	publish(t, b, membership)
	got2 := waitForRevision(t, mgr, p.ID, 2)
	if got2.Latest == nil || got2.Latest.DisplayName != "NewMember" {
		t.Fatalf("Latest = %+v, want NewMember", got2.Latest)
	}
}

func TestLatestSubscriberIgnoresGiftBatchSummary(t *testing.T) {
	fc := newFakeClock()
	mgr, b, lister := newTestManager(t, fc)
	p := lister.add(newProfile("widget_1", domain.WidgetProfileKindLatestSubscriber))

	qty := int64(5)
	batch := engagement.Event{
		SchemaVersion: engagement.CurrentSchemaVersion, ProviderID: engagement.ProviderTwitch,
		ConnectedAccountID: "acct_1", Type: engagement.TypeSubscriptionGiftBatch, PlatformTimestamp: time.Now().UTC(), DedupeKey: "dk_batch",
		Quantity: &qty, User: &engagement.User{DisplayName: "Gifter"},
	}
	publish(t, b, batch)
	time.Sleep(50 * time.Millisecond)
	if got := mgr.Snapshot(p.ID); got.Latest != nil {
		t.Fatalf("Latest = %+v, want nil (a gift batch summary is never the latest subscriber)", got.Latest)
	}
}

// --- latest_donation / largest_donation (docs/supporter-widgets.md §9) ---

func TestLatestDonationHidesMessageByDefault(t *testing.T) {
	fc := newFakeClock()
	mgr, b, lister := newTestManager(t, fc)
	p := lister.add(newProfile("widget_1", domain.WidgetProfileKindLatestDonation))

	publish(t, b, donationEvent("dk_1", 5_000_000, "USD", "Donor", "great stream!"))
	got := waitForRevision(t, mgr, p.ID, 1)
	if got.Latest == nil || got.Latest.Message != "" {
		t.Fatalf("Message = %q, want empty (showMessage defaults to false)", got.Latest.Message)
	}
	if got.Latest.AmountMicros != 5_000_000 || got.Latest.Currency != "USD" {
		t.Errorf("amount/currency = %d/%s, want 5000000/USD", got.Latest.AmountMicros, got.Latest.Currency)
	}
}

func TestLatestDonationShowsMessageWhenEnabled(t *testing.T) {
	fc := newFakeClock()
	mgr, b, lister := newTestManager(t, fc)
	p := newProfile("widget_1", domain.WidgetProfileKindLatestDonation)
	p.ShowMessage = true
	lister.add(p)

	publish(t, b, donationEvent("dk_1", 1_000_000, "USD", "Donor", "hello"))
	got := waitForRevision(t, mgr, p.ID, 1)
	if got.Latest == nil || got.Latest.Message != "hello" {
		t.Fatalf("Message = %q, want hello", got.Latest.Message)
	}
}

func TestLatestDonationAnonymousDonorHasNoDisplayName(t *testing.T) {
	fc := newFakeClock()
	mgr, b, lister := newTestManager(t, fc)
	p := lister.add(newProfile("widget_1", domain.WidgetProfileKindLatestDonation))

	publish(t, b, donationEvent("dk_1", 1_000_000, "USD", "", ""))
	got := waitForRevision(t, mgr, p.ID, 1)
	if got.Latest == nil || got.Latest.DisplayName != "" {
		t.Fatalf("DisplayName = %q, want empty for an anonymous donor", got.Latest.DisplayName)
	}
}

func TestLargestDonationExactMicrosComparisonAndTieRule(t *testing.T) {
	fc := newFakeClock()
	mgr, b, lister := newTestManager(t, fc)
	p := newProfile("widget_1", domain.WidgetProfileKindLargestDonation)
	p.Currency = "USD"
	lister.add(p)

	publish(t, b, donationEvent("dk_1", 10_000_000, "USD", "First", ""))
	waitForRevision(t, mgr, p.ID, 1)

	// A smaller donation never replaces the winner.
	publish(t, b, donationEvent("dk_2", 5_000_000, "USD", "Smaller", ""))
	time.Sleep(50 * time.Millisecond)
	if got := mgr.Snapshot(p.ID); got.Largest.DisplayName != "First" {
		t.Fatalf("Largest = %+v, want it to stay First", got.Largest)
	}

	// An exactly equal donation never replaces the winner either.
	publish(t, b, donationEvent("dk_3", 10_000_000, "USD", "Tied", ""))
	time.Sleep(50 * time.Millisecond)
	if got := mgr.Snapshot(p.ID); got.Largest.DisplayName != "First" {
		t.Fatalf("Largest = %+v, want it to stay First (equal amount must not replace)", got.Largest)
	}

	// A strictly larger donation replaces it.
	publish(t, b, donationEvent("dk_4", 10_000_001, "USD", "Bigger", ""))
	got := waitForRevision(t, mgr, p.ID, 2)
	if got.Largest.DisplayName != "Bigger" {
		t.Fatalf("Largest = %+v, want Bigger", got.Largest)
	}
}

func TestLargestDonationIgnoresForeignCurrency(t *testing.T) {
	fc := newFakeClock()
	mgr, b, lister := newTestManager(t, fc)
	p := newProfile("widget_1", domain.WidgetProfileKindLargestDonation)
	p.Currency = "USD"
	lister.add(p)

	publish(t, b, donationEvent("dk_1", 100_000_000, "EUR", "BigEuro", ""))
	time.Sleep(50 * time.Millisecond)
	if got := mgr.Snapshot(p.ID); got.Largest != nil {
		t.Fatalf("Largest = %+v, want nil (EUR never matches a USD-configured widget)", got.Largest)
	}
}

func TestLargestDonationRuntimeResetClears(t *testing.T) {
	fc := newFakeClock()
	mgr, b, lister := newTestManager(t, fc)
	p := newProfile("widget_1", domain.WidgetProfileKindLargestDonation)
	p.Currency = "USD"
	lister.add(p)

	publish(t, b, donationEvent("dk_1", 1_000_000, "USD", "Donor", ""))
	waitForRevision(t, mgr, p.ID, 1)

	mgr.Reset(p.ID)
	got := mgr.Snapshot(p.ID)
	if got.Largest != nil || got.Revision != 0 {
		t.Fatalf("Snapshot() after Reset = %+v, want the zero value", got)
	}
}

// --- recent_supporters (docs/supporter-widgets.md §7, §9) ---------------

func TestRecentSupportersBoundedAndNewestFirst(t *testing.T) {
	fc := newFakeClock()
	mgr, b, lister := newTestManager(t, fc)
	p := newProfile("widget_1", domain.WidgetProfileKindRecentSupporters)
	p.MaxItems = 2
	lister.add(p)

	publish(t, b, donationEvent("dk_1", 1_000_000, "USD", "First", ""))
	publish(t, b, donationEvent("dk_2", 1_000_000, "USD", "Second", ""))
	publish(t, b, donationEvent("dk_3", 1_000_000, "USD", "Third", ""))

	got := waitForRevision(t, mgr, p.ID, 3)
	if len(got.Recent) != 2 {
		t.Fatalf("len(Recent) = %d, want 2 (bounded by MaxItems)", len(got.Recent))
	}
	if got.Recent[0].DisplayName != "Third" || got.Recent[1].DisplayName != "Second" {
		t.Fatalf("Recent = %+v, want [Third, Second]", got.Recent)
	}
}

func TestRecentSupportersExcludesFollowAndGiftBatchSummary(t *testing.T) {
	fc := newFakeClock()
	mgr, b, lister := newTestManager(t, fc)
	p := lister.add(newProfile("widget_1", domain.WidgetProfileKindRecentSupporters))

	publish(t, b, followEvent(engagement.ProviderTwitch, "acct_1", "dk_follow", "follower"))
	qty := int64(3)
	publish(t, b, engagement.Event{
		SchemaVersion: engagement.CurrentSchemaVersion, ProviderID: engagement.ProviderTwitch,
		ConnectedAccountID: "acct_1", Type: engagement.TypeSubscriptionGiftBatch, PlatformTimestamp: time.Now().UTC(), DedupeKey: "dk_batch",
		Quantity: &qty, User: &engagement.User{DisplayName: "Gifter"},
	})
	time.Sleep(50 * time.Millisecond)
	if got := mgr.Snapshot(p.ID); len(got.Recent) != 0 {
		t.Fatalf("Recent = %+v, want empty (neither a follow nor a gift batch summary is a supporter row)", got.Recent)
	}

	publish(t, b, engagement.Event{
		SchemaVersion: engagement.CurrentSchemaVersion, ProviderID: engagement.ProviderTwitch,
		ConnectedAccountID: "acct_1", Type: engagement.TypeResubscription, PlatformTimestamp: time.Now().UTC(), DedupeKey: "dk_resub",
		User: &engagement.User{DisplayName: "Loyal"},
	})
	got := waitForRevision(t, mgr, p.ID, 1)
	if len(got.Recent) != 1 || got.Recent[0].DisplayName != "Loyal" {
		t.Fatalf("Recent = %+v, want [Loyal] (a resubscription IS supporter activity)", got.Recent)
	}
}

// --- event_ticker (docs/supporter-widgets.md §8) --------------------------

func TestEventTickerOnlyShowsAllowlistedTypes(t *testing.T) {
	fc := newFakeClock()
	mgr, b, lister := newTestManager(t, fc)
	p := newProfile("widget_1", domain.WidgetProfileKindEventTicker)
	p.EventTypes = []domain.SupporterEventType{domain.EventTypeFollow}
	lister.add(p)

	publish(t, b, donationEvent("dk_1", 1_000_000, "USD", "Donor", ""))
	time.Sleep(50 * time.Millisecond)
	if got := mgr.Snapshot(p.ID); len(got.Ticker) != 0 {
		t.Fatalf("Ticker = %+v, want empty (donation not in this ticker's own allowlist)", got.Ticker)
	}

	publish(t, b, followEvent(engagement.ProviderTwitch, "acct_1", "dk_follow", "follower"))
	got := waitForRevision(t, mgr, p.ID, 1)
	if len(got.Ticker) != 1 || got.Ticker[0].EventType != string(domain.EventTypeFollow) {
		t.Fatalf("Ticker = %+v, want one follow entry", got.Ticker)
	}
}

func TestEventTickerIgnoresEventTypeOutsideItsOwnClosedTable(t *testing.T) {
	fc := newFakeClock()
	mgr, b, lister := newTestManager(t, fc)
	p := newProfile("widget_1", domain.WidgetProfileKindEventTicker)
	p.EventTypes = []domain.SupporterEventType{domain.EventTypeFollow}
	lister.add(p)

	msg := engagement.NewMessage([]engagement.Fragment{{Type: engagement.FragmentText, Text: "hi"}})
	publish(t, b, engagement.Event{
		SchemaVersion: engagement.CurrentSchemaVersion, ProviderID: engagement.ProviderTwitch,
		ConnectedAccountID: "acct_1", Type: engagement.TypeChatMessage, PlatformTimestamp: time.Now().UTC(), DedupeKey: "dk_chat", Message: &msg,
		User: &engagement.User{ProviderUserID: "u1", Login: "viewer", DisplayName: "Viewer"},
	})
	time.Sleep(50 * time.Millisecond)
	if got := mgr.Snapshot(p.ID); len(got.Ticker) != 0 {
		t.Fatalf("Ticker = %+v, want empty - chat.message is outside the ticker's own closed table entirely", got.Ticker)
	}
}

// --- session_counter (docs/supporter-widgets.md §13) ----------------------

func TestSessionCounterFollowsMetric(t *testing.T) {
	fc := newFakeClock()
	mgr, b, lister := newTestManager(t, fc)
	p := newProfile("widget_1", domain.WidgetProfileKindSessionCounter)
	p.Metric = domain.MetricFollows
	lister.add(p)

	publish(t, b, followEvent(engagement.ProviderTwitch, "acct_1", "dk_1", "a"))
	publish(t, b, followEvent(engagement.ProviderTwitch, "acct_1", "dk_2", "b"))
	got := waitForRevision(t, mgr, p.ID, 2)
	if got.Counter != 2 {
		t.Fatalf("Counter = %d, want 2", got.Counter)
	}
}

func TestSessionCounterBitsQuantitySumsExactAmount(t *testing.T) {
	fc := newFakeClock()
	mgr, b, lister := newTestManager(t, fc)
	p := newProfile("widget_1", domain.WidgetProfileKindSessionCounter)
	p.Metric = domain.MetricBitsQuantity
	lister.add(p)

	qty1, qty2 := int64(100), int64(250)
	publish(t, b, engagement.Event{
		SchemaVersion: engagement.CurrentSchemaVersion, ProviderID: engagement.ProviderTwitch,
		ConnectedAccountID: "acct_1", Type: engagement.TypeBits, PlatformTimestamp: time.Now().UTC(), DedupeKey: "dk_1", Quantity: &qty1,
	})
	publish(t, b, engagement.Event{
		SchemaVersion: engagement.CurrentSchemaVersion, ProviderID: engagement.ProviderTwitch,
		ConnectedAccountID: "acct_1", Type: engagement.TypeBits, PlatformTimestamp: time.Now().UTC(), DedupeKey: "dk_2", Quantity: &qty2,
	})
	got := waitForRevision(t, mgr, p.ID, 2)
	if got.Counter != 350 {
		t.Fatalf("Counter = %d, want 350", got.Counter)
	}
}

func TestSessionCounterSupportAmountRequiresExactCurrencyMatch(t *testing.T) {
	fc := newFakeClock()
	mgr, b, lister := newTestManager(t, fc)
	p := newProfile("widget_1", domain.WidgetProfileKindSessionCounter)
	p.Metric = domain.MetricSupportAmount
	p.Currency = "USD"
	lister.add(p)

	publish(t, b, donationEvent("dk_1", 5_000_000, "EUR", "Foreign", ""))
	time.Sleep(50 * time.Millisecond)
	if got := mgr.Snapshot(p.ID); got.Counter != 0 {
		t.Fatalf("Counter = %d, want 0 (EUR never counts toward a USD counter)", got.Counter)
	}

	publish(t, b, donationEvent("dk_2", 3_000_000, "USD", "Domestic", ""))
	got := waitForRevision(t, mgr, p.ID, 1)
	if got.Counter != 3_000_000 {
		t.Fatalf("Counter = %d, want 3000000", got.Counter)
	}
}

func TestSessionCounterResetGoesToZero(t *testing.T) {
	fc := newFakeClock()
	mgr, b, lister := newTestManager(t, fc)
	p := newProfile("widget_1", domain.WidgetProfileKindSessionCounter)
	p.Metric = domain.MetricFollows
	lister.add(p)

	publish(t, b, followEvent(engagement.ProviderTwitch, "acct_1", "dk_1", "a"))
	waitForRevision(t, mgr, p.ID, 1)

	mgr.Reset(p.ID)
	if got := mgr.Snapshot(p.ID); got.Counter != 0 {
		t.Fatalf("Counter after Reset = %d, want 0", got.Counter)
	}
}

// --- one event, multiple independently-updated profiles ------------------

func TestOneEventUpdatesMultipleProfilesIndependently(t *testing.T) {
	fc := newFakeClock()
	mgr, b, lister := newTestManager(t, fc)
	latest := lister.add(newProfile("widget_latest", domain.WidgetProfileKindLatestDonation))
	largest := newProfile("widget_largest", domain.WidgetProfileKindLargestDonation)
	largest.Currency = "USD"
	lister.add(largest)
	recent := lister.add(newProfile("widget_recent", domain.WidgetProfileKindRecentSupporters))
	counter := newProfile("widget_counter", domain.WidgetProfileKindSessionCounter)
	counter.Metric = domain.MetricSupportEventCount
	lister.add(counter)

	publish(t, b, donationEvent("dk_1", 7_000_000, "USD", "Donor", ""))

	waitForRevision(t, mgr, latest.ID, 1)
	waitForRevision(t, mgr, largest.ID, 1)
	waitForRevision(t, mgr, recent.ID, 1)
	waitForRevision(t, mgr, counter.ID, 1)

	if got := mgr.Snapshot(counter.ID); got.Counter != 1 {
		t.Errorf("Counter = %d, want 1", got.Counter)
	}
}

// --- dashboard: no runtime projection of its own --------------------------

func TestDashboardKindNeverGetsAProjection(t *testing.T) {
	fc := newFakeClock()
	mgr, b, lister := newTestManager(t, fc)
	dash := lister.add(newProfile("widget_dash", domain.WidgetProfileKindDashboard))

	publish(t, b, followEvent(engagement.ProviderTwitch, "acct_1", "dk_1", "a"))
	time.Sleep(50 * time.Millisecond)
	if got := mgr.Snapshot(dash.ID); got.Revision != 0 {
		t.Fatalf("Snapshot(dashboard) = %+v, want the zero value - a dashboard has no runtime projection of its own", got)
	}
}
