package branch

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/streaming-tree/server/internal/domain/credential"
	"github.com/streaming-tree/server/internal/domain/output"
	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/runtime/ffmpeg"
	"github.com/streaming-tree/server/internal/runtime/mediamtx"
)

// Restart policy constants.
//
// The exact numbers mirror internal/runtime/mediamtx's policy in spirit
// (bounded exponential backoff, a cap per window, a stable-run reset), but
// are declared independently rather than imported: the two managers
// supervise different kinds of process for different reasons, and coupling
// them for the sake of sharing four constants was judged not worth the
// dependency it would create between otherwise-unrelated packages.
const (
	minBackoff            = 1 * time.Second
	maxBackoff            = 30 * time.Second
	maxRestartsPerWindow  = 5
	restartWindow         = 5 * time.Minute
	stableRunDuration     = 60 * time.Second
	stopGracePeriod       = 5 * time.Second
	reconcileInterval     = 2 * time.Second
	ffmpegRefreshInterval = 5 * time.Minute

	// ingestSettleWindow and ingestSettleInterval bound how long watchExit
	// waits for the MediaMTX ingest snapshot to catch up before classifying
	// an exit as a genuine crash - see waitForSettledIngest's doc comment.
	ingestSettleWindow   = 1500 * time.Millisecond
	ingestSettleInterval = 150 * time.Millisecond
)

// PlatformSource is the subset of the platform service the manager needs.
type PlatformSource interface {
	List(ctx context.Context) ([]platform.Platform, error)
	Get(ctx context.Context, id string) (platform.Platform, error)
}

// OutputSource is the subset of the output-settings service the manager
// needs.
type OutputSource interface {
	Get(ctx context.Context, platformID string) (output.Settings, error)
}

// CredentialSource is the subset of the credential service the manager
// needs.
//
// RetrieveForProcessStart returns a secret value and is deliberately absent
// from httpapi.CredentialService, the narrower interface the HTTP layer is
// given - this interface exists so only this package, which starts real
// processes, can ever call it.
type CredentialSource interface {
	Status(ctx context.Context, platformID string) (credential.Status, credential.StoreStatus, error)
	RetrieveForProcessStart(ctx context.Context, platformID string) (string, error)
}

// FFmpegSource resolves the FFmpeg dependency.
type FFmpegSource interface {
	Resolve(ctx context.Context) ffmpeg.Resolution
}

// IngestSource reports the current MediaMTX and ingest state.
type IngestSource interface {
	Snapshot() mediamtx.Snapshot
}

// Options carries everything the manager needs from its caller.
type Options struct {
	Platforms   PlatformSource
	Outputs     OutputSource
	Credentials CredentialSource
	FFmpeg      FFmpegSource
	Ingest      IngestSource
	Logger      *slog.Logger
}

// branchState is one platform's supervised state. All fields are only ever
// read or written while holding Manager.mu.
type branchState struct {
	state          State
	desiredRunning bool
	blockers       []string
	startedAt      time.Time
	liveAt         time.Time
	stoppedAt      time.Time
	restartCount   int
	lastProgress   *Progress
	lastError      *RuntimeError

	proc          processHandle
	generation    uint64
	backoff       time.Duration
	restartTimes  []time.Time
	stopRequested bool
}

// launchInputs is what one launch attempt needs, gathered once while
// evaluating eligibility so launch() itself does not have to re-fetch it.
type launchInputs struct {
	settings output.Settings
	inputURL string
}

// processHandle is the subset of *process the manager depends on - narrow
// enough that tests can substitute a fake process without spawning a real
// executable, reserving real FFmpeg process spawning for the dedicated
// integration verification script (scripts/verify-ffmpeg-branches.mjs).
type processHandle interface {
	Exited() <-chan struct{}
	stop(grace time.Duration) error
}

// processLauncher spawns one branch process. The production default wraps
// startProcess (process.go); tests inject a fake.
type processLauncher func(path string, args []string, redactor *Redactor, logger *slog.Logger, report onProgress) (processHandle, error)

func defaultProcessLauncher(path string, args []string, redactor *Redactor, logger *slog.Logger, report onProgress) (processHandle, error) {
	return startProcess(path, args, redactor, logger, report)
}

// Outcome is the result of a single-branch start or restart request.
type Outcome struct {
	Accepted bool
	Blockers []string
	Conflict bool
}

// Manager supervises one FFmpeg branch process per configured destination.
//
// Concurrency model: one mutex guards every branch's state. Given this
// supervises a handful of local destinations at most, a single lock kept
// only for state bookkeeping (never across process spawn, credential
// retrieval or database reads) is simpler to reason about correctly than
// per-branch locking, and cheap enough that the simplicity is worth it.
type Manager struct {
	opts Options

	mu       sync.Mutex
	branches map[string]*branchState

	ffmpegMu         sync.Mutex
	ffmpegCached     ffmpeg.Resolution
	ffmpegResolvedAt time.Time

	launchProcess processLauncher

	// reconcileEvery and refreshFFmpegEvery default to reconcileInterval and
	// ffmpegRefreshInterval; tests shorten them so reconciliation-dependent
	// behavior does not require a real multi-second sleep per test.
	reconcileEvery     time.Duration
	refreshFFmpegEvery time.Duration

	// Restart-policy parameters, defaulting to the package constants of the
	// same name; tests shorten them so restart-policy behavior does not
	// require tens of real seconds of backoff per test.
	policyMinBackoff           time.Duration
	policyMaxBackoff           time.Duration
	policyMaxRestartsPerWindow int
	policyRestartWindow        time.Duration
	policyStableRunDuration    time.Duration

	// ingestSettleWindow/Interval default to the package constants of the
	// same name; tests shorten them for the same reason as the restart
	// policy fields above.
	ingestSettleWindowFor   time.Duration
	ingestSettleIntervalFor time.Duration

	logger    *slog.Logger
	lifecycle context.Context
	cancel    context.CancelFunc
	workers   sync.WaitGroup
}

// NewManager builds a Manager. Call Start to begin reconciliation.
func NewManager(opts Options) *Manager {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{
		opts:               opts,
		branches:           make(map[string]*branchState),
		logger:             logger,
		launchProcess:      defaultProcessLauncher,
		reconcileEvery:     reconcileInterval,
		refreshFFmpegEvery: ffmpegRefreshInterval,

		policyMinBackoff:           minBackoff,
		policyMaxBackoff:           maxBackoff,
		policyMaxRestartsPerWindow: maxRestartsPerWindow,
		policyRestartWindow:        restartWindow,
		policyStableRunDuration:    stableRunDuration,

		ingestSettleWindowFor:   ingestSettleWindow,
		ingestSettleIntervalFor: ingestSettleInterval,
	}
}

// Start resolves FFmpeg once and begins the background reconciliation loop.
//
// No branch is started here: every platform begins with desiredRunning
// false, exactly as the task requires - a backend restart never resumes a
// broadcast on its own.
func (m *Manager) Start(ctx context.Context) {
	m.lifecycle, m.cancel = context.WithCancel(context.Background())

	m.refreshFFmpeg(ctx)

	m.workers.Add(1)
	go func() {
		defer m.workers.Done()
		m.reconcileLoop()
	}()
}

func (m *Manager) refreshFFmpeg(ctx context.Context) {
	resolution := m.opts.FFmpeg.Resolve(ctx)
	m.ffmpegMu.Lock()
	m.ffmpegCached = resolution
	m.ffmpegResolvedAt = time.Now()
	m.ffmpegMu.Unlock()
}

func (m *Manager) cachedFFmpeg() ffmpeg.Resolution {
	m.ffmpegMu.Lock()
	defer m.ffmpegMu.Unlock()
	return m.ffmpegCached
}

// reconcileLoop periodically resumes desired branches once their blockers
// clear (chiefly: ingest returning) and stops a branch whose input has
// disappeared but whose process has not exited on its own yet.
func (m *Manager) reconcileLoop() {
	ticker := time.NewTicker(m.reconcileEvery)
	defer ticker.Stop()

	ffmpegTicker := time.NewTicker(m.refreshFFmpegEvery)
	defer ffmpegTicker.Stop()

	for {
		select {
		case <-m.lifecycle.Done():
			return
		case <-ffmpegTicker.C:
			m.refreshFFmpeg(m.lifecycle)
		case <-ticker.C:
			m.reconcileOnce()
		}
	}
}

func (m *Manager) reconcileOnce() {
	platforms, err := m.opts.Platforms.List(m.lifecycle)
	if err != nil {
		return
	}

	for _, p := range platforms {
		m.mu.Lock()
		b, tracked := m.branches[p.ID]
		m.mu.Unlock()
		if !tracked || !b.desiredRunning {
			continue
		}

		switch b.state {
		case StateWaitingForIngest:
			m.attemptResume(p.ID)
		case StateLive, StateStarting:
			// If ingest has disappeared and the process has not exited on
			// its own yet, stop it proactively rather than waiting
			// indefinitely - see the ingest-loss handling in watchExit for
			// the common case where FFmpeg exits by itself first.
			snapshot := m.opts.Ingest.Snapshot()
			if snapshot.Ingest.State != mediamtx.IngestReceiving {
				m.mu.Lock()
				proc := b.proc
				m.mu.Unlock()
				if proc != nil {
					_ = proc.stop(stopGracePeriod)
				}
			}
		}
	}
}

// attemptResume re-checks eligibility for a branch that is desired running
// but currently idle-on-ingest, and launches it if every blocker has
// cleared.
func (m *Manager) attemptResume(platformID string) {
	p, err := m.opts.Platforms.Get(m.lifecycle, platformID)
	if err != nil {
		return
	}

	blockers, inputs, err := m.computeBlockers(m.lifecycle, p)
	if err != nil || len(blockers) > 0 {
		return
	}

	m.mu.Lock()
	b := m.branches[platformID]
	if b == nil || !b.desiredRunning || b.proc != nil {
		m.mu.Unlock()
		return
	}
	b.generation++
	generation := b.generation
	b.state = StateStarting
	b.startedAt = time.Now()
	// liveAt is scoped to the attempt about to be launched, not any earlier
	// one - see scheduleRestart's stable-run check for why a stale value
	// here would be a bug (a branch that went live once, long ago, and has
	// failed on every attempt since must not keep looking "stable").
	b.liveAt = time.Time{}
	b.blockers = nil
	m.mu.Unlock()

	m.launch(platformID, inputs, generation)
}

// computeBlockers evaluates every requirement in Part 6's eligibility list.
func (m *Manager) computeBlockers(ctx context.Context, p platform.Platform) ([]string, launchInputs, error) {
	var blockers []string

	if !p.Enabled {
		blockers = append(blockers, BlockerPlatformDisabled)
	}

	settings, err := m.opts.Outputs.Get(ctx, p.ID)
	if err != nil && !errors.Is(err, output.ErrNotFound) {
		return nil, launchInputs{}, err
	}
	if settings.ServerURL == "" {
		blockers = append(blockers, BlockerOutputServerMissing)
	}

	credStatus, credStore, err := m.opts.Credentials.Status(ctx, p.ID)
	if err != nil {
		return nil, launchInputs{}, err
	}
	if !credStore.Available {
		blockers = append(blockers, BlockerCredentialUnavailable)
	} else if !credStatus.Configured {
		blockers = append(blockers, BlockerStreamKeyMissing)
	}

	resolution := m.cachedFFmpeg()
	if resolution.Source == ffmpeg.SourceMissing {
		blockers = append(blockers, BlockerFFmpegMissing)
	} else if !resolution.Compatible {
		blockers = append(blockers, BlockerFFmpegIncompatible)
	}

	snapshot := m.opts.Ingest.Snapshot()
	if snapshot.MediaMTX.State != mediamtx.StateReady {
		blockers = append(blockers, BlockerMediaMTXNotReady)
	} else if snapshot.Ingest.State != mediamtx.IngestReceiving {
		blockers = append(blockers, BlockerIngestNotReceiving)
	}

	return blockers, launchInputs{settings: settings, inputURL: snapshot.Connection.PublishURL}, nil
}

// onlyIngestBlockers reports whether every blocker in the list is one that
// can be expected to clear on its own once a publisher reconnects, without
// any other configuration change - the case the reconciliation loop resumes
// automatically.
func onlyIngestBlockers(blockers []string) bool {
	for _, b := range blockers {
		if b != BlockerMediaMTXNotReady && b != BlockerIngestNotReceiving {
			return false
		}
	}
	return len(blockers) > 0
}

func (m *Manager) getOrCreateLocked(platformID string) *branchState {
	b, ok := m.branches[platformID]
	if !ok {
		b = &branchState{state: StateIdle, backoff: m.policyMinBackoff}
		m.branches[platformID] = b
	}
	return b
}

// StartBranch explicitly starts one destination. This is a real, deliberate
// user action: it begins actual outgoing transmission once eligible.
func (m *Manager) StartBranch(ctx context.Context, platformID string) (Outcome, error) {
	p, err := m.opts.Platforms.Get(ctx, platformID)
	if err != nil {
		return Outcome{}, ErrNotFound
	}

	m.mu.Lock()
	b := m.getOrCreateLocked(platformID)
	if b.state == StateStarting || b.state == StateLive || b.state == StateRestarting {
		m.mu.Unlock()
		return Outcome{Conflict: true}, nil
	}
	m.mu.Unlock()

	blockers, inputs, err := m.computeBlockers(ctx, p)
	if err != nil {
		return Outcome{}, err
	}

	m.mu.Lock()
	b = m.getOrCreateLocked(platformID)
	if b.state == StateStarting || b.state == StateLive || b.state == StateRestarting {
		m.mu.Unlock()
		return Outcome{Conflict: true}, nil
	}
	if len(blockers) > 0 {
		b.state = StateBlocked
		b.blockers = blockers
		b.desiredRunning = false
		m.mu.Unlock()
		return Outcome{Blockers: blockers}, nil
	}

	b.desiredRunning = true
	b.blockers = nil
	b.stopRequested = false
	b.restartCount = 0
	b.backoff = m.policyMinBackoff
	b.restartTimes = nil
	b.lastError = nil
	b.generation++
	generation := b.generation
	b.state = StateStarting
	b.startedAt = time.Now()
	b.liveAt = time.Time{} // see attemptResume's comment on the same line
	m.mu.Unlock()

	m.launch(platformID, inputs, generation)
	return Outcome{Accepted: true}, nil
}

// launch retrieves the stream key, builds the destination URL, and spawns
// FFmpeg. The secret is held only for the duration of this call.
func (m *Manager) launch(platformID string, inputs launchInputs, generation uint64) {
	streamKey, err := m.opts.Credentials.RetrieveForProcessStart(m.lifecycle, platformID)
	if err != nil {
		m.failStart(platformID, generation,
			NewRuntimeError(CodeStartFailed, "The stream key could not be retrieved to start this destination."))
		return
	}

	destinationURL := buildDestinationURL(inputs.settings.ServerURL, streamKey)
	args := buildArgs(inputs.inputURL, destinationURL)
	redactor := newRedactor(streamKey, destinationURL)

	resolution := m.cachedFFmpeg()

	proc, err := m.launchProcess(resolution.Path, args, redactor, m.logger, func(p Progress) {
		m.onProgress(platformID, generation, p)
	})

	// Go gives no guarantee that clearing a string's only reference actually
	// zeroes the backing memory - see docs/progress.md for why this stage
	// accepts that limitation rather than claiming otherwise. This at least
	// drops every reference this function held as soon as they are no
	// longer needed, so nothing here keeps them reachable any longer than
	// starting the process required.
	streamKey = ""
	destinationURL = ""
	args = nil

	if err != nil {
		m.failStart(platformID, generation,
			NewRuntimeError(CodeStartFailed, "FFmpeg could not be started."))
		return
	}

	m.mu.Lock()
	b := m.branches[platformID]
	if b == nil || b.generation != generation {
		m.mu.Unlock()
		_ = proc.stop(stopGracePeriod)
		return
	}
	b.proc = proc
	m.mu.Unlock()

	m.workers.Add(1)
	go func() {
		defer m.workers.Done()
		m.watchExit(platformID, proc, generation)
	}()
}

func (m *Manager) failStart(platformID string, generation uint64, runtimeErr *RuntimeError) {
	m.mu.Lock()
	defer m.mu.Unlock()

	b := m.branches[platformID]
	if b == nil || b.generation != generation {
		return
	}
	b.state = StateError
	b.desiredRunning = false
	b.lastError = runtimeErr
	b.stoppedAt = time.Now()
}

func (m *Manager) onProgress(platformID string, generation uint64, p Progress) {
	m.mu.Lock()
	defer m.mu.Unlock()

	b := m.branches[platformID]
	if b == nil || b.generation != generation {
		return
	}
	b.lastProgress = &p
	if b.state == StateStarting && p.hasAdvanced() {
		b.state = StateLive
		b.liveAt = time.Now()
	}
}

// watchExit waits for the process to exit and decides what happens next:
// a clean stop, an ingest-loss pause, or the restart policy.
func (m *Manager) watchExit(platformID string, proc processHandle, generation uint64) {
	<-proc.Exited()

	m.mu.Lock()
	b := m.branches[platformID]
	if b == nil || b.generation != generation {
		m.mu.Unlock()
		return
	}
	b.proc = nil
	b.stoppedAt = time.Now()

	if b.stopRequested {
		b.state = StateIdle
		b.desiredRunning = false
		b.stopRequested = false
		m.mu.Unlock()
		return
	}
	desired := b.desiredRunning
	m.mu.Unlock()

	if !desired {
		return
	}

	// Was this exit caused by the local input disappearing? If so, this is
	// not a failure: wait for ingest to return rather than applying the
	// restart policy against a missing input.
	//
	// MediaMTX can drop the branch's reader connection within milliseconds
	// of its publisher disappearing, but this application's own ingest
	// snapshot is only refreshed on a periodic poll (see
	// mediamtx.ingestPollInterval) - so FFmpeg's exit can outrace that poll
	// by a fraction of a second. waitForSettledIngest gives the poll a
	// short, bounded window to catch up before this is classified as a
	// genuine crash, so a plain ingest disconnect does not spuriously
	// consume the restart-limit budget.
	snapshot := m.waitForSettledIngest()
	if snapshot.Ingest.State != mediamtx.IngestReceiving {
		m.mu.Lock()
		if b.generation == generation {
			b.state = StateWaitingForIngest
			b.blockers = []string{BlockerIngestNotReceiving}
		}
		m.mu.Unlock()
		return
	}

	m.mu.Lock()
	if b.generation == generation {
		b.lastError = NewRuntimeError(CodeExitedUnexpectedly, "FFmpeg exited unexpectedly.")
	}
	m.mu.Unlock()

	m.scheduleRestart(platformID, generation)
}

// waitForSettledIngest polls the ingest snapshot for up to
// ingestSettleWindowFor, returning as soon as it reports anything other than
// "receiving" - see watchExit's call site for why this exists. If ingest
// stays "receiving" for the whole window, the last snapshot taken is
// returned, and the caller proceeds on the (now well-settled) assumption
// that the exit was a genuine crash.
func (m *Manager) waitForSettledIngest() mediamtx.Snapshot {
	deadline := time.Now().Add(m.ingestSettleWindowFor)
	snapshot := m.opts.Ingest.Snapshot()
	for snapshot.Ingest.State == mediamtx.IngestReceiving && time.Now().Before(deadline) {
		time.Sleep(m.ingestSettleIntervalFor)
		snapshot = m.opts.Ingest.Snapshot()
	}
	return snapshot
}

// scheduleRestart applies the bounded-exponential-backoff restart policy.
// An explicit Stop never reaches here, since it clears desiredRunning and
// is handled entirely inside watchExit before this is called.
func (m *Manager) scheduleRestart(platformID string, generation uint64) {
	m.mu.Lock()
	b := m.branches[platformID]
	if b == nil || b.generation != generation || !b.desiredRunning {
		m.mu.Unlock()
		return
	}

	now := time.Now()

	// A branch that ran live for a while before failing gets a fresh backoff:
	// an occasional crash after a long stable run should not inherit a long
	// delay accumulated from an earlier bad patch.
	if !b.liveAt.IsZero() && now.Sub(b.liveAt) >= m.policyStableRunDuration {
		b.backoff = m.policyMinBackoff
		b.restartTimes = nil
	}

	kept := b.restartTimes[:0]
	for _, at := range b.restartTimes {
		if now.Sub(at) < m.policyRestartWindow {
			kept = append(kept, at)
		}
	}
	b.restartTimes = kept

	if len(b.restartTimes) >= m.policyMaxRestartsPerWindow {
		b.state = StateError
		b.desiredRunning = false
		b.lastError = NewRuntimeError(CodeRestartLimit, fmt.Sprintf(
			"FFmpeg failed %d times in %s and will not be restarted automatically. "+
				"Fix the underlying problem and start this destination manually.",
			m.policyMaxRestartsPerWindow, m.policyRestartWindow))
		m.mu.Unlock()
		return
	}

	b.restartTimes = append(b.restartTimes, now)
	b.restartCount++
	delay := b.backoff
	if b.backoff < m.policyMaxBackoff {
		b.backoff *= 2
		if b.backoff > m.policyMaxBackoff {
			b.backoff = m.policyMaxBackoff
		}
	}
	b.state = StateRestarting
	m.mu.Unlock()

	m.workers.Add(1)
	go func() {
		defer m.workers.Done()
		select {
		case <-time.After(delay):
		case <-m.lifecycle.Done():
			return
		}
		m.retryAfterBackoff(platformID, generation)
	}()
}

// retryAfterBackoff re-evaluates eligibility once the backoff delay has
// elapsed and either relaunches or parks the branch waiting for ingest.
func (m *Manager) retryAfterBackoff(platformID string, generation uint64) {
	m.mu.Lock()
	b := m.branches[platformID]
	if b == nil || b.generation != generation || !b.desiredRunning {
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()

	p, err := m.opts.Platforms.Get(m.lifecycle, platformID)
	if err != nil {
		return
	}
	blockers, inputs, err := m.computeBlockers(m.lifecycle, p)
	if err != nil {
		return
	}

	m.mu.Lock()
	b = m.branches[platformID]
	if b == nil || b.generation != generation || !b.desiredRunning {
		m.mu.Unlock()
		return
	}

	if len(blockers) > 0 {
		if onlyIngestBlockers(blockers) {
			b.state = StateWaitingForIngest
			b.blockers = blockers
			m.mu.Unlock()
			return
		}
		b.state = StateBlocked
		b.blockers = blockers
		b.desiredRunning = false
		m.mu.Unlock()
		return
	}

	b.state = StateStarting
	b.startedAt = time.Now()
	b.liveAt = time.Time{} // see attemptResume's comment on the same line
	b.blockers = nil
	m.mu.Unlock()

	m.launch(platformID, inputs, generation)
}

// StopBranch explicitly stops one destination and suppresses any pending or
// future automatic restart until it is started again.
func (m *Manager) StopBranch(ctx context.Context, platformID string) error {
	m.mu.Lock()
	b, tracked := m.branches[platformID]
	if !tracked || (!b.desiredRunning && b.state != StateBlocked) {
		m.mu.Unlock()
		return ErrNotRunning
	}

	b.desiredRunning = false
	proc := b.proc

	if proc == nil {
		// Idle, blocked, or waiting-for-ingest: nothing to terminate: just
		// clear desired state so the reconciliation loop leaves it alone.
		b.state = StateIdle
		b.blockers = nil
		m.mu.Unlock()
		return nil
	}

	b.stopRequested = true
	b.state = StateStopping
	m.mu.Unlock()

	if err := proc.stop(stopGracePeriod); err != nil {
		m.logger.Warn("branch did not stop cleanly",
			slog.String("platform_id", platformID), slog.Any("error", err))
	}
	return nil
}

// RestartBranch performs one controlled stop followed by a start.
func (m *Manager) RestartBranch(ctx context.Context, platformID string) (Outcome, error) {
	m.mu.Lock()
	b, tracked := m.branches[platformID]
	running := tracked && b.proc != nil
	m.mu.Unlock()

	if running {
		if err := m.StopBranch(ctx, platformID); err != nil && !errors.Is(err, ErrNotRunning) {
			return Outcome{}, err
		}
		// stop() above blocks until the process has exited, so it is safe
		// to start immediately.
	}

	return m.StartBranch(ctx, platformID)
}

// StartEnabledResult is one platform's outcome from StartEnabled.
type StartEnabledResult struct {
	PlatformID string
	Outcome    Outcome
}

// StartEnabled starts every eligible enabled destination. One ineligible
// destination never prevents another from starting.
func (m *Manager) StartEnabled(ctx context.Context) []StartEnabledResult {
	platforms, err := m.opts.Platforms.List(ctx)
	if err != nil {
		return nil
	}

	results := make([]StartEnabledResult, 0, len(platforms))
	for _, p := range platforms {
		if !p.Enabled {
			continue
		}
		outcome, err := m.StartBranch(ctx, p.ID)
		if err != nil {
			continue
		}
		results = append(results, StartEnabledResult{PlatformID: p.ID, Outcome: outcome})
	}
	return results
}

// StopAll stops every desired-running or active branch.
func (m *Manager) StopAll(ctx context.Context) {
	m.mu.Lock()
	ids := make([]string, 0, len(m.branches))
	for id, b := range m.branches {
		if b.desiredRunning || b.proc != nil || b.state == StateBlocked || b.state == StateWaitingForIngest {
			ids = append(ids, id)
		}
	}
	m.mu.Unlock()

	for _, id := range ids {
		_ = m.StopBranch(ctx, id)
	}
}

// Snapshot returns one runtime view per configured platform.
func (m *Manager) Snapshot(ctx context.Context) ([]Snapshot, error) {
	platforms, err := m.opts.Platforms.List(ctx)
	if err != nil {
		return nil, err
	}

	snapshots := make([]Snapshot, 0, len(platforms))
	for _, p := range platforms {
		snapshots = append(snapshots, m.snapshotOne(p.ID))
	}
	return snapshots, nil
}

func (m *Manager) snapshotOne(platformID string) Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()

	b, ok := m.branches[platformID]
	if !ok {
		return Snapshot{PlatformID: platformID, State: StateIdle, Blockers: []string{}}
	}

	blockers := b.blockers
	if blockers == nil {
		blockers = []string{}
	}

	return Snapshot{
		PlatformID:     platformID,
		State:          b.state,
		DesiredRunning: b.desiredRunning,
		Blockers:       blockers,
		StartedAt:      formatTime(b.startedAt),
		LiveAt:         formatTime(b.liveAt),
		StoppedAt:      formatTime(b.stoppedAt),
		RestartCount:   b.restartCount,
		Progress:       b.lastProgress,
		LastError:      b.lastError,
	}
}

// Forget stops (best-effort) and removes a branch's tracked state, called
// when its platform is deleted so no entry lingers for an id that no longer
// exists.
func (m *Manager) Forget(ctx context.Context, platformID string) {
	_ = m.StopBranch(ctx, platformID)

	m.mu.Lock()
	delete(m.branches, platformID)
	m.mu.Unlock()
}

// FFmpegStatus reports the cached FFmpeg dependency resolution.
func (m *Manager) FFmpegStatus() ffmpeg.Resolution {
	return m.cachedFFmpeg()
}

// Shutdown stops every branch and waits for their workers to finish.
//
// Called before the MediaMTX supervisor's own Shutdown - see cmd/server/main.go
// for the full order and why: stopping branches first means they cannot keep
// reconnecting against an input that is itself in the middle of shutting
// down.
func (m *Manager) Shutdown(ctx context.Context) {
	if m.cancel == nil {
		return
	}

	m.mu.Lock()
	ids := make([]string, 0, len(m.branches))
	for id := range m.branches {
		ids = append(ids, id)
	}
	m.mu.Unlock()

	for _, id := range ids {
		_ = m.StopBranch(ctx, id)
	}

	m.cancel()

	done := make(chan struct{})
	go func() {
		m.workers.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		m.logger.Warn("timed out waiting for branch workers to finish")
	}
}
