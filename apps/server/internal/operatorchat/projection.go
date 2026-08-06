package operatorchat

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"sync"
	"time"

	engagement "github.com/streaming-tree/server/internal/domain/engagement"
	bus "github.com/streaming-tree/server/internal/engagement"
)

// Capacity bounds - see internal/config for the validated
// STREAMING_TREE_OPERATOR_CHAT_BUFFER_SIZE override, deliberately
// independent of the Engagement Event Bus's own capacity (see
// internal/engagement.DefaultCapacity and friends).
const (
	DefaultCapacity = 500
	MinCapacity     = 100
	MaxCapacity     = 5000
)

// maxSubscriberChannelCapacity bounds one subscriber's buffered channel,
// exactly mirroring internal/engagement.Bus's own reasoning: per-subscriber
// memory stays bounded independently of the operator's buffer-size choice.
const maxSubscriberChannelCapacity = 2048

// EventSource is the subset of internal/engagement.Bus this package needs -
// narrow on purpose so a test can supply a real bus.Bus (cheap, deterministic
// with an injected clock) without this package importing anything beyond
// the one method it actually calls.
type EventSource interface {
	Subscribe(after uint64) (*bus.Subscription, bool, error)
}

// DestinationLookup resolves a connected account's linked destination id,
// when unambiguous - mirrors internal/runtime/twitchengagement's own
// DestinationLookup field exactly, so both consult the same policy without
// this package importing internal/domain/account.
type DestinationLookup func(connectedAccountID string) (destinationID string, ok bool)

// Options constructs a Projection.
type Options struct {
	Source       EventSource
	Capacity     int
	Now          func() time.Time
	Logger       *slog.Logger
	Destinations DestinationLookup
}

// Projection is the Stage 9 unified-operator-chat projection. See this
// package's own doc comment for the full design.
//
// Every exported method is safe for concurrent use. No exported method
// blocks on a slow subscriber - see Subscription's own doc comment.
type Projection struct {
	capacity     int
	now          func() time.Time
	logger       *slog.Logger
	destinations DestinationLookup

	mu         sync.Mutex
	seq        uint64
	ring       *ring
	latestByID map[string]Item // current state per item id (message items only)
	subs       map[uint64]*Subscription
	nextSub    uint64
	closed     bool
	busGap     bool

	source    EventSource
	lifecycle context.Context
	cancel    context.CancelFunc
	workers   sync.WaitGroup
}

// New builds a Projection. Call Start before it processes any event.
func New(opts Options) *Projection {
	capacity := opts.Capacity
	if capacity <= 0 {
		capacity = DefaultCapacity
	}
	now := opts.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Projection{
		capacity:     capacity,
		now:          now,
		logger:       logger,
		destinations: opts.Destinations,
		ring:         newRing(capacity),
		latestByID:   make(map[string]Item),
		subs:         make(map[uint64]*Subscription),
		source:       opts.Source,
	}
}

// Start subscribes to the Engagement Event Bus and begins processing.
// The projection begins empty regardless of what the bus already retains
// from earlier in this same process's life - see Subscribe's own atomic
// replay-then-live guarantee for why no event between "now" and "whenever
// Start runs" can be lost.
func (p *Projection) Start(ctx context.Context) error {
	p.lifecycle, p.cancel = context.WithCancel(context.Background())
	_ = ctx // parent context not retained; Shutdown provides an explicit stop, matching this project's other runtime managers

	sub, gap, err := p.source.Subscribe(0)
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.busGap = gap
	p.mu.Unlock()

	p.workers.Add(1)
	go p.run(sub)
	return nil
}

// Shutdown closes every live subscriber and stops consuming the Event Bus.
func (p *Projection) Shutdown(ctx context.Context) {
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

	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	subs := make([]*Subscription, 0, len(p.subs))
	for _, s := range p.subs {
		subs = append(subs, s)
	}
	p.subs = make(map[uint64]*Subscription)
	p.mu.Unlock()

	for _, s := range subs {
		s.close(ReasonShutdown)
	}
}

// run consumes the Event Bus subscription for the projection's lifetime,
// resubscribing (never losing the "began empty at Start" guarantee, since
// resubscribing is itself just another atomic Subscribe call) if this
// projection's own bus-side subscription is ever dropped as a slow
// consumer - this projection must never silently stop updating.
func (p *Projection) run(sub *bus.Subscription) {
	defer p.workers.Done()
	current := sub
	for {
		select {
		case <-p.lifecycle.Done():
			current.Cancel()
			return
		case evt, open := <-current.Events():
			if !open {
				select {
				case reason := <-current.Closed():
					if reason == bus.ReasonShutdown || p.lifecycle.Err() != nil {
						return
					}
					p.logger.Warn("operator chat projection's event bus subscription ended unexpectedly, resubscribing",
						slog.String("reason", reason))
				default:
				}
				next, gap, err := p.source.Subscribe(0)
				if err != nil {
					p.logger.Error("operator chat projection could not resubscribe to the event bus", slog.Any("error", err))
					return
				}
				p.mu.Lock()
				p.busGap = p.busGap || gap
				p.mu.Unlock()
				current = next
				continue
			}
			p.handleEvent(evt)
		}
	}
}

func (p *Projection) handleEvent(evt engagement.Event) {
	var items []Item
	switch evt.Type {
	case engagement.TypeChatMessage:
		items = []Item{p.buildMessageItem(evt)}
	case engagement.TypeChatMessageDeleted:
		items = p.applyMessageDeleted(evt)
	case engagement.TypeChatCleared:
		items = p.applyChatCleared(evt)
	case engagement.TypeModeration:
		items = p.applyModeration(evt)
	case engagement.TypeFollow, engagement.TypeSubscription, engagement.TypeResubscription,
		engagement.TypeGiftedSubscription, engagement.TypeSubscriptionGiftBatch, engagement.TypeBits,
		engagement.TypeRaid, engagement.TypeChannelPointRedemption,
		engagement.TypeStreamOnline, engagement.TypeStreamOffline:
		items = []Item{p.buildActivityItem(evt)}
	default:
		// Unsupported/unknown normalized type - ignored safely, never
		// crashes the projection. A future type is either added here
		// deliberately or stays invisible to operator chat.
		return
	}

	for _, item := range items {
		p.appendRevision(item)
	}
}

// appendRevision assigns the next projection sequence, stores the revision,
// updates the current-state index for message items, and fans it out to
// every live subscriber without blocking on a slow one.
func (p *Projection) appendRevision(item Item) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.seq++
	item.Sequence = p.seq
	item.Version = CurrentVersion
	p.ring.push(item)
	if item.Kind == KindMessage {
		p.latestByID[item.ID] = item
	}

	subs := make([]*Subscription, 0, len(p.subs))
	for _, s := range p.subs {
		subs = append(subs, s)
	}
	p.mu.Unlock()

	for _, s := range subs {
		select {
		case s.items <- item:
		default:
			p.unsubscribe(s.id, ReasonSlowConsumer)
		}
	}
}

// resolveDestination attaches a destination id when the account is linked
// to exactly one configured destination - never guessed otherwise.
func (p *Projection) resolveDestination(connectedAccountID string) string {
	if p.destinations == nil {
		return ""
	}
	id, ok := p.destinations(connectedAccountID)
	if !ok {
		return ""
	}
	return id
}

func newItemID(prefix string) string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return prefix + "_fallback"
	}
	return prefix + "_" + hex.EncodeToString(buf)
}

// Subscribe registers a live subscriber - the same atomic replay-then-live
// primitive internal/engagement.Bus.Subscribe implements, operating on
// item revisions instead of normalized events. See that type's own doc
// comment for the concurrency argument; it applies identically here.
func (p *Projection) Subscribe(after uint64) (sub *Subscription, gap bool, err error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, false, ErrClosed
	}

	p.nextSub++
	s := &Subscription{
		id:          p.nextSub,
		proj:        p,
		items:       make(chan Item, p.subscriberCapacityLocked()),
		closeReason: make(chan string, 1),
	}

	replay := p.ring.after(after)
	gap = p.hasGapLocked(after)
	p.subs[s.id] = s
	p.mu.Unlock()

	for _, item := range replay {
		select {
		case s.items <- item:
		default:
			p.unsubscribe(s.id, ReasonSlowConsumer)
			return s, gap, nil
		}
	}
	return s, gap, nil
}

func (p *Projection) subscriberCapacityLocked() int {
	c := p.capacity
	if c > maxSubscriberChannelCapacity {
		c = maxSubscriberChannelCapacity
	}
	return c
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

// ItemsAfter returns up to limit retained revisions with Sequence > after,
// in ascending order - the same append-only revision stream Subscribe
// replays, for the bounded, non-subscribing snapshot endpoint.
func (p *Projection) ItemsAfter(after uint64, limit int) (items []Item, gap bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	items = p.ring.after(after)
	gap = p.hasGapLocked(after)
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, gap
}

// Status is the projection's own status - no message content.
type Status struct {
	SchemaVersion     int
	Capacity          int
	RetainedCount     int
	OldestSequence    uint64
	NewestSequence    uint64
	ActiveSubscribers int
	// BusGap is true once this projection has ever detected a gap between
	// what it consumed from the Engagement Event Bus and what the bus
	// actually retained - a one-way honest flag, never cleared, since a
	// past gap is a past gap regardless of current bus health.
	BusGap bool
}

// Snapshot reports the projection's own status.
func (p *Projection) Snapshot() Status {
	p.mu.Lock()
	defer p.mu.Unlock()
	return Status{
		SchemaVersion:     CurrentVersion,
		Capacity:          p.capacity,
		RetainedCount:     p.ring.len(),
		OldestSequence:    p.ring.oldestSequence(),
		NewestSequence:    p.ring.newestSequence(),
		ActiveSubscribers: len(p.subs),
		BusGap:            p.busGap,
	}
}

func (p *Projection) unsubscribe(id uint64, reason string) {
	p.mu.Lock()
	s, ok := p.subs[id]
	if ok {
		delete(p.subs, id)
	}
	p.mu.Unlock()
	if ok {
		s.close(reason)
	}
}
