package chatoverlay

import (
	"testing"
	"time"
)

func TestExpiryQueueOrdersByEarliestFirst(t *testing.T) {
	q := newExpiryQueue()
	base := testTime
	q.schedule("late", base.Add(30*time.Second))
	q.schedule("early", base.Add(5*time.Second))
	q.schedule("mid", base.Add(15*time.Second))

	id, at, ok := q.peekEarliest()
	if !ok || id != "early" || !at.Equal(base.Add(5*time.Second)) {
		t.Fatalf("peekEarliest() = %q, %v, %v, want 'early'", id, at, ok)
	}
}

func TestExpiryQueueCancelRemovesEntry(t *testing.T) {
	q := newExpiryQueue()
	base := testTime
	q.schedule("a", base.Add(5*time.Second))
	q.schedule("b", base.Add(10*time.Second))
	q.cancel("a")

	id, _, ok := q.peekEarliest()
	if !ok || id != "b" {
		t.Fatalf("expected 'b' to be the only remaining entry after cancelling 'a', got %q, %v", id, ok)
	}
	if q.len() != 1 {
		t.Errorf("len() = %d, want 1", q.len())
	}
}

func TestExpiryQueueCancelUnknownIDIsHarmless(t *testing.T) {
	q := newExpiryQueue()
	q.cancel("never-scheduled") // must not panic
	if q.len() != 0 {
		t.Errorf("len() = %d, want 0", q.len())
	}
}

func TestExpiryQueueRescheduleUpdatesOrder(t *testing.T) {
	q := newExpiryQueue()
	base := testTime
	q.schedule("a", base.Add(30*time.Second))
	q.schedule("b", base.Add(5*time.Second))

	// Re-scheduling "a" to fire sooner than "b" must reorder the heap, not
	// create a second entry.
	q.schedule("a", base.Add(1*time.Second))
	if q.len() != 2 {
		t.Fatalf("len() = %d, want 2 (reschedule must replace, not duplicate)", q.len())
	}
	id, _, _ := q.peekEarliest()
	if id != "a" {
		t.Fatalf("peekEarliest() = %q, want 'a' after it was rescheduled sooner", id)
	}
}

func TestExpiryQueuePopDueReturnsOnlyDueEntriesInOrder(t *testing.T) {
	q := newExpiryQueue()
	base := testTime
	q.schedule("a", base.Add(5*time.Second))
	q.schedule("b", base.Add(10*time.Second))
	q.schedule("c", base.Add(20*time.Second))

	due := q.popDue(base.Add(12 * time.Second))
	if len(due) != 2 || due[0] != "a" || due[1] != "b" {
		t.Fatalf("popDue() = %v, want [a b] in expiry order", due)
	}
	if q.len() != 1 {
		t.Errorf("len() = %d, want 1 remaining", q.len())
	}

	id, _, ok := q.peekEarliest()
	if !ok || id != "c" {
		t.Fatalf("expected 'c' to remain scheduled, got %q, %v", id, ok)
	}
}

func TestExpiryQueuePeekEarliestEmptyIsFalse(t *testing.T) {
	q := newExpiryQueue()
	if _, _, ok := q.peekEarliest(); ok {
		t.Error("expected peekEarliest() on an empty queue to report ok=false")
	}
	if due := q.popDue(testTime.Add(time.Hour)); due != nil {
		t.Errorf("expected popDue() on an empty queue to return nothing, got %v", due)
	}
}
