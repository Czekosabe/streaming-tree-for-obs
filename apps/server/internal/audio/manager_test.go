package audio

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	domain "github.com/streaming-tree/server/internal/domain/audio"
	engagement "github.com/streaming-tree/server/internal/domain/engagement"
	bus "github.com/streaming-tree/server/internal/engagement"
	"github.com/streaming-tree/server/internal/provider/tts"
)

// --- test doubles -----------------------------------------------------

type fakeSettingsRepo struct {
	mu       sync.Mutex
	settings domain.Settings
	found    bool
}

func (f *fakeSettingsRepo) GetSettings(ctx context.Context) (domain.Settings, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.settings, f.found, nil
}

func (f *fakeSettingsRepo) SetSettings(ctx context.Context, s domain.Settings, now time.Time) (domain.Settings, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if s.CreatedAt.IsZero() {
		s.CreatedAt = now
	}
	s.UpdatedAt = now
	f.settings = s
	f.found = true
	return s, nil
}

type fakeProvider struct {
	available   bool
	audio       []byte
	contentType string
	err         error
	delay       time.Duration
	calls       int32
}

func (f *fakeProvider) Capabilities() tts.Capabilities {
	return tts.Capabilities{Available: f.available}
}

func (f *fakeProvider) ListVoices(ctx context.Context) ([]tts.Voice, error) {
	return []tts.Voice{{ID: "voice-1", Name: "Test Voice", Language: "en-US", IsDefault: true}}, nil
}

func (f *fakeProvider) Synthesize(ctx context.Context, in tts.SynthesizeInput) (tts.SynthesizeResult, error) {
	atomic.AddInt32(&f.calls, 1)
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return tts.SynthesizeResult{}, ctx.Err()
		}
	}
	if f.err != nil {
		return tts.SynthesizeResult{}, f.err
	}
	ct := f.contentType
	if ct == "" {
		ct = "audio/wav"
	}
	ab := f.audio
	if ab == nil {
		ab = []byte{0x01, 0x02, 0x03}
	}
	return tts.SynthesizeResult{ContentType: ct, Audio: ab}, nil
}

func availableProvider() *fakeProvider { return &fakeProvider{available: true} }

func waitUntil(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}

// newTestManager builds a Manager over a real Bus and a real
// domain.Service (backed by an in-memory fake repository), with
// settings already saved as given. Starts the manager and waits for
// its Event Bus subscription to be live before returning.
func newTestManager(t *testing.T, provider tts.Provider, settings domain.Settings) (*Manager, *bus.Bus) {
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
		SettingsService:  svc,
		Bus:              b,
		Provider:         provider,
		SynthesisTimeout: 2 * time.Second,
		ItemExpiry:       time.Minute,
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

func newMsg(text string) *engagement.Message {
	m := engagement.NewMessage([]engagement.Fragment{{Type: engagement.FragmentText, Text: text}})
	return &m
}

func chatEvent(providerUserID, displayName, text string) engagement.Event {
	return engagement.Event{
		SchemaVersion:      engagement.CurrentSchemaVersion,
		ProviderID:         engagement.ProviderTwitch,
		ConnectedAccountID: "acct_1",
		Type:               engagement.TypeChatMessage,
		PlatformTimestamp:  time.Now().UTC(),
		DedupeKey:          "dk_" + providerUserID + "_" + text,
		User:               &engagement.User{ProviderUserID: providerUserID, DisplayName: displayName},
		Message:            newMsg(text),
	}
}

func donationEvent(dedupeKey string, micros int64, currency string) engagement.Event {
	return engagement.Event{
		SchemaVersion:      engagement.CurrentSchemaVersion,
		ProviderID:         engagement.ProviderStreamElements,
		ConnectedAccountID: "donsrc_1",
		Type:               engagement.TypeDonation,
		PlatformTimestamp:  time.Now().UTC(),
		DedupeKey:          dedupeKey,
		User:               &engagement.User{ProviderUserID: "donor1", DisplayName: "Donor"},
		Money:              &engagement.Money{AmountMicros: micros, Currency: currency, DisplayAmount: "$x"},
	}
}

func baseTestSettings() domain.Settings {
	s := domain.Default()
	s.Enabled = true
	s.ProviderMode = domain.ProviderModeSystem
	s.EnabledEventTypes = []engagement.Type{engagement.TypeChatMessage, engagement.TypeDonation, engagement.TypeBits}
	return s
}

// --- eligibility pipeline tests ---------------------------------------

func TestManagerEnqueuesEligibleChatMessage(t *testing.T) {
	mgr, b := newTestManager(t, availableProvider(), baseTestSettings())
	if _, _, err := b.Publish(chatEvent("u1", "Ada", "hello there")); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	waitUntil(t, time.Second, func() bool { return mgr.Status().ReadyQueueCount == 1 })
}

func TestManagerIgnoresDisabledSettings(t *testing.T) {
	settings := baseTestSettings()
	settings.Enabled = false
	mgr, b := newTestManager(t, availableProvider(), settings)
	b.Publish(chatEvent("u1", "Ada", "hello"))
	time.Sleep(100 * time.Millisecond)
	if got := mgr.Status().ReadyQueueCount; got != 0 {
		t.Errorf("ReadyQueueCount = %d, want 0 (disabled)", got)
	}
}

func TestManagerIgnoresSyntheticEvent(t *testing.T) {
	mgr, b := newTestManager(t, availableProvider(), baseTestSettings())
	evt := chatEvent("u1", "Ada", "hello")
	evt.Synthetic = true
	b.Publish(evt)
	time.Sleep(100 * time.Millisecond)
	if got := mgr.Status().ReadyQueueCount; got != 0 {
		t.Errorf("ReadyQueueCount = %d, want 0 (synthetic must never enter the real queue)", got)
	}
}

func TestManagerIgnoresEventTypeNotEnabled(t *testing.T) {
	settings := baseTestSettings()
	settings.EnabledEventTypes = []engagement.Type{engagement.TypeDonation}
	mgr, b := newTestManager(t, availableProvider(), settings)
	b.Publish(chatEvent("u1", "Ada", "hello"))
	time.Sleep(100 * time.Millisecond)
	if got := mgr.Status().ReadyQueueCount; got != 0 {
		t.Errorf("ReadyQueueCount = %d, want 0 (chat.message not enabled)", got)
	}
}

func TestManagerSupporterOnlyModeBypassesEnabledEventTypes(t *testing.T) {
	settings := baseTestSettings()
	settings.SupporterOnlyMode = true
	settings.EnabledEventTypes = nil // deliberately empty - supporter-only bypasses this
	mgr, b := newTestManager(t, availableProvider(), settings)

	b.Publish(donationEvent("dk1", 5_000_000, "USD"))
	waitUntil(t, time.Second, func() bool { return mgr.Status().ReadyQueueCount == 1 })

	b.Publish(chatEvent("u1", "Ada", "hello"))
	time.Sleep(100 * time.Millisecond)
	if got := mgr.Status().ReadyQueueCount; got != 1 {
		t.Errorf("ReadyQueueCount = %d, want 1 (chat.message is never supporter-family)", got)
	}
}

func TestManagerProviderIDFilter(t *testing.T) {
	settings := baseTestSettings()
	settings.EnabledProviderIDs = []engagement.ProviderID{engagement.ProviderYouTube}
	mgr, b := newTestManager(t, availableProvider(), settings)
	b.Publish(chatEvent("u1", "Ada", "hello")) // Twitch
	time.Sleep(100 * time.Millisecond)
	if got := mgr.Status().ReadyQueueCount; got != 0 {
		t.Errorf("ReadyQueueCount = %d, want 0 (Twitch not in EnabledProviderIDs)", got)
	}
}

func TestManagerSourceIDFilterIncludesDonationSources(t *testing.T) {
	settings := baseTestSettings()
	settings.SupporterOnlyMode = true
	settings.EnabledSourceIDs = []string{"donsrc_other"}
	mgr, b := newTestManager(t, availableProvider(), settings)
	b.Publish(donationEvent("dk1", 5_000_000, "USD")) // ConnectedAccountID: donsrc_1
	time.Sleep(100 * time.Millisecond)
	if got := mgr.Status().ReadyQueueCount; got != 0 {
		t.Errorf("ReadyQueueCount = %d, want 0 (donsrc_1 not in EnabledSourceIDs)", got)
	}
}

func TestManagerExactCurrencyThresholdSameCurrencyPasses(t *testing.T) {
	settings := baseTestSettings()
	settings.SupporterOnlyMode = true
	min := int64(5_000_000)
	settings.ThresholdCurrency = "USD"
	settings.ThresholdMinimumAmountMicros = &min
	mgr, b := newTestManager(t, availableProvider(), settings)
	b.Publish(donationEvent("dk1", 5_000_000, "USD"))
	waitUntil(t, time.Second, func() bool { return mgr.Status().ReadyQueueCount == 1 })
}

func TestManagerExactCurrencyThresholdDifferentCurrencyNeverCompared(t *testing.T) {
	settings := baseTestSettings()
	settings.SupporterOnlyMode = true
	min := int64(1_000_000)
	settings.ThresholdCurrency = "USD"
	settings.ThresholdMinimumAmountMicros = &min
	mgr, b := newTestManager(t, availableProvider(), settings)
	// A large EUR donation must never be treated as satisfying a USD
	// threshold merely because the number is bigger.
	b.Publish(donationEvent("dk1", 50_000_000, "EUR"))
	time.Sleep(100 * time.Millisecond)
	if got := mgr.Status().ReadyQueueCount; got != 0 {
		t.Errorf("ReadyQueueCount = %d, want 0 (EUR never compared against a USD threshold)", got)
	}
}

func TestManagerBitsThresholdIsIntegerSemantics(t *testing.T) {
	settings := baseTestSettings()
	minBits := int64(100)
	settings.MinimumBits = &minBits
	mgr, b := newTestManager(t, availableProvider(), settings)

	below := int64(50)
	b.Publish(engagement.Event{
		SchemaVersion: engagement.CurrentSchemaVersion, ProviderID: engagement.ProviderTwitch,
		ConnectedAccountID: "acct_1", Type: engagement.TypeBits, PlatformTimestamp: time.Now().UTC(), DedupeKey: "bits1",
		User: &engagement.User{ProviderUserID: "u1", DisplayName: "Ada"}, Quantity: &below,
	})
	time.Sleep(100 * time.Millisecond)
	if got := mgr.Status().ReadyQueueCount; got != 0 {
		t.Errorf("ReadyQueueCount = %d, want 0 (below Bits threshold)", got)
	}

	above := int64(150)
	b.Publish(engagement.Event{
		SchemaVersion: engagement.CurrentSchemaVersion, ProviderID: engagement.ProviderTwitch,
		ConnectedAccountID: "acct_1", Type: engagement.TypeBits, PlatformTimestamp: time.Now().UTC(), DedupeKey: "bits2",
		User: &engagement.User{ProviderUserID: "u1", DisplayName: "Ada"}, Quantity: &above,
	})
	waitUntil(t, time.Second, func() bool { return mgr.Status().ReadyQueueCount == 1 })
}

func TestManagerRecognizedCommandSuppressedForChatOnly(t *testing.T) {
	settings := baseTestSettings()
	settings.SuppressCommands = true
	mgr, b := newTestManager(t, availableProvider(), settings)
	b.Publish(chatEvent("u1", "Ada", "!uptime"))
	time.Sleep(100 * time.Millisecond)
	if got := mgr.Status().ReadyQueueCount; got != 0 {
		t.Errorf("ReadyQueueCount = %d, want 0 (recognized command suppressed)", got)
	}
}

func TestManagerDonationStartingWithBangIsNeverSuppressedAsCommand(t *testing.T) {
	settings := baseTestSettings()
	settings.SupporterOnlyMode = true
	settings.SuppressCommands = true
	mgr, b := newTestManager(t, availableProvider(), settings)
	evt := donationEvent("dk1", 5_000_000, "USD")
	evt.Message = newMsg("!not a command, just a message")
	b.Publish(evt)
	waitUntil(t, time.Second, func() bool { return mgr.Status().ReadyQueueCount == 1 })
}

func TestManagerSelfMessageSuppressed(t *testing.T) {
	settings := baseTestSettings()
	repo := &fakeSettingsRepo{}
	svc := domain.NewService(domain.Options{Repository: repo})
	svc.Get(context.Background())
	svc.Update(context.Background(), settings)
	b := bus.New(bus.Options{})
	mgr := NewManager(Options{
		SettingsService: svc, Bus: b, Provider: availableProvider(),
		SelfLookup: func(accountID string) (string, bool) { return "self_id", accountID == "acct_1" },
	})
	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { mgr.Shutdown(context.Background()) })
	waitUntil(t, time.Second, mgr.Subscribed)

	b.Publish(chatEvent("self_id", "MyBot", "hello"))
	time.Sleep(100 * time.Millisecond)
	if got := mgr.Status().ReadyQueueCount; got != 0 {
		t.Errorf("ReadyQueueCount = %d, want 0 (self message suppressed)", got)
	}
}

func TestManagerBotMessageSuppressedViaDirectHelper(t *testing.T) {
	mgr := &Manager{botUsers: map[string]struct{}{
		botKey("twitch", "acct_1", "bot1"): {},
	}}
	evt := chatEvent("bot1", "SomeBot", "hi")
	if !mgr.isBot(evt) {
		t.Error("isBot() = false for a configured bot user, want true")
	}
	if mgr.isBot(chatEvent("human1", "Ada", "hi")) {
		t.Error("isBot() = true for a non-bot user, want false")
	}
}

// --- manual approval / synthetic isolation ------------------------------

func TestManagerManualApprovalRoutesToPending(t *testing.T) {
	settings := baseTestSettings()
	settings.ManualApproval = true
	mgr, b := newTestManager(t, availableProvider(), settings)
	b.Publish(chatEvent("u1", "Ada", "hello"))
	waitUntil(t, time.Second, func() bool { return mgr.Status().PendingApprovalCount == 1 })
	if got := mgr.Status().ReadyQueueCount; got != 0 {
		t.Errorf("ReadyQueueCount = %d, want 0 while pending approval", got)
	}
}

func TestManagerApproveAndReject(t *testing.T) {
	settings := baseTestSettings()
	settings.ManualApproval = true
	mgr, b := newTestManager(t, availableProvider(), settings)
	b.Publish(chatEvent("u1", "Ada", "hello"))
	waitUntil(t, time.Second, func() bool { return mgr.Status().PendingApprovalCount == 1 })

	pending := mgr.PendingList()
	if len(pending) != 1 {
		t.Fatalf("PendingList() = %v, want 1 item", pending)
	}
	if !mgr.Approve(pending[0].ID) {
		t.Fatal("Approve() = false, want true")
	}
	if got := mgr.Status().ReadyQueueCount; got != 1 {
		t.Errorf("ReadyQueueCount = %d, want 1 after approve", got)
	}

	if mgr.Reject("nonexistent") {
		t.Error("Reject() = true for an unknown id, want false")
	}
}

func TestManagerTestSpeakBypassesCooldownAndApproval(t *testing.T) {
	settings := baseTestSettings()
	settings.ManualApproval = true
	settings.GlobalCooldownSeconds = 300
	mgr, _ := newTestManager(t, availableProvider(), settings)

	item, err := mgr.TestSpeak("hello from test speak")
	if err != nil {
		t.Fatalf("TestSpeak() error = %v", err)
	}
	if !item.Synthetic {
		t.Error("TestSpeak() item.Synthetic = false, want true")
	}
	if got := mgr.Status().ReadyQueueCount; got != 1 {
		t.Errorf("ReadyQueueCount = %d, want 1 (Test Speak bypasses manual approval)", got)
	}
}

func TestManagerTestSpeakNeverMutatesRealCooldown(t *testing.T) {
	settings := baseTestSettings()
	settings.GlobalCooldownSeconds = 300
	mgr, b := newTestManager(t, availableProvider(), settings)

	for i := 0; i < 5; i++ {
		if _, err := mgr.TestSpeak("synthetic speech"); err != nil {
			t.Fatalf("TestSpeak() error = %v", err)
		}
	}
	b.Publish(chatEvent("u1", "Ada", "a real message"))
	waitUntil(t, time.Second, func() bool { return mgr.Status().ReadyQueueCount == 6 })
}

func TestManagerTestSpeakRequiresEnabled(t *testing.T) {
	settings := baseTestSettings()
	settings.Enabled = false
	mgr, _ := newTestManager(t, availableProvider(), settings)
	if _, err := mgr.TestSpeak("hello"); !errors.Is(err, ErrDisabled) {
		t.Errorf("TestSpeak() error = %v, want ErrDisabled", err)
	}
}

func TestManagerTestSpeakRejectsEmptyAfterPreprocessing(t *testing.T) {
	settings := baseTestSettings()
	settings.BlockedWords = []string{"hello"}
	mgr, _ := newTestManager(t, availableProvider(), settings)
	if _, err := mgr.TestSpeak("hello"); !errors.Is(err, ErrEmptyText) {
		t.Errorf("TestSpeak() error = %v, want ErrEmptyText", err)
	}
}

// --- cooldown reservation timing ---------------------------------------

func TestManagerPerUserCooldownSuppressesSecondMessage(t *testing.T) {
	settings := baseTestSettings()
	settings.PerUserCooldownSeconds = 300
	settings.GlobalCooldownSeconds = 0
	mgr, b := newTestManager(t, availableProvider(), settings)

	b.Publish(chatEvent("u1", "Ada", "first"))
	waitUntil(t, time.Second, func() bool { return mgr.Status().ReadyQueueCount == 1 })

	b.Publish(chatEvent("u1", "Ada", "second"))
	time.Sleep(100 * time.Millisecond)
	if got := mgr.Status().ReadyQueueCount; got != 1 {
		t.Errorf("ReadyQueueCount = %d, want 1 (second message suppressed by per-user cooldown)", got)
	}

	// A different user is unaffected by userA's own cooldown.
	b.Publish(chatEvent("u2", "Grace", "hi"))
	waitUntil(t, time.Second, func() bool { return mgr.Status().ReadyQueueCount == 2 })
}

func TestManagerAnonymousEventUsesGlobalCooldownOnly(t *testing.T) {
	settings := baseTestSettings()
	settings.SupporterOnlyMode = true
	settings.GlobalCooldownSeconds = 300
	mgr, b := newTestManager(t, availableProvider(), settings)

	anon1 := donationEvent("dk1", 5_000_000, "USD")
	anon1.User = &engagement.User{Anonymous: true}
	b.Publish(anon1)
	waitUntil(t, time.Second, func() bool { return mgr.Status().ReadyQueueCount == 1 })

	anon2 := donationEvent("dk2", 5_000_000, "USD")
	anon2.User = &engagement.User{Anonymous: true}
	b.Publish(anon2)
	time.Sleep(100 * time.Millisecond)
	if got := mgr.Status().ReadyQueueCount; got != 1 {
		t.Errorf("ReadyQueueCount = %d, want 1 (second anonymous donation suppressed by global cooldown)", got)
	}
}

// --- queue capacity ------------------------------------------------------

func TestManagerQueueCapacityDropsBeyondBound(t *testing.T) {
	settings := baseTestSettings()
	settings.QueueCapacity = domain.MinQueueCapacity
	settings.PerUserCooldownSeconds = 0
	settings.GlobalCooldownSeconds = 0
	mgr, b := newTestManager(t, availableProvider(), settings)

	for i := 0; i < domain.MinQueueCapacity+3; i++ {
		user := "user_" + string(rune('a'+i))
		b.Publish(chatEvent(user, "U", "hi"))
	}
	waitUntil(t, time.Second, func() bool { return mgr.Status().ReadyQueueCount == domain.MinQueueCapacity })
	time.Sleep(50 * time.Millisecond)
	if got := mgr.Status().Counters.TotalCapacityDropped; got == 0 {
		t.Error("TotalCapacityDropped = 0, want > 0")
	}
}

// --- promotion, synthesis, renderer, and acknowledgement ----------------

func TestManagerPromotesAndSynthesizesWhenRendererConnected(t *testing.T) {
	provider := &fakeProvider{available: true, audio: []byte{1, 2, 3, 4}, contentType: "audio/wav"}
	mgr, b := newTestManager(t, provider, baseTestSettings())

	token, err := mgr.ConnectRenderer()
	if err != nil {
		t.Fatalf("ConnectRenderer() error = %v", err)
	}
	b.Publish(chatEvent("u1", "Ada", "hello"))

	waitUntil(t, 2*time.Second, func() bool { return mgr.PublicCurrentSnapshot().HasItem })
	snap := mgr.PublicCurrentSnapshot()
	audioBytes, contentType, ok := mgr.CurrentAudioBytes(snap.ItemID, snap.BytesToken)
	if !ok {
		t.Fatal("CurrentAudioBytes() ok = false, want true")
	}
	if contentType != "audio/wav" || len(audioBytes) != 4 {
		t.Errorf("CurrentAudioBytes() = %v, %q, want 4 bytes, audio/wav", audioBytes, contentType)
	}

	if err := mgr.Ack(token, snap.ItemID, AckStarted); err != nil {
		t.Fatalf("Ack(started) error = %v", err)
	}
	if err := mgr.Ack(token, snap.ItemID, AckEnded); err != nil {
		t.Fatalf("Ack(ended) error = %v", err)
	}
	if got := mgr.Status().TotalPlayed; got != 1 {
		t.Errorf("TotalPlayed = %d, want 1", got)
	}
	if mgr.Status().HasCurrentItem {
		t.Error("HasCurrentItem = true after playback ended, want false")
	}
}

func TestManagerNoPromotionWithoutRenderer(t *testing.T) {
	mgr, b := newTestManager(t, availableProvider(), baseTestSettings())
	b.Publish(chatEvent("u1", "Ada", "hello"))
	waitUntil(t, time.Second, func() bool { return mgr.Status().ReadyQueueCount == 1 })
	time.Sleep(100 * time.Millisecond)
	if mgr.Status().HasCurrentItem {
		t.Error("HasCurrentItem = true with no renderer connected, want false (waiting_for_renderer)")
	}
}

func TestManagerAckRejectsWrongToken(t *testing.T) {
	provider := &fakeProvider{available: true}
	mgr, b := newTestManager(t, provider, baseTestSettings())
	mgr.ConnectRenderer()
	b.Publish(chatEvent("u1", "Ada", "hello"))
	waitUntil(t, 2*time.Second, func() bool { return mgr.PublicCurrentSnapshot().HasItem })
	snap := mgr.PublicCurrentSnapshot()

	if err := mgr.Ack("wrong-token", snap.ItemID, AckStarted); !errors.Is(err, ErrAckRejected) {
		t.Errorf("Ack() error = %v, want ErrAckRejected", err)
	}
}

func TestManagerAckRejectsStaleSessionAfterNewRendererConnects(t *testing.T) {
	provider := &fakeProvider{available: true}
	mgr, b := newTestManager(t, provider, baseTestSettings())
	oldToken, _ := mgr.ConnectRenderer()
	b.Publish(chatEvent("u1", "Ada", "hello"))
	waitUntil(t, 2*time.Second, func() bool { return mgr.PublicCurrentSnapshot().HasItem })
	snap := mgr.PublicCurrentSnapshot()

	newToken, _ := mgr.ConnectRenderer()
	if newToken == oldToken {
		t.Fatal("new renderer session token collided with the old one")
	}
	if err := mgr.Ack(oldToken, snap.ItemID, AckStarted); !errors.Is(err, ErrAckRejected) {
		t.Errorf("Ack(old session) error = %v, want ErrAckRejected", err)
	}
	if err := mgr.Ack(newToken, snap.ItemID, AckStarted); err != nil {
		t.Errorf("Ack(new session) error = %v, want nil", err)
	}
}

func TestManagerDuplicateEndedAckRejected(t *testing.T) {
	provider := &fakeProvider{available: true}
	mgr, b := newTestManager(t, provider, baseTestSettings())
	token, _ := mgr.ConnectRenderer()
	b.Publish(chatEvent("u1", "Ada", "hello"))
	waitUntil(t, 2*time.Second, func() bool { return mgr.PublicCurrentSnapshot().HasItem })
	snap := mgr.PublicCurrentSnapshot()

	if err := mgr.Ack(token, snap.ItemID, AckStarted); err != nil {
		t.Fatalf("Ack(started) error = %v", err)
	}
	if err := mgr.Ack(token, snap.ItemID, AckEnded); err != nil {
		t.Fatalf("Ack(ended) error = %v", err)
	}
	if err := mgr.Ack(token, snap.ItemID, AckEnded); !errors.Is(err, ErrAckRejected) {
		t.Errorf("duplicate Ack(ended) error = %v, want ErrAckRejected", err)
	}
}

func TestManagerRendererDisconnectWhilePlayingMarksInterrupted(t *testing.T) {
	provider := &fakeProvider{available: true}
	mgr, b := newTestManager(t, provider, baseTestSettings())
	token, _ := mgr.ConnectRenderer()
	b.Publish(chatEvent("u1", "Ada", "hello"))
	waitUntil(t, 2*time.Second, func() bool { return mgr.PublicCurrentSnapshot().HasItem })
	snap := mgr.PublicCurrentSnapshot()
	if err := mgr.Ack(token, snap.ItemID, AckStarted); err != nil {
		t.Fatalf("Ack(started) error = %v", err)
	}

	mgr.DisconnectRenderer(token)

	if mgr.Status().HasCurrentItem {
		t.Error("HasCurrentItem = true after disconnect mid-playback, want false (discarded, not replayed)")
	}
	if got := mgr.Status().TotalInterrupted; got != 1 {
		t.Errorf("TotalInterrupted = %d, want 1", got)
	}
	if got := mgr.Status().TotalPlayed; got != 0 {
		t.Errorf("TotalPlayed = %d, want 0 (an interrupted item is never counted as played)", got)
	}
}

func TestManagerRendererDisconnectBeforePlaybackKeepsItemForNextRenderer(t *testing.T) {
	provider := &fakeProvider{available: true}
	mgr, b := newTestManager(t, provider, baseTestSettings())
	token, _ := mgr.ConnectRenderer()
	b.Publish(chatEvent("u1", "Ada", "hello"))
	waitUntil(t, 2*time.Second, func() bool { return mgr.PublicCurrentSnapshot().HasItem })
	firstSnap := mgr.PublicCurrentSnapshot()

	mgr.DisconnectRenderer(token)
	if !mgr.Status().HasCurrentItem {
		t.Fatal("HasCurrentItem = false after disconnect before playback started, want true (item retained)")
	}

	mgr.ConnectRenderer()
	waitUntil(t, time.Second, func() bool { return mgr.PublicCurrentSnapshot().HasItem })
	secondSnap := mgr.PublicCurrentSnapshot()
	if secondSnap.ItemID != firstSnap.ItemID {
		t.Errorf("ItemID changed across reconnect: %q -> %q, want same item retained", firstSnap.ItemID, secondSnap.ItemID)
	}
}

func TestManagerSynthesisFailureIsolatesOneItem(t *testing.T) {
	provider := &fakeProvider{available: true, err: errors.New("boom")}
	settings := baseTestSettings()
	settings.PerUserCooldownSeconds = 0
	settings.GlobalCooldownSeconds = 0
	mgr, b := newTestManager(t, provider, settings)
	mgr.ConnectRenderer()

	b.Publish(chatEvent("u1", "Ada", "first"))
	waitUntil(t, 2*time.Second, func() bool { return mgr.Status().TotalSynthesisFailed == 1 })
	if mgr.Status().HasCurrentItem {
		t.Error("HasCurrentItem = true after a synthesis failure, want false")
	}

	// The manager must keep processing subsequent items - one failure
	// never stops the whole runtime.
	b.Publish(chatEvent("u2", "Grace", "second"))
	waitUntil(t, 2*time.Second, func() bool { return mgr.Status().TotalSynthesisFailed == 2 })
}

func TestManagerOversizedSynthesisOutputRejected(t *testing.T) {
	provider := &fakeProvider{available: true, audio: make([]byte, 100)}
	repo := &fakeSettingsRepo{}
	svc := domain.NewService(domain.Options{Repository: repo})
	svc.Get(context.Background())
	svc.Update(context.Background(), baseTestSettings())
	b := bus.New(bus.Options{})
	mgr := NewManager(Options{SettingsService: svc, Bus: b, Provider: provider, MaxAudioBytes: 10, SynthesisTimeout: 2 * time.Second})
	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { mgr.Shutdown(context.Background()) })
	waitUntil(t, time.Second, mgr.Subscribed)
	mgr.ConnectRenderer()

	b.Publish(chatEvent("u1", "Ada", "hello"))
	waitUntil(t, 2*time.Second, func() bool { return mgr.Status().TotalSynthesisFailed == 1 })
}

func TestManagerSkipCurrentCancelsSynthesis(t *testing.T) {
	provider := &fakeProvider{available: true, delay: 5 * time.Second}
	mgr, b := newTestManager(t, provider, baseTestSettings())
	mgr.ConnectRenderer()
	b.Publish(chatEvent("u1", "Ada", "hello"))
	waitUntil(t, time.Second, func() bool { return mgr.Status().HasCurrentItem })

	if !mgr.SkipCurrent() {
		t.Fatal("SkipCurrent() = false, want true")
	}
	if mgr.Status().HasCurrentItem {
		t.Error("HasCurrentItem = true immediately after SkipCurrent(), want false")
	}
	if got := mgr.Status().Counters.TotalManuallySkipped; got != 1 {
		t.Errorf("TotalManuallySkipped = %d, want 1", got)
	}
}

func TestManagerClearQueueNeverTouchesCurrentItem(t *testing.T) {
	provider := &fakeProvider{available: true, delay: 5 * time.Second}
	settings := baseTestSettings()
	settings.PerUserCooldownSeconds = 0
	settings.GlobalCooldownSeconds = 0
	mgr, b := newTestManager(t, provider, settings)
	mgr.ConnectRenderer()

	b.Publish(chatEvent("u1", "Ada", "current one"))
	waitUntil(t, time.Second, func() bool { return mgr.Status().HasCurrentItem })
	b.Publish(chatEvent("u2", "Grace", "queued one"))
	waitUntil(t, time.Second, func() bool { return mgr.Status().ReadyQueueCount == 1 })

	n := mgr.ClearQueue()
	if n != 1 {
		t.Errorf("ClearQueue() = %d, want 1", n)
	}
	if !mgr.Status().HasCurrentItem {
		t.Error("HasCurrentItem = false after ClearQueue(), want true (current item is never cleared)")
	}
}

func TestManagerShutdownStopsCleanly(t *testing.T) {
	repo := &fakeSettingsRepo{}
	svc := domain.NewService(domain.Options{Repository: repo})
	svc.Get(context.Background())
	svc.Update(context.Background(), baseTestSettings())
	b := bus.New(bus.Options{})
	mgr := NewManager(Options{SettingsService: svc, Bus: b, Provider: availableProvider()})
	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	waitUntil(t, time.Second, mgr.Subscribed)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := mgr.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}
