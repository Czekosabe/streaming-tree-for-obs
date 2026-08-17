// Package supporterwidgets is the Stage 18B runtime: the ONE Event Bus
// subscription that turns real, normalized engagement.Event values into
// in-memory presentation projections for every enabled, non-goal,
// non-dashboard widget profile (docs/supporter-widgets.md §4). Never a
// second accumulation engine alongside internal/goals, never one
// subscription per widget profile, and never a direct call into
// internal/provider/twitch, internal/provider/youtube, or
// internal/provider/streamelements.
//
// Deliberately holds no persisted state of its own - every projection
// here is event-derived presentation content (a display name, a
// donation message, a ticker row) this project has always kept out of
// SQLite (docs/supporter-widgets.md §3). A backend restart clears every
// projection; only internal/domain/goals.WidgetProfile's own
// configuration (kind, filters, style, dashboard composition) survives,
// exactly as designed.
package supporterwidgets

import (
	"context"
	"sync"
	"time"

	engagement "github.com/streaming-tree/server/internal/domain/engagement"
	domain "github.com/streaming-tree/server/internal/domain/goals"
	bus "github.com/streaming-tree/server/internal/engagement"
)

// clock returns the current time; injected so tests are deterministic.
type clock func() time.Time

// resubscribeBackoff mirrors internal/goals.Manager's own identical
// constant/reasoning exactly.
const resubscribeBackoff = time.Second

// WidgetProfileLister is the subset of *domain/goals.Service the
// Manager needs - re-read on every event, exactly like
// internal/goals.Manager.handleEvent's own ListGoals call, so a widget
// profile's config change takes effect on the very next event with no
// separate cache-invalidation channel required (docs/supporter-
// widgets.md §4).
type WidgetProfileLister interface {
	ListWidgetProfiles(ctx context.Context, goalID string) ([]domain.WidgetProfile, error)
}

// ManagerOptions configures a Manager.
type ManagerOptions struct {
	Profiles WidgetProfileLister
	Bus      *bus.Bus
	// Now is a test-only fake-clock override; production code leaves it
	// nil.
	Now clock
}

// Manager is the Stage 18B supporter-widgets runtime - one Event Bus
// subscription at current position only (docs/supporter-widgets.md §4,
// mirroring internal/goals.Manager's own identical "never Subscribe(0)"
// pattern), one in-memory projection per matching widget profile id.
type Manager struct {
	profiles WidgetProfileLister
	source   *bus.Bus
	now      clock

	mu          sync.Mutex
	projections map[string]*projectionState

	subscribedFlag boolFlag

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// projectionState is one widget profile's own current runtime state,
// plus the last Bus sequence applied to it (docs/supporter-widgets.md
// §17's own "an internal retry can never double-apply the same event to
// the same profile" guard - defensive, since this single-subscriber
// design never actually retries, but cheap insurance).
type projectionState struct {
	projection Projection
	lastSeq    uint64
}

// boolFlag is a tiny atomic-bool substitute kept local to avoid pulling
// in sync/atomic for one field - mirrors the simplicity
// internal/goals.Manager affords itself with atomic.Bool; this package
// uses a mutex-guarded bool instead purely because Manager already holds
// mu for the projections map and a second lock would add nothing.
type boolFlag struct {
	mu sync.Mutex
	v  bool
}

func (f *boolFlag) set(v bool) { f.mu.Lock(); f.v = v; f.mu.Unlock() }
func (f *boolFlag) get() bool  { f.mu.Lock(); defer f.mu.Unlock(); return f.v }

// NewManager builds a Manager. Call Start to begin consuming the Event
// Bus.
func NewManager(opts ManagerOptions) *Manager {
	now := opts.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		profiles: opts.Profiles, source: opts.Bus, now: now,
		projections: make(map[string]*projectionState),
		ctx:         ctx, cancel: cancel,
	}
}

// Start begins the Event Bus subscription, running until Shutdown.
func (m *Manager) Start(context.Context) error {
	m.wg.Add(1)
	go m.runSubscription()
	return nil
}

// Shutdown stops the subscription loop, waiting for it to exit, bounded
// by ctx.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.cancel()
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Subscribed reports whether the Event Bus subscription is currently
// live - exposed so tests can wait deterministically for Start's own
// subscription goroutine before publishing an event, mirroring
// internal/goals.Manager.Subscribed exactly.
func (m *Manager) Subscribed() bool { return m.subscribedFlag.get() }

func (m *Manager) runSubscription() {
	defer m.wg.Done()
	for {
		select {
		case <-m.ctx.Done():
			return
		default:
		}

		// Current position only, zero replay (docs/supporter-widgets.md
		// §4) - the exact internal/goals.Manager.runSubscription
		// precedent: Subscribe(0) would replay every retained event
		// still in the ring, never what "current position" means.
		snap := m.source.Snapshot()
		sub, _, err := m.source.Subscribe(snap.NewestSequence)
		if err != nil {
			select {
			case <-m.ctx.Done():
				return
			case <-time.After(resubscribeBackoff):
				continue
			}
		}
		m.subscribedFlag.set(true)
		m.consume(sub)
		m.subscribedFlag.set(false)
	}
}

func (m *Manager) consume(sub *bus.Subscription) {
	for {
		select {
		case <-m.ctx.Done():
			sub.Cancel()
			return
		case evt, ok := <-sub.Events():
			if !ok {
				return
			}
			m.handleEvent(evt)
		case <-sub.Closed():
			return
		}
	}
}

// handleEvent applies evt to every currently enabled, matching,
// non-goal, non-dashboard widget profile (docs/supporter-widgets.md
// §6-§9). Synthetic events are rejected first, mirroring
// internal/goals.Manager.handleEvent's own identical defense-in-depth.
func (m *Manager) handleEvent(evt engagement.Event) {
	if evt.Synthetic {
		return
	}

	list, err := m.profiles.ListWidgetProfiles(m.ctx, "")
	if err != nil {
		return
	}
	for _, p := range list {
		if !p.Enabled || !p.Kind.HasOwnFilters() {
			continue
		}
		if !providerMatches(p.Providers, evt.ProviderID) {
			continue
		}
		if !accountMatches(p.Accounts, evt.ConnectedAccountID) {
			continue
		}
		m.applyToProfile(p, evt)
	}
}

func (m *Manager) applyToProfile(p domain.WidgetProfile, evt engagement.Event) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.projections[p.ID]
	if !ok {
		state = &projectionState{}
		m.projections[p.ID] = state
	}
	if evt.Sequence != 0 && evt.Sequence <= state.lastSeq {
		return // already applied - defensive guard, docs/supporter-widgets.md §17.
	}

	changed := applyEventToProjection(p, evt, &state.projection)
	if !changed {
		return
	}
	if evt.Sequence != 0 {
		state.lastSeq = evt.Sequence
	}
	state.projection.Revision++
	state.projection.UpdatedAt = m.now()
}

// Snapshot returns profileID's own current projection (the zero value,
// Revision 0, when nothing has been observed yet or the id is unknown -
// a valid, well-defined empty state, docs/supporter-widgets.md §12/§9's
// own "renderer-safe empty presentation" rule).
func (m *Manager) Snapshot(profileID string) Projection {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, ok := m.projections[profileID]
	if !ok {
		return Projection{}
	}
	return state.projection.clone()
}

// Reset clears profileID's own runtime projection - used by the manual
// "reset runtime" action and by a semantic profile edit (docs/
// supporter-widgets.md §14, §16). Never touches persisted configuration,
// never publishes an Engagement Event.
func (m *Manager) Reset(profileID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.projections, profileID)
}
