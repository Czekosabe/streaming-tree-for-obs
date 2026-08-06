package operatorchat

import (
	"context"
	"testing"
	"time"

	bus "github.com/streaming-tree/server/internal/engagement"
)

func TestMultipleSubscribersEachReceiveEveryItem(t *testing.T) {
	p, b := newTestProjection(t, 100)
	subA, _, _ := p.Subscribe(0)
	subB, _, _ := p.Subscribe(0)
	defer subA.Cancel()
	defer subB.Cancel()

	b.Publish(chatMessageEvent("acct_1", "msg_1", "dedupe_1", "u1", "viewer", "hi"))

	itemA := waitForItem(t, subA, time.Second)
	itemB := waitForItem(t, subB, time.Second)
	if itemA.ID != itemB.ID || itemA.Sequence != itemB.Sequence {
		t.Errorf("expected both subscribers to see the same item, got %+v vs %+v", itemA, itemB)
	}
}

func TestSlowSubscriberIsDroppedWithoutBlockingOthers(t *testing.T) {
	// A tiny projection capacity keeps the subscriber channel small
	// (subscriberCapacityLocked mirrors the projection capacity), so a
	// consumer that never reads falls behind after a few publishes.
	p, b := newTestProjection(t, 3)
	slow, _, _ := p.Subscribe(0)
	fast, _, _ := p.Subscribe(0)
	defer fast.Cancel()

	for i := 0; i < 10; i++ {
		b.Publish(chatMessageEvent("acct_1", "msg", "dedupe_"+string(rune('a'+i)), "u1", "viewer", "hi"))
	}

	// The fast subscriber must still receive every item even though the
	// slow one never drained its channel.
	for i := 0; i < 10; i++ {
		waitForItem(t, fast, time.Second)
	}

	select {
	case reason := <-slow.Closed():
		if reason != ReasonSlowConsumer {
			t.Errorf("Closed() reason = %q, want %q", reason, ReasonSlowConsumer)
		}
	case <-time.After(time.Second):
		t.Fatal("expected the slow subscriber to be dropped with ReasonSlowConsumer")
	}
}

func TestSnapshotThenLiveHandoffIsContiguous(t *testing.T) {
	p, b := newTestProjection(t, 100)

	b.Publish(chatMessageEvent("acct_1", "msg_1", "dedupe_1", "u1", "viewer", "before"))
	b.Publish(chatMessageEvent("acct_1", "msg_2", "dedupe_2", "u1", "viewer", "before2"))

	// Give the projection's own goroutine a moment to have definitely
	// applied both revisions before we take our "snapshot" via Subscribe.
	deadline := time.Now().Add(time.Second)
	for {
		items, _ := p.ItemsAfter(0, 0)
		if len(items) == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for both prior items to land, got %d", len(items))
		}
		time.Sleep(time.Millisecond)
	}

	sub, gap, err := p.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer sub.Cancel()
	if gap {
		t.Error("expected no gap when nothing has been evicted yet")
	}

	first := waitForItem(t, sub, time.Second)
	second := waitForItem(t, sub, time.Second)
	if first.Sequence != 1 || second.Sequence != 2 {
		t.Fatalf("expected replayed sequences 1,2 - got %d,%d", first.Sequence, second.Sequence)
	}

	b.Publish(chatMessageEvent("acct_1", "msg_3", "dedupe_3", "u1", "viewer", "live"))
	third := waitForItem(t, sub, time.Second)
	if third.Sequence != 3 {
		t.Fatalf("expected the live item to continue the same sequence space at 3, got %d", third.Sequence)
	}
}

func TestSubscribeAfterEvictionReportsGap(t *testing.T) {
	// Capacity 2 retains only the newest 2 revisions. After 4 publishes,
	// sequences 1 and 2 are evicted, leaving 3 and 4 retained (oldest=3).
	// A subscriber resuming from after=1 (it last saw sequence 1, unaware
	// sequence 2 also came and went) is missing sequence 2 - a genuine gap,
	// unlike resuming from after=2 which is exactly contiguous with the
	// retained window and reports no gap.
	p, b := newTestProjection(t, 2)

	for i := 0; i < 4; i++ {
		b.Publish(chatMessageEvent("acct_1", "msg", "dedupe_"+string(rune('a'+i)), "u1", "viewer", "hi"))
	}

	deadline := time.Now().Add(time.Second)
	for {
		items, _ := p.ItemsAfter(0, 0)
		if len(items) == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for the ring to fill and evict")
		}
		time.Sleep(time.Millisecond)
	}

	_, gap, err := p.Subscribe(1)
	if err != nil {
		t.Fatalf("Subscribe(1) error = %v", err)
	}
	if !gap {
		t.Error("expected Subscribe(1) to report a gap: sequence 2 was evicted and never delivered")
	}

	_, noGap, err := p.Subscribe(2)
	if err != nil {
		t.Fatalf("Subscribe(2) error = %v", err)
	}
	if noGap {
		t.Error("expected Subscribe(2) to report no gap: contiguous with the retained window starting at 3")
	}
}

func TestItemsAfterRespectsLimit(t *testing.T) {
	p, b := newTestProjection(t, 100)
	for i := 0; i < 5; i++ {
		b.Publish(chatMessageEvent("acct_1", "msg", "dedupe_"+string(rune('a'+i)), "u1", "viewer", "hi"))
	}

	deadline := time.Now().Add(time.Second)
	for {
		items, _ := p.ItemsAfter(0, 0)
		if len(items) == 5 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for all 5 items")
		}
		time.Sleep(time.Millisecond)
	}

	limited, _ := p.ItemsAfter(0, 2)
	if len(limited) != 2 {
		t.Fatalf("ItemsAfter(0, 2) len = %d, want 2", len(limited))
	}
	if limited[0].Sequence != 1 || limited[1].Sequence != 2 {
		t.Errorf("expected the first two sequences 1,2 - got %d,%d", limited[0].Sequence, limited[1].Sequence)
	}
}

func TestShutdownClosesLiveSubscriptions(t *testing.T) {
	b := bus.New(bus.Options{Capacity: 100})
	defer b.Shutdown()

	p := New(Options{Source: b, Capacity: 100})
	if err := p.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	sub, _, err := p.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	p.Shutdown(ctx)

	select {
	case reason := <-sub.Closed():
		if reason != ReasonShutdown {
			t.Errorf("Closed() reason = %q, want %q", reason, ReasonShutdown)
		}
	case <-time.After(time.Second):
		t.Fatal("expected Shutdown to close the live subscription")
	}
}

func TestSubscribeAfterShutdownReturnsErrClosed(t *testing.T) {
	b := bus.New(bus.Options{Capacity: 100})
	defer b.Shutdown()

	p := New(Options{Source: b, Capacity: 100})
	if err := p.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	p.Shutdown(ctx)

	if _, _, err := p.Subscribe(0); err != ErrClosed {
		t.Errorf("Subscribe() after shutdown error = %v, want ErrClosed", err)
	}
}

func TestStatusReportsRetainedCountsAndSubscriberCount(t *testing.T) {
	p, b := newTestProjection(t, 100)
	sub, _, _ := p.Subscribe(0)
	defer sub.Cancel()

	b.Publish(chatMessageEvent("acct_1", "msg_1", "dedupe_1", "u1", "viewer", "hi"))
	waitForItem(t, sub, time.Second)

	status := p.Snapshot()
	if status.RetainedCount != 1 {
		t.Errorf("RetainedCount = %d, want 1", status.RetainedCount)
	}
	if status.NewestSequence != 1 || status.OldestSequence != 1 {
		t.Errorf("Oldest/NewestSequence = %d/%d, want 1/1", status.OldestSequence, status.NewestSequence)
	}
	if status.ActiveSubscribers != 1 {
		t.Errorf("ActiveSubscribers = %d, want 1", status.ActiveSubscribers)
	}
	if status.BusGap {
		t.Error("expected BusGap = false for a freshly-started projection consuming from sequence 0")
	}
}
