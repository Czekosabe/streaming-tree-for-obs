package branch

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/credential"
	"github.com/streaming-tree/server/internal/domain/output"
	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/runtime/ffmpeg"
	"github.com/streaming-tree/server/internal/runtime/mediamtx"
)

// --- fakes -------------------------------------------------------------

type fakePlatforms struct {
	mu    sync.Mutex
	items map[string]platform.Platform
}

func newFakePlatforms(items ...platform.Platform) *fakePlatforms {
	f := &fakePlatforms{items: make(map[string]platform.Platform)}
	for _, p := range items {
		f.items[p.ID] = p
	}
	return f
}

func (f *fakePlatforms) List(context.Context) ([]platform.Platform, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]platform.Platform, 0, len(f.items))
	for _, p := range f.items {
		out = append(out, p)
	}
	return out, nil
}

func (f *fakePlatforms) Get(_ context.Context, id string) (platform.Platform, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.items[id]
	if !ok {
		return platform.Platform{}, platform.ErrNotFound
	}
	return p, nil
}

func (f *fakePlatforms) setEnabled(id string, enabled bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p := f.items[id]
	p.Enabled = enabled
	f.items[id] = p
}

type fakeOutputs struct {
	mu    sync.Mutex
	items map[string]output.Settings
}

func newFakeOutputs() *fakeOutputs {
	return &fakeOutputs{items: make(map[string]output.Settings)}
}

func (f *fakeOutputs) Get(_ context.Context, id string) (output.Settings, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.items[id]
	if !ok {
		return output.Settings{}, output.ErrNotFound
	}
	return s, nil
}

func (f *fakeOutputs) set(id, serverURL string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.items[id] = output.Settings{ServerURL: serverURL, AutoRestart: true}
}

type fakeCredentials struct {
	mu             sync.Mutex
	storeAvailable bool
	keys           map[string]string
}

func newFakeCredentials() *fakeCredentials {
	return &fakeCredentials{storeAvailable: true, keys: make(map[string]string)}
}

func (f *fakeCredentials) Status(_ context.Context, id string) (credential.Status, credential.StoreStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, configured := f.keys[id]
	return credential.Status{Configured: configured}, credential.StoreStatus{Available: f.storeAvailable}, nil
}

func (f *fakeCredentials) RetrieveForProcessStart(_ context.Context, id string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.storeAvailable {
		return "", errors.New("store unavailable")
	}
	key, ok := f.keys[id]
	if !ok {
		return "", errors.New("no key")
	}
	return key, nil
}

func (f *fakeCredentials) setKey(id, key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.keys[id] = key
}

type fakeFFmpeg struct {
	mu         sync.Mutex
	resolution ffmpeg.Resolution
}

func (f *fakeFFmpeg) Resolve(context.Context) ffmpeg.Resolution {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.resolution
}

func compatibleFFmpeg() *fakeFFmpeg {
	return &fakeFFmpeg{resolution: ffmpeg.Resolution{
		Source: ffmpeg.SourcePath, Path: "/usr/bin/ffmpeg", Compatible: true,
	}}
}

type fakeIngest struct {
	mu       sync.Mutex
	snapshot mediamtx.Snapshot
}

func readyIngest() *fakeIngest {
	return &fakeIngest{snapshot: mediamtx.Snapshot{
		MediaMTX:   mediamtx.MediaMTXSnapshot{State: mediamtx.StateReady},
		Ingest:     mediamtx.IngestSnapshot{State: mediamtx.IngestReceiving},
		Connection: mediamtx.ConnectionSnapshot{PublishURL: "rtmp://127.0.0.1:1935/live"},
	}}
}

func (f *fakeIngest) Snapshot() mediamtx.Snapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.snapshot
}

func (f *fakeIngest) setIngestState(state mediamtx.IngestState) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.snapshot.Ingest.State = state
}

// fakeProc is a controllable processHandle standing in for a real FFmpeg
// child: tests drive it directly instead of spawning a real process, so the
// state machine is exercised without any executable on disk.
type fakeProc struct {
	exited chan struct{}
	report onProgress

	mu      sync.Mutex
	stopped bool
}

func (p *fakeProc) Exited() <-chan struct{} { return p.exited }

func (p *fakeProc) stop(time.Duration) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.stopped {
		p.stopped = true
		close(p.exited)
	}
	return nil
}

// crash simulates FFmpeg exiting on its own (a crash, or the input ending),
// as opposed to being asked to stop.
func (p *fakeProc) crash() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.stopped {
		p.stopped = true
		close(p.exited)
	}
}

// fakeLaunchRecorder collects every process the fake launcher created, in
// order, so tests can drive the most recent one.
type fakeLaunchRecorder struct {
	mu    sync.Mutex
	procs []*fakeProc
	fail  bool
}

func (r *fakeLaunchRecorder) launcher() processLauncher {
	return func(path string, args []string, redactor *Redactor, logger *slog.Logger, report onProgress) (processHandle, error) {
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.fail {
			return nil, errors.New("simulated launch failure")
		}
		p := &fakeProc{exited: make(chan struct{}), report: report}
		r.procs = append(r.procs, p)
		return p, nil
	}
}

func (r *fakeLaunchRecorder) latest() *fakeProc {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.procs) == 0 {
		return nil
	}
	return r.procs[len(r.procs)-1]
}

func (r *fakeLaunchRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.procs)
}

// --- test setup ----------------------------------------------------------

func testPlatform(id string, enabled bool) platform.Platform {
	now := time.Now()
	return platform.Platform{
		ID:          id,
		ProviderID:  platform.ProviderTwitch,
		DisplayName: "Test " + id,
		Enabled:     enabled,
		CreatedAt:   now,
		UpdatedAt:   now,
		Metadata:    platform.Metadata{Tags: []string{}, UpdatedAt: now},
	}
}

// newTestManager wires a Manager where everything is eligible by default for
// platform id: enabled, a valid output server, a stored key, a compatible
// FFmpeg, and MediaMTX ready with ingest receiving. Individual tests mutate
// one fake to introduce exactly the condition they are testing.
func newTestManager(t *testing.T, id string) (*Manager, *fakePlatforms, *fakeOutputs, *fakeCredentials, *fakeIngest, *fakeLaunchRecorder) {
	t.Helper()

	platforms := newFakePlatforms(testPlatform(id, true))
	outputs := newFakeOutputs()
	outputs.set(id, "rtmp://example.invalid/app")
	creds := newFakeCredentials()
	creds.setKey(id, "sk_live_test_key")
	ingest := readyIngest()
	recorder := &fakeLaunchRecorder{}

	m := NewManager(Options{
		Platforms:   platforms,
		Outputs:     outputs,
		Credentials: creds,
		FFmpeg:      compatibleFFmpeg(),
		Ingest:      ingest,
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	m.launchProcess = recorder.launcher()
	m.reconcileEvery = 20 * time.Millisecond
	m.refreshFFmpegEvery = time.Hour
	// A fast restart policy so backoff-dependent tests do not require tens
	// of real seconds; the policy's shape (bounded exponential backoff, a
	// cap per window, a stable-run reset) is what is under test, not the
	// production timing constants.
	m.policyMinBackoff = 5 * time.Millisecond
	m.policyMaxBackoff = 20 * time.Millisecond
	m.policyRestartWindow = 10 * time.Second
	m.policyStableRunDuration = time.Hour
	m.Start(context.Background())
	t.Cleanup(func() { m.Shutdown(context.Background()) })

	return m, platforms, outputs, creds, ingest, recorder
}

func waitForState(t *testing.T, m *Manager, id string, want State) Snapshot {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	var last Snapshot
	for time.Now().Before(deadline) {
		snaps, err := m.Snapshot(context.Background())
		if err != nil {
			t.Fatalf("Snapshot() error = %v", err)
		}
		for _, s := range snaps {
			if s.PlatformID == id {
				last = s
				if s.State == want {
					return s
				}
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("platform %s never reached state %s, last seen %+v", id, want, last)
	return Snapshot{}
}

func snapshotFor(t *testing.T, m *Manager, id string) Snapshot {
	t.Helper()
	snaps, err := m.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	for _, s := range snaps {
		if s.PlatformID == id {
			return s
		}
	}
	t.Fatalf("no snapshot for platform %s", id)
	return Snapshot{}
}

// --- initial state and blockers -------------------------------------------

func TestNewBranchStartsIdle(t *testing.T) {
	m, _, _, _, _, _ := newTestManager(t, "pf_1")
	snap := snapshotFor(t, m, "pf_1")
	if snap.State != StateIdle {
		t.Errorf("State = %s, want idle", snap.State)
	}
	if snap.DesiredRunning {
		t.Error("DesiredRunning = true for a branch that was never started")
	}
}

func TestStartReportsPlatformDisabledBlocker(t *testing.T) {
	m, platforms, _, _, _, _ := newTestManager(t, "pf_1")
	platforms.setEnabled("pf_1", false)

	outcome, err := m.StartBranch(context.Background(), "pf_1")
	if err != nil {
		t.Fatalf("StartBranch() error = %v", err)
	}
	if outcome.Accepted {
		t.Fatal("a disabled platform was accepted")
	}
	if !containsStr(outcome.Blockers, BlockerPlatformDisabled) {
		t.Errorf("blockers = %v, want %s", outcome.Blockers, BlockerPlatformDisabled)
	}
}

func TestStartReportsOutputServerMissingBlocker(t *testing.T) {
	m, _, outputs, _, _, _ := newTestManager(t, "pf_1")
	outputs.set("pf_1", "")

	outcome, _ := m.StartBranch(context.Background(), "pf_1")
	if !containsStr(outcome.Blockers, BlockerOutputServerMissing) {
		t.Errorf("blockers = %v, want %s", outcome.Blockers, BlockerOutputServerMissing)
	}
}

func TestStartReportsStreamKeyMissingBlocker(t *testing.T) {
	m, _, _, creds, _, _ := newTestManager(t, "pf_1")
	creds.mu.Lock()
	delete(creds.keys, "pf_1")
	creds.mu.Unlock()

	outcome, _ := m.StartBranch(context.Background(), "pf_1")
	if !containsStr(outcome.Blockers, BlockerStreamKeyMissing) {
		t.Errorf("blockers = %v, want %s", outcome.Blockers, BlockerStreamKeyMissing)
	}
}

func TestStartReportsCredentialStoreUnavailableBlocker(t *testing.T) {
	m, _, _, creds, _, _ := newTestManager(t, "pf_1")
	creds.mu.Lock()
	creds.storeAvailable = false
	creds.mu.Unlock()

	outcome, _ := m.StartBranch(context.Background(), "pf_1")
	if !containsStr(outcome.Blockers, BlockerCredentialUnavailable) {
		t.Errorf("blockers = %v, want %s", outcome.Blockers, BlockerCredentialUnavailable)
	}
	// Unavailable is a different, more specific fact than merely missing.
	if containsStr(outcome.Blockers, BlockerStreamKeyMissing) {
		t.Errorf("blockers = %v, should not also claim the key is missing", outcome.Blockers)
	}
}

func TestStartReportsFFmpegMissingBlocker(t *testing.T) {
	id := "pf_1"
	platforms := newFakePlatforms(testPlatform(id, true))
	outputs := newFakeOutputs()
	outputs.set(id, "rtmp://example.invalid/app")
	creds := newFakeCredentials()
	creds.setKey(id, "sk_live_test_key")

	m := NewManager(Options{
		Platforms: platforms, Outputs: outputs, Credentials: creds,
		FFmpeg: &fakeFFmpeg{resolution: ffmpeg.Resolution{Source: ffmpeg.SourceMissing}},
		Ingest: readyIngest(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	m.launchProcess = (&fakeLaunchRecorder{}).launcher()
	m.reconcileEvery = time.Hour
	m.Start(context.Background())
	t.Cleanup(func() { m.Shutdown(context.Background()) })

	outcome, _ := m.StartBranch(context.Background(), id)
	if !containsStr(outcome.Blockers, BlockerFFmpegMissing) {
		t.Errorf("blockers = %v, want %s", outcome.Blockers, BlockerFFmpegMissing)
	}
}

func TestStartReportsFFmpegIncompatibleBlocker(t *testing.T) {
	id := "pf_1"
	platforms := newFakePlatforms(testPlatform(id, true))
	outputs := newFakeOutputs()
	outputs.set(id, "rtmp://example.invalid/app")
	creds := newFakeCredentials()
	creds.setKey(id, "sk_live_test_key")

	m := NewManager(Options{
		Platforms: platforms, Outputs: outputs, Credentials: creds,
		FFmpeg: &fakeFFmpeg{resolution: ffmpeg.Resolution{Source: ffmpeg.SourcePath, Compatible: false}},
		Ingest: readyIngest(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	m.launchProcess = (&fakeLaunchRecorder{}).launcher()
	m.reconcileEvery = time.Hour
	m.Start(context.Background())
	t.Cleanup(func() { m.Shutdown(context.Background()) })

	outcome, _ := m.StartBranch(context.Background(), id)
	if !containsStr(outcome.Blockers, BlockerFFmpegIncompatible) {
		t.Errorf("blockers = %v, want %s", outcome.Blockers, BlockerFFmpegIncompatible)
	}
}

func TestStartReportsMediaMTXNotReadyBlocker(t *testing.T) {
	m, _, _, _, ingest, _ := newTestManager(t, "pf_1")
	ingest.mu.Lock()
	ingest.snapshot.MediaMTX.State = mediamtx.StateStopped
	ingest.mu.Unlock()

	outcome, _ := m.StartBranch(context.Background(), "pf_1")
	if !containsStr(outcome.Blockers, BlockerMediaMTXNotReady) {
		t.Errorf("blockers = %v, want %s", outcome.Blockers, BlockerMediaMTXNotReady)
	}
}

func TestStartReportsIngestNotReceivingBlocker(t *testing.T) {
	m, _, _, _, ingest, _ := newTestManager(t, "pf_1")
	ingest.setIngestState(mediamtx.IngestWaiting)

	outcome, _ := m.StartBranch(context.Background(), "pf_1")
	if !containsStr(outcome.Blockers, BlockerIngestNotReceiving) {
		t.Errorf("blockers = %v, want %s", outcome.Blockers, BlockerIngestNotReceiving)
	}
}

func TestStartOnAnUnknownPlatformReturnsNotFound(t *testing.T) {
	m, _, _, _, _, _ := newTestManager(t, "pf_1")
	_, err := m.StartBranch(context.Background(), "pf_does_not_exist")
	if err != ErrNotFound {
		t.Errorf("StartBranch() error = %v, want ErrNotFound", err)
	}
}

// --- start / live / stop ---------------------------------------------------

func TestStartTransitionsToStartingThenLiveOnlyAfterAdvancingProgress(t *testing.T) {
	m, _, _, _, _, recorder := newTestManager(t, "pf_1")

	outcome, err := m.StartBranch(context.Background(), "pf_1")
	if err != nil || !outcome.Accepted {
		t.Fatalf("StartBranch() = %+v, %v", outcome, err)
	}

	snap := snapshotFor(t, m, "pf_1")
	if snap.State != StateStarting {
		t.Fatalf("State immediately after Start = %s, want starting", snap.State)
	}

	proc := recorder.latest()
	if proc == nil {
		t.Fatal("no process was launched")
	}

	// An all-zero first tick must not flip the branch to live.
	proc.report(Progress{FrameCount: -1})
	snap = snapshotFor(t, m, "pf_1")
	if snap.State != StateStarting {
		t.Fatalf("State after a non-advancing progress tick = %s, want still starting", snap.State)
	}

	proc.report(Progress{OutTimeMs: 1000, FrameCount: 10})
	snap = snapshotFor(t, m, "pf_1")
	if snap.State != StateLive {
		t.Fatalf("State after advancing progress = %s, want live", snap.State)
	}
	if snap.LiveAt == "" {
		t.Error("LiveAt was not set once the branch went live")
	}
	if snap.Progress == nil || snap.Progress.OutTimeMs != 1000 {
		t.Errorf("Progress = %+v, want the last reported block", snap.Progress)
	}
}

func TestExplicitStopTerminatesTheProcessAndClearsDesiredRunning(t *testing.T) {
	m, _, _, _, _, recorder := newTestManager(t, "pf_1")

	if _, err := m.StartBranch(context.Background(), "pf_1"); err != nil {
		t.Fatalf("StartBranch() error = %v", err)
	}
	proc := recorder.latest()

	if err := m.StopBranch(context.Background(), "pf_1"); err != nil {
		t.Fatalf("StopBranch() error = %v", err)
	}

	select {
	case <-proc.Exited():
	case <-time.After(time.Second):
		t.Fatal("stop() did not terminate the process")
	}

	snap := waitForState(t, m, "pf_1", StateIdle)
	if snap.DesiredRunning {
		t.Error("DesiredRunning = true after an explicit stop")
	}
}

func TestStopOnAnIdleBranchReturnsNotRunning(t *testing.T) {
	m, _, _, _, _, _ := newTestManager(t, "pf_1")
	err := m.StopBranch(context.Background(), "pf_1")
	if err != ErrNotRunning {
		t.Errorf("StopBranch() error = %v, want ErrNotRunning", err)
	}
}

func TestStartWhileAlreadyStartingIsAConflict(t *testing.T) {
	m, _, _, _, _, _ := newTestManager(t, "pf_1")

	if _, err := m.StartBranch(context.Background(), "pf_1"); err != nil {
		t.Fatalf("first StartBranch() error = %v", err)
	}

	outcome, err := m.StartBranch(context.Background(), "pf_1")
	if err != nil {
		t.Fatalf("second StartBranch() error = %v", err)
	}
	if !outcome.Conflict {
		t.Error("a concurrent Start on an already-running branch was accepted")
	}
}

func TestRestartStopsThenStarts(t *testing.T) {
	m, _, _, _, _, recorder := newTestManager(t, "pf_1")

	if _, err := m.StartBranch(context.Background(), "pf_1"); err != nil {
		t.Fatalf("StartBranch() error = %v", err)
	}
	first := recorder.latest()

	outcome, err := m.RestartBranch(context.Background(), "pf_1")
	if err != nil {
		t.Fatalf("RestartBranch() error = %v", err)
	}
	if !outcome.Accepted {
		t.Fatalf("Restart outcome = %+v, want accepted", outcome)
	}

	select {
	case <-first.Exited():
	default:
		t.Error("the original process was not stopped by Restart")
	}

	if recorder.count() != 2 {
		t.Errorf("launched %d processes, want 2 (one for Start, one for Restart)", recorder.count())
	}
}

// --- isolation between branches ---------------------------------------------

func TestTwoBranchesGetIndependentProcesses(t *testing.T) {
	id1, id2 := "pf_1", "pf_2"
	platforms := newFakePlatforms(testPlatform(id1, true), testPlatform(id2, true))
	outputs := newFakeOutputs()
	outputs.set(id1, "rtmp://example.invalid/app1")
	outputs.set(id2, "rtmp://example.invalid/app2")
	creds := newFakeCredentials()
	creds.setKey(id1, "sk_live_one")
	creds.setKey(id2, "sk_live_two")
	recorder := &fakeLaunchRecorder{}

	m := NewManager(Options{
		Platforms: platforms, Outputs: outputs, Credentials: creds,
		FFmpeg: compatibleFFmpeg(), Ingest: readyIngest(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	m.launchProcess = recorder.launcher()
	m.reconcileEvery = time.Hour
	m.Start(context.Background())
	t.Cleanup(func() { m.Shutdown(context.Background()) })

	if _, err := m.StartBranch(context.Background(), id1); err != nil {
		t.Fatalf("StartBranch(%s) error = %v", id1, err)
	}
	if _, err := m.StartBranch(context.Background(), id2); err != nil {
		t.Fatalf("StartBranch(%s) error = %v", id2, err)
	}

	if recorder.count() != 2 {
		t.Fatalf("launched %d processes, want 2", recorder.count())
	}

	// Crashing one must not disturb the other.
	recorder.procs[0].crash()
	waitForState(t, m, id1, StateRestarting)

	snap2 := snapshotFor(t, m, id2)
	if snap2.State == StateRestarting || snap2.State == StateError {
		t.Errorf("platform %s's state changed because of a different branch's crash: %s", id2, snap2.State)
	}
}

// --- restart policy ----------------------------------------------------

func TestUnexpectedExitTriggersARestart(t *testing.T) {
	m, _, _, _, _, recorder := newTestManager(t, "pf_1")

	if _, err := m.StartBranch(context.Background(), "pf_1"); err != nil {
		t.Fatalf("StartBranch() error = %v", err)
	}
	first := recorder.latest()
	first.crash()

	// The manager schedules the relaunch after a backoff delay; wait for
	// the second process to appear. The intermediate "restarting" state is
	// not asserted directly here - with a short test backoff it can already
	// have moved on to "starting" by the time this polls, which would make
	// the assertion flaky without testing anything the process count and
	// restart count below do not already cover.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && recorder.count() < 2 {
		time.Sleep(10 * time.Millisecond)
	}
	if recorder.count() < 2 {
		t.Fatal("no restart was launched after an unexpected exit")
	}
	if snapshotFor(t, m, "pf_1").RestartCount < 1 {
		t.Error("RestartCount was not incremented")
	}
}

func TestRestartLimitEntersErrorAndStopsRetrying(t *testing.T) {
	m, _, _, _, _, recorder := newTestManager(t, "pf_1")
	if _, err := m.StartBranch(context.Background(), "pf_1"); err != nil {
		t.Fatalf("StartBranch() error = %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		snap := snapshotFor(t, m, "pf_1")
		if snap.State == StateError {
			if snap.LastError == nil || snap.LastError.Code != CodeRestartLimit {
				t.Errorf("LastError = %+v, want code %s", snap.LastError, CodeRestartLimit)
			}
			if snap.DesiredRunning {
				t.Error("DesiredRunning = true after the restart limit was reached")
			}
			return
		}
		if proc := recorder.latest(); proc != nil {
			select {
			case <-proc.Exited():
			default:
				proc.crash()
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the branch never reached the error state after repeated crashes")
}

// --- ingest loss and resume -------------------------------------------------

func TestIngestLossTransitionsToWaitingForIngestAndRetainsDesire(t *testing.T) {
	m, _, _, _, ingest, recorder := newTestManager(t, "pf_1")

	if _, err := m.StartBranch(context.Background(), "pf_1"); err != nil {
		t.Fatalf("StartBranch() error = %v", err)
	}
	proc := recorder.latest()
	proc.report(Progress{OutTimeMs: 1000})
	waitForState(t, m, "pf_1", StateLive)

	ingest.setIngestState(mediamtx.IngestWaiting)
	proc.crash() // the input disappearing is what makes ffmpeg's own read end

	snap := waitForState(t, m, "pf_1", StateWaitingForIngest)
	if !snap.DesiredRunning {
		t.Error("DesiredRunning = false while waiting for ingest to return - it should be retained")
	}
}

func TestIngestReturnResumesADesiredBranch(t *testing.T) {
	m, _, _, _, ingest, recorder := newTestManager(t, "pf_1")

	if _, err := m.StartBranch(context.Background(), "pf_1"); err != nil {
		t.Fatalf("StartBranch() error = %v", err)
	}
	proc := recorder.latest()
	proc.report(Progress{OutTimeMs: 1000})
	waitForState(t, m, "pf_1", StateLive)

	ingest.setIngestState(mediamtx.IngestWaiting)
	proc.crash()
	waitForState(t, m, "pf_1", StateWaitingForIngest)

	ingest.setIngestState(mediamtx.IngestReceiving)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && recorder.count() < 2 {
		time.Sleep(10 * time.Millisecond)
	}
	if recorder.count() < 2 {
		t.Fatal("the branch was not relaunched once ingest returned")
	}
}

func TestExplicitStopWhileWaitingForIngestSuppressesResume(t *testing.T) {
	m, _, _, _, ingest, recorder := newTestManager(t, "pf_1")

	if _, err := m.StartBranch(context.Background(), "pf_1"); err != nil {
		t.Fatalf("StartBranch() error = %v", err)
	}
	proc := recorder.latest()
	proc.report(Progress{OutTimeMs: 1000})
	waitForState(t, m, "pf_1", StateLive)

	ingest.setIngestState(mediamtx.IngestWaiting)
	proc.crash()
	waitForState(t, m, "pf_1", StateWaitingForIngest)

	if err := m.StopBranch(context.Background(), "pf_1"); err != nil {
		t.Fatalf("StopBranch() error = %v", err)
	}

	ingest.setIngestState(mediamtx.IngestReceiving)
	time.Sleep(200 * time.Millisecond)

	snap := snapshotFor(t, m, "pf_1")
	if snap.State != StateIdle {
		t.Errorf("State = %s, want idle - an explicit stop while waiting must not resume", snap.State)
	}
	if recorder.count() != 1 {
		t.Errorf("launched %d processes, want 1 (no resume after an explicit stop)", recorder.count())
	}
}

// --- shutdown ------------------------------------------------------------

func TestShutdownStopsEveryRunningBranch(t *testing.T) {
	id1, id2 := "pf_1", "pf_2"
	platforms := newFakePlatforms(testPlatform(id1, true), testPlatform(id2, true))
	outputs := newFakeOutputs()
	outputs.set(id1, "rtmp://example.invalid/app1")
	outputs.set(id2, "rtmp://example.invalid/app2")
	creds := newFakeCredentials()
	creds.setKey(id1, "sk_live_one")
	creds.setKey(id2, "sk_live_two")
	recorder := &fakeLaunchRecorder{}

	m := NewManager(Options{
		Platforms: platforms, Outputs: outputs, Credentials: creds,
		FFmpeg: compatibleFFmpeg(), Ingest: readyIngest(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	m.launchProcess = recorder.launcher()
	m.reconcileEvery = time.Hour
	m.Start(context.Background())

	if _, err := m.StartBranch(context.Background(), id1); err != nil {
		t.Fatalf("StartBranch(%s) error = %v", id1, err)
	}
	if _, err := m.StartBranch(context.Background(), id2); err != nil {
		t.Fatalf("StartBranch(%s) error = %v", id2, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	m.Shutdown(ctx)

	for _, proc := range recorder.procs {
		select {
		case <-proc.Exited():
		default:
			t.Error("a process was still running after Shutdown")
		}
	}
}

// --- secret safety -----------------------------------------------------

func TestSnapshotNeverContainsTheStreamKeyOrDestinationURL(t *testing.T) {
	m, _, outputs, creds, _, recorder := newTestManager(t, "pf_1")
	outputs.set("pf_1", "rtmp://example.invalid/very-specific-app-path")
	creds.setKey("pf_1", "sk_live_should_never_appear_anywhere")

	if _, err := m.StartBranch(context.Background(), "pf_1"); err != nil {
		t.Fatalf("StartBranch() error = %v", err)
	}
	recorder.latest().report(Progress{OutTimeMs: 1000})
	waitForState(t, m, "pf_1", StateLive)

	encoded, err := json.Marshal(snapshotFor(t, m, "pf_1"))
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	text := string(encoded)
	if strings.Contains(text, "sk_live_should_never_appear_anywhere") {
		t.Fatalf("snapshot leaked the stream key: %s", text)
	}
	if strings.Contains(text, "very-specific-app-path") {
		t.Fatalf("snapshot leaked the destination server address: %s", text)
	}
}

func containsStr(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
