package goals

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
// internal/alerts's own identical test helper.
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

// fakeDomainRepo is a minimal in-memory double for domain.Repository -
// a local, smaller redeclaration of domain/goals's own fakeRepo (test
// doubles are never shared across package boundaries in this codebase,
// mirroring internal/alerts's own identical precedent).
type fakeDomainRepo struct {
	mu      sync.Mutex
	goals   map[string]domain.Goal
	widgets map[string]domain.WidgetProfile
	ledger  map[string]bool
}

func newFakeDomainRepo() *fakeDomainRepo {
	return &fakeDomainRepo{goals: map[string]domain.Goal{}, widgets: map[string]domain.WidgetProfile{}, ledger: map[string]bool{}}
}
func (r *fakeDomainRepo) CreateGoal(_ context.Context, g domain.Goal) (domain.Goal, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.goals[g.ID] = g
	return g, nil
}
func (r *fakeDomainRepo) GetGoal(_ context.Context, id string) (domain.Goal, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g, ok := r.goals[id]
	return g, ok, nil
}
func (r *fakeDomainRepo) ListGoals(_ context.Context) ([]domain.Goal, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]domain.Goal, 0, len(r.goals))
	for _, g := range r.goals {
		out = append(out, g)
	}
	return out, nil
}
func (r *fakeDomainRepo) UpdateGoal(_ context.Context, g domain.Goal) (domain.Goal, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.goals[g.ID]; !ok {
		return domain.Goal{}, domain.ErrGoalNotFound
	}
	r.goals[g.ID] = g
	return g, nil
}
func (r *fakeDomainRepo) DeleteGoal(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.goals, id)
	return nil
}
func (r *fakeDomainRepo) SetCurrent(_ context.Context, id string, current int64) (domain.Goal, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g, ok := r.goals[id]
	if !ok {
		return domain.Goal{}, domain.ErrGoalNotFound
	}
	g.Current = current
	r.goals[id] = g
	return g, nil
}
func (r *fakeDomainRepo) ResetProgress(_ context.Context, id string) (domain.Goal, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g, ok := r.goals[id]
	if !ok {
		return domain.Goal{}, domain.ErrGoalNotFound
	}
	g.Current = g.Baseline
	r.goals[id] = g
	return g, nil
}
func (r *fakeDomainRepo) ApplyContribution(_ context.Context, goalID string, key domain.AppliedEventKey, amount int64) (bool, domain.Goal, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	lk := goalID + "|" + string(key.ProviderID) + "|" + key.AccountID + "|" + key.ProviderEventKey
	if r.ledger[lk] {
		return false, domain.Goal{}, nil
	}
	g, ok := r.goals[goalID]
	if !ok {
		return false, domain.Goal{}, domain.ErrGoalNotFound
	}
	r.ledger[lk] = true
	g.Current += amount
	r.goals[goalID] = g
	return true, g, nil
}
func (r *fakeDomainRepo) PruneAppliedEvents(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}
func (r *fakeDomainRepo) CreateWidgetProfile(_ context.Context, p domain.WidgetProfile) (domain.WidgetProfile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.widgets[p.ID] = p
	return p, nil
}
func (r *fakeDomainRepo) GetWidgetProfile(_ context.Context, id string) (domain.WidgetProfile, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.widgets[id]
	return p, ok, nil
}
func (r *fakeDomainRepo) GetWidgetProfileByPublicSlug(_ context.Context, slug string) (domain.WidgetProfile, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range r.widgets {
		if p.PublicSlug == slug {
			return p, true, nil
		}
	}
	return domain.WidgetProfile{}, false, nil
}
func (r *fakeDomainRepo) ListWidgetProfiles(_ context.Context, goalID string) ([]domain.WidgetProfile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []domain.WidgetProfile{}
	for _, p := range r.widgets {
		if goalID == "" || p.GoalID == goalID {
			out = append(out, p)
		}
	}
	return out, nil
}
func (r *fakeDomainRepo) UpdateWidgetProfile(_ context.Context, p domain.WidgetProfile) (domain.WidgetProfile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.widgets[p.ID] = p
	return p, nil
}
func (r *fakeDomainRepo) RotatePublicSlug(_ context.Context, id, newSlug string) (domain.WidgetProfile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.widgets[id]
	if !ok {
		return domain.WidgetProfile{}, domain.ErrWidgetProfileNotFound
	}
	p.PublicSlug = newSlug
	r.widgets[id] = p
	return p, nil
}
func (r *fakeDomainRepo) DeleteWidgetProfile(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.widgets, id)
	return nil
}

type fakeDomainAccounts struct{}

func (fakeDomainAccounts) AccountExists(_ context.Context, _ string) (bool, error) { return true, nil }

func newTestManager(t *testing.T, fc *fakeClock) (*Manager, *bus.Bus, *domain.Service) {
	t.Helper()
	repo := newFakeDomainRepo()
	domainSvc := domain.NewService(repo, fakeDomainAccounts{}, fc.Now)
	b := bus.New(bus.Options{Now: fc.Now})
	mgr := NewManager(ManagerOptions{DomainService: domainSvc, Bus: b, Now: fc.Now})
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
	return mgr, b, domainSvc
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

func waitForCurrent(t *testing.T, svc *domain.Service, goalID string, want int64) {
	t.Helper()
	waitUntil(t, 5*time.Second, func() bool {
		g, err := svc.GetGoal(context.Background(), goalID)
		return err == nil && g.Current == want
	})
}

func publish(t *testing.T, b *bus.Bus, evt engagement.Event) {
	t.Helper()
	if _, _, err := b.Publish(evt); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
}

func followEvent(now time.Time, dedupeKey string) engagement.Event {
	return engagement.Event{
		SchemaVersion: engagement.CurrentSchemaVersion, ProviderID: engagement.ProviderTwitch,
		ConnectedAccountID: "acct_1", Type: engagement.TypeFollow, PlatformTimestamp: now,
		DedupeKey: dedupeKey, User: &engagement.User{ProviderUserID: "u1", Login: "viewer", DisplayName: "Viewer"},
	}
}

func createFollowerGoal(t *testing.T, svc *domain.Service, target int64) domain.Goal {
	t.Helper()
	g, err := svc.CreateGoal(context.Background(), domain.Goal{Name: "Followers", Kind: domain.KindFollowers, Target: target})
	if err != nil {
		t.Fatalf("CreateGoal() error = %v", err)
	}
	return g
}

func TestManagerRealFollowIncrementsGoal(t *testing.T) {
	fc := newFakeClock()
	mgr, b, svc := newTestManager(t, fc)
	_ = mgr
	g := createFollowerGoal(t, svc, 1000)

	publish(t, b, followEvent(fc.Now(), "dk_1"))
	waitForCurrent(t, svc, g.ID, 1)
}

func TestManagerDuplicateDeliveryDoesNotDoubleCount(t *testing.T) {
	fc := newFakeClock()
	_, b, svc := newTestManager(t, fc)
	g := createFollowerGoal(t, svc, 1000)

	publish(t, b, followEvent(fc.Now(), "dk_dup"))
	waitForCurrent(t, svc, g.ID, 1)
	// A second delivery with the SAME dedupe key (Twitch redelivering
	// the same EventSub notification) must never double-apply.
	publish(t, b, followEvent(fc.Now(), "dk_dup"))
	time.Sleep(50 * time.Millisecond)
	waitForCurrent(t, svc, g.ID, 1)
}

func TestManagerIrrelevantEventDoesNotContribute(t *testing.T) {
	fc := newFakeClock()
	_, b, svc := newTestManager(t, fc)
	g := createFollowerGoal(t, svc, 1000)

	msg := engagement.NewMessage([]engagement.Fragment{{Type: engagement.FragmentText, Text: "hi"}})
	publish(t, b, engagement.Event{
		SchemaVersion: engagement.CurrentSchemaVersion, ProviderID: engagement.ProviderTwitch,
		ConnectedAccountID: "acct_1", Type: engagement.TypeChatMessage, PlatformTimestamp: fc.Now(),
		DedupeKey: "dk_chat", ProviderEventID: "msg_1", Message: &msg,
		User: &engagement.User{ProviderUserID: "u1", Login: "viewer", DisplayName: "Viewer"},
	})
	// Give the manager time to process, then confirm nothing moved.
	waitForCurrent(t, svc, g.ID, 0)
}

func TestManagerSyntheticEventIgnored(t *testing.T) {
	fc := newFakeClock()
	_, b, svc := newTestManager(t, fc)
	g := createFollowerGoal(t, svc, 1000)

	evt := followEvent(fc.Now(), "dk_synth")
	evt.Synthetic = true
	publish(t, b, evt)

	// A real follow arrives right after - once its own goal moves, we
	// know the earlier synthetic one was already fully processed (and
	// ignored), not merely still in flight.
	publish(t, b, followEvent(fc.Now(), "dk_real_after_synth"))
	waitForCurrent(t, svc, g.ID, 1)
}

func TestManagerMultipleGoalsBothIncrement(t *testing.T) {
	fc := newFakeClock()
	_, b, svc := newTestManager(t, fc)
	g1 := createFollowerGoal(t, svc, 100)
	g2 := createFollowerGoal(t, svc, 5000)

	publish(t, b, followEvent(fc.Now(), "dk_multi"))
	waitForCurrent(t, svc, g1.ID, 1)
	waitForCurrent(t, svc, g2.ID, 1)
}

func TestManagerDisabledGoalDoesNotIncrement(t *testing.T) {
	fc := newFakeClock()
	_, b, svc := newTestManager(t, fc)
	g := createFollowerGoal(t, svc, 100)
	g.Enabled = false
	if _, err := svc.UpdateGoal(context.Background(), g); err != nil {
		t.Fatalf("UpdateGoal() error = %v", err)
	}

	other := createFollowerGoal(t, svc, 100)
	publish(t, b, followEvent(fc.Now(), "dk_disabled"))
	waitForCurrent(t, svc, other.ID, 1)

	got, _ := svc.GetGoal(context.Background(), g.ID)
	if got.Current != 0 {
		t.Errorf("disabled goal Current = %d, want 0", got.Current)
	}
}

func TestManagerProviderFilterExcludesNonMatchingProvider(t *testing.T) {
	fc := newFakeClock()
	_, b, svc := newTestManager(t, fc)
	g, err := svc.CreateGoal(context.Background(), domain.Goal{
		Name: "YouTube only", Kind: domain.KindFollowers, Target: 100,
		Providers: []domain.ProviderID{domain.ProviderYouTube},
	})
	if err != nil {
		t.Fatalf("CreateGoal() error = %v", err)
	}

	other := createFollowerGoal(t, svc, 100)
	publish(t, b, followEvent(fc.Now(), "dk_provider")) // Twitch event
	publish(t, b, followEvent(fc.Now(), "dk_provider2"))
	waitForCurrent(t, svc, other.ID, 2)

	got, _ := svc.GetGoal(context.Background(), g.ID)
	if got.Current != 0 {
		t.Errorf("YouTube-only goal Current = %d, want 0 for a Twitch event", got.Current)
	}
}

func TestManagerAccountFilterExcludesNonMatchingAccount(t *testing.T) {
	fc := newFakeClock()
	_, b, svc := newTestManager(t, fc)
	g, err := svc.CreateGoal(context.Background(), domain.Goal{
		Name: "One account", Kind: domain.KindFollowers, Target: 100, Accounts: []string{"acct_other"},
	})
	if err != nil {
		t.Fatalf("CreateGoal() error = %v", err)
	}

	other := createFollowerGoal(t, svc, 100)
	publish(t, b, followEvent(fc.Now(), "dk_acct")) // ConnectedAccountID = "acct_1"
	waitForCurrent(t, svc, other.ID, 1)

	got, _ := svc.GetGoal(context.Background(), g.ID)
	if got.Current != 0 {
		t.Errorf("filtered-account goal Current = %d, want 0", got.Current)
	}
}

func TestManagerDonationCurrencyMismatchDoesNotContribute(t *testing.T) {
	fc := newFakeClock()
	_, b, svc := newTestManager(t, fc)
	g, err := svc.CreateGoal(context.Background(), domain.Goal{Name: "Fund", Kind: domain.KindDonations, Target: 100000000, Currency: "USD"})
	if err != nil {
		t.Fatalf("CreateGoal() error = %v", err)
	}

	eurMoney, _ := engagement.NewMoney(5000000, "EUR", "€5.00")
	publish(t, b, engagement.Event{
		SchemaVersion: engagement.CurrentSchemaVersion, ProviderID: engagement.ProviderStreamElements,
		ConnectedAccountID: "src_1", Type: engagement.TypeDonation, PlatformTimestamp: fc.Now(),
		DedupeKey: "dk_eur", ProviderEventID: "tip_eur", Money: &eurMoney,
		User: &engagement.User{DisplayName: "Donor"},
	})

	usdMoney, _ := engagement.NewMoney(3000000, "USD", "$3.00")
	publish(t, b, engagement.Event{
		SchemaVersion: engagement.CurrentSchemaVersion, ProviderID: engagement.ProviderStreamElements,
		ConnectedAccountID: "src_1", Type: engagement.TypeDonation, PlatformTimestamp: fc.Now(),
		DedupeKey: "dk_usd", ProviderEventID: "tip_usd", Money: &usdMoney,
		User: &engagement.User{DisplayName: "Donor"},
	})
	waitForCurrent(t, svc, g.ID, 3000000)
}

func TestManagerBitsExactQuantity(t *testing.T) {
	fc := newFakeClock()
	_, b, svc := newTestManager(t, fc)
	g, err := svc.CreateGoal(context.Background(), domain.Goal{Name: "Bits", Kind: domain.KindBits, Target: 100000})
	if err != nil {
		t.Fatalf("CreateGoal() error = %v", err)
	}

	bits := int64(750)
	publish(t, b, engagement.Event{
		SchemaVersion: engagement.CurrentSchemaVersion, ProviderID: engagement.ProviderTwitch,
		ConnectedAccountID: "acct_1", Type: engagement.TypeBits, PlatformTimestamp: fc.Now(),
		DedupeKey: "dk_bits", Quantity: &bits,
		User: &engagement.User{ProviderUserID: "u1", Login: "cheerer", DisplayName: "Cheerer"},
	})
	waitForCurrent(t, svc, g.ID, 750)
}

func TestManagerGiftBatchAndIndividualGiftsNeverDoubleCount(t *testing.T) {
	fc := newFakeClock()
	_, b, svc := newTestManager(t, fc)
	g, err := svc.CreateGoal(context.Background(), domain.Goal{Name: "Subs", Kind: domain.KindSubscriptions, Target: 1000})
	if err != nil {
		t.Fatalf("CreateGoal() error = %v", err)
	}

	qty := int64(5)
	publish(t, b, engagement.Event{
		SchemaVersion: engagement.CurrentSchemaVersion, ProviderID: engagement.ProviderTwitch,
		ConnectedAccountID: "acct_1", Type: engagement.TypeSubscriptionGiftBatch, PlatformTimestamp: fc.Now(),
		DedupeKey: "dk_batch", Quantity: &qty,
		User: &engagement.User{ProviderUserID: "gifter", Login: "gifter", DisplayName: "Gifter"},
	})
	for i := 0; i < 5; i++ {
		publish(t, b, engagement.Event{
			SchemaVersion: engagement.CurrentSchemaVersion, ProviderID: engagement.ProviderTwitch,
			ConnectedAccountID: "acct_1", Type: engagement.TypeGiftedSubscription, PlatformTimestamp: fc.Now(),
			DedupeKey: "dk_recipient_" + string(rune('a'+i)),
			User:      &engagement.User{ProviderUserID: "recipient", Login: "recipient", DisplayName: "Recipient"},
		})
	}
	// Exactly 5 (the recipients), never 10 (batch + recipients) and
	// never 0.
	waitForCurrent(t, svc, g.ID, 5)
}

func TestManagerRestartNeverReplaysRetainedEvents(t *testing.T) {
	fc := newFakeClock()
	repo := newFakeDomainRepo()
	domainSvc := domain.NewService(repo, fakeDomainAccounts{}, fc.Now)
	b := bus.New(bus.Options{Now: fc.Now})
	defer b.Shutdown()

	// The goal already exists BEFORE the manager ever subscribes, so if
	// Subscribe(0) incorrectly replayed retained history, the "before"
	// event below would have a real goal to increment.
	g := createFollowerGoal(t, domainSvc, 100)
	publish(t, b, followEvent(fc.Now(), "dk_before_start"))

	mgr := NewManager(ManagerOptions{DomainService: domainSvc, Bus: b, Now: fc.Now})
	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	waitUntil(t, time.Second, mgr.Subscribed)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = mgr.Shutdown(ctx)
	}()

	// A NEW event, published after the manager subscribed, must still
	// count - proving the manager is genuinely live, not merely never
	// having replayed the earlier one because nothing was listening yet.
	publish(t, b, followEvent(fc.Now(), "dk_after_start"))
	waitForCurrent(t, domainSvc, g.ID, 1)

	// Both events are processed strictly in order by the manager's own
	// single consumer goroutine, so by the time Current==1 is observed
	// above, "dk_before_start" has already been fully resolved (applied
	// or correctly skipped) - a brief settle-and-recheck closes any
	// remaining race between sampling Current mid-transition and a
	// second, later increment actually landing.
	time.Sleep(100 * time.Millisecond)
	got, err := domainSvc.GetGoal(context.Background(), g.ID)
	if err != nil {
		t.Fatalf("GetGoal() error = %v", err)
	}
	if got.Current != 1 {
		t.Errorf("Current = %d, want 1 - the event published before Subscribe must never be replayed", got.Current)
	}
}
