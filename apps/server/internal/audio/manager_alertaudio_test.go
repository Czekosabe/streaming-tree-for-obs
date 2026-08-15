package audio

import (
	"context"
	"testing"
	"time"

	domain "github.com/streaming-tree/server/internal/domain/audio"
	bus "github.com/streaming-tree/server/internal/engagement"
)

// --- Stage 17B: alert-owned playback and synchronization
// (docs/alert-audio.md §8/§9) -------------------------------------------

// fakeAssetResolver is a minimal AudioAssetResolver test double - a
// fixed map from asset ID to bytes/content-type, exactly like
// audioasset.Service.ResolveSoundAsset's own ok=false-on-miss contract.
type fakeAssetResolver struct {
	assets map[string][2]string // id -> [contentType, data]
}

func newFakeAssetResolver() *fakeAssetResolver {
	return &fakeAssetResolver{assets: map[string][2]string{}}
}

func (f *fakeAssetResolver) addAsset(id, contentType, data string) {
	f.assets[id] = [2]string{contentType, data}
}

func (f *fakeAssetResolver) ResolveSoundAsset(_ context.Context, assetID string) ([]byte, string, bool) {
	v, ok := f.assets[assetID]
	if !ok {
		return nil, "", false
	}
	return []byte(v[1]), v[0], true
}

var _ AudioAssetResolver = (*fakeAssetResolver)(nil)

// newTestManagerWithResolver mirrors newTestManager exactly, but also
// wires an AudioAssetResolver - the one extra dependency Stage 17B's
// SourceAlertSound path needs.
func newTestManagerWithResolver(t *testing.T, provider *fakeProvider, resolver AudioAssetResolver, settings domain.Settings) (*Manager, *bus.Bus) {
	t.Helper()
	repo := &fakeSettingsRepo{}
	svc := domain.NewService(domain.Options{Repository: repo})
	if _, err := svc.Get(context.Background()); err != nil {
		t.Fatalf("seed defaults: %v", err)
	}
	if _, err := svc.Update(context.Background(), settings); err != nil {
		t.Fatalf("save test settings: %v", err)
	}

	b := bus.New(bus.Options{})
	mgr := NewManager(Options{
		SettingsService:    svc,
		Bus:                b,
		Provider:           provider,
		AudioAssetResolver: resolver,
		SynthesisTimeout:   2 * time.Second,
		ItemExpiry:         time.Minute,
	})
	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = mgr.Shutdown(ctx)
	})
	waitUntil(t, time.Second, mgr.Subscribed)
	return mgr, b
}

func soundRequest(assetID string, volume float64) AlertAudioRequest {
	return AlertAudioRequest{Source: SourceAlertSound, AssetID: assetID, Volume: volume}
}

func ttsRequest(text string, volume float64) AlertAudioRequest {
	return AlertAudioRequest{Source: SourceAlertTTS, Text: text, Volume: volume, VoiceID: "voice-1"}
}

func TestEnqueueAlertAudioPromotesImmediatelyWhenIdle(t *testing.T) {
	resolver := newFakeAssetResolver()
	resolver.addAsset("audioasset_1", "audio/wav", "RIFF....")
	mgr, _ := newTestManagerWithResolver(t, availableProvider(), resolver, baseTestSettings())

	if _, err := mgr.ConnectRenderer(); err != nil {
		t.Fatalf("ConnectRenderer() error = %v", err)
	}
	mgr.EnqueueAlertAudio("profile_1", "inst_1", []AlertAudioRequest{soundRequest("audioasset_1", 1.0)})

	// AlertAudioState reports Started as soon as the item is promoted
	// (playbackStateSynthesizing), before resolution/synthesis actually
	// completes and PublicCurrentSnapshot's own bytesToken-gated HasItem
	// becomes true - wait on the snapshot directly, the stronger of the
	// two conditions.
	waitUntil(t, time.Second, func() bool { return mgr.PublicCurrentSnapshot().HasItem })
	if state := mgr.AlertAudioState("inst_1"); state == AlertAudioNeverRequested {
		t.Errorf("AlertAudioState(inst_1) = %v, want Started/Playing", state)
	}
}

func TestEnqueueAlertAudioResolvesSoundBytesNeverCallsProvider(t *testing.T) {
	resolver := newFakeAssetResolver()
	resolver.addAsset("audioasset_1", "audio/wav", "sound-bytes")
	provider := availableProvider()
	mgr, _ := newTestManagerWithResolver(t, provider, resolver, baseTestSettings())
	mgr.ConnectRenderer()

	mgr.EnqueueAlertAudio("profile_1", "inst_1", []AlertAudioRequest{soundRequest("audioasset_1", 1.0)})
	waitUntil(t, time.Second, func() bool { return mgr.PublicCurrentSnapshot().HasItem })

	if provider.calls != 0 {
		t.Errorf("provider.Synthesize was called %d times, want 0 - a persistent sound never touches the TTS provider", provider.calls)
	}
	snap := mgr.PublicCurrentSnapshot()
	if snap.ContentType != "audio/wav" {
		t.Errorf("ContentType = %q, want audio/wav", snap.ContentType)
	}
}

func TestEnqueueAlertAudioPreemptsPlayingGlobalTTS(t *testing.T) {
	provider := &fakeProvider{available: true, delay: 200 * time.Millisecond}
	resolver := newFakeAssetResolver()
	resolver.addAsset("audioasset_1", "audio/wav", "sound-bytes")
	settings := baseTestSettings()
	mgr, b := newTestManagerWithResolver(t, provider, resolver, settings)
	mgr.ConnectRenderer()

	if _, _, err := b.Publish(chatEvent("u1", "Ada", "hello there")); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	// The global TTS item is now synthesizing (provider has a 200ms
	// delay) - it must become current before the preemption below can
	// prove anything.
	waitUntil(t, time.Second, func() bool { return mgr.PublicCurrentSnapshot().Idle == false || mgr.Status().HasCurrentItem })
	if !mgr.Status().HasCurrentItem {
		t.Fatal("expected the global TTS item to be current before preemption")
	}

	mgr.EnqueueAlertAudio("profile_1", "inst_1", []AlertAudioRequest{soundRequest("audioasset_1", 1.0)})

	waitUntil(t, time.Second, func() bool {
		snap := mgr.PublicCurrentSnapshot()
		return snap.HasItem && snap.ContentType == "audio/wav"
	})
	if got := mgr.Status().TotalInterruptedByAlert; got != 1 {
		t.Errorf("TotalInterruptedByAlert = %d, want 1", got)
	}
}

func TestEnqueueAlertAudioNeverPreemptsAnotherAlertInstance(t *testing.T) {
	provider := availableProvider()
	resolver := newFakeAssetResolver()
	resolver.addAsset("audioasset_1", "audio/wav", "first")
	resolver.addAsset("audioasset_2", "audio/wav", "second")
	mgr, _ := newTestManagerWithResolver(t, provider, resolver, baseTestSettings())
	mgr.ConnectRenderer()

	mgr.EnqueueAlertAudio("profile_1", "inst_1", []AlertAudioRequest{soundRequest("audioasset_1", 1.0)})
	waitUntil(t, time.Second, func() bool { return mgr.AlertAudioState("inst_1") != AlertAudioNeverRequested })

	// inst_2's audio arrives while inst_1's is still current - it must
	// wait in the ordinary queue, never preempt inst_1.
	mgr.EnqueueAlertAudio("profile_2", "inst_2", []AlertAudioRequest{soundRequest("audioasset_2", 1.0)})
	time.Sleep(50 * time.Millisecond)

	if state := mgr.AlertAudioState("inst_2"); state != AlertAudioStarted {
		t.Errorf("AlertAudioState(inst_2) = %v, want AlertAudioStarted (queued, not promoted)", state)
	}
	if mgr.Status().TotalInterruptedByAlert != 0 {
		t.Error("inst_1's own alert-owned audio must never be counted as interrupted-by-alert")
	}
}

func TestAlertAudioChainPlaysSoundThenTTS(t *testing.T) {
	resolver := newFakeAssetResolver()
	resolver.addAsset("audioasset_1", "audio/wav", "sound-bytes")
	mgr, _ := newTestManagerWithResolver(t, availableProvider(), resolver, baseTestSettings())
	token, _ := mgr.ConnectRenderer()

	mgr.EnqueueAlertAudio("profile_1", "inst_1", []AlertAudioRequest{
		soundRequest("audioasset_1", 1.0),
		ttsRequest("hello from the chain", 1.0),
	})

	// First: the sound half becomes current.
	waitUntil(t, time.Second, func() bool { return mgr.PublicCurrentSnapshot().HasItem })
	firstSnap := mgr.PublicCurrentSnapshot()
	if firstSnap.ContentType != "audio/wav" {
		t.Fatalf("first item ContentType = %q, want audio/wav", firstSnap.ContentType)
	}

	// Acknowledge the sound half as started, then ended - the chain must
	// advance to the TTS half automatically, never falling back to
	// AlertAudioNeverRequested/Ended in between.
	if err := mgr.Ack(token, firstSnap.ItemID, AckStarted); err != nil {
		t.Fatalf("Ack(started) error = %v", err)
	}
	if err := mgr.Ack(token, firstSnap.ItemID, AckEnded); err != nil {
		t.Fatalf("Ack(ended) error = %v", err)
	}

	waitUntil(t, time.Second, func() bool {
		snap := mgr.PublicCurrentSnapshot()
		return snap.HasItem && snap.ItemID != firstSnap.ItemID
	})
	if state := mgr.AlertAudioState("inst_1"); state == AlertAudioNeverRequested {
		t.Error("AlertAudioState after chain advance = NeverRequested, want Started/Playing (TTS half now current)")
	}
}

func TestCancelAlertAudioClearsCurrentAndQueued(t *testing.T) {
	resolver := newFakeAssetResolver()
	resolver.addAsset("audioasset_1", "audio/wav", "first")
	resolver.addAsset("audioasset_2", "audio/wav", "second")
	mgr, _ := newTestManagerWithResolver(t, availableProvider(), resolver, baseTestSettings())
	mgr.ConnectRenderer()

	mgr.EnqueueAlertAudio("profile_1", "inst_1", []AlertAudioRequest{soundRequest("audioasset_1", 1.0)})
	waitUntil(t, time.Second, func() bool { return mgr.PublicCurrentSnapshot().HasItem })
	// inst_2 waits in queue behind inst_1.
	mgr.EnqueueAlertAudio("profile_1", "inst_2", []AlertAudioRequest{soundRequest("audioasset_2", 1.0)})
	time.Sleep(30 * time.Millisecond)

	mgr.CancelAlertAudio("inst_1")
	waitUntil(t, time.Second, func() bool { return !mgr.Status().HasCurrentItem })

	if state := mgr.AlertAudioState("inst_1"); state != AlertAudioNeverRequested {
		t.Errorf("AlertAudioState(inst_1) after cancel = %v, want AlertAudioNeverRequested", state)
	}

	// Cancel inst_2 too, while it is still only queued (never promoted) -
	// it must be removed from the queue outright, never later promoted.
	mgr.CancelAlertAudio("inst_2")
	time.Sleep(50 * time.Millisecond)
	if mgr.Status().HasCurrentItem {
		t.Error("inst_2 was promoted after being cancelled while still queued")
	}
}

func TestAlertAudioStateReturnsNeverRequestedForUnknownInstance(t *testing.T) {
	mgr, _ := newTestManagerWithResolver(t, availableProvider(), newFakeAssetResolver(), baseTestSettings())
	if state := mgr.AlertAudioState("inst_never_seen"); state != AlertAudioNeverRequested {
		t.Errorf("AlertAudioState() = %v, want AlertAudioNeverRequested", state)
	}
}

func TestAlertAudioStateNoRendererWhenNoneConnected(t *testing.T) {
	resolver := newFakeAssetResolver()
	resolver.addAsset("audioasset_1", "audio/wav", "sound-bytes")
	mgr, _ := newTestManagerWithResolver(t, availableProvider(), resolver, baseTestSettings())
	// Deliberately never call ConnectRenderer.

	mgr.EnqueueAlertAudio("profile_1", "inst_1", []AlertAudioRequest{soundRequest("audioasset_1", 1.0)})
	waitUntil(t, time.Second, func() bool { return mgr.AlertAudioState("inst_1") == AlertAudioNoRenderer })
}

func TestAlertAudioResolverFailureReportsFailed(t *testing.T) {
	mgr, _ := newTestManagerWithResolver(t, availableProvider(), newFakeAssetResolver() /* empty: every asset ID misses */, baseTestSettings())
	mgr.ConnectRenderer()

	mgr.EnqueueAlertAudio("profile_1", "inst_1", []AlertAudioRequest{soundRequest("audioasset_missing", 1.0)})
	waitUntil(t, time.Second, func() bool { return mgr.AlertAudioState("inst_1") == AlertAudioFailed })
}

func TestAlertAudioSkipCurrentAbortsWithoutContinuingChain(t *testing.T) {
	resolver := newFakeAssetResolver()
	resolver.addAsset("audioasset_1", "audio/wav", "sound-bytes")
	mgr, _ := newTestManagerWithResolver(t, availableProvider(), resolver, baseTestSettings())
	mgr.ConnectRenderer()

	mgr.EnqueueAlertAudio("profile_1", "inst_1", []AlertAudioRequest{
		soundRequest("audioasset_1", 1.0),
		ttsRequest("should never be spoken", 1.0),
	})
	waitUntil(t, time.Second, func() bool { return mgr.PublicCurrentSnapshot().HasItem })

	if !mgr.SkipCurrent() {
		t.Fatal("SkipCurrent() = false, want true")
	}
	time.Sleep(50 * time.Millisecond)
	if mgr.Status().HasCurrentItem {
		t.Error("SkipCurrent must abort the whole chain, never advance to the TTS half")
	}
}

func TestAlertAudioVolumeIsCarriedVerbatimOnPublicSnapshot(t *testing.T) {
	resolver := newFakeAssetResolver()
	resolver.addAsset("audioasset_1", "audio/wav", "sound-bytes")
	mgr, _ := newTestManagerWithResolver(t, availableProvider(), resolver, baseTestSettings())
	mgr.ConnectRenderer()

	mgr.EnqueueAlertAudio("profile_1", "inst_1", []AlertAudioRequest{soundRequest("audioasset_1", 0.42)})
	waitUntil(t, time.Second, func() bool { return mgr.PublicCurrentSnapshot().HasItem })

	if got := mgr.PublicCurrentSnapshot().Volume; got != 0.42 {
		t.Errorf("Volume = %v, want 0.42 (already-combined value, never recombined)", got)
	}
}

func TestAlertAudioStaleAckRejectedAfterNewRendererConnects(t *testing.T) {
	resolver := newFakeAssetResolver()
	resolver.addAsset("audioasset_1", "audio/wav", "sound-bytes")
	mgr, _ := newTestManagerWithResolver(t, availableProvider(), resolver, baseTestSettings())
	staleToken, _ := mgr.ConnectRenderer()

	mgr.EnqueueAlertAudio("profile_1", "inst_1", []AlertAudioRequest{soundRequest("audioasset_1", 1.0)})
	waitUntil(t, time.Second, func() bool { return mgr.PublicCurrentSnapshot().HasItem })
	itemID := mgr.PublicCurrentSnapshot().ItemID

	// A new renderer session supersedes the old one.
	mgr.ConnectRenderer()

	if err := mgr.Ack(staleToken, itemID, AckStarted); err == nil {
		t.Error("Ack() with a stale renderer session succeeded, want ErrAckRejected")
	}
}
