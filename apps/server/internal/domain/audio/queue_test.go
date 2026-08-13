package audio

import (
	"testing"
	"time"
)

func testItem(id string, enqueuedAt time.Time, ttl time.Duration) Item {
	it := Item{ID: id, Text: "hello", EnqueuedAt: enqueuedAt}
	if ttl > 0 {
		it.ExpiresAt = enqueuedAt.Add(ttl)
	}
	return it
}

func TestQueueEnqueueUpToCapacityThenDrops(t *testing.T) {
	q := NewQueue(2)
	now := time.Now().UTC()

	if !q.Enqueue(testItem("a", now, time.Minute)) {
		t.Fatal("Enqueue() = false for item 1, want true")
	}
	if !q.Enqueue(testItem("b", now, time.Minute)) {
		t.Fatal("Enqueue() = false for item 2, want true")
	}
	if q.Enqueue(testItem("c", now, time.Minute)) {
		t.Fatal("Enqueue() = true at capacity, want false (dropped)")
	}

	counters := q.Counters()
	if counters.TotalEnqueued != 2 || counters.TotalCapacityDropped != 1 {
		t.Errorf("counters = %+v, want 2 enqueued, 1 dropped", counters)
	}
}

func TestQueuePendingSharesCapacityWithReady(t *testing.T) {
	q := NewQueue(2)
	now := time.Now().UTC()

	if !q.Enqueue(testItem("a", now, time.Minute)) {
		t.Fatal("Enqueue() = false, want true")
	}
	if !q.EnqueuePending(testItem("b", now, time.Minute)) {
		t.Fatal("EnqueuePending() = false, want true")
	}
	if q.EnqueuePending(testItem("c", now, time.Minute)) {
		t.Fatal("EnqueuePending() = true beyond combined capacity, want false")
	}
	if q.ReadyLen() != 1 || q.PendingLen() != 1 {
		t.Errorf("ReadyLen/PendingLen = %d/%d, want 1/1", q.ReadyLen(), q.PendingLen())
	}
}

func TestQueueApproveMovesPendingToReadyUnchanged(t *testing.T) {
	q := NewQueue(10)
	now := time.Now().UTC()
	it := testItem("a", now, time.Minute)
	it.Snapshot = ItemSnapshot{VoiceID: "voice-1"}
	q.EnqueuePending(it)

	approved, ok := q.Approve("a")
	if !ok {
		t.Fatal("Approve() ok = false, want true")
	}
	if approved.Snapshot.VoiceID != "voice-1" {
		t.Errorf("approved.Snapshot.VoiceID = %q, want unchanged voice-1", approved.Snapshot.VoiceID)
	}
	if q.PendingLen() != 0 || q.ReadyLen() != 1 {
		t.Errorf("PendingLen/ReadyLen = %d/%d after approve, want 0/1", q.PendingLen(), q.ReadyLen())
	}
}

func TestQueueApproveUnknownIDFails(t *testing.T) {
	q := NewQueue(10)
	if _, ok := q.Approve("nope"); ok {
		t.Error("Approve() ok = true for an unknown id, want false")
	}
}

func TestQueueRejectDiscardsAndCounts(t *testing.T) {
	q := NewQueue(10)
	now := time.Now().UTC()
	q.EnqueuePending(testItem("a", now, time.Minute))

	rejected, ok := q.Reject("a")
	if !ok || rejected.ID != "a" {
		t.Fatalf("Reject() = %+v, %v, want item a, true", rejected, ok)
	}
	if q.PendingLen() != 0 || q.ReadyLen() != 0 {
		t.Errorf("queue not empty after reject: ready=%d pending=%d", q.ReadyLen(), q.PendingLen())
	}
	if q.Counters().TotalRejected != 1 {
		t.Errorf("TotalRejected = %d, want 1", q.Counters().TotalRejected)
	}
}

func TestQueuePopNextEligibleFIFO(t *testing.T) {
	q := NewQueue(10)
	now := time.Now().UTC()
	q.Enqueue(testItem("a", now, time.Minute))
	q.Enqueue(testItem("b", now.Add(time.Second), time.Minute))

	first, ok := q.PopNextEligible(now.Add(2 * time.Second))
	if !ok || first.ID != "a" {
		t.Fatalf("PopNextEligible() = %+v, %v, want item a first", first, ok)
	}
	second, ok := q.PopNextEligible(now.Add(2 * time.Second))
	if !ok || second.ID != "b" {
		t.Fatalf("PopNextEligible() = %+v, %v, want item b second", second, ok)
	}
	if _, ok := q.PopNextEligible(now); ok {
		t.Error("PopNextEligible() ok = true on an empty queue, want false")
	}
}

func TestQueuePopNextEligibleSkipsExpiredAndCounts(t *testing.T) {
	q := NewQueue(10)
	now := time.Now().UTC()
	q.Enqueue(testItem("expired", now, time.Second))
	q.Enqueue(testItem("fresh", now, time.Hour))

	got, ok := q.PopNextEligible(now.Add(time.Minute))
	if !ok || got.ID != "fresh" {
		t.Fatalf("PopNextEligible() = %+v, %v, want the fresh item, expired one skipped", got, ok)
	}
	if q.Counters().TotalExpired != 1 {
		t.Errorf("TotalExpired = %d, want 1", q.Counters().TotalExpired)
	}
}

func TestQueueClearEmptiesBothListsAndReturnsCount(t *testing.T) {
	q := NewQueue(10)
	now := time.Now().UTC()
	q.Enqueue(testItem("a", now, time.Minute))
	q.EnqueuePending(testItem("b", now, time.Minute))

	n := q.Clear()
	if n != 2 {
		t.Errorf("Clear() = %d, want 2", n)
	}
	if q.ReadyLen() != 0 || q.PendingLen() != 0 {
		t.Errorf("queue not empty after Clear(): ready=%d pending=%d", q.ReadyLen(), q.PendingLen())
	}
}

func TestQueuePendingListIsDefensiveCopy(t *testing.T) {
	q := NewQueue(10)
	now := time.Now().UTC()
	q.EnqueuePending(testItem("a", now, time.Minute))

	list := q.PendingList()
	list[0].ID = "mutated"

	if q.PendingList()[0].ID != "a" {
		t.Error("PendingList() returned a slice that aliases internal state")
	}
}

func TestQueueSyntheticCounterAndManualSkip(t *testing.T) {
	q := NewQueue(10)
	now := time.Now().UTC()
	it := testItem("a", now, time.Minute)
	it.Synthetic = true
	q.Enqueue(it)

	if q.Counters().TotalSynthetic != 1 {
		t.Errorf("TotalSynthetic = %d, want 1", q.Counters().TotalSynthetic)
	}

	q.RecordManualSkip()
	if q.Counters().TotalManuallySkipped != 1 {
		t.Errorf("TotalManuallySkipped = %d, want 1", q.Counters().TotalManuallySkipped)
	}
}

func TestNewItemIDIsUniqueAndPrefixed(t *testing.T) {
	a, err := NewItemID()
	if err != nil {
		t.Fatalf("NewItemID() error = %v", err)
	}
	b, err := NewItemID()
	if err != nil {
		t.Fatalf("NewItemID() error = %v", err)
	}
	if a == b {
		t.Error("NewItemID() returned the same id twice")
	}
	if len(a) < len("auditem_") || a[:len("auditem_")] != "auditem_" {
		t.Errorf("NewItemID() = %q, want auditem_ prefix", a)
	}
}
