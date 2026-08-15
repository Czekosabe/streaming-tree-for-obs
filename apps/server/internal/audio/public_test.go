package audio

import (
	"context"
	"testing"
	"time"

	domain "github.com/streaming-tree/server/internal/domain/audio"
	bus "github.com/streaming-tree/server/internal/engagement"
	"github.com/streaming-tree/server/internal/provider/tts"
)

func TestManagerUpdateSettingsRefreshesCacheAndCapacity(t *testing.T) {
	mgr, _ := newTestManager(t, availableProvider(), baseTestSettings())

	current, err := mgr.GetSettings(context.Background())
	if err != nil {
		t.Fatalf("GetSettings() error = %v", err)
	}
	current.QueueCapacity = domain.MinQueueCapacity
	saved, err := mgr.UpdateSettings(context.Background(), current)
	if err != nil {
		t.Fatalf("UpdateSettings() error = %v", err)
	}
	if saved.QueueCapacity != domain.MinQueueCapacity {
		t.Errorf("saved.QueueCapacity = %d, want %d", saved.QueueCapacity, domain.MinQueueCapacity)
	}
	if got := mgr.Status().Capacity; got != domain.MinQueueCapacity {
		t.Errorf("Status().Capacity = %d, want %d (cache not refreshed)", got, domain.MinQueueCapacity)
	}
}

func TestManagerRotatePublicSlugUpdatesCurrentPublicSlug(t *testing.T) {
	mgr, _ := newTestManager(t, availableProvider(), baseTestSettings())

	before := mgr.CurrentPublicSlug()
	if before == "" {
		t.Fatal("CurrentPublicSlug() is empty before rotation")
	}
	saved, err := mgr.RotatePublicSlug(context.Background())
	if err != nil {
		t.Fatalf("RotatePublicSlug() error = %v", err)
	}
	if saved.PublicSlug == before {
		t.Error("RotatePublicSlug() did not change the slug")
	}
	if got := mgr.CurrentPublicSlug(); got != saved.PublicSlug {
		t.Errorf("CurrentPublicSlug() = %q, want %q (cache not refreshed)", got, saved.PublicSlug)
	}
}

func TestManagerProviderCapabilitiesNilProvider(t *testing.T) {
	repo := &fakeSettingsRepo{}
	svc := domain.NewService(domain.Options{Repository: repo})
	svc.Get(context.Background())
	svc.Update(context.Background(), baseTestSettings())
	b := bus.New(bus.Options{})
	mgr := NewManager(Options{SettingsService: svc, Bus: b})
	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { mgr.Shutdown(context.Background()) })

	caps := mgr.ProviderCapabilities()
	if caps.Available {
		t.Error("ProviderCapabilities().Available = true with a nil provider, want false")
	}
	if _, err := mgr.ListVoices(context.Background()); err == nil {
		t.Error("ListVoices() error = nil with a nil provider, want ErrUnavailable")
	}
}

func TestManagerListVoicesProxiesToProvider(t *testing.T) {
	provider := availableProvider()
	mgr, _ := newTestManager(t, provider, baseTestSettings())

	voices, err := mgr.ListVoices(context.Background())
	if err != nil {
		t.Fatalf("ListVoices() error = %v", err)
	}
	if len(voices) != 1 || voices[0].ID != "voice-1" {
		t.Errorf("ListVoices() = %+v, want the fake provider's one voice", voices)
	}
}

func TestManagerSubscribeCurrentChangesFiresOnPromotion(t *testing.T) {
	provider := &fakeProvider{available: true}
	mgr, b := newTestManager(t, provider, baseTestSettings())
	mgr.ConnectRenderer()

	id, ch := mgr.SubscribeCurrentChanges()
	defer mgr.UnsubscribeCurrentChanges(id)

	b.Publish(chatEvent("u1", "Ada", "hello"))

	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("no change notification received before timeout")
	}
}

func TestManagerUnsubscribeCurrentChangesClosesChannel(t *testing.T) {
	mgr, _ := newTestManager(t, availableProvider(), baseTestSettings())
	id, ch := mgr.SubscribeCurrentChanges()
	mgr.UnsubscribeCurrentChanges(id)

	select {
	case _, open := <-ch:
		if open {
			t.Error("channel received a value instead of being closed")
		}
	case <-time.After(time.Second):
		t.Fatal("channel was not closed before timeout")
	}
}

var _ tts.Provider = (*fakeProvider)(nil)
