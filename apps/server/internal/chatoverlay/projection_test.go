package chatoverlay

import (
	"context"
	"testing"
	"time"

	chatoverlaydomain "github.com/streaming-tree/server/internal/domain/chatoverlay"
)

func newTestProjectionWithRingCapacity(t *testing.T, settings resolvedSettings, ringCapacity int) (*Projection, *fakeSource) {
	t.Helper()
	source := &fakeSource{}
	p := NewProjection(settings.profile.ID, source, nil, nil)
	if ringCapacity > 0 {
		p.ring = newRing(ringCapacity)
	}
	p.Start(context.Background())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		p.Shutdown(ctx)
	})
	p.Configure(settings)
	return p, source
}

func newTestProjection(t *testing.T, settings resolvedSettings) (*Projection, *fakeSource) {
	return newTestProjectionWithRingCapacity(t, settings, 0)
}

func TestProjectionUpsertThenRemoveOnDeletion(t *testing.T) {
	settings := testSettings(func(p *chatoverlaydomain.Profile) { p.ShowDeletedPlaceholder = false })
	p, source := newTestProjection(t, settings)

	sub, gap, err := p.Subscribe(0)
	if err != nil || gap {
		t.Fatalf("Subscribe() error = %v, gap = %v", err, gap)
	}
	defer sub.Cancel()
	// Drain the initial (empty) reset from Configure.
	if rev, ok := waitRevision(time.Second, sub.Revisions()); !ok || rev.Operation != OpReset {
		t.Fatalf("expected an initial empty reset, got %+v, ok=%v", rev, ok)
	}

	item := messageItem("m1", "acct_1", "u1", "viewer", "hello")
	source.add(item)
	p.HandleUpstreamItem(item)

	rev, ok := waitRevision(time.Second, sub.Revisions())
	if !ok || rev.Operation != OpUpsert || rev.Item == nil || rev.Item.ID != "m1" {
		t.Fatalf("expected an upsert for m1, got %+v, ok=%v", rev, ok)
	}

	deleted := deletedMessageItem("m1", "acct_1", "u1", "viewer", "hello")
	source.add(deleted)
	p.HandleUpstreamItem(deleted)

	rev, ok = waitRevision(time.Second, sub.Revisions())
	if !ok || rev.Operation != OpRemove || rev.RemovedID != "m1" {
		t.Fatalf("expected a remove for m1 after deletion, got %+v, ok=%v", rev, ok)
	}

	if items := p.CurrentItems(); len(items) != 0 {
		t.Errorf("expected no visible items after deletion, got %v", items)
	}
}

func TestProjectionDeletedMessageNeverLeaksOriginalTextInLaterSnapshot(t *testing.T) {
	settings := testSettings(func(p *chatoverlaydomain.Profile) { p.ShowDeletedPlaceholder = true })
	p, source := newTestProjection(t, settings)

	item := messageItem("m1", "acct_1", "u1", "viewer", "the original secret text")
	source.add(item)
	p.HandleUpstreamItem(item)

	deleted := deletedMessageItem("m1", "acct_1", "u1", "viewer", "the original secret text")
	source.add(deleted)
	p.HandleUpstreamItem(deleted)

	items := p.CurrentItems()
	if len(items) != 1 {
		t.Fatalf("expected the placeholder to remain visible, got %d items", len(items))
	}
	if !items[0].Deleted {
		t.Error("expected Deleted = true")
	}
	if items[0].Message != nil {
		t.Errorf("expected Message = nil in the snapshot, got %+v - original text must never leak", items[0].Message)
	}
}

func TestProjectionDeterministicMonotonicSequence(t *testing.T) {
	p, source := newTestProjection(t, testSettings(nil))
	sub, _, _ := p.Subscribe(0)
	defer sub.Cancel()
	waitRevision(time.Second, sub.Revisions()) // initial reset

	for i, id := range []string{"m1", "m2", "m3"} {
		item := messageItem(id, "acct_1", "u1", "viewer", "hello")
		source.add(item)
		p.HandleUpstreamItem(item)
		_ = i
	}

	var last uint64
	for i := 0; i < 3; i++ {
		rev, ok := waitRevision(time.Second, sub.Revisions())
		if !ok {
			t.Fatalf("expected revision %d, channel closed", i)
		}
		if rev.Sequence <= last {
			t.Fatalf("expected strictly increasing sequence, got %d after %d", rev.Sequence, last)
		}
		last = rev.Sequence
	}
}

func TestProjectionDuplicateUpstreamItemIsHarmless(t *testing.T) {
	p, source := newTestProjection(t, testSettings(nil))
	item := messageItem("m1", "acct_1", "u1", "viewer", "hello")
	source.add(item)

	p.HandleUpstreamItem(item)
	p.HandleUpstreamItem(item) // exact duplicate - must not panic or corrupt state

	items := p.CurrentItems()
	if len(items) != 1 {
		t.Fatalf("expected exactly one visible item after a duplicate upstream item, got %d", len(items))
	}
}

func TestProjectionMaxVisibleItemsEvictsOldest(t *testing.T) {
	settings := testSettings(func(p *chatoverlaydomain.Profile) { p.MaxVisibleItems = 2 })
	p, source := newTestProjection(t, settings)
	sub, _, _ := p.Subscribe(0)
	defer sub.Cancel()
	waitRevision(time.Second, sub.Revisions()) // initial reset

	for _, id := range []string{"m1", "m2", "m3"} {
		item := messageItem(id, "acct_1", "u1", "viewer", "hi")
		source.add(item)
		p.HandleUpstreamItem(item)
	}

	items := p.CurrentItems()
	if len(items) != 2 {
		t.Fatalf("expected exactly MaxVisibleItems=2 visible items, got %d: %+v", len(items), items)
	}
	if items[0].ID != "m2" || items[1].ID != "m3" {
		t.Errorf("expected the oldest item (m1) to be evicted, got %q then %q", items[0].ID, items[1].ID)
	}

	// Three upserts (m1, m2, m3) plus one eviction remove (m1) - the
	// eviction must reach the subscriber as its own explicit revision,
	// never a silent drop (see upsertVisibleLocked's own doc comment).
	sawEvictionRemove := false
	for i := 0; i < 4; i++ {
		rev, ok := waitRevision(time.Second, sub.Revisions())
		if !ok {
			break
		}
		if rev.Operation == OpRemove && rev.RemovedID == "m1" {
			sawEvictionRemove = true
		}
	}
	if !sawEvictionRemove {
		t.Error("expected a remove revision for the evicted item m1")
	}
}

// Message lifetime expiry is driven by a real *time.Timer (see
// projection.go's own doc comment: one managed timer per overlay, never
// one per message) - rather than faking the timer itself, this test uses
// a short real duration by giving the item an OccurredAt already close to
// its own expiry, keeping the test fast (well under a second) without
// needing a fake-timer abstraction. expiry_test.go already covers the
// pure scheduling logic deterministically with a fake clock.
func TestProjectionExpiryRemovesItemAfterLifetime(t *testing.T) {
	settings := testSettings(func(p *chatoverlaydomain.Profile) { p.MessageLifetimeSeconds = 1 })
	source := &fakeSource{}
	p := NewProjection(settings.profile.ID, source, nil, nil)
	p.Start(context.Background())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		p.Shutdown(ctx)
	})
	p.Configure(settings)

	sub, _, _ := p.Subscribe(0)
	defer sub.Cancel()
	waitRevision(time.Second, sub.Revisions()) // initial reset

	item := messageItem("m1", "acct_1", "u1", "viewer", "hi")
	item.OccurredAt = time.Now().Add(-900 * time.Millisecond) // ~100ms until expiry
	source.add(item)
	p.HandleUpstreamItem(item)

	rev, ok := waitRevision(time.Second, sub.Revisions())
	if !ok || rev.Operation != OpUpsert {
		t.Fatalf("expected an upsert first, got %+v, ok=%v", rev, ok)
	}

	rev, ok = waitRevision(2*time.Second, sub.Revisions())
	if !ok || rev.Operation != OpRemove || rev.RemovedID != "m1" {
		t.Fatalf("expected an expiry remove for m1, got %+v, ok=%v", rev, ok)
	}
	if items := p.CurrentItems(); len(items) != 0 {
		t.Errorf("expected no visible items after expiry, got %v", items)
	}
}

func TestProjectionExpiryTimerFiresForEarliestFirst(t *testing.T) {
	settings := testSettings(func(p *chatoverlaydomain.Profile) { p.MessageLifetimeSeconds = 5 })
	source := &fakeSource{}
	p := NewProjection(settings.profile.ID, source, nil, nil)
	p.Start(context.Background())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		p.Shutdown(ctx)
	})
	p.Configure(settings)

	sub, _, _ := p.Subscribe(0)
	defer sub.Cancel()
	waitRevision(time.Second, sub.Revisions()) // initial reset

	now := time.Now()
	slow := messageItem("slow", "acct_1", "u1", "viewer", "hi")
	slow.OccurredAt = now.Add(-4800 * time.Millisecond) // expires in ~200ms
	fast := messageItem("fast", "acct_1", "u2", "viewer2", "hi")
	fast.OccurredAt = now.Add(-4900 * time.Millisecond) // expires in ~100ms, scheduled second

	source.add(slow)
	p.HandleUpstreamItem(slow)
	waitRevision(time.Second, sub.Revisions()) // upsert slow

	source.add(fast)
	p.HandleUpstreamItem(fast)
	waitRevision(time.Second, sub.Revisions()) // upsert fast

	rev, ok := waitRevision(2*time.Second, sub.Revisions())
	if !ok || rev.Operation != OpRemove || rev.RemovedID != "fast" {
		t.Fatalf("expected the earlier-expiring item to be removed first, got %+v, ok=%v", rev, ok)
	}
}

func TestProjectionConfigureProducesReset(t *testing.T) {
	p, source := newTestProjection(t, testSettings(nil))
	item := messageItem("m1", "acct_1", "u1", "viewer", "hi")
	source.add(item)
	p.HandleUpstreamItem(item)

	sub, _, _ := p.Subscribe(0)
	defer sub.Cancel()
	// Drain the subscribe-time replay (the initial empty reset, then the
	// m1 upsert) before triggering Configure, so the next read is
	// Configure's own new reset rather than backlog.
	waitRevision(time.Second, sub.Revisions())
	waitRevision(time.Second, sub.Revisions())

	newSettings := testSettings(func(p *chatoverlaydomain.Profile) { p.ShowAvatar = true })
	p.Configure(newSettings)

	rev, ok := waitRevision(time.Second, sub.Revisions())
	if !ok || rev.Operation != OpReset {
		t.Fatalf("expected a reset after Configure, got %+v, ok=%v", rev, ok)
	}
	if len(rev.ResetItems) != 1 || rev.ResetItems[0].ID != "m1" {
		t.Fatalf("expected the reset to contain the recomputed visible set, got %+v", rev.ResetItems)
	}
}

func TestProjectionConfigureAccountFilterChangeHidesPreviouslyVisibleItems(t *testing.T) {
	p, source := newTestProjection(t, testSettings(nil))
	item := messageItem("m1", "acct_2", "u1", "viewer", "hi")
	source.add(item)
	p.HandleUpstreamItem(item)
	if len(p.CurrentItems()) != 1 {
		t.Fatal("expected the item to be visible before the account filter changes")
	}

	restricted := testSettings(nil)
	restricted.accountIDs = toSet([]string{"acct_1"}) // acct_2 no longer selected
	p.Configure(restricted)

	if items := p.CurrentItems(); len(items) != 0 {
		t.Errorf("expected the item from the now-excluded account to be gone, got %v", items)
	}
}

func TestProjectionSubscribeReplaysThenGoesLive(t *testing.T) {
	p, source := newTestProjection(t, testSettings(nil))
	item := messageItem("m1", "acct_1", "u1", "viewer", "hi")
	source.add(item)
	p.HandleUpstreamItem(item)

	sub, gap, err := p.Subscribe(0)
	if err != nil || gap {
		t.Fatalf("Subscribe(0) error = %v, gap = %v", err, gap)
	}
	defer sub.Cancel()

	replayed, ok := waitRevision(time.Second, sub.Revisions())
	if !ok || replayed.Operation != OpReset {
		t.Fatalf("expected replay to start with the retained reset, got %+v, ok=%v", replayed, ok)
	}
	upsert, ok := waitRevision(time.Second, sub.Revisions())
	if !ok || upsert.Operation != OpUpsert || upsert.Item.ID != "m1" {
		t.Fatalf("expected the replayed upsert for m1, got %+v, ok=%v", upsert, ok)
	}

	item2 := messageItem("m2", "acct_1", "u1", "viewer", "second")
	source.add(item2)
	p.HandleUpstreamItem(item2)
	live, ok := waitRevision(time.Second, sub.Revisions())
	if !ok || live.Operation != OpUpsert || live.Item.ID != "m2" {
		t.Fatalf("expected a live upsert for m2 after replay, got %+v, ok=%v", live, ok)
	}
}

func TestProjectionSubscribeReportsGapWhenAfterIsTooOld(t *testing.T) {
	p, source := newTestProjectionWithRingCapacity(t, testSettings(nil), 3)
	for _, id := range []string{"m1", "m2", "m3", "m4", "m5"} {
		item := messageItem(id, "acct_1", "u1", "viewer", "hi")
		source.add(item)
		p.HandleUpstreamItem(item)
	}

	_, gap, err := p.Subscribe(1)
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	if !gap {
		t.Error("expected a gap when the requested sequence has been evicted from the retained ring")
	}
}

func TestProjectionSnapshotStatusReportsCounts(t *testing.T) {
	settings := testSettings(func(p *chatoverlaydomain.Profile) { p.MaxVisibleItems = 10 })
	p, source := newTestProjection(t, settings)
	item := messageItem("m1", "acct_1", "u1", "viewer", "hi")
	source.add(item)
	p.HandleUpstreamItem(item)

	status := p.Status()
	if status.VisibleCount != 1 {
		t.Errorf("VisibleCount = %d, want 1", status.VisibleCount)
	}
	if status.MaxVisibleItems != 10 {
		t.Errorf("MaxVisibleItems = %d, want 10", status.MaxVisibleItems)
	}
	if status.SchemaVersion != CurrentVersion {
		t.Errorf("SchemaVersion = %d, want %d", status.SchemaVersion, CurrentVersion)
	}
}

func TestProjectionCurrentItemsIsSnapshotNotRevisionLog(t *testing.T) {
	p, source := newTestProjection(t, testSettings(nil))
	for _, id := range []string{"m1", "m2"} {
		item := messageItem(id, "acct_1", "u1", "viewer", "hi")
		source.add(item)
		p.HandleUpstreamItem(item)
	}
	deleted := deletedMessageItem("m1", "acct_1", "u1", "viewer", "hi")
	source.add(deleted)
	p.HandleUpstreamItem(deleted)

	items := p.CurrentItems()
	if len(items) != 1 || items[0].ID != "m2" {
		t.Fatalf("expected the snapshot to reflect only current state (m2), got %+v", items)
	}
}

func TestProjectionShutdownClosesSubscribersWithReasonShutdown(t *testing.T) {
	source := &fakeSource{}
	p := NewProjection("ov_shutdown", source, nil, nil)
	p.Start(context.Background())
	p.Configure(testSettings(nil))

	sub, _, _ := p.Subscribe(0)
	waitRevision(time.Second, sub.Revisions())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	p.Shutdown(ctx)

	select {
	case reason := <-sub.Closed():
		if reason != ReasonShutdown {
			t.Errorf("close reason = %q, want %q", reason, ReasonShutdown)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the subscription to close after Shutdown")
	}
}

func TestProjectionIndependentOverlaysDoNotShareState(t *testing.T) {
	hidden := testSettings(func(p *chatoverlaydomain.Profile) { p.ID = "ov_a" })
	hidden.hiddenUsers = toSet([]string{userKey("twitch", "acct_1", "u1")})
	visible := testSettings(func(p *chatoverlaydomain.Profile) { p.ID = "ov_b" })

	pa, sourceA := newTestProjection(t, hidden)
	pb, sourceB := newTestProjection(t, visible)

	item := messageItem("m1", "acct_1", "u1", "viewer", "hi")
	sourceA.add(item)
	sourceB.add(item)
	pa.HandleUpstreamItem(item)
	pb.HandleUpstreamItem(item)

	if len(pa.CurrentItems()) != 0 {
		t.Error("expected overlay A to hide the item (its own hidden-user list)")
	}
	if len(pb.CurrentItems()) != 1 {
		t.Error("expected overlay B, with no hidden-user entry, to show the item")
	}
}
