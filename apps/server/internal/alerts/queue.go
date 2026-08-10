package alerts

import (
	"time"

	domain "github.com/streaming-tree/server/internal/domain/alerts"
)

// item is one queue entry plus its insertion sequence, used to break
// same-priority ties FIFO (Part 13: "within the same priority use queue
// insertion sequence FIFO").
type item struct {
	instance Instance
	seq      uint64
}

// queue is one profile's bounded, in-memory pending-alert queue - never
// the currently-playing alert (that is tracked separately by
// profileRuntime in playback.go) and never persisted (Part 12).
//
// Play/eviction order is always priority descending, then insertion
// sequence ascending (oldest first) within equal priority - never raw
// slice/append order and never SQLite row order, matching the Stage
// 12A task's own Part 8/13 requirements. Not concurrency-safe on its
// own; every caller holds profileRuntime's own mutex.
type queue struct {
	maxItems int
	maxAge   time.Duration
	items    []item
	nextSeq  uint64
}

func newQueue(maxItems int, maxAge time.Duration) *queue {
	return &queue{maxItems: maxItems, maxAge: maxAge}
}

func (q *queue) len() int { return len(q.items) }

// bestIndex returns the index of the highest-priority, oldest-among-ties
// item, or -1 if the queue is empty.
func (q *queue) bestIndex() int {
	if len(q.items) == 0 {
		return -1
	}
	best := 0
	for i := 1; i < len(q.items); i++ {
		if q.items[i].instance.Priority > q.items[best].instance.Priority ||
			(q.items[i].instance.Priority == q.items[best].instance.Priority && q.items[i].seq < q.items[best].seq) {
			best = i
		}
	}
	return best
}

// worstIndex returns the index of the lowest-priority, oldest-among-ties
// item - the eviction candidate when the queue is at capacity.
func (q *queue) worstIndex() int {
	if len(q.items) == 0 {
		return -1
	}
	worst := 0
	for i := 1; i < len(q.items); i++ {
		if q.items[i].instance.Priority < q.items[worst].instance.Priority ||
			(q.items[i].instance.Priority == q.items[worst].instance.Priority && q.items[i].seq < q.items[worst].seq) {
			worst = i
		}
	}
	return worst
}

func (q *queue) removeAt(i int) item {
	it := q.items[i]
	q.items = append(q.items[:i], q.items[i+1:]...)
	return it
}

// enqueue applies the Stage 12A task's own Part 14 capacity policy,
// deterministically:
//  1. Below capacity: always accept.
//  2. At capacity: find the current lowest-priority (oldest-among-ties)
//     queued item. If the new instance's priority is strictly higher,
//     evict that item and accept the new one. Otherwise (new priority
//     is lower than or equal to the worst queued item's), reject the
//     new instance outright - the queue is never grown past maxItems,
//     and the currently-playing alert (not part of this struct) is
//     never evicted by this method.
func (q *queue) enqueue(inst Instance, now time.Time) (accepted bool, evicted *Instance) {
	if len(q.items) < q.maxItems {
		q.items = append(q.items, item{instance: inst, seq: q.nextSeq})
		q.nextSeq++
		return true, nil
	}
	worst := q.worstIndex()
	if worst < 0 || inst.Priority <= q.items[worst].instance.Priority {
		return false, nil
	}
	removed := q.removeAt(worst)
	q.items = append(q.items, item{instance: inst, seq: q.nextSeq})
	q.nextSeq++
	return true, &removed.instance
}

// popNextEligible repeatedly removes the best (highest-priority,
// oldest-among-ties) queued item; any item whose age exceeds maxAge is
// discarded (Part 15: "before promoting the next queued item to
// current... if older than maximumQueueAgeSeconds, discard it") and
// counted in expiredCount, and the search continues, until an eligible
// item is found or the queue is empty.
func (q *queue) popNextEligible(now time.Time) (inst Instance, expiredCount int, ok bool) {
	for {
		i := q.bestIndex()
		if i < 0 {
			return Instance{}, expiredCount, false
		}
		candidate := q.removeAt(i)
		if q.maxAge > 0 && now.Sub(candidate.instance.QueuedAt) > q.maxAge {
			expiredCount++
			continue
		}
		return candidate.instance, expiredCount, true
	}
}

// clear empties the queue and returns every item that was queued, for
// the caller's own counters - never affects the currently-playing
// alert.
func (q *queue) clear() []Instance {
	out := make([]Instance, 0, len(q.items))
	for _, it := range q.items {
		out = append(out, it.instance)
	}
	q.items = nil
	return out
}

// list returns up to limit queued instances in play order
// (priority desc, then FIFO) - a bounded management-only snapshot
// (Part 30: "next few queued safe summaries, bounded e.g. 20"). Never
// mutates the queue.
func (q *queue) list(limit int) []Instance {
	ordered := make([]item, len(q.items))
	copy(ordered, q.items)
	// Simple insertion sort by (priority desc, seq asc) - queue sizes
	// are bounded (<=500), so this stays cheap and needs no extra
	// dependency.
	for i := 1; i < len(ordered); i++ {
		for j := i; j > 0; j-- {
			a, b := ordered[j-1], ordered[j]
			less := b.instance.Priority > a.instance.Priority || (b.instance.Priority == a.instance.Priority && b.seq < a.seq)
			if !less {
				break
			}
			ordered[j-1], ordered[j] = ordered[j], ordered[j-1]
		}
	}
	if limit > 0 && len(ordered) > limit {
		ordered = ordered[:limit]
	}
	out := make([]Instance, 0, len(ordered))
	for _, it := range ordered {
		out = append(out, it.instance)
	}
	return out
}

// findGroupable returns the index of a still-queued item eligible to
// absorb a candidate sharing key - within its own fixed window (anchored
// to that item's own QueuedAt, never extended) and not yet at the
// bounded member cap - or -1 if none exists (Stage 12B task Part 4-6).
// Never mutates the queue.
func (q *queue) findGroupable(key groupKey, now time.Time) int {
	for i, it := range q.items {
		if groupKeyFor(it.instance) != key {
			continue
		}
		if it.instance.GroupCount >= domain.MaxGroupMembers {
			continue
		}
		if !windowOpen(it.instance.QueuedAt, it.instance.GroupWindowMS, now) {
			continue
		}
		return i
	}
	return -1
}
