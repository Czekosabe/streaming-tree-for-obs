// Package bus is the in-process, concurrency-safe Engagement Event Bus: the
// component every connector (Twitch, and later others) publishes normalized
// events to, and every consumer (the SSE endpoint today; a future operator
// chat, overlay, alert engine, TTS queue and goals system) reads from.
//
// Deliberately in-memory only - see docs/engagement-architecture.md §6.5.
// Nothing in this package ever writes to SQLite, and every retained event is
// lost, by design, on a backend restart.
package bus

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	engagement "github.com/streaming-tree/server/internal/domain/engagement"
)

// Buffer-capacity bounds. See internal/config for the validated
// STREAMING_TREE_ENGAGEMENT_BUFFER_SIZE override that must fall within
// [MinCapacity, MaxCapacity].
const (
	DefaultCapacity = 1000
	MinCapacity     = 100
	MaxCapacity     = 10000
)

// Deduplication defaults. See docs/provider-integrations/twitch-engagement.md
// for why EventSub's metadata.message_id is the dedupe key the Twitch
// connector uses, and docs/progress.md's Stage 8A entry for why these exact
// numbers were chosen: 5 minutes comfortably outlasts any realistic
// reconnect-and-redeliver window without holding keys "forever," and 4x the
// default event-buffer capacity keeps the dedupe set from being the limiting
// factor during a burst that also fills the ring buffer.
const (
	defaultDedupeTTL      = 5 * time.Minute
	defaultDedupeCapacity = 4000
)

// maxSubscriberChannelCapacity bounds how large one subscriber's buffered
// channel (and therefore worst-case replay-on-subscribe) can be, even when
// the bus's own retained-event capacity is configured much larger. Keeps
// per-subscriber memory bounded independently of the operator's buffer-size
// choice.
const maxSubscriberChannelCapacity = 2048

// ErrClosed is returned by Publish and Subscribe once the Bus has been shut
// down.
var ErrClosed = errors.New("engagement event bus is closed")

// Options constructs a Bus. Every field is optional; zero values fall back
// to documented defaults.
type Options struct {
	// Capacity is the ring buffer's retained-event capacity. Falls back to
	// DefaultCapacity when <= 0. Callers that accept an operator-configured
	// value must validate it against [MinCapacity, MaxCapacity] themselves
	// (see internal/config) - Bus does not silently clamp an out-of-range
	// value, so a configuration mistake is a startup error, not a silently
	// different buffer size than requested.
	Capacity int
	// DedupeTTL and DedupeCapacity override the deduplication window - see
	// the package-level defaults above.
	DedupeTTL      time.Duration
	DedupeCapacity int
	// Now overrides the clock, for deterministic tests. Defaults to
	// time.Now().UTC.
	Now func() time.Time
}

// Bus is the concurrency-safe Engagement Event Bus. See the package doc
// comment.
//
// Every exported method is safe for concurrent use by multiple publishers
// and multiple subscribers. No exported method blocks on a slow subscriber:
// a subscriber that cannot keep up is dropped (see Subscription's doc
// comment), never allowed to stall Publish for everyone else.
type Bus struct {
	mu       sync.Mutex
	capacity int
	seq      uint64
	ring     *ring
	dedupe   *dedupeSet
	subs     map[uint64]*Subscription
	nextSub  uint64
	now      func() time.Time
	closed   bool
}

// New builds a Bus. There is nothing to Start - a Bus is ready to use
// immediately; Shutdown releases every live subscriber.
func New(opts Options) *Bus {
	capacity := opts.Capacity
	if capacity <= 0 {
		capacity = DefaultCapacity
	}
	now := opts.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	dedupeTTL := opts.DedupeTTL
	if dedupeTTL <= 0 {
		dedupeTTL = defaultDedupeTTL
	}
	dedupeCapacity := opts.DedupeCapacity
	if dedupeCapacity <= 0 {
		dedupeCapacity = defaultDedupeCapacity
	}
	return &Bus{
		capacity: capacity,
		ring:     newRing(capacity),
		dedupe:   newDedupeSet(dedupeCapacity, dedupeTTL, now),
		subs:     make(map[uint64]*Subscription),
		now:      now,
	}
}

func newEventID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "evt_" + hex.EncodeToString(buf), nil
}

// Publish validates evt, assigns its ID/Sequence/ReceivedAt, deduplicates it
// against evt.DedupeKey, retains it in the ring buffer, and fans it out to
// every live subscriber.
//
// published is false, with a nil error, when evt was dropped as a duplicate
// - not an error condition, a normal and expected outcome of Twitch (or any
// provider) occasionally redelivering a notification. A non-nil error means
// evt was structurally invalid (see engagement.Event.Validate) or the bus is
// closed; neither case retains or delivers anything.
func (b *Bus) Publish(evt engagement.Event) (published bool, seq uint64, err error) {
	if err := evt.Validate(); err != nil {
		return false, 0, err
	}

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return false, 0, ErrClosed
	}
	if b.dedupe.seen(evt.DedupeKey) {
		b.mu.Unlock()
		return false, 0, nil
	}

	id, idErr := newEventID()
	if idErr != nil {
		b.mu.Unlock()
		return false, 0, idErr
	}

	b.seq++
	evt.Sequence = b.seq
	evt.ID = id
	evt.ReceivedAt = b.now()

	b.ring.push(evt)

	subs := make([]*Subscription, 0, len(b.subs))
	for _, s := range b.subs {
		subs = append(subs, s)
	}
	b.mu.Unlock()

	for _, s := range subs {
		select {
		case s.events <- evt:
		default:
			b.unsubscribe(s.id, ReasonSlowConsumer)
		}
	}

	return true, evt.Sequence, nil
}

// Subscribe registers a live subscriber. When after > 0, every retained
// event with Sequence > after is delivered first (in order), before any
// newly published event - the same replay-then-live-tail contract the SSE
// endpoint exposes via Last-Event-ID.
//
// gap is true when after refers to a sequence older than the oldest event
// still retained (already evicted), meaning replay is necessarily
// incomplete - the caller (see internal/httpapi's SSE handler) must signal
// this honestly to its own consumer rather than silently understating what
// was missed.
//
// If replaying would exceed this subscriber's bounded channel capacity, the
// subscription is dropped immediately with ReasonSlowConsumer rather than
// blocking Subscribe itself or growing the channel without bound.
func (b *Bus) Subscribe(after uint64) (sub *Subscription, gap bool, err error) {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil, false, ErrClosed
	}

	b.nextSub++
	s := &Subscription{
		id:          b.nextSub,
		bus:         b,
		events:      make(chan engagement.Event, b.subscriberCapacityLocked()),
		closeReason: make(chan string, 1),
	}

	replay := b.ring.after(after)
	gap = b.hasGapLocked(after)
	b.subs[s.id] = s
	b.mu.Unlock()

	for _, e := range replay {
		select {
		case s.events <- e:
		default:
			b.unsubscribe(s.id, ReasonSlowConsumer)
			return s, gap, nil
		}
	}

	return s, gap, nil
}

func (b *Bus) subscriberCapacityLocked() int {
	c := b.capacity
	if c > maxSubscriberChannelCapacity {
		c = maxSubscriberChannelCapacity
	}
	return c
}

// hasGapLocked reports whether after references a sequence already evicted
// from the ring buffer. Callers must hold b.mu.
func (b *Bus) hasGapLocked(after uint64) bool {
	if after == 0 {
		return false
	}
	if b.ring.len() == 0 {
		return b.seq > 0 && after < b.seq
	}
	oldest := b.ring.oldestSequence()
	if oldest <= 1 {
		return false
	}
	return after < oldest-1
}

// EventsAfter returns up to limit retained events with Sequence > after, in
// ascending order, for the bounded snapshot endpoint
// (GET /api/engagement/events) - unlike Subscribe, this never registers a
// live subscriber and never blocks on delivery.
func (b *Bus) EventsAfter(after uint64, limit int) (events []engagement.Event, gap bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	events = b.ring.after(after)
	gap = b.hasGapLocked(after)
	if limit > 0 && len(events) > limit {
		events = events[:limit]
	}
	return events, gap
}

// Snapshot reports the bus's own status - see Snapshot's doc comment.
func (b *Bus) Snapshot() Snapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	return Snapshot{
		SchemaVersion:     int(engagement.CurrentSchemaVersion),
		Capacity:          b.capacity,
		RetainedCount:     b.ring.len(),
		OldestSequence:    b.ring.oldestSequence(),
		NewestSequence:    b.ring.newestSequence(),
		ActiveSubscribers: len(b.subs),
	}
}

func (b *Bus) unsubscribe(id uint64, reason string) {
	b.mu.Lock()
	s, ok := b.subs[id]
	if ok {
		delete(b.subs, id)
	}
	b.mu.Unlock()
	if ok {
		s.close(reason)
	}
}

// Shutdown closes every live subscriber with ReasonShutdown and marks the
// bus closed - Publish and Subscribe both return ErrClosed afterward.
// Idempotent.
func (b *Bus) Shutdown() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	subs := make([]*Subscription, 0, len(b.subs))
	for _, s := range b.subs {
		subs = append(subs, s)
	}
	b.subs = make(map[uint64]*Subscription)
	b.mu.Unlock()

	for _, s := range subs {
		s.close(ReasonShutdown)
	}
}
