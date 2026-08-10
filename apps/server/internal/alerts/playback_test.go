package alerts

import (
	"testing"
	"time"

	domain "github.com/streaming-tree/server/internal/domain/alerts"
)

func testProfile() domain.Profile {
	p := domain.DefaultProfile("Main")
	p.ID = "alprof_1"
	p.PublicSlug = "slug1"
	p.MaxQueueItems = 10
	p.MaximumQueueAgeSeconds = 120
	return p
}

func staticID() (string, error) { return "alinst_test", nil }

func newTestRuntime() *profileRuntime {
	return newProfileRuntime("alprof_1", testProfile(), staticID)
}

func TestProfileRuntimeTickPromotesHighestPriority(t *testing.T) {
	pr := newTestRuntime()
	now := time.Now()
	pr.enqueueMatched([]Instance{
		mkInstance("low", 10, now), mkInstance("high", 90, now),
	}, now, staticID)

	pr.tick(now)
	st := pr.status()
	if st.Current == nil || st.Current.AlertID != "alinst_test" {
		t.Fatalf("status().Current = %+v, want a current alert", st.Current)
	}
	if st.QueuedCount != 1 {
		t.Errorf("QueuedCount = %d, want 1 (the low-priority item still queued)", st.QueuedCount)
	}
}

func TestProfileRuntimeCompletesAfterDuration(t *testing.T) {
	pr := newTestRuntime()
	now := time.Now()
	inst := mkInstance("a", 50, now)
	inst.DurationMS = 1000
	pr.enqueueMatched([]Instance{inst}, now, staticID)
	pr.tick(now)
	if pr.status().Current == nil {
		t.Fatal("expected a current alert after the first tick")
	}

	later := now.Add(2 * time.Second)
	pr.tick(later)
	st := pr.status()
	if st.Current != nil {
		t.Errorf("Current = %+v after duration elapsed, want nil", st.Current)
	}
	if st.TotalPlayed != 1 {
		t.Errorf("TotalPlayed = %d, want 1", st.TotalPlayed)
	}
	if !st.ReplayAvailable {
		t.Error("ReplayAvailable = false after a natural completion, want true")
	}
}

func TestProfileRuntimeOneActiveAlertAtATime(t *testing.T) {
	pr := newTestRuntime()
	now := time.Now()
	pr.enqueueMatched([]Instance{mkInstance("a", 50, now), mkInstance("b", 50, now)}, now, staticID)
	pr.tick(now)
	pr.tick(now) // a second tick while "a" is still within its duration must not promote "b"
	if pr.status().QueuedCount != 1 {
		t.Errorf("QueuedCount = %d, want 1 (b still waiting, one active alert at a time)", pr.status().QueuedCount)
	}
}

func TestProfileRuntimePauseFinishesCurrentButDoesNotAdvance(t *testing.T) {
	pr := newTestRuntime()
	now := time.Now()
	inst := mkInstance("a", 50, now)
	inst.DurationMS = 1000
	pr.enqueueMatched([]Instance{inst, mkInstance("b", 50, now)}, now, staticID)
	pr.tick(now)
	pr.pause()

	later := now.Add(2 * time.Second)
	pr.tick(later) // current alert's own duration elapsed - must finish normally even while paused
	st := pr.status()
	if st.Current != nil {
		t.Errorf("Current = %+v while paused past duration, want nil (finishes normally)", st.Current)
	}
	if st.TotalPlayed != 1 {
		t.Errorf("TotalPlayed = %d, want 1", st.TotalPlayed)
	}
	if st.QueuedCount != 1 {
		t.Errorf("QueuedCount = %d while paused, want 1 (b never promoted)", st.QueuedCount)
	}

	pr.resume()
	pr.tick(later)
	if pr.status().Current == nil {
		t.Error("Current = nil after resume, want b promoted")
	}
}

func TestProfileRuntimeSkipCurrentDoesNotRequeue(t *testing.T) {
	pr := newTestRuntime()
	now := time.Now()
	pr.enqueueMatched([]Instance{mkInstance("a", 50, now)}, now, staticID)
	pr.tick(now)
	if !pr.skipCurrent(now) {
		t.Fatal("skipCurrent() = false, want true")
	}
	st := pr.status()
	if st.Current != nil {
		t.Error("Current != nil after skip, want nil")
	}
	if st.TotalManuallySkipped != 1 {
		t.Errorf("TotalSkipped = %d, want 1", st.TotalManuallySkipped)
	}
	if st.TotalPlayed != 0 {
		t.Errorf("TotalPlayed = %d after a skip, want 0 (skip is never counted as played)", st.TotalPlayed)
	}
	if st.QueuedCount != 0 {
		t.Errorf("QueuedCount = %d after skip, want 0 (never requeued)", st.QueuedCount)
	}
}

func TestProfileRuntimeSkipCurrentEmptyReturnsFalse(t *testing.T) {
	pr := newTestRuntime()
	if pr.skipCurrent(time.Now()) {
		t.Error("skipCurrent() with nothing playing = true, want false")
	}
}

func TestProfileRuntimeReplayPreviousRequiresASnapshot(t *testing.T) {
	pr := newTestRuntime()
	if err := pr.replayPrevious(); err != ErrNoReplaySnapshot {
		t.Errorf("replayPrevious() with nothing completed error = %v, want ErrNoReplaySnapshot", err)
	}
}

func TestProfileRuntimeReplayPreviousJumpsQueue(t *testing.T) {
	pr := newTestRuntime()
	now := time.Now()
	inst := mkInstance("a", 50, now)
	inst.DurationMS = 1000
	pr.enqueueMatched([]Instance{inst}, now, staticID)
	pr.tick(now)
	later := now.Add(2 * time.Second)
	pr.tick(later) // "a" completes, becomes the replay snapshot

	// Enqueue something else that would normally play next.
	pr.enqueueMatched([]Instance{mkInstance("b", 50, later)}, later, staticID)

	if err := pr.replayPrevious(); err != nil {
		t.Fatalf("replayPrevious() error = %v", err)
	}
	pr.tick(later)
	st := pr.status()
	if st.Current == nil {
		t.Fatal("expected a current alert after replay")
	}
	if !st.Current.Replayed {
		t.Error("Current.Replayed = false, want true")
	}
	if st.QueuedCount != 1 {
		t.Errorf("QueuedCount = %d, want 1 (b still queued, untouched by replay)", st.QueuedCount)
	}
}

func TestProfileRuntimeReplayNeverCreatesEventBusEvent(t *testing.T) {
	// Structural: replayPrevious's own signature takes no engagement.Event
	// and calls nothing that publishes to a bus - it only ever mutates
	// pendingReplay from an already-in-memory Instance snapshot.
	pr := newTestRuntime()
	now := time.Now()
	inst := mkInstance("a", 50, now)
	inst.DurationMS = 1000
	pr.enqueueMatched([]Instance{inst}, now, staticID)
	pr.tick(now)
	pr.tick(now.Add(2 * time.Second))
	if err := pr.replayPrevious(); err != nil {
		t.Fatalf("replayPrevious() error = %v", err)
	}
}

func TestProfileRuntimeClearQueueLeavesCurrentAlone(t *testing.T) {
	pr := newTestRuntime()
	now := time.Now()
	current := mkInstance("a", 50, now)
	current.DurationMS = 10000
	pr.enqueueMatched([]Instance{current}, now, staticID)
	pr.tick(now)
	pr.enqueueMatched([]Instance{mkInstance("b", 50, now), mkInstance("c", 50, now)}, now, staticID)

	cleared := pr.clearQueue()
	if cleared != 2 {
		t.Errorf("clearQueue() = %d, want 2", cleared)
	}
	st := pr.status()
	if st.Current == nil {
		t.Error("Current == nil after clearQueue, want the alert untouched")
	}
	if st.QueuedCount != 0 {
		t.Errorf("QueuedCount = %d after clearQueue, want 0", st.QueuedCount)
	}
}

func TestProfileRuntimeDisableHidesCurrentAndClearsQueue(t *testing.T) {
	pr := newTestRuntime()
	now := time.Now()
	current := mkInstance("a", 50, now)
	current.DurationMS = 10000
	pr.enqueueMatched([]Instance{current}, now, staticID)
	pr.tick(now)
	pr.enqueueMatched([]Instance{mkInstance("b", 50, now)}, now, staticID)

	p := testProfile()
	p.Enabled = false
	pr.applyProfile(p)

	st := pr.status()
	if st.Enabled {
		t.Error("Enabled = true after disabling, want false")
	}
	if st.Current != nil {
		t.Error("Current != nil after disabling, want nil (hidden immediately)")
	}
	if st.QueuedCount != 0 {
		t.Errorf("QueuedCount = %d after disabling, want 0", st.QueuedCount)
	}
}

func TestProfileRuntimeReEnableStartsEmpty(t *testing.T) {
	pr := newTestRuntime()
	now := time.Now()
	pr.enqueueMatched([]Instance{mkInstance("a", 50, now)}, now, staticID)

	off := testProfile()
	off.Enabled = false
	pr.applyProfile(off)

	// While disabled, a new match must never be accepted.
	pr.enqueueMatched([]Instance{mkInstance("b", 50, now)}, now, staticID)

	on := testProfile()
	on.Enabled = true
	pr.applyProfile(on)

	st := pr.status()
	if st.QueuedCount != 0 || st.Current != nil {
		t.Errorf("status after re-enable = %+v, want empty (no replay of what arrived while disabled)", st)
	}
}

func TestProfileRuntimeDisabledNeverTicks(t *testing.T) {
	pr := newTestRuntime()
	off := testProfile()
	off.Enabled = false
	pr.applyProfile(off)

	now := time.Now()
	pr.enqueueMatched([]Instance{mkInstance("a", 50, now)}, now, staticID) // rejected, disabled
	pr.tick(now)
	if pr.status().Current != nil {
		t.Error("a disabled profile promoted an alert, want none")
	}
}

func TestProfileRuntimeCapacityDroppedCounted(t *testing.T) {
	p := testProfile()
	p.MaxQueueItems = 1
	pr := newProfileRuntime("alprof_1", p, staticID)
	now := time.Now()
	pr.enqueueMatched([]Instance{mkInstance("a", 50, now), mkInstance("b", 40, now)}, now, staticID)
	st := pr.status()
	if st.TotalEnqueued != 1 || st.TotalCapacityDropped != 1 {
		t.Errorf("status = %+v, want TotalEnqueued=1 TotalCapacityDropped=1", st)
	}
}

func TestProfileRuntimeSyntheticCounterSeparate(t *testing.T) {
	pr := newTestRuntime()
	now := time.Now()
	real := mkInstance("real", 50, now)
	synthetic := mkInstance("synthetic", 50, now)
	synthetic.Synthetic = true
	pr.enqueueMatched([]Instance{real, synthetic}, now, staticID)
	st := pr.status()
	if st.TotalEnqueued != 2 {
		t.Errorf("TotalEnqueued = %d, want 2", st.TotalEnqueued)
	}
	if st.TotalSynthetic != 1 {
		t.Errorf("TotalSynthetic = %d, want 1", st.TotalSynthetic)
	}
}
