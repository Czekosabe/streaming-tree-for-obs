package alerts

import "sync"

// Operation is one playback revision's kind - the Stage 12A task's own
// Part 23 protocol.
type Operation string

const (
	OpShow   Operation = "show"
	OpHide   Operation = "hide"
	OpReset  Operation = "reset"
	OpPaused Operation = "paused"
	OpGap    Operation = "gap"
)

// PublicAlert is the versioned, presentation-only public alert model
// (Part 22) - deliberately excludes every field that section forbids:
// no rule internals, no account health, no OAuth scopes, no
// provider-raw payload/ProviderExtra, no token, no stream key, no
// dedupe key, no EventSub session data. AvatarURL is deliberately
// absent entirely rather than always-nil: no Stage 12A event type's
// Twitch normalization populates engagement.User.AvatarURL today (only
// chat.message does), so a field that can never carry real data would
// be actively misleading rather than merely unused - see
// docs/progress.md's Stage 12A queue/playback entry.
type PublicAlert struct {
	SchemaVersion int
	AlertID       string
	EventType     string
	ProviderID    string
	Synthetic     bool
	Replayed      bool

	Username *string
	Message  *string
	Quantity *int64

	RenderedText string

	DurationMS          int
	EntryAnimation      string
	ExitAnimation       string
	AnimationDurationMS int
}

// Revision is one entry in a profile's playback stream - the unit
// delivered over SSE. Alert is nil for OpHide/OpGap and for an OpReset
// with no current alert; Paused always reflects the profile's pause
// state at the time this revision was published, so a client never
// needs a second round trip to learn it.
type Revision struct {
	Sequence  uint64
	Operation Operation
	Alert     *PublicAlert
	Paused    bool
}

// DefaultRevisionCapacity mirrors chatoverlay's own retained-revision
// capacity choice - generous relative to how few revisions one profile
// actually produces (a handful per alert: show, hide, occasionally
// paused/gap), never a scrolling list.
const DefaultRevisionCapacity = 200

// revisionRing is a fixed-capacity FIFO of retained revisions, evicting
// the oldest once full - byte-for-byte the same shape as
// internal/engagement's own ring, retyped for Revision. Not
// concurrency-safe on its own; every call is made while projection.mu
// is held.
type revisionRing struct {
	items    []Revision
	capacity int
	start    int
	count    int
}

func newRevisionRing(capacity int) *revisionRing {
	return &revisionRing{items: make([]Revision, capacity), capacity: capacity}
}

func (r *revisionRing) push(rev Revision) {
	idx := (r.start + r.count) % r.capacity
	r.items[idx] = rev
	if r.count == r.capacity {
		r.start = (r.start + 1) % r.capacity
		return
	}
	r.count++
}

func (r *revisionRing) len() int { return r.count }

func (r *revisionRing) oldestSequence() uint64 {
	if r.count == 0 {
		return 0
	}
	return r.items[r.start].Sequence
}

func (r *revisionRing) newestSequence() uint64 {
	if r.count == 0 {
		return 0
	}
	idx := (r.start + r.count - 1) % r.capacity
	return r.items[idx].Sequence
}

func (r *revisionRing) after(after uint64) []Revision {
	out := make([]Revision, 0, r.count)
	for i := 0; i < r.count; i++ {
		idx := (r.start + i) % r.capacity
		if r.items[idx].Sequence > after {
			out = append(out, r.items[idx])
		}
	}
	return out
}

// Reasons a Subscription's channel closes - mirrors
// internal/engagement's own Reason* constants.
const (
	ReasonCancelled    = "cancelled"
	ReasonSlowConsumer = "slow_consumer"
	ReasonShutdown     = "shutdown"
)

const maxProjectionSubscriberCapacity = 256

// Subscription is one live SSE consumer's view of one profile's
// playback stream.
type Subscription struct {
	id          uint64
	proj        *projection
	revisions   chan Revision
	closeReason chan string
	closeOnce   sync.Once
}

// Revisions returns the channel of live (and, if requested, replayed)
// revisions. Closed exactly once, after which Closed() reports why.
func (s *Subscription) Revisions() <-chan Revision { return s.revisions }

// Closed reports the reason Revisions() closed, exactly once.
func (s *Subscription) Closed() <-chan string { return s.closeReason }

// Cancel ends the subscription explicitly. Safe to call more than once.
func (s *Subscription) Cancel() { s.proj.unsubscribe(s.id, ReasonCancelled) }

func (s *Subscription) close(reason string) {
	s.closeOnce.Do(func() {
		close(s.revisions)
		s.closeReason <- reason
		close(s.closeReason)
	})
}

// projection is one alert profile's own public playback broadcaster -
// mirrors internal/engagement.Bus and internal/chatoverlay's own
// Projection, scoped to a single profile's small revision stream
// instead of a scrolling item list.
type projection struct {
	mu       sync.Mutex
	capacity int
	seq      uint64
	ring     *revisionRing
	subs     map[uint64]*Subscription
	nextSub  uint64
	closed   bool
}

func newProjectionRuntime(capacity int) *projection {
	if capacity <= 0 {
		capacity = DefaultRevisionCapacity
	}
	return &projection{capacity: capacity, ring: newRevisionRing(capacity), subs: make(map[uint64]*Subscription)}
}

// publish assigns the next sequence number, retains rev, and fans it
// out to every live subscriber - never blocks on a slow one (dropped
// with ReasonSlowConsumer instead, exactly like the Engagement Event
// Bus).
func (p *projection) publish(op Operation, alert *PublicAlert, paused bool) Revision {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return Revision{}
	}
	p.seq++
	rev := Revision{Sequence: p.seq, Operation: op, Alert: alert, Paused: paused}
	p.ring.push(rev)
	subs := make([]*Subscription, 0, len(p.subs))
	for _, s := range p.subs {
		subs = append(subs, s)
	}
	p.mu.Unlock()

	for _, s := range subs {
		select {
		case s.revisions <- rev:
		default:
			p.unsubscribe(s.id, ReasonSlowConsumer)
		}
	}
	return rev
}

// Subscribe registers a live subscriber, replaying every retained
// revision with Sequence > after first - mirrors Bus.Subscribe's own
// contract, including honest gap reporting when after references an
// already-evicted sequence.
func (p *projection) Subscribe(after uint64) (sub *Subscription, gap bool, err error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, false, ErrProjectionClosed
	}
	p.nextSub++
	capacity := p.capacity
	if capacity > maxProjectionSubscriberCapacity {
		capacity = maxProjectionSubscriberCapacity
	}
	s := &Subscription{id: p.nextSub, proj: p, revisions: make(chan Revision, capacity), closeReason: make(chan string, 1)}

	replay := p.ring.after(after)
	gap = p.hasGapLocked(after)
	p.subs[s.id] = s
	p.mu.Unlock()

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

func (p *projection) hasGapLocked(after uint64) bool {
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

// latestSequence returns the highest revision sequence published so
// far (0 if none yet) - used by the SSE handler to give a fresh
// connection's synthetic initial reset a meaningful Last-Event-ID, so a
// later reconnect using it resumes from "nothing missed" rather than
// triggering a spurious gap.
func (p *projection) latestSequence() uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.seq
}

func (p *projection) activeSubscribers() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.subs)
}

func (p *projection) unsubscribe(id uint64, reason string) {
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

func (p *projection) shutdown() {
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
