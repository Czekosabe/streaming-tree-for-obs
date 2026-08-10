package alerts

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	domain "github.com/streaming-tree/server/internal/domain/alerts"
	engagement "github.com/streaming-tree/server/internal/domain/engagement"
	bus "github.com/streaming-tree/server/internal/engagement"
)

// alertsPollInterval mirrors internal/chatautomation's own
// schedulerPollInterval reasoning exactly: a real-time poll loop (never
// one timer/goroutine per queued alert - Part 16) so a fake clock's
// Advance() in tests is picked up promptly rather than being invisible
// to an already-scheduled real time.Timer.
const alertsPollInterval = 20 * time.Millisecond

// resubscribeBackoff bounds how quickly the shared Event Bus
// subscription retries after a failed Subscribe call.
const resubscribeBackoff = time.Second

func newInstanceID() (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate alert instance id: %w", err)
	}
	return "alinst_" + hex.EncodeToString(buf), nil
}

// ManagerOptions constructs a Manager.
type ManagerOptions struct {
	DomainService *domain.Service
	Bus           *bus.Bus
	// Now is a test-only fake-clock override; production code leaves it
	// nil.
	Now clock
}

// Manager is the Stage 12A alert runtime: the CRUD façade over
// internal/domain/alerts (triggering a per-profile runtime reload after
// every write, exactly like internal/chatautomation.Manager does for
// its own domain package), one profileRuntime per alert profile, and
// the ONE shared Engagement Event Bus subscription every profile's
// matcher reads from - never a second EventSub connection, never a
// direct call into internal/provider/twitch anywhere in this package.
type Manager struct {
	domainSvc *domain.Service
	source    *bus.Bus
	now       clock

	mu       sync.Mutex
	profiles map[string]*profileRuntime

	inputGap   atomic.Bool
	subscribed atomic.Bool

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewManager builds a Manager. Call Start to load persisted profiles/
// rules and begin the poll loop/subscription; call Shutdown to stop
// both cleanly.
func NewManager(opts ManagerOptions) *Manager {
	now := opts.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		domainSvc: opts.DomainService, source: opts.Bus, now: now,
		profiles: make(map[string]*profileRuntime), ctx: ctx, cancel: cancel,
	}
}

// Start loads every persisted profile and its rules, then begins the
// poll loop and the shared Event Bus subscription.
func (m *Manager) Start(ctx context.Context) error {
	profiles, err := m.domainSvc.ListProfiles(ctx)
	if err != nil {
		return err
	}
	m.mu.Lock()
	for _, p := range profiles {
		m.profiles[p.ID] = newProfileRuntime(p.ID, p, newInstanceID)
	}
	m.mu.Unlock()
	for _, p := range profiles {
		rules, err := m.domainSvc.ListRules(ctx, p.ID)
		if err != nil {
			return err
		}
		if pr, ok := m.getRuntime(p.ID); ok {
			pr.setRules(rules)
		}
	}

	m.wg.Add(2)
	go m.runLoop()
	go m.runSubscription()
	return nil
}

// Shutdown stops the poll loop, the shared subscription, and every
// profile's own projection, waiting for the poll/subscription
// goroutines to exit, bounded by ctx.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.cancel()

	m.mu.Lock()
	for _, pr := range m.profiles {
		pr.shutdown()
	}
	m.mu.Unlock()

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

func (m *Manager) runLoop() {
	defer m.wg.Done()
	ticker := time.NewTicker(alertsPollInterval)
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
	for _, pr := range m.runtimesSnapshot() {
		pr.tick(now)
	}
}

func (m *Manager) runtimesSnapshot() []*profileRuntime {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*profileRuntime, 0, len(m.profiles))
	for _, pr := range m.profiles {
		out = append(out, pr)
	}
	return out
}

func (m *Manager) getRuntime(profileID string) (*profileRuntime, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	pr, ok := m.profiles[profileID]
	return pr, ok
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

// Subscribed reports whether the shared Event Bus subscription is
// currently live - exposed primarily so tests can wait deterministically
// for Start's own subscription goroutine to actually be subscribed
// before publishing an event, rather than racing it (Subscribe's own
// "resume from current position" semantics mean an event published in
// the narrow window before the first successful Subscribe call would
// otherwise be silently missed, not replayed - by design for a
// reconnect, but worth synchronizing on explicitly in a test).
func (m *Manager) Subscribed() bool { return m.subscribed.Load() }

// consume reads sub until it closes for any reason - a drop reconnects
// in runSubscription's own loop using the bus's CURRENT position, never
// replaying historical events as fresh alerts (Part 31, mirroring
// internal/chatautomation's own identical safety rule).
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

// handleEvent matches evt against every profile's own current rule set
// (Part 32's ordering: synthetic events are rejected inside MatchEvent
// itself) and enqueues every resulting Instance into its owning
// profile's queue.
func (m *Manager) handleEvent(evt engagement.Event) {
	now := m.now()
	for _, pr := range m.runtimesSnapshot() {
		rules := pr.rulesSnapshot()
		lang := pr.languageSnapshot()
		matches := MatchEvent(evt, rules, now, lang)
		if len(matches) > 0 {
			pr.enqueueMatched(matches, now, newInstanceID)
		}
	}
}

// --- profile CRUD façade ---------------------------------------------------

func (m *Manager) CreateProfile(ctx context.Context, name string) (domain.Profile, error) {
	p, err := m.domainSvc.CreateProfile(ctx, name)
	if err != nil {
		return domain.Profile{}, err
	}
	m.reloadProfileLocked(p)
	return p, nil
}

func (m *Manager) GetProfile(ctx context.Context, id string) (domain.Profile, error) {
	return m.domainSvc.GetProfile(ctx, id)
}

func (m *Manager) GetProfileByPublicSlug(ctx context.Context, slug string) (domain.Profile, error) {
	return m.domainSvc.GetProfileByPublicSlug(ctx, slug)
}

func (m *Manager) ListProfiles(ctx context.Context) ([]domain.Profile, error) {
	return m.domainSvc.ListProfiles(ctx)
}

func (m *Manager) ReplaceProfile(ctx context.Context, id string, in domain.ProfileInput) (domain.Profile, error) {
	p, err := m.domainSvc.ReplaceProfile(ctx, id, in)
	if err != nil {
		return domain.Profile{}, err
	}
	m.reloadProfileLocked(p)
	return p, nil
}

func (m *Manager) RotatePublicSlug(ctx context.Context, id string) (domain.Profile, error) {
	return m.domainSvc.RotatePublicSlug(ctx, id)
}

// DeleteProfile removes a profile's persisted definition and stops its
// runtime immediately - closes every live SSE subscriber, invalidating
// the public slug (Part 34).
func (m *Manager) DeleteProfile(ctx context.Context, id string) error {
	if err := m.domainSvc.DeleteProfile(ctx, id); err != nil {
		return err
	}
	m.mu.Lock()
	pr, ok := m.profiles[id]
	delete(m.profiles, id)
	m.mu.Unlock()
	if ok {
		pr.shutdown()
	}
	return nil
}

// reloadProfileLocked ensures a profileRuntime exists for p and syncs
// its cached settings - called after every profile write.
func (m *Manager) reloadProfileLocked(p domain.Profile) {
	m.mu.Lock()
	pr, ok := m.profiles[p.ID]
	if !ok {
		pr = newProfileRuntime(p.ID, p, newInstanceID)
		m.profiles[p.ID] = pr
	}
	m.mu.Unlock()
	pr.applyProfile(p)
}

// --- rule CRUD façade -------------------------------------------------------

func (m *Manager) CreateRule(ctx context.Context, profileID string, in domain.RuleInput) (domain.Rule, error) {
	r, err := m.domainSvc.CreateRule(ctx, profileID, in)
	if err != nil {
		return domain.Rule{}, err
	}
	m.reloadRules(ctx, profileID)
	return r, nil
}

func (m *Manager) GetRule(ctx context.Context, id string) (domain.Rule, error) {
	return m.domainSvc.GetRule(ctx, id)
}

func (m *Manager) ListRules(ctx context.Context, profileID string) ([]domain.Rule, error) {
	return m.domainSvc.ListRules(ctx, profileID)
}

func (m *Manager) ReplaceRule(ctx context.Context, id string, in domain.RuleInput) (domain.Rule, error) {
	existing, err := m.domainSvc.GetRule(ctx, id)
	if err != nil {
		return domain.Rule{}, err
	}
	r, err := m.domainSvc.ReplaceRule(ctx, id, in)
	if err != nil {
		return domain.Rule{}, err
	}
	m.reloadRules(ctx, existing.ProfileID)
	return r, nil
}

func (m *Manager) DeleteRule(ctx context.Context, id string) error {
	existing, err := m.domainSvc.GetRule(ctx, id)
	if err != nil {
		return err
	}
	if err := m.domainSvc.DeleteRule(ctx, id); err != nil {
		return err
	}
	m.reloadRules(ctx, existing.ProfileID)
	return nil
}

func (m *Manager) reloadRules(ctx context.Context, profileID string) {
	pr, ok := m.getRuntime(profileID)
	if !ok {
		return
	}
	if rules, err := m.domainSvc.ListRules(ctx, profileID); err == nil {
		pr.setRules(rules)
	}
}

func (m *Manager) OverlapWarnings(ctx context.Context, profileID string) ([]domain.OverlapWarning, error) {
	return m.domainSvc.OverlapWarnings(ctx, profileID)
}

// --- queue commands ---------------------------------------------------

func (m *Manager) Pause(profileID string) error {
	pr, ok := m.getRuntime(profileID)
	if !ok {
		return ErrProfileNotFound
	}
	pr.pause()
	return nil
}

func (m *Manager) Resume(profileID string) error {
	pr, ok := m.getRuntime(profileID)
	if !ok {
		return ErrProfileNotFound
	}
	pr.resume()
	return nil
}

func (m *Manager) SkipCurrent(profileID string) error {
	pr, ok := m.getRuntime(profileID)
	if !ok {
		return ErrProfileNotFound
	}
	if !pr.skipCurrent(m.now()) {
		return ErrQueueEmpty
	}
	return nil
}

func (m *Manager) ReplayPrevious(profileID string) error {
	pr, ok := m.getRuntime(profileID)
	if !ok {
		return ErrProfileNotFound
	}
	return pr.replayPrevious()
}

func (m *Manager) ClearQueue(profileID string) (int, error) {
	pr, ok := m.getRuntime(profileID)
	if !ok {
		return 0, ErrProfileNotFound
	}
	return pr.clearQueue(), nil
}

// --- status -------------------------------------------------------------

func (m *Manager) ProfileStatus(profileID string) (ProfileStatus, error) {
	pr, ok := m.getRuntime(profileID)
	if !ok {
		return ProfileStatus{}, ErrProfileNotFound
	}
	st := pr.status()
	st.InputGap = m.inputGap.Load()
	return st, nil
}

// --- public playback subscription (Part 23) ------------------------------

// SubscribeProfile registers a live SSE subscriber for profileID's own
// playback stream - see projection.go for the replay/gap contract.
func (m *Manager) SubscribeProfile(profileID string, after uint64) (*Subscription, bool, error) {
	pr, ok := m.getRuntime(profileID)
	if !ok {
		return nil, false, ErrProfileNotFound
	}
	return pr.subscribe(after)
}

// CurrentReset returns the complete current-state reset a fresh SSE
// connection receives first (Part 23).
func (m *Manager) CurrentReset(profileID string) (Revision, error) {
	pr, ok := m.getRuntime(profileID)
	if !ok {
		return Revision{}, ErrProfileNotFound
	}
	return pr.currentResetRevision(), nil
}

// LatestSequence returns profileID's own latest published revision
// sequence (0 if none yet or the profile is unknown) - used to give a
// fresh connection's synthetic reset a meaningful Last-Event-ID.
func (m *Manager) LatestSequence(profileID string) uint64 {
	pr, ok := m.getRuntime(profileID)
	if !ok {
		return 0
	}
	return pr.latestSequence()
}

// --- test alerts (Part 27/28) -------------------------------------------

// TestRule creates one synthetic alert instance using rule's own real
// presentation, regardless of its provider/account thresholds -
// requires no real Twitch account or Event Bus event, and goes through
// the exact same queue/playback/public-route path a real match would
// (Part 27: "test alerts must go through the real alert queue/playback
// renderer path"). edgeScenario may be empty for the plain
// representative fixture of rule's own event type.
func (m *Manager) TestRule(ctx context.Context, ruleID, edgeScenario string) (AlertSummary, error) {
	rule, err := m.domainSvc.GetRule(ctx, ruleID)
	if err != nil {
		return AlertSummary{}, err
	}
	profile, err := m.domainSvc.GetProfile(ctx, rule.ProfileID)
	if err != nil {
		return AlertSummary{}, err
	}
	if !profile.Enabled {
		return AlertSummary{}, ErrProfileDisabled
	}
	pr, ok := m.getRuntime(rule.ProfileID)
	if !ok {
		return AlertSummary{}, ErrProfileNotFound
	}

	now := m.now()
	inst := BuildTestInstance(rule, edgeScenario, now, profile.Language)

	stored, accepted := pr.enqueueTest(inst, now, newInstanceID)
	if !accepted {
		return AlertSummary{}, ErrQueueFull
	}
	return summarize(stored), nil
}
