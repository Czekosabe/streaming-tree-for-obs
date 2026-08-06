package chatoverlay

import (
	"container/heap"
	"time"
)

// expiryEntry is one item's scheduled removal time.
type expiryEntry struct {
	id        string
	expiresAt time.Time
	index     int
}

// expiryHeap is a min-heap ordered by expiresAt - container/heap.Interface
// implementation only, no locking or timers of its own. A Projection owns
// exactly one of these plus one time.Timer (see projection.go) - never
// one timer or goroutine per message, satisfying the Stage 10 task's own
// Part 10 requirement.
type expiryHeap []*expiryEntry

func (h expiryHeap) Len() int           { return len(h) }
func (h expiryHeap) Less(i, j int) bool { return h[i].expiresAt.Before(h[j].expiresAt) }
func (h expiryHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i]; h[i].index = i; h[j].index = j }
func (h *expiryHeap) Push(x any) {
	entry := x.(*expiryEntry)
	entry.index = len(*h)
	*h = append(*h, entry)
}
func (h *expiryHeap) Pop() any {
	old := *h
	n := len(old)
	entry := old[n-1]
	old[n-1] = nil
	entry.index = -1
	*h = old[:n-1]
	return entry
}

// expiryQueue is a bounded, id-addressable priority queue: schedule an
// id's expiry, cancel it early (the item was removed for another reason
// first), or find out when the next one is due. Not concurrency-safe on
// its own - every call is made while Projection.mu is held, exactly like
// ring.
type expiryQueue struct {
	h    expiryHeap
	byID map[string]*expiryEntry
}

func newExpiryQueue() *expiryQueue {
	return &expiryQueue{byID: make(map[string]*expiryEntry)}
}

// schedule sets (or replaces) id's expiry time.
func (q *expiryQueue) schedule(id string, expiresAt time.Time) {
	if entry, ok := q.byID[id]; ok {
		entry.expiresAt = expiresAt
		heap.Fix(&q.h, entry.index)
		return
	}
	entry := &expiryEntry{id: id, expiresAt: expiresAt}
	heap.Push(&q.h, entry)
	q.byID[id] = entry
}

// cancel removes id's scheduled expiry, if any - used when an item is
// removed for a different reason (deletion, capacity eviction, a
// settings change) before its timer would have fired.
func (q *expiryQueue) cancel(id string) {
	entry, ok := q.byID[id]
	if !ok {
		return
	}
	heap.Remove(&q.h, entry.index)
	delete(q.byID, id)
}

// peekEarliest reports the next id due to expire, without removing it.
func (q *expiryQueue) peekEarliest() (id string, expiresAt time.Time, ok bool) {
	if len(q.h) == 0 {
		return "", time.Time{}, false
	}
	return q.h[0].id, q.h[0].expiresAt, true
}

// popDue removes and returns every id whose expiry is at or before now,
// in expiry order.
func (q *expiryQueue) popDue(now time.Time) []string {
	var due []string
	for len(q.h) > 0 && !q.h[0].expiresAt.After(now) {
		entry := heap.Pop(&q.h).(*expiryEntry)
		delete(q.byID, entry.id)
		due = append(due, entry.id)
	}
	return due
}

func (q *expiryQueue) len() int {
	return len(q.h)
}
