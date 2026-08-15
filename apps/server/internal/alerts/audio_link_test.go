package alerts

import (
	"sync"
	"testing"
	"time"

	audio "github.com/streaming-tree/server/internal/audio"
)

// fakeAudioLink is a minimal in-memory double for AlertAudioLink - it
// never touches internal/audio.Manager at all, so these tests exercise
// only the internal/alerts side of the Stage 17B wiring (what gets
// called, when, and with what arguments), never audio synthesis or
// queueing itself (already covered by internal/audio's own
// manager_alertaudio_test.go).
type fakeAudioLink struct {
	mu        sync.Mutex
	enqueued  []fakeEnqueueCall
	cancelled []string
	states    map[string]audio.AlertAudioState
}

type fakeEnqueueCall struct {
	ProfileID  string
	InstanceID string
	Items      []audio.AlertAudioRequest
}

func (f *fakeAudioLink) EnqueueAlertAudio(profileID, instanceID string, items []audio.AlertAudioRequest) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.enqueued = append(f.enqueued, fakeEnqueueCall{ProfileID: profileID, InstanceID: instanceID, Items: items})
}

func (f *fakeAudioLink) CancelAlertAudio(instanceID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancelled = append(f.cancelled, instanceID)
}

func (f *fakeAudioLink) AlertAudioState(instanceID string) audio.AlertAudioState {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.states[instanceID]
}

func (f *fakeAudioLink) setState(instanceID string, st audio.AlertAudioState) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.states == nil {
		f.states = map[string]audio.AlertAudioState{}
	}
	f.states[instanceID] = st
}

func (f *fakeAudioLink) enqueueCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.enqueued)
}

func (f *fakeAudioLink) lastEnqueue() fakeEnqueueCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.enqueued[len(f.enqueued)-1]
}

func (f *fakeAudioLink) cancelledCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.cancelled)
}

func instanceWithAudio(id string, priority int, now time.Time, audioSnap *AlertAudioSnapshot) Instance {
	inst := mkInstance(id, priority, now)
	inst.Audio = audioSnap
	return inst
}

func TestAlertAudioRequestsForOrdersSoundThenTTS(t *testing.T) {
	snap := &AlertAudioSnapshot{
		SoundEnabled: true, SoundAssetID: "audioasset_1", SoundVolume: 0.8,
		TTSEnabled: true, TTSText: "hello there", TTSVolume: 0.5,
	}
	items := alertAudioRequestsFor(snap)
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].Source != audio.SourceAlertSound || items[0].AssetID != "audioasset_1" || items[0].Volume != 0.8 {
		t.Errorf("items[0] = %+v, want the sound request first", items[0])
	}
	if items[1].Source != audio.SourceAlertTTS || items[1].Text != "hello there" || items[1].Volume != 0.5 {
		t.Errorf("items[1] = %+v, want the TTS request second", items[1])
	}
}

func TestAlertAudioRequestsForSoundOnly(t *testing.T) {
	snap := &AlertAudioSnapshot{SoundEnabled: true, SoundAssetID: "audioasset_1", SoundVolume: 1}
	items := alertAudioRequestsFor(snap)
	if len(items) != 1 || items[0].Source != audio.SourceAlertSound {
		t.Fatalf("items = %+v, want exactly one sound request", items)
	}
}

func TestAlertAudioRequestsForNilSnapshotIsNoOp(t *testing.T) {
	if items := alertAudioRequestsFor(nil); items != nil {
		t.Errorf("alertAudioRequestsFor(nil) = %+v, want nil", items)
	}
}

func TestAlertAudioRequestsForNeitherEnabledIsNoOp(t *testing.T) {
	if items := alertAudioRequestsFor(&AlertAudioSnapshot{}); items != nil {
		t.Errorf("alertAudioRequestsFor(empty) = %+v, want nil", items)
	}
}

func TestStartCurrentLockedEnqueuesAudioWhenLinked(t *testing.T) {
	link := &fakeAudioLink{}
	pr := newProfileRuntime("alprof_1", testProfile(), staticID, link)
	now := time.Now()
	inst := instanceWithAudio("alinst_test", 50, now, &AlertAudioSnapshot{SoundEnabled: true, SoundAssetID: "a1", SoundVolume: 1})
	pr.enqueueMatched([]Instance{inst}, now, staticID)
	pr.tick(now)

	if got := link.enqueueCount(); got != 1 {
		t.Fatalf("enqueueCount() = %d, want 1", got)
	}
	call := link.lastEnqueue()
	if call.ProfileID != "alprof_1" || call.InstanceID != "alinst_test" {
		t.Errorf("lastEnqueue() = %+v, want profile alprof_1 / instance alinst_test", call)
	}
}

func TestStartCurrentLockedNeverEnqueuesWithoutAudio(t *testing.T) {
	link := &fakeAudioLink{}
	pr := newProfileRuntime("alprof_1", testProfile(), staticID, link)
	now := time.Now()
	pr.enqueueMatched([]Instance{mkInstance("alinst_test", 50, now)}, now, staticID)
	pr.tick(now)

	if got := link.enqueueCount(); got != 0 {
		t.Errorf("enqueueCount() = %d, want 0 for an instance with no rule-owned audio", got)
	}
}

func TestCompleteCurrentLockedCancelsLinkedAudioOnNaturalCompletion(t *testing.T) {
	link := &fakeAudioLink{}
	pr := newProfileRuntime("alprof_1", testProfile(), staticID, link)
	now := time.Now()
	inst := instanceWithAudio("alinst_test", 50, now, &AlertAudioSnapshot{SoundEnabled: true, SoundAssetID: "a1", SoundVolume: 1})
	inst.DurationMS = 1000
	pr.enqueueMatched([]Instance{inst}, now, staticID)
	pr.tick(now)

	link.setState("alinst_test", audio.AlertAudioEnded)
	later := now.Add(2 * time.Second)
	pr.tick(later)

	if pr.status().Current != nil {
		t.Fatal("expected the alert to complete once its audio has ended and the duration has elapsed")
	}
	if got := link.cancelledCount(); got != 1 {
		t.Errorf("cancelledCount() = %d, want 1 (CancelAlertAudio is idempotent/unconditional on completion)", got)
	}
}

func TestSkipCurrentCancelsLinkedAudio(t *testing.T) {
	link := &fakeAudioLink{}
	pr := newProfileRuntime("alprof_1", testProfile(), staticID, link)
	now := time.Now()
	inst := instanceWithAudio("alinst_test", 50, now, &AlertAudioSnapshot{SoundEnabled: true, SoundAssetID: "a1", SoundVolume: 1})
	pr.enqueueMatched([]Instance{inst}, now, staticID)
	pr.tick(now)

	if !pr.skipCurrent(now) {
		t.Fatal("skipCurrent() = false, want true")
	}
	if got := link.cancelledCount(); got != 1 {
		t.Errorf("cancelledCount() = %d, want 1", got)
	}
	if got := link.enqueueCount(); got != 1 {
		t.Errorf("enqueueCount() = %d, want 1 (skip must never re-enqueue)", got)
	}
}

func TestShouldHoldForAudioKeepsAlertVisibleWhilePlaying(t *testing.T) {
	link := &fakeAudioLink{}
	pr := newProfileRuntime("alprof_1", testProfile(), staticID, link)
	now := time.Now()
	inst := instanceWithAudio("alinst_test", 50, now, &AlertAudioSnapshot{SoundEnabled: true, SoundAssetID: "a1", SoundVolume: 1})
	inst.DurationMS = 1000
	pr.enqueueMatched([]Instance{inst}, now, staticID)
	pr.tick(now)

	link.setState("alinst_test", audio.AlertAudioPlaying)
	afterDuration := now.Add(2 * time.Second)
	pr.tick(afterDuration)

	if pr.status().Current == nil {
		t.Fatal("expected the alert to stay visible while its linked audio is still Playing, even past DurationMS")
	}

	link.setState("alinst_test", audio.AlertAudioEnded)
	pr.tick(afterDuration.Add(10 * time.Millisecond))
	if pr.status().Current != nil {
		t.Fatal("expected the alert to complete once its linked audio reports Ended")
	}
}

func TestShouldHoldForAudioForceCancelsAtBound(t *testing.T) {
	link := &fakeAudioLink{}
	pr := newProfileRuntime("alprof_1", testProfile(), staticID, link)
	now := time.Now()
	inst := instanceWithAudio("alinst_test", 50, now, &AlertAudioSnapshot{SoundEnabled: true, SoundAssetID: "a1", SoundVolume: 1})
	inst.DurationMS = 1000
	pr.enqueueMatched([]Instance{inst}, now, staticID)
	pr.tick(now)

	link.setState("alinst_test", audio.AlertAudioPlaying)
	beyondBound := now.Add(1*time.Second + maxAudioHoldDuration + time.Second)
	pr.tick(beyondBound)

	if pr.status().Current != nil {
		t.Fatal("expected the alert to complete once the bounded hold is exceeded, even while audio reports Playing")
	}
	// CancelAlertAudio is idempotent (docs/alert-audio.md §8.5), so both
	// the bound's own force-cancel and completeCurrentLocked's own
	// unconditional cancel fire - two calls for the one instance.
	if got := link.cancelledCount(); got != 2 {
		t.Errorf("cancelledCount() = %d, want 2 (reaching the bound must force-cancel the audio outright)", got)
	}
}

func TestShouldHoldForAudioProceedsImmediatelyWithoutARenderer(t *testing.T) {
	link := &fakeAudioLink{}
	pr := newProfileRuntime("alprof_1", testProfile(), staticID, link)
	now := time.Now()
	inst := instanceWithAudio("alinst_test", 50, now, &AlertAudioSnapshot{SoundEnabled: true, SoundAssetID: "a1", SoundVolume: 1})
	inst.DurationMS = 1000
	pr.enqueueMatched([]Instance{inst}, now, staticID)
	pr.tick(now)

	link.setState("alinst_test", audio.AlertAudioNoRenderer)
	afterDuration := now.Add(2 * time.Second)
	pr.tick(afterDuration)

	if pr.status().Current != nil {
		t.Error("expected the alert to complete on normal timing when there is no renderer to hold for")
	}
}

func TestShouldHoldForAudioFalseWithoutALink(t *testing.T) {
	pr := newTestRuntime()
	now := time.Now()
	inst := instanceWithAudio("alinst_test", 50, now, &AlertAudioSnapshot{SoundEnabled: true, SoundAssetID: "a1", SoundVolume: 1})
	inst.DurationMS = 1000
	pr.enqueueMatched([]Instance{inst}, now, staticID)
	pr.tick(now)

	afterDuration := now.Add(2 * time.Second)
	pr.tick(afterDuration)
	if pr.status().Current != nil {
		t.Error("expected normal completion timing when profileRuntime has no audioLink at all")
	}
}
