package alerts

import (
	"sync"
	"time"

	domain "github.com/streaming-tree/server/internal/domain/alerts"
	"github.com/streaming-tree/server/internal/domain/visualdesign"
)

// AlertSummary is one management-only, bounded view of an Instance -
// unlike PublicAlert it may carry every user-facing field, since the
// management API is private (Part 30: "Management safe summaries may
// contain user-facing alert content because this is the private
// operator API") - but still never a raw provider payload or token.
type AlertSummary struct {
	AlertID       string
	RuleID        string
	EventType     domain.EventType
	QueuedAt      time.Time
	Priority      int
	Username      string
	Message       string
	Quantity      *int64
	RenderedText  string
	Synthetic     bool
	Replayed      bool
	GroupCount    int
	Interruptible bool
}

func summarize(inst Instance) AlertSummary {
	return AlertSummary{
		AlertID: inst.ID, RuleID: inst.RuleID, EventType: inst.EventType, QueuedAt: inst.QueuedAt,
		Priority: inst.Priority, Username: inst.Username, Message: inst.Message, Quantity: inst.Quantity,
		RenderedText: inst.RenderedText, Synthetic: inst.Synthetic, Replayed: inst.Replayed,
		GroupCount: inst.GroupCount, Interruptible: inst.Interruptible,
	}
}

func toPublicAlert(inst Instance) *PublicAlert {
	pa := &PublicAlert{
		SchemaVersion: 1, AlertID: inst.ID, EventType: string(inst.EventType), ProviderID: string(inst.ProviderID),
		Synthetic: inst.Synthetic, Replayed: inst.Replayed, RenderedText: inst.RenderedText,
		DurationMS: inst.DurationMS, EntryAnimation: string(inst.EntryAnimation), ExitAnimation: string(inst.ExitAnimation),
		AnimationDurationMS: inst.AnimationDurationMS, GroupCount: inst.GroupCount,
		RenderingMode: RenderingLegacy,
	}
	if inst.VisualDesign != nil {
		pa.RenderingMode = RenderingVisualDesign
		pa.VisualDesign = inst.VisualDesign
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

	// TotalGroupedMembers/TotalGroupsCreated: Stage 12B grouping
	// counters (task Part 15). TotalGroupedMembers increments once per
	// event merged into an existing group (never for the group's own
	// first, founding member). TotalGroupsCreated increments exactly
	// once per queued item, the moment its GroupCount first grows from 1
	// to 2 - never again for that same item afterward.
	TotalGroupedMembers int64
	TotalGroupsCreated  int64
	// TotalPreempted: Stage 12B task Part 18 - a currently-playing alert
	// replaced by a strictly-higher-priority incoming one before its own
	// duration finished. Never counted toward TotalPlayed or
	// TotalManuallySkipped.
	TotalPreempted int64

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
	// designs is Stage 13A's own rule-id -> saved-visual-design cache,
	// refreshed by the Manager alongside rules (Start, every rule CRUD
	// call) and additionally on every visual-design save/delete - see
	// internal/alerts.Manager's own design CRUD façade. A rule id with
	// no entry (or a nil value) has no saved design; buildInstance
	// copies whatever is here at match time, never re-reading it later
	// (Part 22).
	designs map[string]*visualdesign.PublicDocument
	maxAge  time.Duration

	queue           *queue
	current         *Instance
	currentDeadline time.Time
	paused          bool

	pendingReplay *Instance
	lastCompleted *Instance // the one replay-eligible slot (Part 19: "at most one safe replay snapshot")

	totalEnqueued, totalPlayed, totalExpired, totalDropped, totalSkipped, totalSynthetic int64
	totalGroupedMembers, totalGroupsCreated, totalPreempted                              int64
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

// setDesigns replaces the entire rule-id -> visual-design cache -
// called by the Manager whenever rules are reloaded (a rule's own
// design is unaffected by a rule edit, but designs are cheap enough to
// recompute in full alongside rules) and whenever a single rule's
// design is saved/deleted (setRuleDesign below, the narrower update).
func (pr *profileRuntime) setDesigns(designs map[string]*visualdesign.PublicDocument) {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	pr.designs = designs
}

// setRuleDesign updates exactly one rule's cached design snapshot -
// used after a visual-design Save/Delete, which never touches the
// rule's own row (so a full setDesigns recompute is unnecessary
// overhead, though also correct).
func (pr *profileRuntime) setRuleDesign(ruleID string, design *visualdesign.PublicDocument) {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	if pr.designs == nil {
		pr.designs = make(map[string]*visualdesign.PublicDocument)
	}
	if design == nil {
		delete(pr.designs, ruleID)
		return
	}
	pr.designs[ruleID] = design
}

func (pr *profileRuntime) designsSnapshot() map[string]*visualdesign.PublicDocument {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	out := make(map[string]*visualdesign.PublicDocument, len(pr.designs))
	for k, v := range pr.designs {
		out[k] = v
	}
	return out
}

// designForRule returns ruleID's own cached visual-design snapshot, or
// nil if it has none saved.
func (pr *profileRuntime) designForRule(ruleID string) *visualdesign.PublicDocument {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	return pr.designs[ruleID]
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
		hiddenID := pr.current.ID
		pr.current = nil
		pr.proj.publishHide(hiddenID, HideReasonProfileDisabled, pr.paused)
	}
}

// enqueueMatched processes every instance MatchEvent produced for one
// live, non-synthetic event, in the Stage 12B task's own Part 26
// deterministic order:
//
//  1. sort candidates (Part 27: priority desc, then rule id asc - never
//     MatchEvent's own return order, which itself only reflects the
//     caller's rule-snapshot slice order),
//  2. at most one candidate may immediately preempt the current alert in
//     this turn - the first (highest-priority) eligible one, so an
//     urgent alert is never buried by a stale queued group or a lower-
//     priority candidate processed first,
//  3. every other candidate is offered to grouping before falling back
//     to the normal capacity-policy enqueue.
func (pr *profileRuntime) enqueueMatched(instances []Instance, now time.Time, newID func() (string, error)) {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	if !pr.enabled {
		return
	}
	sorted := sortCandidates(instances)
	preempted := false
	for _, inst := range sorted {
		if !preempted && pr.canPreemptLocked(inst) {
			pr.preemptCurrentLocked(inst, now, newID)
			preempted = true
			continue
		}
		if pr.tryGroupLocked(inst, now) {
			continue
		}
		_, _ = pr.enqueueLocked(inst, now, newID)
	}
}

// sortCandidates orders instances deterministically - descending
// priority, then ascending rule id as a stable tiebreaker (Stage 12B
// task Part 27) - never SQLite row order, Go map iteration, or
// MatchEvent's own return order. A new slice; instances itself is never
// mutated.
func sortCandidates(instances []Instance) []Instance {
	sorted := make([]Instance, len(instances))
	copy(sorted, instances)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0; j-- {
			a, b := sorted[j-1], sorted[j]
			less := b.Priority > a.Priority || (b.Priority == a.Priority && b.RuleID < a.RuleID)
			if !less {
				break
			}
			sorted[j-1], sorted[j] = sorted[j], sorted[j-1]
		}
	}
	return sorted
}

// enqueueLocked assigns inst a fresh id and offers it to the queue's own
// capacity policy, returning the (possibly id-assigned) instance and
// whether it was accepted - the caller-visible id matters for TestRule's
// own response (Part 27), which must report the same id the queue itself
// now holds.
func (pr *profileRuntime) enqueueLocked(inst Instance, now time.Time, newID func() (string, error)) (Instance, bool) {
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
	return inst, accepted
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
		pr.completeCurrentLocked(now, HideReasonCompleted)
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

// completeCurrentLocked ends the current alert for reason. In every case
// the completed instance becomes the one replay-eligible snapshot (Part
// 19: a naturally completed, manually skipped, or preempted alert may
// all be replayed) - never requeued, never resumed later.
func (pr *profileRuntime) completeCurrentLocked(now time.Time, reason HideReason) {
	if pr.current == nil {
		return
	}
	completed := *pr.current
	pr.current = nil
	pr.lastCompleted = &completed
	switch reason {
	case HideReasonSkipped:
		pr.totalSkipped++
		pr.lastSkipReason = SkipManual
	case HideReasonPreempted:
		pr.totalPreempted++
		pr.lastSkipReason = SkipPreempted
	default:
		pr.totalPlayed++
	}
	pr.proj.publishHide(completed.ID, reason, pr.paused)
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
	pr.completeCurrentLocked(now, HideReasonSkipped)
	if !pr.paused {
		pr.promoteNextLocked(now)
	}
	return true
}

// canPreemptLocked reports whether inst (a newly matched candidate) may
// immediately replace the current alert (Stage 12B task Part 17/25): the
// queue must not be paused, a current alert must exist, inst must never
// be a replay (Part 24: "the replayed alert itself must not be allowed
// to preempt another alert"), a synthetic inst may only ever preempt a
// synthetic current - never a real one (Part 25: "synthetic test alerts
// never preempt real alerts," while "synthetic tests may preempt other
// synthetic tests... allows safe UI verification," and a real inst may
// preempt either a real or a synthetic current), inst's own rule
// snapshot must explicitly opt in to interrupting, the current alert's
// own rule snapshot must not be protected from interruption, and inst's
// priority must be strictly greater - equal or lower priority never
// interrupts.
func (pr *profileRuntime) canPreemptLocked(inst Instance) bool {
	if pr.paused || pr.current == nil {
		return false
	}
	if inst.Replayed {
		return false
	}
	if inst.Synthetic && !pr.current.Synthetic {
		return false
	}
	if inst.InterruptMode != domain.InterruptLowerPriority {
		return false
	}
	if !pr.current.Interruptible {
		return false
	}
	return inst.Priority > pr.current.Priority
}

// preemptCurrentLocked implements the Stage 12B task's own Part 18
// deterministic no-resume semantics: the current alert is hidden with
// reason "preempted" (cancelling its own duration timer implicitly,
// since pr.current/pr.currentDeadline are the only state tick() ever
// reads - there is no separate per-alert timer object to cancel, so a
// stale callback can never fire against the wrong instance), becomes the
// one safe replay snapshot, and inst is promoted to current immediately
// with its own fresh duration - never a resumed remainder of the
// interrupted alert's own duration.
func (pr *profileRuntime) preemptCurrentLocked(inst Instance, now time.Time, newID func() (string, error)) {
	pr.completeCurrentLocked(now, HideReasonPreempted)
	id, err := newID()
	if err == nil {
		inst.ID = id
	}
	pr.startCurrentLocked(inst, now)
}

// tryGroupLocked offers inst to grouping (Stage 12B task Part 4-6):
// merges it into a compatible, still-queued, in-window, not-yet-full
// group when one exists, and reports true if it did (the caller must
// then never also enqueue inst separately). A candidate that is not
// itself grouping-eligible, or for which no compatible group is found,
// is left for the caller's own normal enqueue path.
func (pr *profileRuntime) tryGroupLocked(inst Instance, now time.Time) bool {
	if !groupingEligible(inst) {
		return false
	}
	idx := pr.queue.findGroupable(groupKeyFor(inst), now)
	if idx < 0 {
		return false
	}
	member := &pr.queue.items[idx].instance
	wasSingle := member.GroupCount == 1
	mergeGroupMember(member, inst)
	pr.totalGroupedMembers++
	if wasSingle && member.GroupCount > 1 {
		pr.totalGroupsCreated++
	}
	return true
}

// enqueueTest is TestRule's own entry point (Stage 12B task Part 32): a
// synthetic candidate never groups (groupingEligible excludes every
// synthetic instance unconditionally) but may preempt a currently
// SYNTHETIC alert exactly like a real candidate can - canPreemptLocked's
// own synthetic-vs-real guard already ensures it can never preempt a
// real one. Returns the accepted instance (with its final id) and
// whether it was accepted at all.
func (pr *profileRuntime) enqueueTest(inst Instance, now time.Time, newID func() (string, error)) (Instance, bool) {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	if pr.canPreemptLocked(inst) {
		pr.preemptCurrentLocked(inst, now, newID)
		return *pr.current, true
	}
	return pr.enqueueLocked(inst, now, newID)
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
		TotalGroupedMembers: pr.totalGroupedMembers, TotalGroupsCreated: pr.totalGroupsCreated, TotalPreempted: pr.totalPreempted,
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
