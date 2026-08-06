package chatoverlay

// ring is a fixed-capacity, in-memory-only FIFO of retained public
// revisions, evicting the oldest entry once full - structurally identical
// to internal/operatorchat's own ring, operating on Revision instead of
// Item. Not concurrency-safe on its own; every call is made while
// Projection.mu is held.
type ring struct {
	items    []Revision
	capacity int
	start    int
	count    int
}

func newRing(capacity int) *ring {
	return &ring{items: make([]Revision, capacity), capacity: capacity}
}

// push appends a revision, evicting the oldest retained one if full.
func (r *ring) push(rev Revision) {
	idx := (r.start + r.count) % r.capacity
	r.items[idx] = rev
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
func (r *ring) after(after uint64) []Revision {
	out := make([]Revision, 0, r.count)
	for i := 0; i < r.count; i++ {
		idx := (r.start + i) % r.capacity
		if r.items[idx].Sequence > after {
			out = append(out, r.items[idx])
		}
	}
	return out
}
