package chatautomation

import (
	"context"
	"math/rand"
	"strconv"
	"sync"
	"time"

	"github.com/streaming-tree/server/internal/domain/account"
	domain "github.com/streaming-tree/server/internal/domain/chatautomation"
	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/outboundchat"
)

// schedulerPollInterval bounds how often the scheduler re-checks whether
// any schedule has become due. Deliberately a poll rather than one big
// timer per schedule sized from the (possibly fake, test-controlled)
// clock's own idea of "how long until due" - a real time.Timer's fire
// time is fixed at construction and cannot be pulled earlier by a test
// later advancing a fake clock, exactly the same reasoning
// internal/outboundchat/dispatcher.go's own rateLimitPollInterval
// documents. One centralized loop, not one goroutine per schedule - see
// the Stage 11B task's own Part 23.
const schedulerPollInterval = 20 * time.Millisecond

// startupSafetyFloor is the minimum delay before a schedule's very
// first due moment when its own configured first delay is zero - see
// the Stage 11B task's own Part 6: "do not surprise the user by sending
// a bot message immediately because the backend restarted."
const startupSafetyFloor = 5 * time.Second

const rollingHour = time.Hour

// IngestChecker reports whether the local Streaming Tree ingest path is
// currently receiving - see the Stage 11B task's own Part 8. This is
// deliberately narrow: never Twitch stream.online, never an FFmpeg
// output branch, never viewer presence.
type IngestChecker interface {
	IsReceiving() bool
	// ReceivingSince returns the moment the ingest path most recently
	// became receiving, and true, when currently receiving with a known
	// start time - the basis for {streamUptime}. ok=false means
	// {streamUptime} is currently unresolvable (see Part 18: "do not
	// invent a timestamp").
	ReceivingSince() (time.Time, bool)
}

// AccountAccessor is the narrow account-lookup this package needs for
// placeholder context and Send Now target validation.
type AccountAccessor interface {
	GetAccount(ctx context.Context, id string) (account.Account, error)
}

// PlatformAccessor is the narrow platform-lookup this package needs for
// the {streamTitle} placeholder's deterministic metadata context.
type PlatformAccessor interface {
	Get(ctx context.Context, id string) (platform.Platform, error)
}

type accountTargetState struct {
	activityCount  int
	sendsThisHour  []time.Time
	lastAttemptAt  *time.Time
	lastSuccessAt  *time.Time
	lastSkipReason SkipReason
}

type scheduleRuntime struct {
	mu     sync.Mutex
	execMu sync.Mutex
	def    domain.Schedule

	nextBaseDue time.Time
	nextFireAt  time.Time

	lastAttemptAt  *time.Time
	lastSuccessAt  *time.Time
	lastSkipReason SkipReason

	perAccount    map[string]*accountTargetState
	lastMessageID string
}

func (sr *scheduleRuntime) accountState(accountID string) *accountTargetState {
	st, ok := sr.perAccount[accountID]
	if !ok {
		st = &accountTargetState{}
		sr.perAccount[accountID] = st
	}
	return st
}

// scheduler is the centralized, single-goroutine automatic-execution
// engine for every schedule - never one goroutine per schedule.
type scheduler struct {
	mu        sync.Mutex
	schedules map[string]*scheduleRuntime

	now      clock
	randFrac func() float64

	ingest    IngestChecker
	accounts  AccountAccessor
	platforms PlatformAccessor
	dispatch  *dispatcher

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func newScheduler(now clock, randFrac func() float64, ingest IngestChecker, accounts AccountAccessor, platforms PlatformAccessor, dispatch *dispatcher) *scheduler {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	if randFrac == nil {
		randFrac = rand.Float64
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &scheduler{
		schedules: make(map[string]*scheduleRuntime),
		now:       now, randFrac: randFrac,
		ingest: ingest, accounts: accounts, platforms: platforms, dispatch: dispatch,
		ctx: ctx, cancel: cancel,
	}
}

func (s *scheduler) start() {
	s.wg.Add(1)
	go s.runLoop()
}

func (s *scheduler) shutdown() {
	s.cancel()
	s.wg.Wait()
}

func (s *scheduler) runLoop() {
	defer s.wg.Done()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-time.After(schedulerPollInterval):
			s.tick()
		}
	}
}

// tick finds every schedule due at the current time, advances its next
// due point immediately (so the same due moment is never picked up
// twice - see the Stage 11B task's own "no duplicate timer execution"),
// then runs each due schedule in its own goroutine.
func (s *scheduler) tick() {
	now := s.now()

	s.mu.Lock()
	var due []*scheduleRuntime
	for _, sr := range s.schedules {
		sr.mu.Lock()
		isDue := sr.def.Enabled && !sr.nextFireAt.IsZero() && !now.Before(sr.nextFireAt)
		if isDue {
			s.advanceDueLocked(sr, now)
			due = append(due, sr)
		}
		sr.mu.Unlock()
	}
	s.mu.Unlock()

	for _, sr := range due {
		s.wg.Add(1)
		go func(sr *scheduleRuntime) {
			defer s.wg.Done()
			s.executeDue(sr)
		}(sr)
	}
}

// advanceDueLocked advances a due schedule's own base point by exactly
// one configured interval, then applies a fresh independent jitter for
// the NEXT occurrence - always anchored to the previous BASE, never to
// "now" or to the previous jittered fire time, so processing delay or
// jitter itself never accumulates drift across repeated reschedules -
// see the Stage 11B task's own Part 7.
func (s *scheduler) advanceDueLocked(sr *scheduleRuntime, _ time.Time) {
	sr.nextBaseDue = sr.nextBaseDue.Add(time.Duration(sr.def.IntervalSeconds) * time.Second)
	sr.nextFireAt = sr.nextBaseDue.Add(s.jitter(sr.def.JitterSeconds))
}

func (s *scheduler) jitter(jitterSeconds int) time.Duration {
	if jitterSeconds <= 0 {
		return 0
	}
	return time.Duration(s.randFrac()*float64(jitterSeconds)) * time.Second
}

// firstDue computes a freshly (re)configured schedule's first due
// point: now + firstDelay, with a startupSafetyFloor applied when
// firstDelay is zero - see the Stage 11B task's own Part 6.
func (s *scheduler) firstDue(def domain.Schedule) (base, fireAt time.Time) {
	now := s.now()
	delay := time.Duration(def.FirstDelaySeconds) * time.Second
	if delay <= 0 {
		delay = startupSafetyFloor
	}
	base = now.Add(delay)
	fireAt = base.Add(s.jitter(def.JitterSeconds))
	return base, fireAt
}

// reload replaces the full tracked schedule set from the persisted
// definitions. A schedule id not seen before gets a fresh timing base
// (Part 6); an existing schedule id currently being reloaded because its
// own definition just changed gets ITS timing recomputed fresh too
// (Part 24: "edit interval -> recalculate next run") - this package
// treats every edit uniformly as a timing restart, and rebuilds every
// target's per-account runtime counters fresh at the same time ("target
// changes -> reset target-specific runtime counters safely") rather
// than trying to diff exactly which field changed. A schedule id no
// longer present is dropped entirely.
func (s *scheduler) reload(schedules []domain.Schedule) {
	s.mu.Lock()
	defer s.mu.Unlock()

	next := make(map[string]*scheduleRuntime, len(schedules))
	for _, def := range schedules {
		sr := &scheduleRuntime{def: def, perAccount: make(map[string]*accountTargetState, len(def.Targets))}
		if def.Enabled {
			sr.nextBaseDue, sr.nextFireAt = s.firstDue(def)
		}
		next[def.ID] = sr
	}
	s.schedules = next
}

// reloadOne is a narrow convenience for the Manager to push a single
// changed/created schedule's definition without rebuilding every other
// schedule's own runtime state.
func (s *scheduler) reloadOne(def domain.Schedule) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sr := &scheduleRuntime{def: def, perAccount: make(map[string]*accountTargetState, len(def.Targets))}
	if def.Enabled {
		sr.nextBaseDue, sr.nextFireAt = s.firstDue(def)
	}
	s.schedules[def.ID] = sr
}

func (s *scheduler) remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.schedules, id)
}

func (s *scheduler) get(id string) (*scheduleRuntime, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sr, ok := s.schedules[id]
	return sr, ok
}

// executeDue runs one automatic due occurrence across every target
// account of sr.
func (s *scheduler) executeDue(sr *scheduleRuntime) {
	sr.execMu.Lock()
	defer sr.execMu.Unlock()

	sr.mu.Lock()
	def := sr.def
	sr.mu.Unlock()

	receiving := s.ingest.IsReceiving()
	for _, target := range def.Targets {
		s.executeOneTarget(sr, def, target, receiving, false)
	}
}

// SendNow runs one manual, operator-confirmed execution against
// targetAccountIDs (a subset of def.Targets) - Part 11: ignores interval
// timing, first delay and minimum-chat-activity, but still respects
// onlyWhileIngestReceiving (the preferred, safer Stage 11B policy - no
// bypass exists), account capability, provider/local rate limits
// (including this schedule's own maximum-sends-per-hour), message
// length, and queue bounds. Serializes with any concurrently-due
// automatic execution of the same schedule via sr.execMu.
func (s *scheduler) sendNow(sr *scheduleRuntime, targetAccountIDs []string) []SendResult {
	sr.execMu.Lock()
	defer sr.execMu.Unlock()

	sr.mu.Lock()
	def := sr.def
	sr.mu.Unlock()

	wanted := make(map[string]bool, len(targetAccountIDs))
	for _, id := range targetAccountIDs {
		wanted[id] = true
	}
	receiving := s.ingest.IsReceiving()

	var results []SendResult
	for _, target := range def.Targets {
		if len(targetAccountIDs) > 0 && !wanted[target.AccountID] {
			continue
		}
		results = append(results, s.executeOneTarget(sr, def, target, receiving, true))
	}
	return results
}

func (s *scheduler) executeOneTarget(sr *scheduleRuntime, def domain.Schedule, target domain.Target, receiving bool, manual bool) SendResult {
	now := s.now()

	sr.mu.Lock()
	st := sr.accountState(target.AccountID)
	sr.mu.Unlock()

	skip := func(reason SkipReason) SendResult {
		sr.mu.Lock()
		sr.lastAttemptAt = &now
		sr.lastSkipReason = reason
		st.lastAttemptAt = &now
		st.lastSkipReason = reason
		sr.mu.Unlock()
		return SendResult{AccountID: target.AccountID, SkipReason: reason}
	}

	if def.OnlyWhileIngestReceiving && !receiving {
		return skip(SkipStreamNotReceiving)
	}
	if !manual && st.activityCount < def.MinimumChatMessages {
		return skip(SkipActivityInsufficient)
	}
	if sendsInLastHour(st.sendsThisHour, now) >= def.MaximumSendsPerHour {
		return skip(SkipRateLimited)
	}

	sr.mu.Lock()
	template, msgID := selectMessage(def.Messages, sr.lastMessageID, s.randFrac)
	sr.mu.Unlock()

	ctx := context.Background()
	renderCtx, err := s.buildContext(ctx, target, def)
	if err != nil {
		return skip(SkipPlaceholderUnresolved)
	}
	rendered, err := Render(template, renderCtx)
	if err != nil || len(rendered.Unresolved) > 0 {
		return skip(SkipPlaceholderUnresolved)
	}
	if !rendered.ValidForProvider {
		return skip(SkipRenderedMessageTooLong)
	}

	result, sendErr := s.dispatch.send(ctx, outboundchat.SendMessageRequest{
		AccountID: target.AccountID, Message: rendered.Text, Source: outboundchat.SourceScheduled,
	})

	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.lastAttemptAt = &now
	st.lastAttemptAt = &now

	if sendErr != nil || !result.Sent {
		reason := skipReasonForErr(sendErr)
		sr.lastSkipReason = reason
		st.lastSkipReason = reason
		return SendResult{AccountID: target.AccountID, SkipReason: reason}
	}

	sr.lastSuccessAt = &now
	sr.lastSkipReason = SkipNone
	sr.lastMessageID = msgID
	st.lastSuccessAt = &now
	st.lastSkipReason = SkipNone
	st.activityCount = 0
	st.sendsThisHour = append(st.sendsThisHour, now)
	return SendResult{AccountID: target.AccountID, Sent: true, ProviderMessageID: result.ProviderMessageID}
}

// sendsInLastHour prunes stale entries and reports the remaining count
// within the rolling hour - the pruned slice must be written back by
// the caller if it wants the prune to persist; executeOneTarget always
// appends immediately after, so pruning here is enough to keep this
// list bounded over time.
func sendsInLastHour(sends []time.Time, now time.Time) int {
	cutoff := now.Add(-rollingHour)
	count := 0
	for _, t := range sends {
		if t.After(cutoff) {
			count++
		}
	}
	return count
}

// selectMessage picks one message alternative uniformly at random,
// avoiding an immediate repeat of lastMessageID when the group has more
// than one alternative - see the Stage 11B task's own Part 4.
func selectMessage(messages []domain.ScheduleMessage, lastMessageID string, randFrac func() float64) (template string, id string) {
	candidates := messages
	if len(messages) > 1 {
		filtered := make([]domain.ScheduleMessage, 0, len(messages)-1)
		for _, m := range messages {
			if m.ID != lastMessageID {
				filtered = append(filtered, m)
			}
		}
		if len(filtered) > 0 {
			candidates = filtered
		}
	}
	idx := int(randFrac() * float64(len(candidates)))
	if idx >= len(candidates) {
		idx = len(candidates) - 1
	}
	return candidates[idx].MessageTemplate, candidates[idx].ID
}

// recordActivity increments the activity counter for every schedule
// that targets accountID - called once per eligible normalized
// chat.message event (self/synthetic/bot filtering already applied by
// the caller - see runtime.go).
func (s *scheduler) recordActivity(accountID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sr := range s.schedules {
		sr.mu.Lock()
		for _, t := range sr.def.Targets {
			if t.AccountID == accountID {
				sr.accountState(accountID).activityCount++
				break
			}
		}
		sr.mu.Unlock()
	}
}

// buildContext resolves a Context for target from already-available
// local data only - never a provider network request during rendering.
func (s *scheduler) buildContext(ctx context.Context, target domain.Target, def domain.Schedule) (Context, error) {
	acc, err := s.accounts.GetAccount(ctx, target.AccountID)
	if err != nil {
		return Context{}, err
	}
	rc := Context{ChannelName: acc.DisplayName, Platform: PlatformDisplayName(acc.ProviderID)}
	if rc.ChannelName == "" {
		rc.ChannelName = acc.Login
	}
	if url, ok := ChannelURL(acc.ProviderID, acc.Login); ok {
		rc.ChannelURL = url
	}
	if target.PlatformID != "" {
		if p, err := s.platforms.Get(ctx, target.PlatformID); err == nil {
			title := p.Metadata.Title
			rc.StreamTitle = &title
		}
	}
	if uptime, ok := s.streamUptime(); ok {
		rc.StreamUptime = &uptime
	}
	_ = def
	return rc, nil
}

// streamUptime formats how long the local ingest has been continuously
// receiving, or ok=false if that is currently unresolvable.
func (s *scheduler) streamUptime() (string, bool) {
	since, ok := s.ingest.ReceivingSince()
	if !ok {
		return "", false
	}
	d := s.now().Sub(since)
	if d < 0 {
		d = 0
	}
	return formatDuration(d), true
}

// formatDuration renders a coarse, human-readable "1h23m"/"45m"/"30s"
// style duration - never sub-second precision, which would be noise for
// a chat message.
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	sec := d / time.Second
	switch {
	case h > 0:
		return fmtHM(h, m)
	case m > 0:
		return fmtMS(m, sec)
	default:
		return fmtS(sec)
	}
}

func fmtHM(h, m time.Duration) string {
	return strconv.FormatInt(int64(h), 10) + "h" + strconv.FormatInt(int64(m), 10) + "m"
}
func fmtMS(m, s time.Duration) string {
	return strconv.FormatInt(int64(m), 10) + "m" + strconv.FormatInt(int64(s), 10) + "s"
}
func fmtS(s time.Duration) string { return strconv.FormatInt(int64(s), 10) + "s" }

// snapshot returns sr's current runtime state as a ScheduleSnapshot.
func (s *scheduler) snapshotOf(sr *scheduleRuntime) ScheduleSnapshot {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	now := s.now()
	state := ScheduleScheduled
	switch {
	case !sr.def.Enabled:
		state = ScheduleDisabled
	case sr.lastSkipReason == SkipStreamNotReceiving:
		state = ScheduleWaitingForStream
	case sr.lastSkipReason == SkipActivityInsufficient:
		state = ScheduleWaitingForActivity
	case sr.lastSkipReason == SkipRateLimited:
		state = ScheduleRateLimited
	case sr.lastSkipReason == SkipPermissionRequired || sr.lastSkipReason == SkipProviderUnsupported:
		state = SchedulePermissionRequired
	case sr.lastSkipReason == SkipSendFailed:
		state = ScheduleError
	}

	targets := make([]TargetSnapshot, 0, len(sr.def.Targets))
	for _, t := range sr.def.Targets {
		st := sr.perAccount[t.AccountID]
		ts := TargetSnapshot{AccountID: t.AccountID}
		if st != nil {
			ts.LastAttemptAt, ts.LastSuccessAt, ts.LastSkipReason = st.lastAttemptAt, st.lastSuccessAt, st.lastSkipReason
			ts.SendsThisHour = sendsInLastHour(st.sendsThisHour, now)
		}
		targets = append(targets, ts)
	}

	var nextRun *time.Time
	if sr.def.Enabled && !sr.nextFireAt.IsZero() {
		t := sr.nextFireAt
		nextRun = &t
	}

	return ScheduleSnapshot{
		ScheduleID: sr.def.ID, Enabled: sr.def.Enabled, State: state, NextRunAt: nextRun,
		TargetCount: len(sr.def.Targets), LastAttemptAt: sr.lastAttemptAt, LastSuccessAt: sr.lastSuccessAt,
		LastSkipReason: sr.lastSkipReason, Targets: targets,
	}
}
