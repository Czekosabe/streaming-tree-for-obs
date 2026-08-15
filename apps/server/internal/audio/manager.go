package audio

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	domain "github.com/streaming-tree/server/internal/domain/audio"
	engagement "github.com/streaming-tree/server/internal/domain/engagement"
	"github.com/streaming-tree/server/internal/domain/operatorchatprefs"
	bus "github.com/streaming-tree/server/internal/engagement"
	"github.com/streaming-tree/server/internal/provider/tts"
)

// Defaults not exposed as an operator-configurable Settings field -
// docs/audio-tts.md §7.
const (
	defaultSynthesisTimeout = 10 * time.Second
	defaultItemExpiry       = 5 * time.Minute
	defaultMaxAudioBytes    = 8 * 1024 * 1024 // 8 MiB

	// audioPollInterval mirrors internal/alerts's own alertsPollInterval
	// reasoning exactly: a real-time poll loop, never one goroutine/timer
	// per queued item, so a fake clock's Advance() in tests is picked up
	// promptly.
	audioPollInterval = 20 * time.Millisecond
	// resubscribeBackoff bounds how quickly the shared Event Bus
	// subscription retries after a failed Subscribe call.
	resubscribeBackoff = time.Second
)

// Clock returns the current time; injected so tests are deterministic.
type Clock func() time.Time

// SelfUserIDLookup resolves a connected account's own provider user id,
// for self-message loop protection (docs/audio-tts.md/governing task
// §42) - a plain callback so this package never imports
// internal/domain/account, mirroring internal/chatoverlay's own
// AccountLabelLookup convention.
type SelfUserIDLookup func(connectedAccountID string) (providerUserID string, ok bool)

// Sentinel errors Manager methods return.
var (
	ErrDisabled     = errors.New("audio/tts is disabled")
	ErrEmptyText    = errors.New("text is empty after preprocessing")
	ErrQueueFull    = errors.New("audio queue is full")
	ErrAckRejected  = errors.New("playback acknowledgement rejected")
	ErrItemNotFound = errors.New("audio item not found")
)

// Options constructs a Manager.
type Options struct {
	SettingsService *domain.Service
	Bus             *bus.Bus
	// Provider is optional - a nil Provider degrades to every
	// synthesis attempt failing safely (counted, never a panic),
	// mirroring this codebase's established "optional dependency, nil
	// degrades gracefully" convention (internal/alerts.ManagerOptions's
	// own AssetService/VisualDesignService).
	Provider tts.Provider
	// OperatorChatPrefs is optional - nil means bot-message suppression
	// never triggers (the bot list is always empty).
	OperatorChatPrefs *operatorchatprefs.Service
	// SelfLookup is optional - nil means self-message suppression never
	// triggers.
	SelfLookup SelfUserIDLookup
	// Now is a test-only fake-clock override; production code leaves it
	// nil.
	Now Clock

	SynthesisTimeout time.Duration
	ItemExpiry       time.Duration
	MaxAudioBytes    int
}

type playbackState string

const (
	playbackStateSynthesizing       playbackState = "synthesizing"
	playbackStateWaitingForRenderer playbackState = "waiting_for_renderer"
	playbackStateReady              playbackState = "ready"
	playbackStatePlaying            playbackState = "playing"
)

// AckKind is the kind of public playback acknowledgement a renderer
// reports (docs/audio-tts.md §14).
type AckKind string

const (
	AckStarted AckKind = "playback_started"
	AckEnded   AckKind = "playback_ended"
	AckFailed  AckKind = "playback_failed"
)

type currentState struct {
	item          Item
	audio         tts.SynthesizeResult
	bytesToken    string
	sequence      uint64
	playbackState playbackState
}

type rendererSession struct {
	token string
}

// PublicCurrent is the safe, bounded public summary of the current item
// - docs/audio-tts.md §19's privacy boundary: no source event, no
// account/user id, no message text beyond what the audio itself already
// contains.
type PublicCurrent struct {
	HasItem     bool
	ItemID      string
	BytesToken  string
	ContentType string
	Volume      float64
	Sequence    uint64
	Idle        bool
}

// Status is the bounded, safe-to-expose management summary (governing
// task §46) - never a complete historical message list.
type Status struct {
	Enabled              bool
	ProviderMode         domain.ProviderMode
	ProviderAvailable    bool
	RendererConnected    bool
	HasCurrentItem       bool
	CurrentSynthetic     bool
	PendingApprovalCount int
	ReadyQueueCount      int
	Capacity             int
	Counters             QueueCounters
	TotalPlayed          int
	TotalPlaybackFailed  int
	TotalSynthesisFailed int
	TotalInterrupted     int
	InputGap             bool
	Subscribed           bool
}

// Manager is the Stage 17A audio runtime: the ONE Engagement Event Bus
// subscription every eligible event flows through (never a
// Twitch/YouTube/StreamElements-specific callback), the eligibility/
// preprocessing/cooldown pipeline, the bounded queue, just-in-time
// provider synthesis, and the single-active-renderer playback lease.
// Mirrors internal/alerts.Manager's own Options/NewManager/Start/
// Shutdown shape.
type Manager struct {
	settingsSvc *domain.Service
	source      *bus.Bus
	provider    tts.Provider
	opPrefs     *operatorchatprefs.Service
	selfLookup  SelfUserIDLookup
	now         Clock

	synthesisTimeout time.Duration
	itemExpiry       time.Duration
	maxAudioBytes    int

	botMu    sync.RWMutex
	botUsers map[string]struct{}

	mu                   sync.Mutex
	settings             domain.Settings
	queue                *Queue
	cooldowns            *CooldownTracker
	current              *currentState
	currentSynthCancel   context.CancelFunc
	rendererSession      *rendererSession
	sequence             uint64
	totalPlayed          int
	totalPlaybackFailed  int
	totalSynthesisFailed int
	totalInterrupted     int

	inputGap   atomic.Bool
	subscribed atomic.Bool

	changes changeBroadcaster

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewManager builds a Manager. Call Start to load settings and begin
// the poll loop/Event Bus subscription; call Shutdown to stop both
// cleanly.
func NewManager(opts Options) *Manager {
	now := opts.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	synthesisTimeout := opts.SynthesisTimeout
	if synthesisTimeout <= 0 {
		synthesisTimeout = defaultSynthesisTimeout
	}
	itemExpiry := opts.ItemExpiry
	if itemExpiry <= 0 {
		itemExpiry = defaultItemExpiry
	}
	maxAudioBytes := opts.MaxAudioBytes
	if maxAudioBytes <= 0 {
		maxAudioBytes = defaultMaxAudioBytes
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		settingsSvc: opts.SettingsService, source: opts.Bus, provider: opts.Provider,
		opPrefs: opts.OperatorChatPrefs, selfLookup: opts.SelfLookup, now: now,
		synthesisTimeout: synthesisTimeout, itemExpiry: itemExpiry, maxAudioBytes: maxAudioBytes,
		botUsers:  make(map[string]struct{}),
		queue:     NewQueue(domain.DefaultQueueCapacity),
		cooldowns: NewCooldownTracker(),
		ctx:       ctx, cancel: cancel,
	}
}

// Start loads current settings, refreshes the bot-user cache, then
// begins the poll loop and the shared Event Bus subscription.
func (m *Manager) Start(ctx context.Context) error {
	settings, err := m.settingsSvc.Get(ctx)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.settings = settings
	m.queue.SetCapacity(settings.QueueCapacity)
	m.mu.Unlock()

	_ = m.RefreshBotUsers(ctx)

	m.wg.Add(2)
	go m.runLoop()
	go m.runSubscription()
	return nil
}

// Shutdown cancels the poll loop, the subscription, and any in-flight
// synthesis, waiting for every goroutine to exit, bounded by ctx.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.cancel()
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ReloadSettings re-reads persisted settings - called by the future
// HTTP layer after every settings write, mirroring
// internal/alerts.Manager's own reload-after-write convention.
func (m *Manager) ReloadSettings(ctx context.Context) error {
	settings, err := m.settingsSvc.Get(ctx)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.settings = settings
	m.queue.SetCapacity(settings.QueueCapacity)
	m.mu.Unlock()
	return nil
}

// RefreshBotUsers re-reads the operator-maintained bot-user list. A nil
// OperatorChatPrefs leaves the bot list empty; a fetch error is not
// fatal to Start (bot suppression simply never triggers until the next
// successful refresh).
func (m *Manager) RefreshBotUsers(ctx context.Context) error {
	if m.opPrefs == nil {
		return nil
	}
	refs, err := m.opPrefs.BotUsers(ctx)
	if err != nil {
		return err
	}
	next := make(map[string]struct{}, len(refs))
	for _, r := range refs {
		next[botKey(string(r.ProviderID), r.ConnectedAccountID, r.ProviderUserID)] = struct{}{}
	}
	m.botMu.Lock()
	m.botUsers = next
	m.botMu.Unlock()
	return nil
}

func botKey(providerID, connectedAccountID, providerUserID string) string {
	return providerID + "|" + connectedAccountID + "|" + providerUserID
}

// Subscribed reports whether the shared Event Bus subscription is
// currently live - primarily for tests to synchronize on, mirroring
// internal/alerts.Manager.Subscribed's own identical reasoning.
func (m *Manager) Subscribed() bool { return m.subscribed.Load() }

func (m *Manager) runLoop() {
	defer m.wg.Done()
	ticker := time.NewTicker(audioPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.tick()
		}
	}
}

func (m *Manager) tick() {
	now := m.now()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queue.DiscardExpired(now)
	if m.current != nil {
		return
	}
	if m.rendererSession == nil {
		return
	}
	it, ok := m.queue.PopNextEligible(now)
	if !ok {
		return
	}
	m.promoteLocked(it)
}

// promoteLocked starts just-in-time synthesis for it - caller must hold
// m.mu. The synthesis itself runs in its own goroutine so the poll loop
// is never blocked on a slow/timed-out provider call.
func (m *Manager) promoteLocked(it Item) {
	ctx, cancel := context.WithTimeout(m.ctx, m.synthesisTimeout)
	m.current = &currentState{item: it, playbackState: playbackStateSynthesizing}
	m.currentSynthCancel = cancel
	m.wg.Add(1)
	go m.synthesize(ctx, it)
	m.changes.notify()
}

func (m *Manager) synthesize(ctx context.Context, it Item) {
	defer m.wg.Done()

	var result tts.SynthesizeResult
	var synthErr error
	switch {
	case m.provider == nil:
		synthErr = tts.ErrUnavailable
	default:
		if caps := m.provider.Capabilities(); !caps.Available {
			synthErr = tts.ErrUnavailable
		} else {
			result, synthErr = m.provider.Synthesize(ctx, tts.SynthesizeInput{
				Text: it.Text, VoiceID: it.Snapshot.VoiceID, Language: it.Snapshot.Language,
				Speed: it.Snapshot.Speed, Volume: it.Snapshot.Volume,
			})
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	defer m.changes.notify()
	m.currentSynthCancel = nil

	if m.current == nil || m.current.item.ID != it.ID {
		// Skipped or cleared while synthesizing - discard the result,
		// never resurrect it as a new current item.
		return
	}
	if synthErr != nil {
		m.totalSynthesisFailed++
		m.current = nil
		return
	}
	if m.maxAudioBytes > 0 && len(result.Audio) > m.maxAudioBytes {
		m.totalSynthesisFailed++
		m.current = nil
		return
	}
	token, err := newSessionToken()
	if err != nil {
		m.totalSynthesisFailed++
		m.current = nil
		return
	}

	m.current.audio = result
	m.current.bytesToken = token
	m.sequence++
	m.current.sequence = m.sequence
	if m.rendererSession != nil {
		m.current.playbackState = playbackStateReady
	} else {
		m.current.playbackState = playbackStateWaitingForRenderer
	}
}

func (m *Manager) runSubscription() {
	defer m.wg.Done()
	for {
		select {
		case <-m.ctx.Done():
			return
		default:
		}

		snap := m.source.Snapshot()
		sub, _, err := m.source.Subscribe(snap.NewestSequence)
		if err != nil {
			m.inputGap.Store(true)
			select {
			case <-m.ctx.Done():
				return
			case <-time.After(resubscribeBackoff):
				continue
			}
		}
		m.subscribed.Store(true)
		m.consume(sub)
		m.subscribed.Store(false)
	}
}

func (m *Manager) consume(sub *bus.Subscription) {
	for {
		select {
		case <-m.ctx.Done():
			sub.Cancel()
			return
		case evt, ok := <-sub.Events():
			if !ok {
				m.inputGap.Store(true)
				return
			}
			m.handleEvent(evt)
		case <-sub.Closed():
			m.inputGap.Store(true)
			return
		}
	}
}

// handleEvent is the full eligibility/preprocessing/cooldown pipeline
// (docs/audio-tts.md §9-§11) for one real Event Bus event. Never called
// for a synthetic event (Synthetic events are rejected first, exactly
// like internal/alerts's own matcher).
func (m *Manager) handleEvent(evt engagement.Event) {
	if evt.Synthetic {
		return
	}
	now := m.now()

	m.mu.Lock()
	settings := m.settings
	m.mu.Unlock()

	if !settings.Enabled || settings.ProviderMode == domain.ProviderModeDisabled {
		return
	}

	capa := CapabilityFor(evt.Type)
	if !capa.Speakable {
		return
	}

	if settings.SupporterOnlyMode {
		if !capa.SupporterFamily {
			return
		}
	} else if !containsType(settings.EnabledEventTypes, evt.Type) {
		return
	}

	if len(settings.EnabledProviderIDs) > 0 && !containsProviderID(settings.EnabledProviderIDs, evt.ProviderID) {
		return
	}
	if len(settings.EnabledSourceIDs) > 0 && !containsString(settings.EnabledSourceIDs, evt.ConnectedAccountID) {
		return
	}

	isCommand := false
	if evt.Type == engagement.TypeChatMessage {
		if m.isSelf(evt) || m.isBot(evt) {
			return
		}
		if evt.Message != nil {
			isCommand = isCommandMessage(evt.Message.Text)
		}
	}

	if evt.Type == engagement.TypeBits && settings.MinimumBits != nil {
		if evt.Quantity == nil || *evt.Quantity < *settings.MinimumBits {
			return
		}
	}
	if capa.HasMoney && settings.ThresholdMinimumAmountMicros != nil {
		if evt.Money == nil || evt.Money.Currency != settings.ThresholdCurrency || evt.Money.AmountMicros < *settings.ThresholdMinimumAmountMicros {
			return
		}
	}

	utterance, ok := BuildUtterance(evt)
	if !ok {
		return
	}
	text, ok := Preprocess(utterance, PreprocessConfig{
		SuppressCommands:       settings.SuppressCommands,
		IsCommand:              isCommand,
		RemoveURLs:             settings.RemoveURLs,
		BlockedWords:           settings.BlockedWords,
		NormalizeRepeatedChars: settings.NormalizeRepeatedChars,
		MaxLengthCodePoints:    settings.MaxTextLengthCodePoints,
	})
	if !ok {
		return
	}

	key, hasKey := cooldownKeyForEvent(evt)

	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.cooldowns.Allowed(hasKey, key, now) {
		return
	}

	id, err := NewItemID()
	if err != nil {
		return
	}
	item := Item{
		ID:   id,
		Text: text,
		Snapshot: ItemSnapshot{
			ProviderMode: settings.ProviderMode,
			VoiceID:      settings.VoiceID,
			Language:     settings.Language,
			Speed:        settings.Speed,
			Volume:       settings.Volume,
		},
		EnqueuedAt: now,
		ExpiresAt:  now.Add(m.itemExpiry),
	}

	var accepted bool
	if settings.ManualApproval {
		accepted = m.queue.EnqueuePending(item)
	} else {
		accepted = m.queue.Enqueue(item)
	}
	if !accepted {
		return
	}

	m.cooldowns.Reserve(hasKey, key, now,
		time.Duration(settings.PerUserCooldownSeconds)*time.Second,
		time.Duration(settings.GlobalCooldownSeconds)*time.Second,
	)
}

func cooldownKeyForEvent(evt engagement.Event) (CooldownKey, bool) {
	if evt.User == nil || evt.User.Anonymous {
		return CooldownKey{}, false
	}
	return NewCooldownKey(string(evt.ProviderID), evt.ConnectedAccountID, evt.User.ProviderUserID)
}

func (m *Manager) isSelf(evt engagement.Event) bool {
	if m.selfLookup == nil || evt.User == nil {
		return false
	}
	selfID, ok := m.selfLookup(evt.ConnectedAccountID)
	return ok && selfID != "" && evt.User.ProviderUserID == selfID
}

func (m *Manager) isBot(evt engagement.Event) bool {
	if evt.User == nil || evt.User.ProviderUserID == "" {
		return false
	}
	m.botMu.RLock()
	defer m.botMu.RUnlock()
	_, isBot := m.botUsers[botKey(string(evt.ProviderID), evt.ConnectedAccountID, evt.User.ProviderUserID)]
	return isBot
}

// isCommandMessage mirrors internal/chatoverlay/filtering.go's own
// exact one-line idiom (docs/audio-tts.md §10.3) - never
// internal/chatautomation's private, dispatcher-coupled command parser.
func isCommandMessage(text string) bool {
	return strings.HasPrefix(strings.TrimSpace(text), "!")
}

func containsType(list []engagement.Type, t engagement.Type) bool {
	for _, v := range list {
		if v == t {
			return true
		}
	}
	return false
}

func containsProviderID(list []engagement.ProviderID, p engagement.ProviderID) bool {
	for _, v := range list {
		if v == p {
			return true
		}
	}
	return false
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// TestSpeak enters the synthetic Test Speak path (docs/audio-tts.md
// §37/governing task §37): goes through the same preprocessing and
// real bounded queue as a genuine event, but never touches cooldown
// state, never publishes a fake Engagement Event, and always bypasses
// manual approval (structurally marked synthetic, so the operator can
// verify the pipeline immediately).
func (m *Manager) TestSpeak(text string) (Item, error) {
	m.mu.Lock()
	settings := m.settings
	m.mu.Unlock()

	if !settings.Enabled {
		return Item{}, ErrDisabled
	}

	processed, ok := Preprocess(text, PreprocessConfig{
		RemoveURLs:             settings.RemoveURLs,
		BlockedWords:           settings.BlockedWords,
		NormalizeRepeatedChars: settings.NormalizeRepeatedChars,
		MaxLengthCodePoints:    settings.MaxTextLengthCodePoints,
	})
	if !ok {
		return Item{}, ErrEmptyText
	}

	id, err := NewItemID()
	if err != nil {
		return Item{}, err
	}
	now := m.now()
	item := Item{
		ID:        id,
		Text:      processed,
		Synthetic: true,
		Snapshot: ItemSnapshot{
			ProviderMode: settings.ProviderMode,
			VoiceID:      settings.VoiceID,
			Language:     settings.Language,
			Speed:        settings.Speed,
			Volume:       settings.Volume,
		},
		EnqueuedAt: now,
		ExpiresAt:  now.Add(m.itemExpiry),
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.queue.Enqueue(item) {
		return Item{}, ErrQueueFull
	}
	return item, nil
}

// SkipCurrent cancels any in-flight synthesis and discards the current
// item (if any), counted as a manual skip - the poll loop then promotes
// the next eligible ready item on its own next tick.
func (m *Manager) SkipCurrent() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current == nil {
		return false
	}
	if m.currentSynthCancel != nil {
		m.currentSynthCancel()
	}
	m.queue.RecordManualSkip()
	m.current = nil
	m.changes.notify()
	return true
}

// ClearQueue empties the ready and pending-approval queue - never
// touches the current item (mirrors internal/alerts.Manager.ClearQueue's
// own identical "current is separate" contract).
func (m *Manager) ClearQueue() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.queue.Clear()
}

// Approve moves a pending item into the ready queue.
func (m *Manager) Approve(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.queue.Approve(id)
	return ok
}

// Reject discards a pending item - never a provider side effect.
func (m *Manager) Reject(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.queue.Reject(id)
	return ok
}

// ConnectRenderer establishes a new, high-entropy renderer session,
// immediately superseding any previous one (docs/audio-tts.md §15) - a
// stale session's future Ack calls are rejected because their token no
// longer matches.
func (m *Manager) ConnectRenderer() (string, error) {
	token, err := newSessionToken()
	if err != nil {
		return "", err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rendererSession = &rendererSession{token: token}
	if m.current != nil && m.current.playbackState == playbackStateWaitingForRenderer {
		m.current.playbackState = playbackStateReady
	}
	m.changes.notify()
	return token, nil
}

// DisconnectRenderer releases token's session, if it is still the
// active one (a disconnect from an already-superseded session is
// ignored outright - never clobbers a newer renderer). If the current
// item was genuinely playing, it is marked interrupted and discarded,
// never silently assumed complete and never auto-replayed
// (docs/audio-tts.md §16); if it had not started playing yet, it is
// kept, waiting for the next renderer to connect.
func (m *Manager) DisconnectRenderer(token string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.rendererSession == nil || m.rendererSession.token != token {
		return
	}
	m.rendererSession = nil
	defer m.changes.notify()
	if m.current == nil {
		return
	}
	if m.current.playbackState == playbackStatePlaying {
		m.totalInterrupted++
		m.current = nil
		return
	}
	m.current.playbackState = playbackStateWaitingForRenderer
}

// Ack validates and applies one renderer playback acknowledgement -
// rejected outright (ErrAckRejected, never mutating any state) unless
// token is the exact active renderer session AND itemID is the exact
// current item AND the state transition is valid for kind - proven by
// dedicated tests including deliberately stale/duplicated/wrong-session
// ACKs (docs/audio-tts.md §17).
func (m *Manager) Ack(token, itemID string, kind AckKind) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.rendererSession == nil || m.rendererSession.token != token {
		return ErrAckRejected
	}
	if m.current == nil || m.current.item.ID != itemID {
		return ErrAckRejected
	}

	switch kind {
	case AckStarted:
		if m.current.playbackState != playbackStateReady {
			return ErrAckRejected
		}
		m.current.playbackState = playbackStatePlaying
	case AckEnded:
		if m.current.playbackState != playbackStatePlaying {
			return ErrAckRejected
		}
		m.totalPlayed++
		m.current = nil
	case AckFailed:
		if m.current.playbackState != playbackStatePlaying && m.current.playbackState != playbackStateReady {
			return ErrAckRejected
		}
		m.totalPlaybackFailed++
		m.current = nil
	default:
		return ErrAckRejected
	}
	m.changes.notify()
	return nil
}

// PublicCurrentSnapshot returns the safe public summary of the current
// item, or Idle==true when there is none ready to serve yet (still
// synthesizing, waiting for a renderer, or genuinely empty).
func (m *Manager) PublicCurrentSnapshot() PublicCurrent {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current == nil || m.current.bytesToken == "" {
		return PublicCurrent{Idle: true}
	}
	return PublicCurrent{
		HasItem:     true,
		ItemID:      m.current.item.ID,
		BytesToken:  m.current.bytesToken,
		ContentType: m.current.audio.ContentType,
		Volume:      m.current.item.Snapshot.Volume,
		Sequence:    m.current.sequence,
	}
}

// CurrentAudioBytes returns the current item's already-generated audio
// bytes, only when both itemID and bytesToken exactly match the live
// current item - never serves stale or foreign audio.
func (m *Manager) CurrentAudioBytes(itemID, bytesToken string) (audioBytes []byte, contentType string, ok bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current == nil || m.current.bytesToken == "" {
		return nil, "", false
	}
	if m.current.item.ID != itemID || m.current.bytesToken != bytesToken {
		return nil, "", false
	}
	return m.current.audio.Audio, m.current.audio.ContentType, true
}

// Status returns the bounded, safe-to-expose management summary.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()

	providerAvailable := false
	if m.provider != nil {
		providerAvailable = m.provider.Capabilities().Available
	}

	return Status{
		Enabled:              m.settings.Enabled,
		ProviderMode:         m.settings.ProviderMode,
		ProviderAvailable:    providerAvailable,
		RendererConnected:    m.rendererSession != nil,
		HasCurrentItem:       m.current != nil,
		CurrentSynthetic:     m.current != nil && m.current.item.Synthetic,
		PendingApprovalCount: m.queue.PendingLen(),
		ReadyQueueCount:      m.queue.ReadyLen(),
		Capacity:             m.settings.QueueCapacity,
		Counters:             m.queue.Counters(),
		TotalPlayed:          m.totalPlayed,
		TotalPlaybackFailed:  m.totalPlaybackFailed,
		TotalSynthesisFailed: m.totalSynthesisFailed,
		TotalInterrupted:     m.totalInterrupted,
		InputGap:             m.inputGap.Load(),
		Subscribed:           m.subscribed.Load(),
	}
}

// PendingList returns a bounded snapshot of every currently pending
// item, oldest first.
func (m *Manager) PendingList() []Item {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.queue.PendingList()
}

func newSessionToken() (string, error) {
	buf := make([]byte, 20)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
