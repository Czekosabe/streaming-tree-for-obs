package chatoverlay

import (
	"context"
	"log/slog"
	"sync"
	"time"

	operatorchat "github.com/streaming-tree/server/internal/operatorchat"
)

// Revision-ring capacity bounds - independent of any single overlay's own
// MaxVisibleItems setting, exactly like operatorchat's own capacity is
// independent of the Engagement Event Bus's. A generous multiple of the
// largest allowed MaxVisibleItems (100) so a reconnecting client can
// almost always replay rather than needing a reset.
const (
	DefaultRevisionCapacity      = 400
	maxSubscriberChannelCapacity = 512
)

// OperatorChatSource is the subset of operatorchat.Projection this
// package needs for a settings-change rebuild: replaying every currently
// retained item so a new filter/presentation configuration can be
// applied to it, bounded by whatever operatorchat itself still retains -
// see this package's own doc comment on why a full re-derivation beyond
// that bound is never attempted.
type OperatorChatSource interface {
	ItemsAfter(after uint64, limit int) ([]operatorchat.Item, bool)
}

// maxVisibleItemsFloor guards against a zero/negative MaxVisibleItems
// value reaching the eviction logic - internal/domain/chatoverlay's own
// validation already rejects anything outside 1-100 before a profile is
// ever saved, this is a defensive second floor.
const maxVisibleItemsFloor = 1

// Projection is one overlay profile's own public-item projection. Every
// exported method is safe for concurrent use. No exported method blocks
// on a slow subscriber - see Subscription's own doc comment. One overlay
// failing (a panic recovered by the Manager) or one overlay's subscriber
// falling behind never affects another overlay or the upstream
// operator-chat projection.
type Projection struct {
	overlayID string
	source    OperatorChatSource
	now       func() time.Time
	logger    *slog.Logger

	mu           sync.Mutex
	seq          uint64
	ring         *ring
	latestByID   map[string]Item
	visibleOrder []string // FIFO of visible ids, oldest first
	expiry       *expiryQueue
	subs         map[uint64]*Subscription
	nextSub      uint64
	closed       bool
	settings     resolvedSettings
	upstreamGap  bool

	timer     *time.Timer
	lifecycle context.Context
	cancel    context.CancelFunc
	workers   sync.WaitGroup
}

// NewProjection builds a Projection for one overlay profile. Call Start
// before Configure so its own expiry timer goroutine exists first.
func NewProjection(overlayID string, source OperatorChatSource, now func() time.Time, logger *slog.Logger) *Projection {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Projection{
		overlayID:  overlayID,
		source:     source,
		now:        now,
		logger:     logger,
		ring:       newRing(DefaultRevisionCapacity),
		latestByID: make(map[string]Item),
		expiry:     newExpiryQueue(),
		subs:       make(map[uint64]*Subscription),
	}
}

// Start begins this overlay's own bounded expiry-timer goroutine - one
// per active overlay, never one per message.
func (p *Projection) Start(ctx context.Context) {
	p.lifecycle, p.cancel = context.WithCancel(context.Background())
	_ = ctx
	p.timer = time.NewTimer(24 * time.Hour) // reset immediately once anything is scheduled
	p.timer.Stop()
	p.workers.Add(1)
	go p.runExpiry()
}

// Shutdown stops the expiry goroutine and closes every live subscriber
// with ReasonShutdown - used for whole-server shutdown. A single
// overlay being deleted while other overlays keep running uses
// closeWithReason(ctx, ReasonOverlayDeleted) instead (see manager.go).
func (p *Projection) Shutdown(ctx context.Context) {
	p.closeWithReason(ctx, ReasonShutdown)
}

func (p *Projection) closeWithReason(ctx context.Context, reason string) {
	if p.cancel != nil {
		p.cancel()
	}
	done := make(chan struct{})
	go func() {
		p.workers.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}

	// A defer-guarded lock, here and throughout this file, so a panic
	// inside the locked section (an internal bug in one overlay) can
	// never leave p.mu wedged forever for that overlay - see
	// Manager.dispatchOne's own recover, which this complements rather
	// than replaces.
	subs, alreadyClosed := func() (subs []*Subscription, alreadyClosed bool) {
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.closed {
			return nil, true
		}
		p.closed = true
		subs = p.subsSliceLocked()
		p.subs = make(map[uint64]*Subscription)
		return subs, false
	}()
	if alreadyClosed {
		return
	}

	for _, s := range subs {
		s.close(reason)
	}
}

func (p *Projection) runExpiry() {
	defer p.workers.Done()
	for {
		select {
		case <-p.lifecycle.Done():
			p.timer.Stop()
			return
		case <-p.timer.C:
			p.processExpiry()
		}
	}
}

func (p *Projection) processExpiry() {
	now := p.now()
	revisions, subs, closed := func() (revisions []Revision, subs []*Subscription, closed bool) {
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.closed {
			return nil, nil, true
		}
		due := p.expiry.popDue(now)
		for _, id := range due {
			if _, ok := p.latestByID[id]; !ok {
				continue
			}
			p.removeVisibleLocked(id)
			p.seq++
			revisions = append(revisions, Revision{Sequence: p.seq, Operation: OpRemove, RemovedID: id, Reason: RemoveReasonExpired})
		}
		for _, rev := range revisions {
			p.ring.push(rev)
		}
		p.rescheduleTimerLocked()
		return revisions, p.subsSliceLocked(), false
	}()
	if closed {
		return
	}

	for _, rev := range revisions {
		p.fanOut(rev, subs)
	}
}

// rescheduleTimerLocked resets the single managed timer to fire at the
// earliest scheduled expiry, or stops it when nothing is scheduled -
// callers must hold p.mu.
func (p *Projection) rescheduleTimerLocked() {
	if p.timer == nil {
		return
	}
	if !p.timer.Stop() {
		select {
		case <-p.timer.C:
		default:
		}
	}
	_, expiresAt, ok := p.expiry.peekEarliest()
	if !ok {
		return
	}
	d := expiresAt.Sub(p.now())
	if d < 0 {
		d = 0
	}
	p.timer.Reset(d)
}

// Configure applies a new resolved settings snapshot (profile, selected
// accounts, hidden users, the shared bot list, blocked terms, activity
// types) and rebuilds the entire visible set from whatever the upstream
// operator-chat projection currently retains - see this package's own
// doc comment for why a rebuild's correctness is bounded by that
// retention, exactly like every other "honest gap" boundary in this
// project.
func (p *Projection) Configure(settings resolvedSettings) {
	items, gap := p.source.ItemsAfter(0, 0)

	closed := func() bool {
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.closed {
			return true
		}
		p.settings = settings
		p.latestByID = make(map[string]Item)
		p.visibleOrder = nil
		p.expiry = newExpiryQueue()
		p.upstreamGap = p.upstreamGap || gap
		return false
	}()
	if closed {
		return
	}

	for _, item := range items {
		p.applyUpstreamItem(item, false)
	}

	rev, subs, closed := func() (rev Revision, subs []*Subscription, closed bool) {
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.closed {
			return Revision{}, nil, true
		}
		visible := p.currentItemsLocked()
		p.seq++
		rev = Revision{Sequence: p.seq, Operation: OpReset, ResetItems: visible}
		p.ring.push(rev)
		p.rescheduleTimerLocked()
		return rev, p.subsSliceLocked(), false
	}()
	if closed {
		return
	}

	p.fanOut(rev, subs)
}

// HandleUpstreamItem processes one live revision from the shared
// operator-chat subscription (see Manager.dispatch) - the Manager calls
// this for every registered overlay, recovering from any panic so one
// overlay's bug can never affect another or the shared dispatch loop.
func (p *Projection) HandleUpstreamItem(item operatorchat.Item) {
	p.applyUpstreamItem(item, true)
}

// applyUpstreamItem is the shared entry point for both live processing
// and Configure's own rebuild replay - emit controls whether individual
// revisions are produced (live) or suppressed (rebuild, which emits one
// Reset at the end instead). A single upstream item can produce more
// than one revision: a newly visible item that exceeds MaxVisibleItems
// also evicts the oldest visible item, which must reach subscribers as
// its own explicit remove - never a silent drop a replaying client could
// never learn about.
func (p *Projection) applyUpstreamItem(item operatorchat.Item, emit bool) {
	visible, publicItem := evaluate(item, p.currentSettings())

	revs, subs, closed := func() (revs []Revision, subs []*Subscription, closed bool) {
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.closed {
			return nil, nil, true
		}
		_, wasVisible := p.latestByID[item.ID]

		switch {
		case visible:
			publicItem.Sequence = 0 // assigned below
			evicted := p.upsertVisibleLocked(publicItem)
			if emit {
				p.seq++
				publicItem.Sequence = p.seq
				p.latestByID[item.ID] = publicItem
				revs = append(revs, Revision{Sequence: p.seq, Operation: OpUpsert, Item: &publicItem})
				for _, id := range evicted {
					p.seq++
					revs = append(revs, Revision{Sequence: p.seq, Operation: OpRemove, RemovedID: id, Reason: RemoveReasonCapacityEvicted})
				}
			}
		case wasVisible:
			p.removeVisibleLocked(item.ID)
			if emit {
				p.seq++
				revs = append(revs, Revision{Sequence: p.seq, Operation: OpRemove, RemovedID: item.ID, Reason: deletionRemoveReason(item)})
			}
		}

		for _, rev := range revs {
			p.ring.push(rev)
		}
		if len(revs) > 0 {
			p.rescheduleTimerLocked()
		}
		return revs, p.subsSliceLocked(), false
	}()
	if closed {
		return
	}

	for _, rev := range revs {
		p.fanOut(rev, subs)
	}
}

func (p *Projection) currentSettings() resolvedSettings {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.settings
}

// upsertVisibleLocked stores item as currently visible, appending it to
// the FIFO order if new, schedules its expiry if the profile has a
// message lifetime, and evicts the oldest visible item(s) if doing so
// would exceed MaxVisibleItems - returning their ids so the caller can
// emit its own explicit remove revision for each (see applyUpstreamItem;
// a silent, unannounced eviction would leave a live or replaying
// subscriber with permanently stale state for that id). Callers must
// hold p.mu.
func (p *Projection) upsertVisibleLocked(item Item) (evicted []string) {
	if _, existed := p.latestByID[item.ID]; !existed {
		p.visibleOrder = append(p.visibleOrder, item.ID)
	}
	p.latestByID[item.ID] = item

	if p.settings.profile.MessageLifetimeSeconds > 0 {
		lifetime := time.Duration(p.settings.profile.MessageLifetimeSeconds) * time.Second
		p.expiry.schedule(item.ID, item.OccurredAt.Add(lifetime))
	} else {
		p.expiry.cancel(item.ID)
	}

	maxVisible := p.settings.profile.MaxVisibleItems
	if maxVisible < maxVisibleItemsFloor {
		maxVisible = maxVisibleItemsFloor
	}
	for len(p.visibleOrder) > maxVisible {
		oldest := p.visibleOrder[0]
		p.visibleOrder = p.visibleOrder[1:]
		delete(p.latestByID, oldest)
		p.expiry.cancel(oldest)
		evicted = append(evicted, oldest)
	}
	return evicted
}

func (p *Projection) removeVisibleLocked(id string) {
	if _, ok := p.latestByID[id]; !ok {
		return
	}
	delete(p.latestByID, id)
	p.expiry.cancel(id)
	for i, existing := range p.visibleOrder {
		if existing == id {
			p.visibleOrder = append(p.visibleOrder[:i], p.visibleOrder[i+1:]...)
			break
		}
	}
}

func (p *Projection) currentItemsLocked() []Item {
	out := make([]Item, 0, len(p.visibleOrder))
	for _, id := range p.visibleOrder {
		if item, ok := p.latestByID[id]; ok {
			out = append(out, item)
		}
	}
	return out
}

// CurrentItems returns the current visible set - a plain snapshot of
// state, not a revision stream (see public_model.go's own doc comment on
// why the snapshot endpoint and the SSE stream have different shapes).
func (p *Projection) CurrentItems() []Item {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.currentItemsLocked()
}

// Subscribe registers a live subscriber - the same atomic replay-then-
// live primitive internal/operatorchat.Projection.Subscribe implements,
// operating on public Revisions instead of operator-chat Items.
func (p *Projection) Subscribe(after uint64) (sub *Subscription, gap bool, err error) {
	var s *Subscription
	var replay []Revision
	closed := func() bool {
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.closed {
			return true
		}

		p.nextSub++
		s = &Subscription{
			id: p.nextSub, proj: p,
			revisions:   make(chan Revision, maxSubscriberChannelCapacity),
			closeReason: make(chan string, 1),
		}

		replay = p.ring.after(after)
		gap = p.hasGapLocked(after)
		p.subs[s.id] = s
		return false
	}()
	if closed {
		return nil, false, ErrClosed
	}

	for _, rev := range replay {
		select {
		case s.revisions <- rev:
		default:
			p.unsubscribe(s.id, ReasonSlowConsumer)
			return s, gap, nil
		}
	}
	return s, gap, nil
}

func (p *Projection) hasGapLocked(after uint64) bool {
	if after == 0 {
		return false
	}
	if p.ring.len() == 0 {
		return p.seq > 0 && after < p.seq
	}
	oldest := p.ring.oldestSequence()
	if oldest <= 1 {
		return false
	}
	return after < oldest-1
}

// Status reports this overlay's own runtime status - no message content.
func (p *Projection) Status() Status {
	p.mu.Lock()
	defer p.mu.Unlock()
	return Status{
		SchemaVersion: CurrentVersion, VisibleCount: len(p.visibleOrder),
		MaxVisibleItems: p.settings.profile.MaxVisibleItems, RetainedRevisions: p.ring.len(),
		OldestSequence: p.ring.oldestSequence(), NewestSequence: p.ring.newestSequence(),
		ActiveSubscribers: len(p.subs), UpstreamGap: p.upstreamGap,
	}
}

func (p *Projection) subsSliceLocked() []*Subscription {
	subs := make([]*Subscription, 0, len(p.subs))
	for _, s := range p.subs {
		subs = append(subs, s)
	}
	return subs
}

func (p *Projection) fanOut(rev Revision, subs []*Subscription) {
	for _, s := range subs {
		select {
		case s.revisions <- rev:
		default:
			p.unsubscribe(s.id, ReasonSlowConsumer)
		}
	}
}

func (p *Projection) unsubscribe(id uint64, reason string) {
	s, ok := func() (s *Subscription, ok bool) {
		p.mu.Lock()
		defer p.mu.Unlock()
		s, ok = p.subs[id]
		if ok {
			delete(p.subs, id)
		}
		return s, ok
	}()
	if ok {
		s.close(reason)
	}
}
