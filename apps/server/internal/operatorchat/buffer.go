package operatorchat

// ring is a fixed-capacity, in-memory-only FIFO of retained item
// revisions, evicting the oldest entry once full - structurally identical
// to internal/engagement's own ring, operating on Item instead of
// engagement.Event. Not concurrency-safe on its own; every call is made
// while Projection.mu is held.
type ring struct {
	items    []Item
	capacity int
	start    int
	count    int
}

func newRing(capacity int) *ring {
	return &ring{items: make([]Item, capacity), capacity: capacity}
}

// push appends a revision, evicting the oldest retained one if full.
func (r *ring) push(item Item) {
	idx := (r.start + r.count) % r.capacity
	r.items[idx] = item
	if r.count == r.capacity {
		r.start = (r.start + 1) % r.capacity
		return
	}
	r.count++
}

func (r *ring) len() int {
	return r.count
}

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

// after returns every retained revision with Sequence > after, in
// ascending sequence order.
func (r *ring) after(after uint64) []Item {
	out := make([]Item, 0, r.count)
	for i := 0; i < r.count; i++ {
		idx := (r.start + i) % r.capacity
		if r.items[idx].Sequence > after {
			out = append(out, r.items[idx])
		}
	}
	return out
}
