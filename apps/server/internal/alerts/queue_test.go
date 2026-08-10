package alerts

import (
	"testing"
	"time"
)

func mkInstance(id string, priority int, queuedAt time.Time) Instance {
	return Instance{ID: id, Priority: priority, QueuedAt: queuedAt, DurationMS: 5000}
}

func TestQueueEmptyPopReturnsNotOK(t *testing.T) {
	q := newQueue(10, time.Minute)
	if _, _, ok := q.popNextEligible(time.Now()); ok {
		t.Error("popNextEligible() on an empty queue = ok, want not ok")
	}
}

func TestQueueEnqueueBelowCapacityAlwaysAccepted(t *testing.T) {
	q := newQueue(3, time.Minute)
	now := time.Now()
	for i := 0; i < 3; i++ {
		accepted, evicted := q.enqueue(mkInstance("a", 50, now), now)
		if !accepted || evicted != nil {
			t.Fatalf("enqueue() below capacity = accepted=%v evicted=%v, want true/nil", accepted, evicted)
		}
	}
	if q.len() != 3 {
		t.Errorf("len() = %d, want 3", q.len())
	}
}

func TestQueuePriorityOrdering(t *testing.T) {
	q := newQueue(10, time.Minute)
	now := time.Now()
	q.enqueue(mkInstance("low", 10, now), now)
	q.enqueue(mkInstance("high", 90, now), now)
	q.enqueue(mkInstance("mid", 50, now), now)

	inst, _, ok := q.popNextEligible(now)
	if !ok || inst.ID != "high" {
		t.Fatalf("popNextEligible() = %q, want high", inst.ID)
	}
	inst, _, ok = q.popNextEligible(now)
	if !ok || inst.ID != "mid" {
		t.Fatalf("popNextEligible() = %q, want mid", inst.ID)
	}
	inst, _, ok = q.popNextEligible(now)
	if !ok || inst.ID != "low" {
		t.Fatalf("popNextEligible() = %q, want low", inst.ID)
	}
}

func TestQueueFIFOWithinSamePriority(t *testing.T) {
	q := newQueue(10, time.Minute)
	now := time.Now()
	q.enqueue(mkInstance("first", 50, now), now)
	q.enqueue(mkInstance("second", 50, now), now)
	q.enqueue(mkInstance("third", 50, now), now)

	for _, want := range []string{"first", "second", "third"} {
		inst, _, ok := q.popNextEligible(now)
		if !ok || inst.ID != want {
			t.Fatalf("popNextEligible() = %q, want %q", inst.ID, want)
		}
	}
}

func TestQueueCapacityRejectsLowerOrEqualPriority(t *testing.T) {
	q := newQueue(2, time.Minute)
	now := time.Now()
	q.enqueue(mkInstance("a", 50, now), now)
	q.enqueue(mkInstance("b", 50, now), now)

	accepted, evicted := q.enqueue(mkInstance("c", 50, now), now)
	if accepted || evicted != nil {
		t.Errorf("enqueue() equal priority at capacity = accepted=%v, want rejected", accepted)
	}
	accepted, evicted = q.enqueue(mkInstance("d", 10, now), now)
	if accepted || evicted != nil {
		t.Errorf("enqueue() lower priority at capacity = accepted=%v, want rejected", accepted)
	}
	if q.len() != 2 {
		t.Errorf("len() = %d after rejected enqueues, want 2 (queue unchanged)", q.len())
	}
}

func TestQueueCapacityEvictsStrictlyLowerPriority(t *testing.T) {
	q := newQueue(2, time.Minute)
	now := time.Now()
	q.enqueue(mkInstance("low", 10, now), now)
	q.enqueue(mkInstance("mid", 50, now), now)

	accepted, evicted := q.enqueue(mkInstance("high", 90, now), now)
	if !accepted || evicted == nil || evicted.ID != "low" {
		t.Fatalf("enqueue() higher priority at capacity = accepted=%v evicted=%v, want true/low", accepted, evicted)
	}
	if q.len() != 2 {
		t.Fatalf("len() = %d, want 2 (still at capacity)", q.len())
	}
	inst, _, _ := q.popNextEligible(now)
	if inst.ID != "high" {
		t.Errorf("first popped = %q, want high", inst.ID)
	}
}

func TestQueueNeverEvictsCurrentlyPlaying(t *testing.T) {
	// Structural: queue has no concept of "current" at all - the
	// currently-playing alert lives only in profileRuntime.current, so
	// nothing about this type can ever evict it, by construction.
	q := newQueue(1, time.Minute)
	now := time.Now()
	q.enqueue(mkInstance("only", 0, now), now)
	if q.len() != 1 {
		t.Fatalf("len() = %d, want 1", q.len())
	}
}

func TestQueueExpiredItemsSkippedAndCounted(t *testing.T) {
	q := newQueue(10, 30*time.Second)
	base := time.Now()
	now := base.Add(time.Minute)

	q.enqueue(mkInstance("stale", 50, base), base)                    // 60s old at "now" - expired
	q.enqueue(mkInstance("fresh", 50, now.Add(-10*time.Second)), now) // 10s old at "now" - within bounds

	inst, expiredCount, ok := q.popNextEligible(now)
	if !ok || inst.ID != "fresh" {
		t.Fatalf("popNextEligible() = %q ok=%v, want fresh", inst.ID, ok)
	}
	if expiredCount != 1 {
		t.Errorf("expiredCount = %d, want 1 (the stale item)", expiredCount)
	}
}

func TestQueueMaxAgeZeroMeansNoExpiration(t *testing.T) {
	q := newQueue(10, 0)
	base := time.Now()
	q.enqueue(mkInstance("old", 50, base), base)
	_, expiredCount, ok := q.popNextEligible(base.Add(24 * time.Hour))
	if !ok || expiredCount != 0 {
		t.Errorf("popNextEligible() with maxAge=0 = ok=%v expiredCount=%d, want ok=true expiredCount=0", ok, expiredCount)
	}
}

func TestQueueClearReturnsItemsAndEmpties(t *testing.T) {
	q := newQueue(10, time.Minute)
	now := time.Now()
	q.enqueue(mkInstance("a", 50, now), now)
	q.enqueue(mkInstance("b", 50, now), now)
	cleared := q.clear()
	if len(cleared) != 2 {
		t.Errorf("clear() = %d items, want 2", len(cleared))
	}
	if q.len() != 0 {
		t.Errorf("len() after clear = %d, want 0", q.len())
	}
}

func TestQueueListBoundedAndOrdered(t *testing.T) {
	q := newQueue(10, time.Minute)
	now := time.Now()
	q.enqueue(mkInstance("low", 10, now), now)
	q.enqueue(mkInstance("high", 90, now), now)
	list := q.list(1)
	if len(list) != 1 || list[0].ID != "high" {
		t.Errorf("list(1) = %+v, want [high]", list)
	}
	full := q.list(0)
	if len(full) != 2 || full[0].ID != "high" || full[1].ID != "low" {
		t.Errorf("list(0) = %+v, want [high, low]", full)
	}
	if q.len() != 2 {
		t.Error("list() mutated the queue, want it untouched")
	}
}
