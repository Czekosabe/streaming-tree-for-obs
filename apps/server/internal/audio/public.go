package audio

import (
	"context"
	"sync"

	domain "github.com/streaming-tree/server/internal/domain/audio"
	"github.com/streaming-tree/server/internal/provider/tts"
)

// changeBroadcaster is a minimal "something changed, go re-read the
// current snapshot" signal - deliberately not a queue of past states
// (unlike internal/alerts's own Revision log): the public audio
// protocol only ever needs the CURRENT item (docs/audio-tts.md §14),
// never a history, so a coalescing one-slot-per-subscriber channel is
// enough and simpler than a general pub-sub.
type changeBroadcaster struct {
	mu     sync.Mutex
	subs   map[uint64]chan struct{}
	nextID uint64
}

func (b *changeBroadcaster) subscribe() (id uint64, ch <-chan struct{}) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.subs == nil {
		b.subs = make(map[uint64]chan struct{})
	}
	b.nextID++
	id = b.nextID
	c := make(chan struct{}, 1)
	b.subs[id] = c
	return id, c
}

func (b *changeBroadcaster) unsubscribe(id uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if c, ok := b.subs[id]; ok {
		delete(b.subs, id)
		close(c)
	}
}

func (b *changeBroadcaster) notify() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, c := range b.subs {
		select {
		case c <- struct{}{}:
		default:
		}
	}
}

// SubscribeCurrentChanges registers for a signal every time the public
// current-item snapshot may have changed (promoted, synthesized,
// skipped, cleared, played, failed, or the renderer's own presence
// changed enough to affect PublicCurrentSnapshot's output). The
// channel is closed by UnsubscribeCurrentChanges - callers must always
// unsubscribe (defer) to avoid leaking the subscription entry.
func (m *Manager) SubscribeCurrentChanges() (id uint64, ch <-chan struct{}) {
	return m.changes.subscribe()
}

// UnsubscribeCurrentChanges releases a subscription registered via
// SubscribeCurrentChanges.
func (m *Manager) UnsubscribeCurrentChanges(id uint64) {
	m.changes.unsubscribe(id)
}

// GetSettings returns the current persisted settings.
func (m *Manager) GetSettings(ctx context.Context) (domain.Settings, error) {
	return m.settingsSvc.Get(ctx)
}

// UpdateSettings validates and persists a full settings replacement,
// then refreshes this Manager's own cached copy (and the queue's
// capacity bound) so the change takes effect immediately - mirrors
// internal/alerts.Manager's own "write then reload runtime cache"
// convention.
func (m *Manager) UpdateSettings(ctx context.Context, input domain.Settings) (domain.Settings, error) {
	saved, err := m.settingsSvc.Update(ctx, input)
	if err != nil {
		return domain.Settings{}, err
	}
	m.mu.Lock()
	m.settings = saved
	m.queue.SetCapacity(saved.QueueCapacity)
	m.mu.Unlock()
	return saved, nil
}

// RotatePublicSlug replaces the public audio output URL's own locator
// and refreshes this Manager's cached settings.
func (m *Manager) RotatePublicSlug(ctx context.Context) (domain.Settings, error) {
	saved, err := m.settingsSvc.RotatePublicSlug(ctx)
	if err != nil {
		return domain.Settings{}, err
	}
	m.mu.Lock()
	m.settings = saved
	m.mu.Unlock()
	return saved, nil
}

// CurrentPublicSlug returns the cached settings' own public slug -
// used by the public HTTP routes to validate the slug in the URL
// without a repeated Get(ctx) round trip per request.
func (m *Manager) CurrentPublicSlug() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.settings.PublicSlug
}

// ProviderCapabilities reports whether the configured tts.Provider can
// synthesize anything right now - a nil Provider always reports
// unavailable, never a panic.
func (m *Manager) ProviderCapabilities() tts.Capabilities {
	if m.provider == nil {
		return tts.Capabilities{Available: false, Reason: "no text-to-speech provider is configured"}
	}
	return m.provider.Capabilities()
}

// ListVoices proxies to the configured tts.Provider.
func (m *Manager) ListVoices(ctx context.Context) ([]tts.Voice, error) {
	if m.provider == nil {
		return nil, tts.ErrUnavailable
	}
	return m.provider.ListVoices(ctx)
}
