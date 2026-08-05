package bus

import engagement "github.com/streaming-tree/server/internal/domain/engagement"

// ring is a fixed-capacity, in-memory-only FIFO of retained events, evicting
// the oldest entry once full. Not concurrency-safe on its own - every call
// is made while Bus.mu is held (see bus.go).
type ring struct {
	items    []engagement.Event
	capacity int
	start    int
	count    int
}

func newRing(capacity int) *ring {
	return &ring{items: make([]engagement.Event, capacity), capacity: capacity}
}

// push appends e, evicting the oldest retained event if the ring is full.
func (r *ring) push(e engagement.Event) {
	idx := (r.start + r.count) % r.capacity
	r.items[idx] = e
	if r.count == r.capacity {
		r.start = (r.start + 1) % r.capacity
		return
	}
	r.count++
}

// len reports how many events are currently retained.
func (r *ring) len() int {
	return r.count
}

// oldestSequence and newestSequence report the retained window's bounds.
// Both return 0 when nothing is retained.
func (r *ring) oldestSequence() uint64 {
	if r.count == 0 {
		return 0
	}
	return r.items[r.start].Sequence
}

func (r *ring) newestSequence() uint64 {
	if r.count == 0 {
		return 0
	}
	idx := (r.start + r.count - 1) % r.capacity
	return r.items[idx].Sequence
}

// after returns every retained event with Sequence > after, in ascending
// sequence order.
func (r *ring) after(after uint64) []engagement.Event {
	out := make([]engagement.Event, 0, r.count)
	for i := 0; i < r.count; i++ {
		idx := (r.start + i) % r.capacity
		if r.items[idx].Sequence > after {
			out = append(out, r.items[idx])
		}
	}
	return out
}
