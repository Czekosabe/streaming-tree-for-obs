package chatoverlay

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	chatoverlaydomain "github.com/streaming-tree/server/internal/domain/chatoverlay"
	"github.com/streaming-tree/server/internal/domain/visualasset"
	"github.com/streaming-tree/server/internal/domain/visualdesign"
	operatorchat "github.com/streaming-tree/server/internal/operatorchat"
)

// AssetRefTracker is the narrow subset of visualasset.Service a chat
// visual design that references a Stage 14B managed asset needs -
// identical in shape to internal/alerts.AssetRefTracker, kept as its
// own local interface rather than a shared one so this package never
// needs to import internal/alerts (see this package's own doc comment
// on one-directional dependencies). Optional - nil degrades to "no
// overlay design saved through this Manager may reference a managed
// asset".
type AssetRefTracker interface {
	Get(ctx context.Context, id string) (visualasset.Asset, error)
	SetDesignAssetRefs(ctx context.Context, designID string, assetIDs []string) error
	ClearDesignRefs(ctx context.Context, designID string) error
}

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

	// visualDesignSvc is Stage 13B's own shared visual-design service -
	// optional, mirroring internal/alerts.Manager's own nil-safe
	// visualDesignSvc pattern. A nil value degrades every overlay to
	// legacy presentation rather than panicking; production always
	// wires a real one.
	visualDesignSvc *visualdesign.Service
	// assetSvc is Stage 14B's own managed-asset service - optional, see
	// AssetRefTracker's own doc comment. Set after construction via
	// SetAssetService (kept out of NewManager's own positional argument
	// list so its two existing call sites/tests never need to change).
	assetSvc AssetRefTracker

	mu          sync.Mutex
	projections map[string]*Projection
	started     bool

	upstreamSub upstreamSubscription
	lifecycle   context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

// NewManager builds a Manager. Call Start before registering any
// overlay. visualDesignSvc may be nil (see the Manager field's own doc
// comment).
func NewManager(upstream UpstreamSource, resolver SettingsResolver, visualDesignSvc *visualdesign.Service, logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{
		upstream:        upstream,
		resolver:        resolver,
		visualDesignSvc: visualDesignSvc,
		logger:          logger,
		projections:     make(map[string]*Projection),
	}
}

// SetAssetService wires Stage 14B's managed-asset service in after
// construction (see the Manager field's own doc comment) - call once,
// before Start, from the same place production wiring already sets
// every other optional dependency.
func (m *Manager) SetAssetService(svc AssetRefTracker) {
	m.assetSvc = svc
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
// deleted. Safe to call for an overlay with no running Projection. Also
// deletes overlayID's own saved visual design, if any - explicit
// application-level cleanup mirroring internal/alerts.Manager.
// DeleteRule's own cascade exactly (docs/visual-designs.md §18): a
// polymorphic owner_id cannot be a SQL foreign key, so this is always an
// explicit call, never implicit. A failure here is logged, not returned -
// the profile itself is already gone by the time Remove is called (see
// httpapi's own handleDeleteChatOverlay), so there is nothing left to
// roll back.
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

	if m.visualDesignSvc != nil {
		existing, found, err := m.visualDesignSvc.Get(ctx, visualdesign.OwnerKindChatOverlay, overlayID)
		if err != nil {
			m.logger.Error("failed to look up a chat overlay's own visual design during profile deletion",
				"overlay_id", overlayID, "error", err)
		}
		if err := m.visualDesignSvc.Delete(ctx, visualdesign.OwnerKindChatOverlay, overlayID); err != nil {
			m.logger.Error("failed to delete a chat overlay's own visual design during profile deletion",
				"overlay_id", overlayID, "error", err)
		} else if found && m.assetSvc != nil {
			if err := m.assetSvc.ClearDesignRefs(ctx, existing.ID); err != nil {
				m.logger.Error("failed to clear a chat overlay design's asset references during profile deletion",
					"overlay_id", overlayID, "error", err)
			}
		}
	}
}

// GetVisualDesign returns overlayID's own saved visual design, if any.
func (m *Manager) GetVisualDesign(ctx context.Context, overlayID string) (visualdesign.Record, bool, error) {
	if m.visualDesignSvc == nil {
		return visualdesign.Record{}, false, ErrVisualDesignUnavailable
	}
	return m.visualDesignSvc.Get(ctx, visualdesign.OwnerKindChatOverlay, overlayID)
}

// SaveVisualDesign validates doc against chat-overlay-specific binding
// capability (chatoverlaydomain.ValidateDesignBindingsForChatOverlay,
// beyond the shared package's own owner-agnostic Validate the Service
// itself already runs) and persists it as overlayID's full-replacement
// design, then triggers the Stage 13B public presentation-update
// protocol (docs/visual-designs.md §24/§25): Rebuild re-derives every
// currently-visible item against the newly saved design's own
// data-needs (so an avatar/badge/account-label layer is never silently
// starved by an unrelated legacy toggle), immediately followed by
// NotifyPresentationChanged so an already-connected public client knows
// to refetch its presentation config. Both are best-effort after a
// successful save - the save itself already succeeded, and a failure to
// immediately refresh the live runtime is logged, not returned, since
// the runtime will catch up on its own next rebuild regardless.
func (m *Manager) SaveVisualDesign(ctx context.Context, overlayID string, doc visualdesign.Document, expectedRevision int) (visualdesign.Record, error) {
	if m.visualDesignSvc == nil {
		return visualdesign.Record{}, ErrVisualDesignUnavailable
	}
	if err := chatoverlaydomain.ValidateDesignBindingsForChatOverlay(doc); err != nil {
		return visualdesign.Record{}, err
	}
	if err := visualdesign.ValidateAssetReferences(doc, m.resolveAssetKind(ctx)); err != nil {
		return visualdesign.Record{}, err
	}
	rec, err := m.visualDesignSvc.Save(ctx, visualdesign.OwnerKindChatOverlay, overlayID, doc, expectedRevision)
	if err != nil {
		return visualdesign.Record{}, err
	}
	if m.assetSvc != nil {
		if err := m.assetSvc.SetDesignAssetRefs(ctx, rec.ID, rec.Document.AssetReferences()); err != nil {
			return visualdesign.Record{}, err
		}
	}
	m.refreshPresentation(ctx, overlayID)
	return rec, nil
}

// resolveAssetKind: see internal/alerts.Manager's own identical helper -
// nil if no asset service is wired.
func (m *Manager) resolveAssetKind(ctx context.Context) visualdesign.AssetResolverFunc {
	if m.assetSvc == nil {
		return nil
	}
	return func(assetID string) (string, bool) {
		asset, err := m.assetSvc.Get(ctx, assetID)
		if err != nil {
			return "", false
		}
		return string(asset.Kind), true
	}
}

// DeleteVisualDesign implements "Reset to legacy presentation" (Stage
// 13B, docs/visual-designs.md §23) - idempotent, never deletes the
// profile itself, and triggers the same refresh/notify sequence
// SaveVisualDesign does.
func (m *Manager) DeleteVisualDesign(ctx context.Context, overlayID string) error {
	if m.visualDesignSvc == nil {
		return ErrVisualDesignUnavailable
	}
	existing, found, err := m.visualDesignSvc.Get(ctx, visualdesign.OwnerKindChatOverlay, overlayID)
	if err != nil {
		return err
	}
	if err := m.visualDesignSvc.Delete(ctx, visualdesign.OwnerKindChatOverlay, overlayID); err != nil {
		return err
	}
	if found && m.assetSvc != nil {
		if err := m.assetSvc.ClearDesignRefs(ctx, existing.ID); err != nil {
			return err
		}
	}
	m.refreshPresentation(ctx, overlayID)
	return nil
}

// refreshPresentation re-resolves overlayID's settings (picking up the
// design/data-needs change that just happened) and applies them to its
// running Projection, then emits a presentation-change notification -
// see SaveVisualDesign's own doc comment for why both steps run and why
// failures here are logged rather than surfaced.
func (m *Manager) refreshPresentation(ctx context.Context, overlayID string) {
	if err := m.Rebuild(ctx, overlayID); err != nil {
		m.logger.Error("failed to rebuild the live chat overlay projection after a visual-design change",
			"overlay_id", overlayID, "error", err)
		return
	}
	if p, ok := m.Get(overlayID); ok {
		p.NotifyPresentationChanged()
	}
}
