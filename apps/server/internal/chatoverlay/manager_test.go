package chatoverlay

import (
	"context"
	"testing"
	"time"

	chatoverlaydomain "github.com/streaming-tree/server/internal/domain/chatoverlay"
	operatorchat "github.com/streaming-tree/server/internal/operatorchat"
)

// fakeUpstreamSub is a controllable stand-in for *operatorchat.Subscription -
// Manager only needs Items()/Cancel(), see upstreamSubscription in
// manager.go.
type fakeUpstreamSub struct {
	ch        chan operatorchat.Item
	cancelled chan struct{}
}

func newFakeUpstreamSub() *fakeUpstreamSub {
	return &fakeUpstreamSub{ch: make(chan operatorchat.Item, 64), cancelled: make(chan struct{})}
}

func (s *fakeUpstreamSub) Items() <-chan operatorchat.Item { return s.ch }
func (s *fakeUpstreamSub) Cancel() {
	select {
	case <-s.cancelled:
	default:
		close(s.cancelled)
		close(s.ch)
	}
}

// fakeUpstream is a controllable stand-in for the real
// internal/operatorchat.Projection - a single fixed subscription (this
// package only ever asks for one, at Manager.Start) plus the same
// fakeSource used by projection_test.go for ItemsAfter/Configure replay.
type fakeUpstream struct {
	fakeSource
	sub *fakeUpstreamSub
}

func newFakeUpstream() *fakeUpstream {
	return &fakeUpstream{sub: newFakeUpstreamSub()}
}

func (u *fakeUpstream) Subscribe(after uint64) (upstreamSubscription, bool, error) {
	return u.sub, false, nil
}

type fakeResolver struct {
	settings map[string]resolvedSettings
	calls    map[string]int
}

func newFakeResolver() *fakeResolver {
	return &fakeResolver{settings: make(map[string]resolvedSettings), calls: make(map[string]int)}
}

func (r *fakeResolver) set(overlayID string, s resolvedSettings) {
	r.settings[overlayID] = s
}

func (r *fakeResolver) Resolve(ctx context.Context, overlayID string) (resolvedSettings, error) {
	r.calls[overlayID]++
	s, ok := r.settings[overlayID]
	if !ok {
		return resolvedSettings{}, ErrNotFound
	}
	return s, nil
}

func newTestManager(t *testing.T) (*Manager, *fakeUpstream, *fakeResolver) {
	t.Helper()
	upstream := newFakeUpstream()
	resolver := newFakeResolver()
	m := NewManager(upstream, resolver, nil)
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		m.Shutdown(ctx)
	})
	return m, upstream, resolver
}

func waitForCurrentItems(t *testing.T, p *Projection, want int, timeout time.Duration) []Item {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if items := p.CurrentItems(); len(items) == want {
			return items
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d visible items, got %d", want, len(p.CurrentItems()))
	return nil
}

func TestManagerDispatchFansOutToMultipleOverlaysIndependently(t *testing.T) {
	m, upstream, resolver := newTestManager(t)

	allAccounts := testSettings(func(p *chatoverlaydomain.Profile) { p.ID = "ov_all" })
	onlyAcct2 := testSettings(func(p *chatoverlaydomain.Profile) { p.ID = "ov_acct2" })
	onlyAcct2.accountIDs = toSet([]string{"acct_2"})
	resolver.set("ov_all", allAccounts)
	resolver.set("ov_acct2", onlyAcct2)

	pAll, err := m.EnsureOverlay(context.Background(), "ov_all")
	if err != nil {
		t.Fatalf("EnsureOverlay(ov_all) error = %v", err)
	}
	pAcct2, err := m.EnsureOverlay(context.Background(), "ov_acct2")
	if err != nil {
		t.Fatalf("EnsureOverlay(ov_acct2) error = %v", err)
	}

	item := messageItem("m1", "acct_1", "u1", "viewer", "hello")
	upstream.add(item)
	upstream.sub.ch <- item

	waitForCurrentItems(t, pAll, 1, time.Second)
	waitForCurrentItems(t, pAcct2, 0, time.Second)
}

func TestManagerDispatchOneRecoversFromPanicInOneOverlayWithoutAffectingOthers(t *testing.T) {
	m, upstream, resolver := newTestManager(t)
	resolver.set("ov_broken", testSettings(func(p *chatoverlaydomain.Profile) { p.ID = "ov_broken" }))
	resolver.set("ov_ok", testSettings(func(p *chatoverlaydomain.Profile) { p.ID = "ov_ok" }))

	broken, err := m.EnsureOverlay(context.Background(), "ov_broken")
	if err != nil {
		t.Fatalf("EnsureOverlay(ov_broken) error = %v", err)
	}
	ok, err := m.EnsureOverlay(context.Background(), "ov_ok")
	if err != nil {
		t.Fatalf("EnsureOverlay(ov_ok) error = %v", err)
	}

	// Deliberately corrupt ov_broken's internal state to force a genuine
	// panic inside HandleUpstreamItem, simulating an internal bug in one
	// overlay's projection - dispatchOne's own recover must contain it.
	broken.mu.Lock()
	broken.ring = nil
	broken.mu.Unlock()

	item := messageItem("m1", "acct_1", "u1", "viewer", "hello")
	upstream.add(item)

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("expected Manager.dispatch to recover ov_broken's panic, but it escaped: %v", r)
			}
		}()
		m.dispatch(item)
	}()

	waitForCurrentItems(t, ok, 1, time.Second)
}

func TestManagerEnsureOverlayReusesExistingProjection(t *testing.T) {
	m, _, resolver := newTestManager(t)
	resolver.set("ov_1", testSettings(func(p *chatoverlaydomain.Profile) { p.ID = "ov_1" }))

	first, err := m.EnsureOverlay(context.Background(), "ov_1")
	if err != nil {
		t.Fatalf("EnsureOverlay() error = %v", err)
	}
	second, err := m.EnsureOverlay(context.Background(), "ov_1")
	if err != nil {
		t.Fatalf("EnsureOverlay() error = %v", err)
	}
	if first != second {
		t.Error("expected the second EnsureOverlay call to return the same running Projection")
	}
	if resolver.calls["ov_1"] != 1 {
		t.Errorf("resolver was called %d times, want exactly 1 (no re-resolve on reuse)", resolver.calls["ov_1"])
	}
}

func TestManagerRebuildReResolvesAndProducesReset(t *testing.T) {
	m, _, resolver := newTestManager(t)
	resolver.set("ov_1", testSettings(func(p *chatoverlaydomain.Profile) { p.ID = "ov_1" }))

	p, err := m.EnsureOverlay(context.Background(), "ov_1")
	if err != nil {
		t.Fatalf("EnsureOverlay() error = %v", err)
	}
	sub, _, _ := p.Subscribe(0)
	defer sub.Cancel()
	waitRevision(time.Second, sub.Revisions()) // initial (empty) reset from EnsureOverlay's own Configure

	if err := m.Rebuild(context.Background(), "ov_1"); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	if resolver.calls["ov_1"] != 2 {
		t.Errorf("resolver was called %d times, want 2 (once for Ensure, once for Rebuild)", resolver.calls["ov_1"])
	}
	rev, ok := waitRevision(time.Second, sub.Revisions())
	if !ok || rev.Operation != OpReset {
		t.Fatalf("expected Rebuild to produce a reset, got %+v, ok=%v", rev, ok)
	}
}

func TestManagerRebuildAllRebuildsEveryRegisteredOverlay(t *testing.T) {
	m, _, resolver := newTestManager(t)
	resolver.set("ov_a", testSettings(func(p *chatoverlaydomain.Profile) { p.ID = "ov_a" }))
	resolver.set("ov_b", testSettings(func(p *chatoverlaydomain.Profile) { p.ID = "ov_b" }))
	if _, err := m.EnsureOverlay(context.Background(), "ov_a"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.EnsureOverlay(context.Background(), "ov_b"); err != nil {
		t.Fatal(err)
	}

	m.RebuildAll(context.Background())

	if resolver.calls["ov_a"] != 2 || resolver.calls["ov_b"] != 2 {
		t.Errorf("expected both overlays re-resolved once by RebuildAll, got calls = %v", resolver.calls)
	}
}

func TestManagerRemoveClosesSubscriberWithReasonOverlayDeleted(t *testing.T) {
	m, _, resolver := newTestManager(t)
	resolver.set("ov_1", testSettings(func(p *chatoverlaydomain.Profile) { p.ID = "ov_1" }))
	p, err := m.EnsureOverlay(context.Background(), "ov_1")
	if err != nil {
		t.Fatal(err)
	}
	sub, _, _ := p.Subscribe(0)

	m.Remove(context.Background(), "ov_1")

	select {
	case reason := <-sub.Closed():
		if reason != ReasonOverlayDeleted {
			t.Errorf("close reason = %q, want %q", reason, ReasonOverlayDeleted)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the subscription to close after Remove")
	}

	if _, ok := m.Get("ov_1"); ok {
		t.Error("expected the overlay to no longer be registered after Remove")
	}
}

func TestManagerShutdownClosesUpstreamSubscriptionAndOverlays(t *testing.T) {
	upstream := newFakeUpstream()
	resolver := newFakeResolver()
	resolver.set("ov_1", testSettings(func(p *chatoverlaydomain.Profile) { p.ID = "ov_1" }))
	m := NewManager(upstream, resolver, nil)
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	p, err := m.EnsureOverlay(context.Background(), "ov_1")
	if err != nil {
		t.Fatal(err)
	}
	sub, _, _ := p.Subscribe(0)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	m.Shutdown(ctx)

	select {
	case <-sub.Closed():
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the overlay's own subscriber to close after Manager.Shutdown")
	}
	select {
	case <-upstream.sub.cancelled:
	default:
		t.Error("expected Manager.Shutdown to cancel the shared upstream subscription")
	}
}
