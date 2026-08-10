package alerts

import (
	"sync"
	"time"

	domain "github.com/streaming-tree/server/internal/domain/alerts"
)

// AlertSummary is one management-only, bounded view of an Instance -
// unlike PublicAlert it may carry every user-facing field, since the
// management API is private (Part 30: "Management safe summaries may
// contain user-facing alert content because this is the private
// operator API") - but still never a raw provider payload or token.
type AlertSummary struct {
	AlertID      string
	RuleID       string
	EventType    domain.EventType
	QueuedAt     time.Time
	Priority     int
	Username     string
	Message      string
	Quantity     *int64
	RenderedText string
	Synthetic    bool
	Replayed     bool
}

func summarize(inst Instance) AlertSummary {
	return AlertSummary{
		AlertID: inst.ID, RuleID: inst.RuleID, EventType: inst.EventType, QueuedAt: inst.QueuedAt,
		Priority: inst.Priority, Username: inst.Username, Message: inst.Message, Quantity: inst.Quantity,
		RenderedText: inst.RenderedText, Synthetic: inst.Synthetic, Replayed: inst.Replayed,
	}
}

func toPublicAlert(inst Instance) *PublicAlert {
	pa := &PublicAlert{
		SchemaVersion: 1, AlertID: inst.ID, EventType: string(inst.EventType), ProviderID: string(inst.ProviderID),
		Synthetic: inst.Synthetic, Replayed: inst.Replayed, RenderedText: inst.RenderedText,
		DurationMS: inst.DurationMS, EntryAnimation: string(inst.EntryAnimation), ExitAnimation: string(inst.ExitAnimation),
		AnimationDurationMS: inst.AnimationDurationMS,
	}
	if inst.Username != "" {
		u := inst.Username
		pa.Username = &u
	}
	if inst.Message != "" {
		m := inst.Message
		pa.Message = &m
	}
	if inst.Quantity != nil {
		q := *inst.Quantity
		pa.Quantity = &q
	}
	return pa
}

// bounded, per-Stage-12A-task defaults/limits for a profile's own
// managed queue - see domain/alerts/validation.go for the same numbers
// applied to a persisted profile's own settings; this package clamps
// only defensively (a profile is already validated before it reaches
// here).
const maxQueuedSummaries = 20

// ProfileStatus is the bounded management snapshot for one profile
// (Part 30).
type ProfileStatus struct {
	ProfileID     string
	Enabled       bool
	Paused        bool
	Current       *AlertSummary
	QueuedCount   int
	QueueCapacity int
	NextQueued    []AlertSummary

	TotalEnqueued        int64
	TotalPlayed          int64
	TotalExpired         int64
	TotalCapacityDropped int64
	TotalManuallySkipped int64
	TotalSynthetic       int64

	ReplayAvailable   bool
	ActiveSubscribers int
	LastAlertAt       *time.Time
	LastSkipReason    SkipReason
	InputGap          bool
}

// profileRuntime is one alert profile's own in-memory runtime: its
// bounded queue, currently-playing alert (if any), pause state, single
// replay slot, and counters - never persisted (Part 12). Concurrency-
// safe: every exported-from-this-file method takes pr.mu itself.
type profileRuntime struct {
	mu sync.Mutex

	profileID string
	enabled   bool
	language  domain.Language
	rules     []domain.Rule
	maxAge    time.Duration

	queue           *queue
	current         *Instance
	currentDeadline time.Time
	paused          bool

	pendingReplay *Instance
	lastCompleted *Instance // the one replay-eligible slot (Part 19: "at most one safe replay snapshot")

	totalEnqueued, totalPlayed, totalExpired, totalDropped, totalSkipped, totalSynthetic int64
	lastAlertAt                                                                          *time.Time
	lastSkipReason                                                                       SkipReason

	proj  *projection
	newID func() (string, error)
}

func newProfileRuntime(profileID string, p domain.Profile, newID func() (string, error)) *profileRuntime {
	return &profileRuntime{
		profileID: profileID, enabled: p.Enabled, language: p.Language,
		maxAge: time.Duration(p.MaximumQueueAgeSeconds) * time.Second,
		queue:  newQueue(p.MaxQueueItems, time.Duration(p.MaximumQueueAgeSeconds)*time.Second),
		proj:   newProjectionRuntime(DefaultRevisionCapacity),
		newID:  newID,
	}
}

// applyProfile updates this runtime's cached profile-level settings
// (enabled, queue bounds) without touching the queue's own contents,
// the current alert, or counters - Part 33's "target changes -> reset
// target-specific counters safely" equivalent for alerts is handled by
// the caller (Manager.reloadProfileLocked) deciding whether a full
// reset is warranted; applyProfile itself is the narrow settings sync.
func (pr *profileRuntime) applyProfile(p domain.Profile) {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	wasEnabled := pr.enabled
	pr.enabled = p.Enabled
	pr.language = p.Language
	pr.maxAge = time.Duration(p.MaximumQueueAgeSeconds) * time.Second
	pr.queue.maxItems = p.MaxQueueItems
	pr.queue.maxAge = pr.maxAge

	if wasEnabled && !p.Enabled {
		pr.disableLocked()
	}
	if !wasEnabled && p.Enabled {
		// Part 34: "re-enabling begins empty, does not replay events
		// that arrived while disabled" - queue is already empty from
		// disableLocked, nothing further to do.
	}
}

func (pr *profileRuntime) languageSnapshot() domain.Language {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	return pr.language
}

func (pr *profileRuntime) setRules(rules []domain.Rule) {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	pr.rules = rules
}

func (pr *profileRuntime) rulesSnapshot() []domain.Rule {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	out := make([]domain.Rule, len(pr.rules))
	copy(out, pr.rules)
	return out
}

// disableLocked implements Part 34: stop accepting new alerts, hide the
// current item immediately, clear the queue - runtime stays available
// for management, rules remain cached (so a re-enable is instant).
func (pr *profileRuntime) disableLocked() {
	pr.queue.clear()
	pr.pendingReplay = nil
	if pr.current != nil {
		pr.current = nil
		pr.proj.publish(OpHide, nil, pr.paused)
	}
}

// enqueueMatched enqueues every instance MatchEvent produced for a
// live, non-synthetic event - counts each as "enqueued," and each
// capacity-rejected instance as "dropped."
func (pr *profileRuntime) enqueueMatched(instances []Instance, now time.Time, newID func() (string, error)) {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	if !pr.enabled {
		return
	}
	for _, inst := range instances {
		pr.enqueueLocked(inst, now, newID)
	}
}

func (pr *profileRuntime) enqueueLocked(inst Instance, now time.Time, newID func() (string, error)) bool {
	id, err := newID()
	if err == nil {
		inst.ID = id
	}
	accepted, evicted := pr.queue.enqueue(inst, now)
	if accepted {
		pr.totalEnqueued++
		if inst.Synthetic {
			pr.totalSynthetic++
		}
		// An evicted item never gets shown either - it counts as
		// capacity-dropped exactly like an outright rejection (Part 14),
		// even though the new arrival itself was accepted.
		if evicted != nil {
			pr.totalDropped++
		}
	} else {
		pr.totalDropped++
	}
	return accepted
}

// tick advances this profile's playback state machine by one poll:
// complete an expired current alert, then (unless paused) promote the
// next eligible item - see docs/progress.md's Stage 12A queue/playback
// entry for the exact pause-policy rationale.
func (pr *profileRuntime) tick(now time.Time) {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	if !pr.enabled {
		return
	}
	if pr.current != nil && !now.Before(pr.currentDeadline) {
		pr.completeCurrentLocked(now, false, SkipNone)
	}
	if pr.paused || pr.current != nil {
		return
	}
	pr.promoteNextLocked(now)
}

func (pr *profileRuntime) promoteNextLocked(now time.Time) {
	if pr.pendingReplay != nil {
		inst := *pr.pendingReplay
		pr.pendingReplay = nil
		pr.startCurrentLocked(inst, now)
		return
	}
	inst, expiredCount, ok := pr.queue.popNextEligible(now)
	pr.totalExpired += int64(expiredCount)
	if !ok {
		return
	}
	pr.startCurrentLocked(inst, now)
}

func (pr *profileRuntime) startCurrentLocked(inst Instance, now time.Time) {
	pr.current = &inst
	pr.currentDeadline = now.Add(time.Duration(inst.DurationMS) * time.Millisecond)
	t := now
	pr.lastAlertAt = &t
	pr.proj.publish(OpShow, toPublicAlert(inst), pr.paused)
}

// completeCurrentLocked ends the current alert. manual=true is Skip
// Current; manual=false is a natural duration timeout. Either way the
// completed instance becomes the one replay-eligible snapshot (Part 19:
// both a completed and a skipped alert may be replayed).
func (pr *profileRuntime) completeCurrentLocked(now time.Time, manual bool, reason SkipReason) {
	if pr.current == nil {
		return
	}
	completed := *pr.current
	pr.current = nil
	pr.lastCompleted = &completed
	if manual {
		pr.totalSkipped++
		pr.lastSkipReason = reason
	} else {
		pr.totalPlayed++
	}
	pr.proj.publish(OpHide, nil, pr.paused)
}

// --- operator queue commands ---------------------------------------------

func (pr *profileRuntime) pause() {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	pr.paused = true
	pr.proj.publish(OpPaused, nil, true)
}

func (pr *profileRuntime) resume() {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	pr.paused = false
	pr.proj.publish(OpPaused, nil, false)
}

// skipCurrent removes the current alert immediately (Part 18) - never
// requeued, counted as manually skipped rather than played, and (unless
// paused) the queue advances to the next item on the very next tick.
func (pr *profileRuntime) skipCurrent(now time.Time) bool {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	if pr.current == nil {
		return false
	}
	pr.completeCurrentLocked(now, true, SkipManual)
	if !pr.paused {
		pr.promoteNextLocked(now)
	}
	return true
}

// replayPrevious re-shows the last completed/skipped alert (Part 19):
// an explicit front-of-queue replay, bypassing the normal
// priority/capacity queue entirely via pendingReplay, so it plays next
// regardless of what else is queued, without ever recreating an
// Engagement Event Bus event or displacing another queued item's own
// capacity slot.
func (pr *profileRuntime) replayPrevious() error {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	if pr.lastCompleted == nil {
		return ErrNoReplaySnapshot
	}
	replay := *pr.lastCompleted
	replay.Replayed = true
	pr.pendingReplay = &replay
	return nil
}

// clearQueue removes every queued (not-yet-played) item, including a
// pending replay slot - never the currently-playing alert (Part 20).
func (pr *profileRuntime) clearQueue() int {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	cleared := pr.queue.clear()
	if pr.pendingReplay != nil {
		cleared = append(cleared, *pr.pendingReplay)
		pr.pendingReplay = nil
	}
	return len(cleared)
}

func (pr *profileRuntime) status() ProfileStatus {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	st := ProfileStatus{
		ProfileID: pr.profileID, Enabled: pr.enabled, Paused: pr.paused,
		QueuedCount: pr.queue.len(), QueueCapacity: pr.queue.maxItems,
		TotalEnqueued: pr.totalEnqueued, TotalPlayed: pr.totalPlayed, TotalExpired: pr.totalExpired,
		TotalCapacityDropped: pr.totalDropped, TotalManuallySkipped: pr.totalSkipped, TotalSynthetic: pr.totalSynthetic,
		ReplayAvailable: pr.lastCompleted != nil, ActiveSubscribers: pr.proj.activeSubscribers(),
		LastAlertAt: pr.lastAlertAt, LastSkipReason: pr.lastSkipReason,
	}
	if pr.current != nil {
		s := summarize(*pr.current)
		st.Current = &s
	}
	for _, inst := range pr.queue.list(maxQueuedSummaries) {
		st.NextQueued = append(st.NextQueued, summarize(inst))
	}
	return st
}

// currentResetRevision builds the complete current-state reset every
// new SSE connection receives first (Part 23) - current alert if one
// exists, paused state, never historical queue content.
func (pr *profileRuntime) currentResetRevision() Revision {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	var alert *PublicAlert
	if pr.current != nil {
		alert = toPublicAlert(*pr.current)
	}
	return Revision{Operation: OpReset, Alert: alert, Paused: pr.paused}
}

// subscribe and latestSequence pass through to this profile's own
// projection - see projection.go for the SSE replay/gap contract.
func (pr *profileRuntime) subscribe(after uint64) (*Subscription, bool, error) {
	return pr.proj.Subscribe(after)
}

func (pr *profileRuntime) latestSequence() uint64 {
	return pr.proj.latestSequence()
}

func (pr *profileRuntime) shutdown() {
	pr.proj.shutdown()
}
