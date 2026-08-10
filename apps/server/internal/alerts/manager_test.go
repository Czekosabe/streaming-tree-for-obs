package alerts

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	domain "github.com/streaming-tree/server/internal/domain/alerts"
	engagement "github.com/streaming-tree/server/internal/domain/engagement"
	bus "github.com/streaming-tree/server/internal/engagement"
)

// fakeClock mirrors internal/chatautomation's own test helper exactly -
// see docs/progress.md's Stage 11B entry for why the poll loop stays a
// real time.Ticker while only the "now" comparisons read from this.
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
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

// fakeDomainRepo/fakeDomainAccounts are minimal in-memory doubles for
// domain.Repository/domain.AccountLookup - a local, smaller redeclaration
// of the same shape internal/domain/alerts's own service_test.go uses,
// needed here because that file's fakeRepo is unexported and scoped to
// its own package.
type fakeDomainRepo struct {
	mu       sync.Mutex
	profiles map[string]domain.Profile
	rules    map[string]domain.Rule
}

func newFakeDomainRepo() *fakeDomainRepo {
	return &fakeDomainRepo{profiles: map[string]domain.Profile{}, rules: map[string]domain.Rule{}}
}
func (r *fakeDomainRepo) CreateProfile(_ context.Context, p domain.Profile) (domain.Profile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.profiles[p.ID] = p
	return p, nil
}
func (r *fakeDomainRepo) GetProfile(_ context.Context, id string) (domain.Profile, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.profiles[id]
	return p, ok, nil
}
func (r *fakeDomainRepo) GetProfileByPublicSlug(_ context.Context, slug string) (domain.Profile, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range r.profiles {
		if p.PublicSlug == slug {
			return p, true, nil
		}
	}
	return domain.Profile{}, false, nil
}
func (r *fakeDomainRepo) ListProfiles(_ context.Context) ([]domain.Profile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]domain.Profile, 0, len(r.profiles))
	for _, p := range r.profiles {
		out = append(out, p)
	}
	return out, nil
}
func (r *fakeDomainRepo) UpdateProfile(_ context.Context, p domain.Profile) (domain.Profile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.profiles[p.ID]; !ok {
		return domain.Profile{}, domain.ErrProfileNotFound
	}
	r.profiles[p.ID] = p
	return p, nil
}
func (r *fakeDomainRepo) RotatePublicSlug(_ context.Context, id, newSlug string) (domain.Profile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.profiles[id]
	if !ok {
		return domain.Profile{}, domain.ErrProfileNotFound
	}
	p.PublicSlug = newSlug
	r.profiles[id] = p
	return p, nil
}
func (r *fakeDomainRepo) DeleteProfile(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.profiles, id)
	for rid, ru := range r.rules {
		if ru.ProfileID == id {
			delete(r.rules, rid)
		}
	}
	return nil
}
func (r *fakeDomainRepo) CreateRule(_ context.Context, ru domain.Rule) (domain.Rule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rules[ru.ID] = ru
	return ru, nil
}
func (r *fakeDomainRepo) GetRule(_ context.Context, id string) (domain.Rule, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ru, ok := r.rules[id]
	return ru, ok, nil
}
func (r *fakeDomainRepo) ListRules(_ context.Context, profileID string) ([]domain.Rule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []domain.Rule
	for _, ru := range r.rules {
		if ru.ProfileID == profileID {
			out = append(out, ru)
		}
	}
	return out, nil
}
func (r *fakeDomainRepo) UpdateRule(_ context.Context, ru domain.Rule) (domain.Rule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.rules[ru.ID]; !ok {
		return domain.Rule{}, domain.ErrRuleNotFound
	}
	r.rules[ru.ID] = ru
	return ru, nil
}
func (r *fakeDomainRepo) DeleteRule(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.rules, id)
	return nil
}

type fakeDomainAccounts struct{}

func (fakeDomainAccounts) AccountExists(_ context.Context, _ string) (bool, error) { return true, nil }

var testRuleIDCounter int

func newTestManager(t *testing.T, fc *fakeClock) (*Manager, *bus.Bus) {
	t.Helper()
	repo := newFakeDomainRepo()
	domainSvc := domain.NewService(repo, fakeDomainAccounts{}, fc.Now)
	b := bus.New(bus.Options{Now: fc.Now})
	mgr := NewManager(ManagerOptions{DomainService: domainSvc, Bus: b, Now: fc.Now})
	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	// Wait for the shared Event Bus subscription to actually be live
	// before returning, so a test that publishes immediately afterward
	// never races Subscribe's own "resume from current position"
	// window - see Manager.Subscribed's own doc comment.
	waitUntil(t, time.Second, mgr.Subscribed)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = mgr.Shutdown(ctx)
		b.Shutdown()
	})
	return mgr, b
}

func createTestProfileAndRule(t *testing.T, mgr *Manager, eventType domain.EventType) (domain.Profile, domain.Rule) {
	t.Helper()
	ctx := context.Background()
	p, err := mgr.CreateProfile(ctx, "Main")
	if err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}
	testRuleIDCounter++
	r, err := mgr.CreateRule(ctx, p.ID, domain.RuleInput{
		Name: "r", Enabled: true, EventType: eventType, Priority: 50, DurationMS: 5000,
		RequiredRole: domain.RoleEveryone, ShowPlatform: true, ShowUsername: true,
		TextTemplate: "{username}", EntryAnimation: domain.AnimationFade, ExitAnimation: domain.AnimationFade,
		AnimationDurationMS: 400, GroupWindowMS: domain.DefaultGroupWindowMS, InterruptMode: domain.InterruptNever, Interruptible: true,
	})
	if err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}
	return p, r
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

func publishFollow(t *testing.T, b *bus.Bus, now time.Time) {
	t.Helper()
	evt := engagement.Event{
		SchemaVersion: engagement.CurrentSchemaVersion, ProviderID: engagement.ProviderTwitch,
		ConnectedAccountID: "acct_1", Type: engagement.TypeFollow, PlatformTimestamp: now,
		DedupeKey: "dk_" + now.String(), User: &engagement.User{ProviderUserID: "u1", Login: "viewer", DisplayName: "Viewer"},
	}
	if _, _, err := b.Publish(evt); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
}

func TestManagerStartsWithNoProfiles(t *testing.T) {
	fc := newFakeClock()
	mgr, _ := newTestManager(t, fc)
	list, err := mgr.ListProfiles(context.Background())
	if err != nil {
		t.Fatalf("ListProfiles() error = %v", err)
	}
	if len(list) != 0 {
		t.Errorf("ListProfiles() = %d, want 0", len(list))
	}
}

func TestManagerRealEventMatchesAndPlays(t *testing.T) {
	fc := newFakeClock()
	mgr, b := newTestManager(t, fc)
	p, _ := createTestProfileAndRule(t, mgr, domain.EventFollow)

	publishFollow(t, b, fc.Now())
	waitUntil(t, 5*time.Second, func() bool {
		st, err := mgr.ProfileStatus(p.ID)
		return err == nil && (st.Current != nil || st.QueuedCount > 0)
	})
}

func TestManagerSyntheticRealBusEventIgnored(t *testing.T) {
	fc := newFakeClock()
	mgr, b := newTestManager(t, fc)
	p, _ := createTestProfileAndRule(t, mgr, domain.EventFollow)

	evt := engagement.Event{
		SchemaVersion: engagement.CurrentSchemaVersion, ProviderID: engagement.ProviderTwitch,
		ConnectedAccountID: "acct_1", Type: engagement.TypeFollow, PlatformTimestamp: fc.Now(),
		DedupeKey: "dk_synthetic", Synthetic: true,
		User: &engagement.User{ProviderUserID: "u1", Login: "viewer", DisplayName: "Viewer"},
	}
	if _, _, err := b.Publish(evt); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	time.Sleep(150 * time.Millisecond)
	st, err := mgr.ProfileStatus(p.ID)
	if err != nil {
		t.Fatalf("ProfileStatus() error = %v", err)
	}
	if st.Current != nil || st.QueuedCount != 0 || st.TotalEnqueued != 0 {
		t.Errorf("status = %+v after a synthetic real-bus event, want completely untouched", st)
	}
}

func TestManagerTestRuleGoesThroughRealQueue(t *testing.T) {
	fc := newFakeClock()
	mgr, _ := newTestManager(t, fc)
	_, r := createTestProfileAndRule(t, mgr, domain.EventFollow)

	summary, err := mgr.TestRule(context.Background(), r.ID, "")
	if err != nil {
		t.Fatalf("TestRule() error = %v", err)
	}
	if !summary.Synthetic {
		t.Error("TestRule() summary.Synthetic = false, want true")
	}

	st, err := mgr.ProfileStatus(r.ProfileID)
	if err != nil {
		t.Fatalf("ProfileStatus() error = %v", err)
	}
	if st.TotalSynthetic != 1 {
		t.Errorf("TotalSynthetic = %d, want 1", st.TotalSynthetic)
	}
}

func TestManagerTestRuleRejectsDisabledProfile(t *testing.T) {
	fc := newFakeClock()
	mgr, _ := newTestManager(t, fc)
	p, r := createTestProfileAndRule(t, mgr, domain.EventFollow)

	if _, err := mgr.ReplaceProfile(context.Background(), p.ID, domain.ProfileInput{
		Name: p.Name, Enabled: false, Language: p.Language, Theme: p.Theme, Position: p.Position,
		TextAlign: p.TextAlign, MaxQueueItems: p.MaxQueueItems, MaximumQueueAgeSeconds: p.MaximumQueueAgeSeconds,
	}); err != nil {
		t.Fatalf("ReplaceProfile() error = %v", err)
	}

	if _, err := mgr.TestRule(context.Background(), r.ID, ""); !errors.Is(err, ErrProfileDisabled) {
		t.Errorf("TestRule() on a disabled profile error = %v, want ErrProfileDisabled", err)
	}
}

func TestManagerRuleReloadTakesEffectWithoutRestart(t *testing.T) {
	fc := newFakeClock()
	mgr, b := newTestManager(t, fc)
	p, r := createTestProfileAndRule(t, mgr, domain.EventFollow)

	// Disable the rule; a subsequent matching event must not enqueue.
	if _, err := mgr.ReplaceRule(context.Background(), r.ID, domain.RuleInput{
		Name: r.Name, Enabled: false, EventType: r.EventType, Priority: r.Priority, DurationMS: r.DurationMS,
		RequiredRole: r.RequiredRole, ShowPlatform: r.ShowPlatform, ShowUsername: r.ShowUsername,
		TextTemplate: r.TextTemplate, EntryAnimation: r.EntryAnimation, ExitAnimation: r.ExitAnimation,
		AnimationDurationMS: r.AnimationDurationMS, GroupWindowMS: r.GroupWindowMS, InterruptMode: r.InterruptMode, Interruptible: r.Interruptible,
	}); err != nil {
		t.Fatalf("ReplaceRule() error = %v", err)
	}

	publishFollow(t, b, fc.Now())
	time.Sleep(150 * time.Millisecond)
	st, err := mgr.ProfileStatus(p.ID)
	if err != nil {
		t.Fatalf("ProfileStatus() error = %v", err)
	}
	if st.TotalEnqueued != 0 {
		t.Errorf("TotalEnqueued = %d after disabling the only matching rule, want 0", st.TotalEnqueued)
	}
}

func TestManagerDeleteProfileRemovesRuntime(t *testing.T) {
	fc := newFakeClock()
	mgr, _ := newTestManager(t, fc)
	p, _ := createTestProfileAndRule(t, mgr, domain.EventFollow)

	if err := mgr.DeleteProfile(context.Background(), p.ID); err != nil {
		t.Fatalf("DeleteProfile() error = %v", err)
	}
	if _, err := mgr.ProfileStatus(p.ID); !errors.Is(err, ErrProfileNotFound) {
		t.Errorf("ProfileStatus() after delete error = %v, want ErrProfileNotFound", err)
	}
}

func TestManagerTwoProfilesIsolated(t *testing.T) {
	fc := newFakeClock()
	mgr, b := newTestManager(t, fc)
	p1, _ := createTestProfileAndRule(t, mgr, domain.EventFollow)
	p2, _ := createTestProfileAndRule(t, mgr, domain.EventFollow)

	publishFollow(t, b, fc.Now())
	waitUntil(t, 5*time.Second, func() bool {
		s1, err1 := mgr.ProfileStatus(p1.ID)
		s2, err2 := mgr.ProfileStatus(p2.ID)
		return err1 == nil && err2 == nil && s1.TotalEnqueued == 1 && s2.TotalEnqueued == 1
	})

	// Skip on profile 1 must never affect profile 2.
	waitUntil(t, 5*time.Second, func() bool {
		st, err := mgr.ProfileStatus(p1.ID)
		return err == nil && st.Current != nil
	})
	if err := mgr.SkipCurrent(p1.ID); err != nil {
		t.Fatalf("SkipCurrent() error = %v", err)
	}
	st2, err := mgr.ProfileStatus(p2.ID)
	if err != nil {
		t.Fatalf("ProfileStatus(p2) error = %v", err)
	}
	if st2.TotalManuallySkipped != 0 {
		t.Errorf("profile 2 TotalManuallySkipped = %d after skipping only on profile 1, want 0", st2.TotalManuallySkipped)
	}
}

func TestManagerRestartResetsRuntimeButKeepsDefinitions(t *testing.T) {
	fc := newFakeClock()
	repo := newFakeDomainRepo()
	domainSvc := domain.NewService(repo, fakeDomainAccounts{}, fc.Now)
	b := bus.New(bus.Options{Now: fc.Now})
	defer b.Shutdown()

	mgr1 := NewManager(ManagerOptions{DomainService: domainSvc, Bus: b, Now: fc.Now})
	if err := mgr1.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	waitUntil(t, time.Second, mgr1.Subscribed)
	p, _ := createTestProfileAndRule(t, mgr1, domain.EventFollow)
	publishFollow(t, b, fc.Now())
	waitUntil(t, 5*time.Second, func() bool {
		st, err := mgr1.ProfileStatus(p.ID)
		return err == nil && st.TotalEnqueued == 1
	})
	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := mgr1.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}

	// "Restart": a brand new Manager over the same domain service/repo.
	mgr2 := NewManager(ManagerOptions{DomainService: domainSvc, Bus: b, Now: fc.Now})
	if err := mgr2.Start(context.Background()); err != nil {
		t.Fatalf("Start() (restart) error = %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = mgr2.Shutdown(ctx)
	}()

	list, err := mgr2.ListProfiles(context.Background())
	if err != nil || len(list) != 1 {
		t.Fatalf("ListProfiles() after restart = %+v, err=%v, want the one persisted profile", list, err)
	}
	st, err := mgr2.ProfileStatus(p.ID)
	if err != nil {
		t.Fatalf("ProfileStatus() after restart error = %v", err)
	}
	if st.TotalEnqueued != 0 || st.Current != nil || st.QueuedCount != 0 {
		t.Errorf("status after restart = %+v, want fully reset runtime state (no missed-alert replay)", st)
	}
	if st.TotalGroupedMembers != 0 || st.TotalGroupsCreated != 0 || st.TotalPreempted != 0 {
		t.Errorf("status after restart = %+v, want the Stage 12B counters reset too", st)
	}
}

func publishCheer(t *testing.T, b *bus.Bus, now time.Time, userID string, bits int64, dedupe string) {
	t.Helper()
	evt := engagement.Event{
		SchemaVersion: engagement.CurrentSchemaVersion, ProviderID: engagement.ProviderTwitch,
		ConnectedAccountID: "acct_1", Type: engagement.TypeBits, PlatformTimestamp: now,
		DedupeKey: dedupe, User: &engagement.User{ProviderUserID: userID, Login: userID, DisplayName: userID},
		Quantity: &bits,
	}
	if _, _, err := b.Publish(evt); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
}

// TestManagerRealEventsGroupEndToEnd proves the whole pipeline (real Bus
// event -> Manager.handleEvent -> matcher -> grouping) end to end: two
// Bits cheers from the same actor, inside the rule's own grouping
// window, merge into one queued alert with a truthful summed quantity.
func TestManagerRealEventsGroupEndToEnd(t *testing.T) {
	fc := newFakeClock()
	mgr, b := newTestManager(t, fc)
	p, err := mgr.CreateProfile(context.Background(), "Main")
	if err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}
	if _, err := mgr.CreateRule(context.Background(), p.ID, domain.RuleInput{
		Name: "Bits", Enabled: true, EventType: domain.EventBits, Priority: 50, DurationMS: 5000,
		RequiredRole: domain.RoleEveryone, ShowPlatform: true, ShowUsername: true, ShowQuantity: true,
		TextTemplate:   "{username} cheered {quantity} bits (x{groupCount})",
		EntryAnimation: domain.AnimationFade, ExitAnimation: domain.AnimationFade, AnimationDurationMS: 400,
		AllowGrouping: true, GroupWindowMS: 5000, InterruptMode: domain.InterruptNever, Interruptible: true,
	}); err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}

	publishCheer(t, b, fc.Now(), "u1", 100, "dk_cheer_1")
	waitUntil(t, 5*time.Second, func() bool {
		st, err := mgr.ProfileStatus(p.ID)
		return err == nil && st.TotalEnqueued == 1
	})
	publishCheer(t, b, fc.Now(), "u1", 50, "dk_cheer_2")
	waitUntil(t, 5*time.Second, func() bool {
		st, err := mgr.ProfileStatus(p.ID)
		return err == nil && st.TotalGroupedMembers == 1
	})

	st, err := mgr.ProfileStatus(p.ID)
	if err != nil {
		t.Fatalf("ProfileStatus() error = %v", err)
	}
	if st.QueuedCount+boolToInt(st.Current != nil) != 1 {
		t.Fatalf("status = %+v, want exactly one alert (grouped, not two)", st)
	}
	var quantity *int64
	if st.Current != nil {
		quantity = st.Current.Quantity
	} else if len(st.NextQueued) == 1 {
		quantity = st.NextQueued[0].Quantity
	}
	if quantity == nil || *quantity != 150 {
		t.Errorf("grouped quantity = %v, want 150", quantity)
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// TestManagerRealEventPreemptsEndToEnd proves preemption end to end: a
// real, eligible, strictly-higher-priority raid alert interrupts a
// currently-playing, interruptible, lower-priority follow alert.
func TestManagerRealEventPreemptsEndToEnd(t *testing.T) {
	fc := newFakeClock()
	mgr, b := newTestManager(t, fc)
	p, err := mgr.CreateProfile(context.Background(), "Main")
	if err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}
	if _, err := mgr.CreateRule(context.Background(), p.ID, domain.RuleInput{
		Name: "Follow", Enabled: true, EventType: domain.EventFollow, Priority: 10, DurationMS: 30000,
		RequiredRole: domain.RoleEveryone, ShowPlatform: true, ShowUsername: true,
		TextTemplate:   "{username} followed",
		EntryAnimation: domain.AnimationFade, ExitAnimation: domain.AnimationFade, AnimationDurationMS: 400,
		GroupWindowMS: domain.DefaultGroupWindowMS, InterruptMode: domain.InterruptNever, Interruptible: true,
	}); err != nil {
		t.Fatalf("CreateRule(follow) error = %v", err)
	}
	if _, err := mgr.CreateRule(context.Background(), p.ID, domain.RuleInput{
		Name: "Raid", Enabled: true, EventType: domain.EventRaid, Priority: 100, DurationMS: 5000,
		RequiredRole: domain.RoleEveryone, ShowPlatform: true, ShowUsername: true, ShowQuantity: true,
		TextTemplate:   "{username} raided with {quantity} viewers",
		EntryAnimation: domain.AnimationFade, ExitAnimation: domain.AnimationFade, AnimationDurationMS: 400,
		GroupWindowMS: domain.DefaultGroupWindowMS, InterruptMode: domain.InterruptLowerPriority, Interruptible: true,
	}); err != nil {
		t.Fatalf("CreateRule(raid) error = %v", err)
	}

	publishFollow(t, b, fc.Now())
	waitUntil(t, 5*time.Second, func() bool {
		st, err := mgr.ProfileStatus(p.ID)
		return err == nil && st.Current != nil && st.Current.EventType == domain.EventFollow
	})

	viewers := int64(42)
	evt := engagement.Event{
		SchemaVersion: engagement.CurrentSchemaVersion, ProviderID: engagement.ProviderTwitch,
		ConnectedAccountID: "acct_1", Type: engagement.TypeRaid, PlatformTimestamp: fc.Now(),
		DedupeKey: "dk_raid_1", User: &engagement.User{ProviderUserID: "raider", Login: "raider", DisplayName: "Raider"},
		Quantity: &viewers,
	}
	if _, _, err := b.Publish(evt); err != nil {
		t.Fatalf("Publish(raid) error = %v", err)
	}
	waitUntil(t, 5*time.Second, func() bool {
		st, err := mgr.ProfileStatus(p.ID)
		return err == nil && st.Current != nil && st.Current.EventType == domain.EventRaid
	})

	st, err := mgr.ProfileStatus(p.ID)
	if err != nil {
		t.Fatalf("ProfileStatus() error = %v", err)
	}
	if st.TotalPreempted != 1 {
		t.Errorf("TotalPreempted = %d, want 1", st.TotalPreempted)
	}
	if !st.ReplayAvailable {
		t.Error("ReplayAvailable = false, want true (the preempted follow alert is the safe replay snapshot)")
	}
}
