package chatautomation

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/account"
	domain "github.com/streaming-tree/server/internal/domain/chatautomation"
	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/outboundchat"
)

// --- shared fakes -----------------------------------------------------

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock { return &fakeClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)} }
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

type fakeAccounts struct {
	mu       sync.Mutex
	accounts map[string]account.Account
}

func newFakeAccounts(accounts ...account.Account) *fakeAccounts {
	f := &fakeAccounts{accounts: make(map[string]account.Account)}
	for _, a := range accounts {
		f.accounts[a.ID] = a
	}
	return f
}
func (f *fakeAccounts) GetAccount(_ context.Context, id string) (account.Account, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	acc, ok := f.accounts[id]
	if !ok {
		return account.Account{}, account.ErrNotFound
	}
	return acc, nil
}
func (f *fakeAccounts) WithFreshToken(_ context.Context, _ string, fn func(accessToken string) (bool, error)) error {
	_, err := fn("access-token")
	return err
}
func (f *fakeAccounts) EffectiveClientID(context.Context, account.ProviderID) (string, error) {
	return "client-id", nil
}

type fakePlatforms map[string]platform.Platform

func (f fakePlatforms) Get(_ context.Context, id string) (platform.Platform, error) {
	p, ok := f[id]
	if !ok {
		return platform.Platform{}, platform.ErrNotFound
	}
	return p, nil
}

type fakeIngest struct {
	mu        sync.Mutex
	receiving bool
	since     time.Time
	hasSince  bool
}

func (f *fakeIngest) IsReceiving() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.receiving
}
func (f *fakeIngest) ReceivingSince() (time.Time, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.since, f.hasSince
}
func (f *fakeIngest) setReceiving(v bool, since time.Time) {
	f.mu.Lock()
	f.receiving, f.since, f.hasSince = v, since, v
	f.mu.Unlock()
}

type fakeOutboundProvider struct {
	mu    sync.Mutex
	calls []outboundchat.SendMessageRequest
	fail  error
}

func (p *fakeOutboundProvider) ProviderID() account.ProviderID { return account.ProviderTwitch }
func (p *fakeOutboundProvider) AssessCapability(acc account.Account) outboundchat.Capability {
	// SupportsReply: true mirrors the real Twitch adapter's own capability
	// (internal/provider/twitch/outbound_chat_adapter.go) - this fake
	// simulates a Twitch-like provider throughout this package's test
	// suite, and TestCommandEngineUsesSourceCommandAndReply specifically
	// depends on a reply actually being requested.
	return outboundchat.Capability{Required: []string{"user:write:chat"}, Available: true, SupportsReply: true}
}
func (p *fakeOutboundProvider) SendChatMessage(_ context.Context, _ account.Account, _ account.TokenBundle, _ string, req outboundchat.SendMessageRequest) (outboundchat.SendMessageResult, error) {
	p.mu.Lock()
	p.calls = append(p.calls, req)
	fail := p.fail
	p.mu.Unlock()
	if fail != nil {
		return outboundchat.SendMessageResult{}, fail
	}
	return outboundchat.SendMessageResult{Sent: true, ProviderMessageID: "m_" + req.Message, CompletedAt: time.Now()}, nil
}
func (p *fakeOutboundProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.calls)
}
func (p *fakeOutboundProvider) lastCall() outboundchat.SendMessageRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls[len(p.calls)-1]
}

func newTestDispatcher(clock *fakeClock, accounts *fakeAccounts, provider *fakeOutboundProvider) *dispatcher {
	m := outboundchat.NewManager(outboundchat.ManagerOptions{
		Accounts: accounts, Providers: []outboundchat.Provider{provider}, Now: clock.Now,
	})
	return newDispatcher(m)
}

func constantRand(v float64) func() float64 { return func() float64 { return v } }

func testAccount(id string) account.Account {
	return account.Account{
		ID: id, ProviderID: account.ProviderTwitch, ProviderUserID: id + "_puid",
		Login: "streamer", DisplayName: "Streamer", Status: account.StatusConnected,
		Scopes: []string{"user:write:chat"},
	}
}

func testSchedule(id string, targets ...string) domain.Schedule {
	var t []domain.Target
	for _, a := range targets {
		t = append(t, domain.Target{AccountID: a})
	}
	return domain.Schedule{
		ID: id, Name: "test", Enabled: true, IntervalSeconds: 60, MaximumSendsPerHour: 60,
		Targets:  t,
		Messages: []domain.ScheduleMessage{{ID: "m1", MessageTemplate: "hello"}},
	}
}

// --- tests --------------------------------------------------------------

func TestSchedulerStartupSafetyFloorAppliesWhenFirstDelayZero(t *testing.T) {
	clock := newFakeClock()
	s := newScheduler(clock.Now, constantRand(0), &fakeIngest{}, newFakeAccounts(), fakePlatforms{}, nil)

	def := testSchedule("sched_1", "acct_1")
	def.FirstDelaySeconds = 0
	base, fireAt := s.firstDue(def)
	if base.Before(clock.Now().Add(startupSafetyFloor)) {
		t.Errorf("base = %v, want at least %v later", base, startupSafetyFloor)
	}
	if fireAt.Before(base) {
		t.Errorf("fireAt = %v, want >= base %v", fireAt, base)
	}
}

func TestSchedulerFirstDelayHonoredExactly(t *testing.T) {
	clock := newFakeClock()
	s := newScheduler(clock.Now, constantRand(0), &fakeIngest{}, newFakeAccounts(), fakePlatforms{}, nil)

	def := testSchedule("sched_1", "acct_1")
	def.FirstDelaySeconds = 120
	base, _ := s.firstDue(def)
	want := clock.Now().Add(120 * time.Second)
	if !base.Equal(want) {
		t.Errorf("base = %v, want %v", base, want)
	}
}

func TestSchedulerJitterNeverEarlierThanBaseAndBounded(t *testing.T) {
	clock := newFakeClock()
	s := newScheduler(clock.Now, constantRand(0.999), &fakeIngest{}, newFakeAccounts(), fakePlatforms{}, nil)

	def := testSchedule("sched_1", "acct_1")
	def.FirstDelaySeconds = 100
	def.JitterSeconds = 30
	base, fireAt := s.firstDue(def)
	if fireAt.Before(base) {
		t.Errorf("fireAt = %v must never be before base %v", fireAt, base)
	}
	if fireAt.After(base.Add(30 * time.Second)) {
		t.Errorf("fireAt = %v exceeds base+jitterMax %v", fireAt, base.Add(30*time.Second))
	}
}

func TestSchedulerIntervalRecurrenceAdvancesFromBaseNotFromNow(t *testing.T) {
	clock := newFakeClock()
	s := newScheduler(clock.Now, constantRand(0), &fakeIngest{}, newFakeAccounts(), fakePlatforms{}, nil)

	sr := &scheduleRuntime{def: testSchedule("sched_1", "acct_1"), perAccount: map[string]*accountTargetState{}}
	sr.nextBaseDue = clock.Now().Add(60 * time.Second)
	sr.nextFireAt = sr.nextBaseDue

	firstBase := sr.nextBaseDue
	s.advanceDueLocked(sr, clock.Now())
	if !sr.nextBaseDue.Equal(firstBase.Add(60 * time.Second)) {
		t.Errorf("nextBaseDue = %v, want exactly one interval after the previous base %v", sr.nextBaseDue, firstBase)
	}
}

func TestSchedulerOnlyWhileIngestReceivingSkipsWhenNotReceiving(t *testing.T) {
	clock := newFakeClock()
	accounts := newFakeAccounts(testAccount("acct_1"))
	provider := &fakeOutboundProvider{}
	dispatch := newTestDispatcher(clock, accounts, provider)
	ingest := &fakeIngest{}
	s := newScheduler(clock.Now, constantRand(0), ingest, accounts, fakePlatforms{}, dispatch)

	def := testSchedule("sched_1", "acct_1")
	def.OnlyWhileIngestReceiving = true
	sr := &scheduleRuntime{def: def, perAccount: map[string]*accountTargetState{}}

	result := s.executeOneTarget(sr, def, def.Targets[0], false /* not receiving */, false)
	if result.Sent || result.SkipReason != SkipStreamNotReceiving {
		t.Errorf("result = %+v, want skipped with SkipStreamNotReceiving", result)
	}
	if provider.callCount() != 0 {
		t.Errorf("provider was called %d times, want 0", provider.callCount())
	}
}

func TestSchedulerOnlyWhileIngestReceivingSendsWhenReceiving(t *testing.T) {
	clock := newFakeClock()
	accounts := newFakeAccounts(testAccount("acct_1"))
	provider := &fakeOutboundProvider{}
	dispatch := newTestDispatcher(clock, accounts, provider)
	s := newScheduler(clock.Now, constantRand(0), &fakeIngest{}, accounts, fakePlatforms{}, dispatch)

	def := testSchedule("sched_1", "acct_1")
	def.OnlyWhileIngestReceiving = true
	sr := &scheduleRuntime{def: def, perAccount: map[string]*accountTargetState{}}

	result := s.executeOneTarget(sr, def, def.Targets[0], true, false)
	if !result.Sent {
		t.Errorf("result = %+v, want Sent", result)
	}
	if provider.callCount() != 1 {
		t.Errorf("provider was called %d times, want 1", provider.callCount())
	}
}

func TestSchedulerMinimumChatActivityBlocksThenAllows(t *testing.T) {
	clock := newFakeClock()
	accounts := newFakeAccounts(testAccount("acct_1"))
	provider := &fakeOutboundProvider{}
	dispatch := newTestDispatcher(clock, accounts, provider)
	s := newScheduler(clock.Now, constantRand(0), &fakeIngest{}, accounts, fakePlatforms{}, dispatch)

	def := testSchedule("sched_1", "acct_1")
	def.MinimumChatMessages = 3
	sr := &scheduleRuntime{def: def, perAccount: map[string]*accountTargetState{}}

	result := s.executeOneTarget(sr, def, def.Targets[0], true, false)
	if result.Sent || result.SkipReason != SkipActivityInsufficient {
		t.Errorf("result = %+v, want SkipActivityInsufficient", result)
	}

	// scheduler.recordActivity only touches schedules it is tracking
	// itself (via s.schedules, populated through reload()) - this test
	// builds sr by hand instead, so bump the runtime counter directly.
	sr.accountState("acct_1").activityCount = 3

	result = s.executeOneTarget(sr, def, def.Targets[0], true, false)
	if !result.Sent {
		t.Errorf("result = %+v, want Sent once activity threshold is met", result)
	}
}

func TestSchedulerActivityResetsAfterSuccessfulSend(t *testing.T) {
	clock := newFakeClock()
	accounts := newFakeAccounts(testAccount("acct_1"))
	provider := &fakeOutboundProvider{}
	dispatch := newTestDispatcher(clock, accounts, provider)
	s := newScheduler(clock.Now, constantRand(0), &fakeIngest{}, accounts, fakePlatforms{}, dispatch)

	def := testSchedule("sched_1", "acct_1")
	sr := &scheduleRuntime{def: def, perAccount: map[string]*accountTargetState{}}
	sr.accountState("acct_1").activityCount = 5

	s.executeOneTarget(sr, def, def.Targets[0], true, false)
	if got := sr.accountState("acct_1").activityCount; got != 0 {
		t.Errorf("activityCount after success = %d, want 0", got)
	}
}

func TestSchedulerMaximumSendsPerHourEnforced(t *testing.T) {
	clock := newFakeClock()
	accounts := newFakeAccounts(testAccount("acct_1"))
	provider := &fakeOutboundProvider{}
	dispatch := newTestDispatcher(clock, accounts, provider)
	s := newScheduler(clock.Now, constantRand(0), &fakeIngest{}, accounts, fakePlatforms{}, dispatch)

	def := testSchedule("sched_1", "acct_1")
	def.MaximumSendsPerHour = 1
	sr := &scheduleRuntime{def: def, perAccount: map[string]*accountTargetState{}}

	first := s.executeOneTarget(sr, def, def.Targets[0], true, false)
	if !first.Sent {
		t.Fatalf("first send = %+v, want Sent", first)
	}
	second := s.executeOneTarget(sr, def, def.Targets[0], true, false)
	if second.Sent || second.SkipReason != SkipRateLimited {
		t.Errorf("second send = %+v, want SkipRateLimited", second)
	}

	clock.Advance(61 * time.Minute)
	third := s.executeOneTarget(sr, def, def.Targets[0], true, false)
	if !third.Sent {
		t.Errorf("third send after the rolling hour passed = %+v, want Sent", third)
	}
}

func TestSchedulerMessageGroupAvoidsImmediateRepeat(t *testing.T) {
	messages := []domain.ScheduleMessage{
		{ID: "m1", MessageTemplate: "one"},
		{ID: "m2", MessageTemplate: "two"},
	}
	// randFrac always returns 0, which would pick index 0 of the
	// candidate list every time - but the previous message must be
	// excluded from the candidate list, so the second call must return
	// the OTHER message even with an unchanged, always-0 random source.
	_, id1 := selectMessage(messages, "", constantRand(0))
	if id1 != "m1" {
		t.Fatalf("first selection = %q, want m1 (no previous to exclude)", id1)
	}
	_, id2 := selectMessage(messages, id1, constantRand(0))
	if id2 == id1 {
		t.Errorf("second selection repeated %q immediately, want the other message", id2)
	}
}

func TestSchedulerSendNowIgnoresActivityButRespectsIngest(t *testing.T) {
	clock := newFakeClock()
	accounts := newFakeAccounts(testAccount("acct_1"))
	provider := &fakeOutboundProvider{}
	dispatch := newTestDispatcher(clock, accounts, provider)
	ingest := &fakeIngest{}
	s := newScheduler(clock.Now, constantRand(0), ingest, accounts, fakePlatforms{}, dispatch)

	def := testSchedule("sched_1", "acct_1")
	def.MinimumChatMessages = 100
	def.OnlyWhileIngestReceiving = true
	sr := &scheduleRuntime{def: def, perAccount: map[string]*accountTargetState{}}

	blocked := s.sendNow(sr, nil)
	if len(blocked) != 1 || blocked[0].Sent || blocked[0].SkipReason != SkipStreamNotReceiving {
		t.Errorf("sendNow while not receiving = %+v, want SkipStreamNotReceiving (ingest check still applies)", blocked)
	}

	ingest.setReceiving(true, clock.Now())
	results := s.sendNow(sr, nil)
	if len(results) != 1 || !results[0].Sent {
		t.Errorf("sendNow while receiving = %+v, want Sent even though MinimumChatMessages=100 was never reached", results)
	}
}

func TestSchedulerNoDuplicateTimerExecutionWithinOneTick(t *testing.T) {
	clock := newFakeClock()
	accounts := newFakeAccounts(testAccount("acct_1"))
	provider := &fakeOutboundProvider{}
	dispatch := newTestDispatcher(clock, accounts, provider)
	s := newScheduler(clock.Now, constantRand(0), &fakeIngest{}, accounts, fakePlatforms{}, dispatch)

	s.reload([]domain.Schedule{testSchedule("sched_1", "acct_1")})
	sr, _ := s.get("sched_1")
	sr.mu.Lock()
	sr.nextBaseDue = clock.Now()
	sr.nextFireAt = clock.Now()
	sr.mu.Unlock()

	// Calling tick() repeatedly without advancing the clock must only
	// ever pick up the due schedule once per due moment - advanceDueLocked
	// moves nextFireAt forward by a full interval immediately, before the
	// execution goroutine is even spawned.
	s.tick()
	s.tick()
	s.tick()

	// Give the one spawned execution goroutine a moment to finish.
	deadline := time.Now().Add(2 * time.Second)
	for provider.callCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond)

	if got := provider.callCount(); got != 1 {
		t.Errorf("provider was called %d times across 3 ticks with no clock advance, want exactly 1", got)
	}
}

func TestSchedulerBuildContextResolvesStreamTitleFromPlatform(t *testing.T) {
	clock := newFakeClock()
	accounts := newFakeAccounts(testAccount("acct_1"))
	platforms := fakePlatforms{"pf_1": platform.Platform{ID: "pf_1", Metadata: platform.Metadata{Title: "Ranked grind"}}}
	s := newScheduler(clock.Now, constantRand(0), &fakeIngest{}, accounts, platforms, nil)

	ctx, err := s.buildContext(context.Background(), domain.Target{AccountID: "acct_1", PlatformID: "pf_1"}, testSchedule("sched_1", "acct_1"))
	if err != nil {
		t.Fatalf("buildContext() error = %v", err)
	}
	if ctx.StreamTitle == nil || *ctx.StreamTitle != "Ranked grind" {
		t.Errorf("ctx.StreamTitle = %v, want \"Ranked grind\"", ctx.StreamTitle)
	}
	if ctx.ChannelURL != "https://www.twitch.tv/streamer" {
		t.Errorf("ctx.ChannelURL = %q, want the Twitch channel URL", ctx.ChannelURL)
	}
}

func TestSchedulerBuildContextStreamTitleUnresolvedWithoutPlatform(t *testing.T) {
	clock := newFakeClock()
	accounts := newFakeAccounts(testAccount("acct_1"))
	s := newScheduler(clock.Now, constantRand(0), &fakeIngest{}, accounts, fakePlatforms{}, nil)

	ctx, err := s.buildContext(context.Background(), domain.Target{AccountID: "acct_1"}, testSchedule("sched_1", "acct_1"))
	if err != nil {
		t.Fatalf("buildContext() error = %v", err)
	}
	if ctx.StreamTitle != nil {
		t.Errorf("ctx.StreamTitle = %v, want nil (no platform context supplied)", ctx.StreamTitle)
	}
}
