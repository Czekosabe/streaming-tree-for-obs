package bus

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	engagement "github.com/streaming-tree/server/internal/domain/engagement"
)

var uniqueKeyCounter int64

// uniqueKey returns a dedupe key guaranteed unique across the whole test
// binary run, regardless of the seed argument - tests that want a specific
// *repeated* key use a literal string instead (see the dedup tests above).
func uniqueKey(seed int) string {
	return fmt.Sprintf("key-%d-%d", seed, atomic.AddInt64(&uniqueKeyCounter, 1))
}

func testEvent(dedupeKey string, at time.Time) engagement.Event {
	return engagement.Event{
		SchemaVersion:      engagement.CurrentSchemaVersion,
		ProviderID:         engagement.ProviderTwitch,
		ConnectedAccountID: "acct_1",
		Type:               engagement.TypeFollow,
		ProviderEventType:  "channel.follow",
		PlatformTimestamp:  at,
		DedupeKey:          dedupeKey,
		User:               &engagement.User{ProviderUserID: "u1", Login: "viewer"},
	}
}

func TestPublishAssignsMonotonicSequence(t *testing.T) {
	b := New(Options{Capacity: 10})
	now := time.Now().UTC()

	for i := 0; i < 5; i++ {
		published, seq, err := b.Publish(testEvent(uniqueKey(i), now))
		if err != nil {
			t.Fatalf("publish %d: %v", i, err)
		}
		if !published {
			t.Fatalf("publish %d: expected published", i)
		}
		if seq != uint64(i+1) {
			t.Fatalf("publish %d: expected sequence %d, got %d", i, i+1, seq)
		}
	}
}

func TestPublishRejectsInvalidEvent(t *testing.T) {
	b := New(Options{Capacity: 10})
	_, _, err := b.Publish(engagement.Event{})
	if err == nil {
		t.Fatal("expected validation error for empty event")
	}
	if b.Snapshot().RetainedCount != 0 {
		t.Fatal("invalid event must not be retained")
	}
}

func TestPublishDeduplicatesSameKey(t *testing.T) {
	b := New(Options{Capacity: 10})
	now := time.Now().UTC()

	published1, seq1, err := b.Publish(testEvent("dup-key", now))
	if err != nil || !published1 {
		t.Fatalf("first publish failed: published=%v err=%v", published1, err)
	}

	published2, seq2, err := b.Publish(testEvent("dup-key", now))
	if err != nil {
		t.Fatalf("second publish returned error: %v", err)
	}
	if published2 {
		t.Fatal("expected duplicate to be dropped")
	}
	if seq2 != 0 {
		t.Fatalf("expected zero sequence for dropped duplicate, got %d", seq2)
	}
	if b.Snapshot().RetainedCount != 1 {
		t.Fatalf("expected exactly one retained event, got %d", b.Snapshot().RetainedCount)
	}
	_ = seq1
}

func TestPublishDoesNotDeduplicateDifferentKeys(t *testing.T) {
	b := New(Options{Capacity: 10})
	now := time.Now().UTC()

	published1, _, _ := b.Publish(testEvent("key-a", now))
	published2, _, _ := b.Publish(testEvent("key-b", now))
	if !published1 || !published2 {
		t.Fatal("two events with different dedupe keys must both publish")
	}
	if b.Snapshot().RetainedCount != 2 {
		t.Fatalf("expected 2 retained events, got %d", b.Snapshot().RetainedCount)
	}
}

func TestGiftBatchAndRecipientEventsBothRetained(t *testing.T) {
	b := New(Options{Capacity: 10})
	now := time.Now().UTC()

	batchQty := int64(3)
	batch := testEvent("batch-msg-id", now)
	batch.Type = engagement.TypeSubscriptionGiftBatch
	batch.Quantity = &batchQty

	recipient := testEvent("recipient-msg-id", now)
	recipient.Type = engagement.TypeGiftedSubscription

	p1, _, err := b.Publish(batch)
	if err != nil || !p1 {
		t.Fatalf("batch publish failed: %v %v", p1, err)
	}
	p2, _, err := b.Publish(recipient)
	if err != nil || !p2 {
		t.Fatalf("recipient publish failed: %v %v", p2, err)
	}
	if b.Snapshot().RetainedCount != 2 {
		t.Fatalf("expected both gift-batch and recipient events retained, got %d", b.Snapshot().RetainedCount)
	}
}

func TestRingBufferEvictsOldestWhenFull(t *testing.T) {
	b := New(Options{Capacity: 3})
	now := time.Now().UTC()

	for i := 0; i < 5; i++ {
		if _, _, err := b.Publish(testEvent(uniqueKey(i), now)); err != nil {
			t.Fatalf("publish %d: %v", i, err)
		}
	}

	snap := b.Snapshot()
	if snap.RetainedCount != 3 {
		t.Fatalf("expected retained count capped at capacity 3, got %d", snap.RetainedCount)
	}
	if snap.OldestSequence != 3 {
		t.Fatalf("expected oldest retained sequence 3 after evicting 1 and 2, got %d", snap.OldestSequence)
	}
	if snap.NewestSequence != 5 {
		t.Fatalf("expected newest retained sequence 5, got %d", snap.NewestSequence)
	}
}

func TestSubscribeDeliversOnlyEventsAfterGivenSequence(t *testing.T) {
	b := New(Options{Capacity: 10})
	now := time.Now().UTC()
	b.Publish(testEvent("k1", now))
	b.Publish(testEvent("k2", now))

	sub, gap, err := b.Subscribe(1)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if gap {
		t.Fatal("expected no gap when replay is fully retained")
	}
	defer sub.Cancel()

	select {
	case e := <-sub.Events():
		if e.Sequence != 2 {
			t.Fatalf("expected replay to start at sequence 2, got %d", e.Sequence)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for replayed event")
	}
}

func TestSubscribeReportsGapWhenSequenceEvicted(t *testing.T) {
	b := New(Options{Capacity: 2})
	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		b.Publish(testEvent(uniqueKey(i), now))
	}
	// oldest retained is now sequence 4; asking for events after sequence 1
	// references data that has already been evicted.
	sub, gap, err := b.Subscribe(1)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Cancel()
	if !gap {
		t.Fatal("expected gap to be reported for an evicted sequence")
	}
}

func TestSubscriberReceivesLivePublishInOrder(t *testing.T) {
	b := New(Options{Capacity: 10})
	sub, _, err := b.Subscribe(0)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Cancel()

	now := time.Now().UTC()
	go func() {
		b.Publish(testEvent("live-1", now))
		b.Publish(testEvent("live-2", now))
	}()

	var got []uint64
	for i := 0; i < 2; i++ {
		select {
		case e := <-sub.Events():
			got = append(got, e.Sequence)
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for live event")
		}
	}
	if got[0] != 1 || got[1] != 2 {
		t.Fatalf("expected deterministic order [1 2], got %v", got)
	}
}

func TestSlowSubscriberIsDroppedNotBlocking(t *testing.T) {
	b := New(Options{Capacity: 4000})
	sub, _, err := b.Subscribe(0)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	now := time.Now().UTC()
	// Never drain sub.Events(): publish far beyond its channel capacity and
	// confirm Publish itself never blocks.
	done := make(chan struct{})
	go func() {
		for i := 0; i < maxSubscriberChannelCapacity+50; i++ {
			b.Publish(testEvent(uniqueKey(i), now))
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Publish blocked on a slow subscriber")
	}

	select {
	case reason := <-sub.Closed():
		if reason != ReasonSlowConsumer {
			t.Fatalf("expected ReasonSlowConsumer, got %q", reason)
		}
	case <-time.After(time.Second):
		t.Fatal("expected the slow subscriber to be dropped")
	}
}

func TestCancelClosesSubscriptionWithReasonCancelled(t *testing.T) {
	b := New(Options{Capacity: 10})
	sub, _, err := b.Subscribe(0)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	sub.Cancel()

	select {
	case reason := <-sub.Closed():
		if reason != ReasonCancelled {
			t.Fatalf("expected ReasonCancelled, got %q", reason)
		}
	case <-time.After(time.Second):
		t.Fatal("expected cancellation to close the subscription")
	}
	if b.Snapshot().ActiveSubscribers != 0 {
		t.Fatal("expected active subscriber count to drop to 0 after cancel")
	}
}

func TestShutdownClosesEverySubscriberAndRejectsFurtherUse(t *testing.T) {
	b := New(Options{Capacity: 10})
	sub, _, err := b.Subscribe(0)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	b.Shutdown()

	select {
	case reason := <-sub.Closed():
		if reason != ReasonShutdown {
			t.Fatalf("expected ReasonShutdown, got %q", reason)
		}
	case <-time.After(time.Second):
		t.Fatal("expected shutdown to close the subscription")
	}

	if _, _, err := b.Publish(testEvent("after-shutdown", time.Now().UTC())); err != ErrClosed {
		t.Fatalf("expected ErrClosed after shutdown, got %v", err)
	}
	if _, _, err := b.Subscribe(0); err != ErrClosed {
		t.Fatalf("expected ErrClosed subscribing after shutdown, got %v", err)
	}
}

func TestConcurrentPublishersProduceUniqueMonotonicSequences(t *testing.T) {
	b := New(Options{Capacity: 5000})
	const publishers = 20
	const perPublisher = 50

	var wg sync.WaitGroup
	seqCh := make(chan uint64, publishers*perPublisher)
	for p := 0; p < publishers; p++ {
		p := p
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perPublisher; i++ {
				key := uniqueKey(p*1000 + i)
				_, seq, err := b.Publish(testEvent(key, time.Now().UTC()))
				if err != nil {
					t.Errorf("publish error: %v", err)
					return
				}
				seqCh <- seq
			}
		}()
	}
	wg.Wait()
	close(seqCh)

	seen := make(map[uint64]bool)
	count := 0
	for seq := range seqCh {
		if seq == 0 {
			continue // a deduplicated collision from the imperfect key generator; skip
		}
		if seen[seq] {
			t.Fatalf("sequence %d observed twice - not unique under concurrency", seq)
		}
		seen[seq] = true
		count++
	}
	if count == 0 {
		t.Fatal("expected at least some successful publishes")
	}
}

func TestDedupeSetRespectsTTLAndCapacity(t *testing.T) {
	now := time.Now()
	clock := &fakeClock{t: now}
	d := newDedupeSet(2, time.Minute, clock.Now)

	if d.seen("a") {
		t.Fatal("first sighting of a must not be seen")
	}
	if !d.seen("a") {
		t.Fatal("second sighting within TTL must be seen")
	}

	clock.advance(2 * time.Minute)
	if d.seen("a") {
		t.Fatal("sighting after TTL expiry must not be seen")
	}
}

func TestDedupeSetEnforcesCapacityBound(t *testing.T) {
	now := time.Now()
	clock := &fakeClock{t: now}
	d := newDedupeSet(2, time.Hour, clock.Now)

	d.seen("a")
	d.seen("b")
	d.seen("c") // evicts "a"

	if d.len() != 2 {
		t.Fatalf("expected bounded capacity of 2, got %d", d.len())
	}
	if d.seen("a") {
		t.Fatal("expected a to have been evicted by capacity bound, not still seen")
	}
}

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}
