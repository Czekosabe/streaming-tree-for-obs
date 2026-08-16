// Package goals is the Stage 18A runtime: the ONE Engagement Event Bus
// subscription that turns real, normalized engagement.Event values into
// atomic contributions against internal/domain/goals's own persisted
// goals - never a second accumulation engine, and never a direct call
// into internal/provider/twitch, internal/provider/youtube, or
// internal/provider/streamelements (docs/goals-widgets.md §26).
//
// Deliberately does not hold anything about a goal's own configuration
// or accumulated value - see internal/domain/goals for that. Mirrors
// internal/alerts's own split between "what is configured" (the sibling
// domain package) and "what is happening right now" (this package).
package goals

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	domain "github.com/streaming-tree/server/internal/domain/goals"
	bus "github.com/streaming-tree/server/internal/engagement"
)

// clock returns the current time; injected so tests are deterministic.
type clock func() time.Time

// resubscribeBackoff bounds how quickly the shared Event Bus
// subscription retries after an unexpected Subscribe failure - mirrors
// internal/alerts's own identical constant/reasoning exactly.
const resubscribeBackoff = time.Second

// appliedEventRetention is the durable dedupe ledger's bounded retention
// window (docs/goals-widgets.md §11.5).
const appliedEventRetention = 30 * 24 * time.Hour

// pruneInterval is how often the manager sweeps expired ledger rows -
// generous enough that pruning is never a meaningful load, frequent
// enough that the table never grows unbounded between backend restarts
// on a long-running install.
const pruneInterval = 6 * time.Hour

// ManagerOptions configures a Manager.
type ManagerOptions struct {
	DomainService *domain.Service
	Bus           *bus.Bus
	// Now is a test-only fake-clock override; production code leaves it
	// nil.
	Now clock
}

// Manager is the Stage 18A goals runtime: the CRUD façade's own
// contribution counterpart, subscribing to the Event Bus at current
// position only (docs/goals-widgets.md §10 - never replaying retained
// history into a goal) and applying every accepted, matching event
// through domain/goals.Service.ApplyContribution's own atomic,
// per-goal, durably-deduplicated transaction (§11-§12).
type Manager struct {
	domainSvc *domain.Service
	source    *bus.Bus
	now       clock

	subscribed atomic.Bool

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewManager builds a Manager. Call Start to begin consuming the Event
// Bus.
func NewManager(opts ManagerOptions) *Manager {
	now := opts.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{domainSvc: opts.DomainService, source: opts.Bus, now: now, ctx: ctx, cancel: cancel}
}

// Start begins the Event Bus subscription and the periodic dedupe-ledger
// prune, both running until Shutdown.
func (m *Manager) Start(context.Context) error {
	m.wg.Add(2)
	go m.runSubscription()
	go m.runPrune()
	return nil
}

// Shutdown stops the subscription and prune loops, waiting for them to
// exit, bounded by ctx.
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
// subscription goroutine before publishing an event, rather than racing
// it (mirrors internal/alerts.Manager.Subscribed's identical reasoning:
// Subscribe's own "resume from current position" semantics mean an
// event published before the first successful Subscribe call is
// silently missed, not replayed - by design).
func (m *Manager) Subscribed() bool { return m.subscribed.Load() }

func (m *Manager) runSubscription() {
	defer m.wg.Done()
	for {
		select {
		case <-m.ctx.Done():
			return
		default:
		}

		// Current position only, zero replay (docs/goals-widgets.md
		// §10): Subscribe(0) is NOT "no replay" - Bus.Subscribe always
		// calls ring.after(after) unconditionally, and ring.after(0)
		// returns every retained event with Sequence > 0, i.e.
		// everything still in the ring. The only way to truly start at
		// "now" is to snapshot the newest sequence first and subscribe
		// from there - mirrors internal/alerts.Manager's own identical
		// reconnect logic exactly, for the exact same reason. A
		// reconnect after a drop resumes at whatever is current *then*,
		// never replaying anything the manager already consumed.
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
		m.subscribed.Store(true)
		m.consume(sub)
		m.subscribed.Store(false)
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

func (m *Manager) runPrune() {
	defer m.wg.Done()
	ticker := time.NewTicker(pruneInterval)
	defer ticker.Stop()
	m.prune()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.prune()
		}
	}
}

func (m *Manager) prune() {
	if m.domainSvc == nil {
		return
	}
	_, _ = m.domainSvc.PruneAppliedEvents(m.ctx, m.now().Add(-appliedEventRetention))
}
