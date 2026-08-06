package chatoverlay

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	operatorchat "github.com/streaming-tree/server/internal/operatorchat"
)

// upstreamSubscription is the subset of *operatorchat.Subscription this
// package consumes - an interface (rather than the concrete type
// directly) so a test can fake the upstream projection without wiring a
// real internal/engagement.Bus.
type upstreamSubscription interface {
	Items() <-chan operatorchat.Item
	Cancel()
}

// UpstreamSource is what Manager needs from the shared operator-chat
// projection: ItemsAfter for a settings-change rebuild's replay (see
// Projection.Configure), plus a single subscription this Manager fans
// out to every registered overlay - never one operator-chat subscription
// per overlay, and this package never subscribes to the Engagement Event
// Bus directly (see this package's own doc comment).
type UpstreamSource interface {
	OperatorChatSource
	Subscribe(after uint64) (sub upstreamSubscription, gap bool, err error)
}

// Manager owns every active overlay's Projection, and the single shared
// subscription to the upstream operator-chat projection that feeds all
// of them. One overlay's panic (recovered per-dispatch) or one overlay's
// slow subscriber never affects another overlay or the shared dispatch
// loop.
type Manager struct {
	upstream UpstreamSource
	resolver SettingsResolver
	logger   *slog.Logger

	mu          sync.Mutex
	projections map[string]*Projection
	started     bool

	upstreamSub upstreamSubscription
	lifecycle   context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

// NewManager builds a Manager. Call Start before registering any
// overlay.
func NewManager(upstream UpstreamSource, resolver SettingsResolver, logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{
		upstream:    upstream,
		resolver:    resolver,
		logger:      logger,
		projections: make(map[string]*Projection),
	}
}

// Start subscribes once to the upstream operator-chat projection and
// begins fanning out every item to every registered overlay's own
// Projection.
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return nil
	}
	sub, _, err := m.upstream.Subscribe(0)
	if err != nil {
		m.mu.Unlock()
		return fmt.Errorf("subscribe to operator chat: %w", err)
	}
	m.upstreamSub = sub
	m.lifecycle, m.cancel = context.WithCancel(context.Background())
	m.started = true
	m.mu.Unlock()

	m.wg.Add(1)
	go m.dispatchLoop(sub)
	return nil
}

func (m *Manager) dispatchLoop(sub upstreamSubscription) {
	defer m.wg.Done()
	for item := range sub.Items() {
		m.dispatch(item)
	}
}

// dispatch fans one upstream item out to every currently registered
// overlay, recovering from a panic in any single overlay's own
// evaluation so it can never take down this shared loop or another
// overlay.
func (m *Manager) dispatch(item operatorchat.Item) {
	m.mu.Lock()
	projections := make([]*Projection, 0, len(m.projections))
	for _, p := range m.projections {
		projections = append(projections, p)
	}
	m.mu.Unlock()

	for _, p := range projections {
		m.dispatchOne(p, item)
	}
}

func (m *Manager) dispatchOne(p *Projection, item operatorchat.Item) {
	defer func() {
		if r := recover(); r != nil {
			m.logger.Error("chat overlay projection panicked while handling an upstream item; overlay left running",
				"overlay_id", p.overlayID, "panic", r)
		}
	}()
	p.HandleUpstreamItem(item)
}

// Shutdown cancels the upstream subscription and shuts down every
// registered overlay's own Projection.
func (m *Manager) Shutdown(ctx context.Context) {
	m.mu.Lock()
	if !m.started {
		m.mu.Unlock()
		return
	}
	m.started = false
	if m.upstreamSub != nil {
		m.upstreamSub.Cancel()
	}
	if m.cancel != nil {
		m.cancel()
	}
	projections := make([]*Projection, 0, len(m.projections))
	for _, p := range m.projections {
		projections = append(projections, p)
	}
	m.projections = make(map[string]*Projection)
	m.mu.Unlock()

	m.wg.Wait()
	for _, p := range projections {
		p.Shutdown(ctx)
	}
}

// getOrCreateProjection returns overlayID's Projection, starting (but
// deliberately never configuring) a new one if this is the first time
// it's been referenced since the server started or since it was last
// removed - configuring is always the caller's own job (EnsureOverlay
// and Rebuild each do it exactly once, with their own freshly resolved
// settings), so this helper alone is never responsible for a public
// reset.
func (m *Manager) getOrCreateProjection(ctx context.Context, overlayID string) *Projection {
	m.mu.Lock()
	if p, ok := m.projections[overlayID]; ok {
		m.mu.Unlock()
		return p
	}
	m.mu.Unlock()

	p := NewProjection(overlayID, m.upstream, nil, m.logger)
	p.Start(ctx)

	m.mu.Lock()
	if existing, ok := m.projections[overlayID]; ok {
		m.mu.Unlock()
		p.Shutdown(ctx)
		return existing
	}
	m.projections[overlayID] = p
	m.mu.Unlock()
	return p
}

// EnsureOverlay returns the running Projection for overlayID, creating
// and configuring one from durable storage if this is the first time
// it's been requested since the server started (or since it was last
// removed). Already-running overlays are returned as-is, without a
// redundant re-resolve/re-configure - use Rebuild to apply a settings
// change to one that already exists.
func (m *Manager) EnsureOverlay(ctx context.Context, overlayID string) (*Projection, error) {
	m.mu.Lock()
	if p, ok := m.projections[overlayID]; ok {
		m.mu.Unlock()
		return p, nil
	}
	m.mu.Unlock()

	settings, err := m.resolver.Resolve(ctx, overlayID)
	if err != nil {
		return nil, err
	}
	p := m.getOrCreateProjection(ctx, overlayID)
	p.Configure(settings)
	return p, nil
}

// Get returns the already-running Projection for overlayID, if any -
// callers that must not implicitly create one (e.g. a public-endpoint
// handler for a disabled or unknown overlay) use this instead of
// EnsureOverlay.
func (m *Manager) Get(overlayID string) (*Projection, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.projections[overlayID]
	return p, ok
}

// Rebuild re-resolves overlayID's settings from durable storage and
// applies them to its running Projection (creating one first if none is
// running yet), producing exactly one public reset - called whenever a
// profile's settings, account selection, hidden users, blocked terms, or
// activity types change, and whenever Stage 9's own shared bot-user list
// changes.
func (m *Manager) Rebuild(ctx context.Context, overlayID string) error {
	settings, err := m.resolver.Resolve(ctx, overlayID)
	if err != nil {
		return err
	}
	p := m.getOrCreateProjection(ctx, overlayID)
	p.Configure(settings)
	return nil
}

// RebuildAll re-resolves and applies every currently running overlay's
// settings - used when Stage 9's shared bot-user list changes, since
// that list is not scoped to any single overlay.
func (m *Manager) RebuildAll(ctx context.Context) {
	m.mu.Lock()
	ids := make([]string, 0, len(m.projections))
	for id := range m.projections {
		ids = append(ids, id)
	}
	m.mu.Unlock()

	for _, id := range ids {
		if err := m.Rebuild(ctx, id); err != nil {
			m.logger.Error("failed to rebuild chat overlay after a shared preference change", "overlay_id", id, "error", err)
		}
	}
}

// Remove shuts down and forgets overlayID's Projection, closing any live
// subscriber with ReasonOverlayDeleted - called when a profile is
// deleted. Safe to call for an overlay with no running Projection.
func (m *Manager) Remove(ctx context.Context, overlayID string) {
	m.mu.Lock()
	p, ok := m.projections[overlayID]
	if ok {
		delete(m.projections, overlayID)
	}
	m.mu.Unlock()

	if ok {
		p.closeWithReason(ctx, ReasonOverlayDeleted)
	}
}
