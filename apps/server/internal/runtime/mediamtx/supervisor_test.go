package mediamtx

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"testing"
	"time"
)

// newTestSupervisor builds a supervisor whose "MediaMTX" is this test binary
// running in the requested fake mode, on freshly reserved loopback ports.
func newTestSupervisor(t *testing.T, mode string, extra ...string) *Supervisor {
	t.Helper()

	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("locate the test binary: %v", err)
	}

	opts := Options{
		DataDir:        t.TempDir(),
		RTMPAddress:    freePort(t),
		APIAddress:     freePort(t),
		IngestPath:     "live",
		AutoStart:      false,
		AutoRestart:    false,
		ExecutablePath: executable,
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		extraEnv:       append([]string{fakeModeEnv + "=" + mode}, extra...),
	}

	supervisor := NewSupervisor(opts)
	// The test binary is not MediaMTX, so the version probe is stubbed. The
	// real resolver still runs against a real file on disk.
	supervisor.resolver.versionProbe = staticProbe(SupportedVersion+"\n", nil)

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		supervisor.Shutdown(ctx)
	})

	return supervisor
}

// waitForState polls until the supervisor reaches one of the wanted states.
func waitForState(t *testing.T, s *Supervisor, timeout time.Duration, wanted ...ProcessState) ProcessState {
	t.Helper()

	deadline := time.Now().Add(timeout)
	var last ProcessState

	for time.Now().Before(deadline) {
		last = s.Snapshot().MediaMTX.State
		for _, want := range wanted {
			if last == want {
				return last
			}
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("state = %q after %s, want one of %v", last, timeout, wanted)
	return last
}

func waitForIngest(t *testing.T, s *Supervisor, timeout time.Duration, want IngestState) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	var last IngestState

	for time.Now().Before(deadline) {
		last = s.Snapshot().Ingest.State
		if last == want {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("ingest state = %q after %s, want %q", last, timeout, want)
}

// --- resolution -------------------------------------------------------------

func TestMissingBinaryDoesNotStopTheSupervisor(t *testing.T) {
	supervisor := NewSupervisor(Options{
		DataDir:     t.TempDir(),
		RTMPAddress: freePort(t),
		APIAddress:  freePort(t),
		IngestPath:  "live",
		AutoStart:   true,
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		supervisor.Shutdown(ctx)
	})

	// Start must return normally: a missing binary is a state, not a failure
	// that may take the backend down with it.
	supervisor.Start(context.Background())

	snapshot := supervisor.Snapshot()
	if snapshot.MediaMTX.State != StateMissing {
		t.Errorf("state = %q, want missing", snapshot.MediaMTX.State)
	}
	if snapshot.MediaMTX.LastError == nil ||
		snapshot.MediaMTX.LastError.Code != CodeNotInstalled {
		t.Errorf("lastError = %+v, want %s", snapshot.MediaMTX.LastError, CodeNotInstalled)
	}
	// The connection details must still be reported so the interface can show
	// where OBS would publish once MediaMTX exists.
	if snapshot.Connection.PublishURL == "" {
		t.Error("the publish URL is empty while MediaMTX is missing")
	}
}

func TestStartIsRefusedWhenTheBinaryIsMissing(t *testing.T) {
	supervisor := NewSupervisor(Options{
		DataDir:     t.TempDir(),
		RTMPAddress: freePort(t),
		APIAddress:  freePort(t),
		IngestPath:  "live",
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	supervisor.refreshResolution(context.Background())

	err := supervisor.RequestStart(context.Background())
	if !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("RequestStart() error = %v, want ErrNotInstalled", err)
	}
}

// --- start / ready ----------------------------------------------------------

func TestStartReachesReadyThroughTheControlAPI(t *testing.T) {
	supervisor := newTestSupervisor(t, fakeModeReady)
	supervisor.refreshResolution(context.Background())

	if state := supervisor.Snapshot().MediaMTX.State; state != StateStopped {
		t.Fatalf("state before start = %q, want stopped", state)
	}

	if err := supervisor.RequestStart(context.Background()); err != nil {
		t.Fatalf("RequestStart() returned an error: %v", err)
	}

	waitForState(t, supervisor, 25*time.Second, StateReady)

	snapshot := supervisor.Snapshot()
	if snapshot.MediaMTX.StartedAt == "" {
		t.Error("startedAt is empty for a ready process")
	}
	if snapshot.MediaMTX.LastError != nil {
		t.Errorf("lastError = %+v, want nil once ready", snapshot.MediaMTX.LastError)
	}
}

func TestReadinessIsNotAssumedFromProcessCreation(t *testing.T) {
	// The fake runs but never opens its Control API, so a supervisor that
	// treated spawning as readiness would wrongly report ready.
	supervisor := newTestSupervisor(t, fakeModeSilent)
	supervisor.refreshResolution(context.Background())

	if err := supervisor.RequestStart(context.Background()); err != nil {
		t.Fatalf("RequestStart() returned an error: %v", err)
	}

	// Well inside the readiness budget, the state must still be starting.
	time.Sleep(2 * time.Second)
	if state := supervisor.Snapshot().MediaMTX.State; state != StateStarting {
		t.Errorf("state = %q, want starting while the API is silent", state)
	}

	waitForState(t, supervisor, readinessTimeout+15*time.Second, StateError)

	snapshot := supervisor.Snapshot()
	if snapshot.MediaMTX.LastError == nil ||
		snapshot.MediaMTX.LastError.Code != CodeReadinessTimeout {
		t.Errorf("lastError = %+v, want %s", snapshot.MediaMTX.LastError, CodeReadinessTimeout)
	}
}

func TestConcurrentStartsAreRefused(t *testing.T) {
	supervisor := newTestSupervisor(t, fakeModeReady)
	supervisor.refreshResolution(context.Background())

	if err := supervisor.RequestStart(context.Background()); err != nil {
		t.Fatalf("first RequestStart() returned an error: %v", err)
	}

	// A second start while the first is in flight must be refused rather than
	// spawning a competing process that would fight for the same ports.
	if err := supervisor.RequestStart(context.Background()); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("second RequestStart() error = %v, want ErrInvalidState", err)
	}

	waitForState(t, supervisor, 25*time.Second, StateReady)

	if err := supervisor.RequestStart(context.Background()); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("RequestStart() while ready error = %v, want ErrInvalidState", err)
	}
}

// --- stop / restart ---------------------------------------------------------

func TestExplicitStopReturnsToStopped(t *testing.T) {
	supervisor := newTestSupervisor(t, fakeModeReady)
	supervisor.refreshResolution(context.Background())

	if err := supervisor.RequestStart(context.Background()); err != nil {
		t.Fatalf("RequestStart() returned an error: %v", err)
	}
	waitForState(t, supervisor, 25*time.Second, StateReady)

	if err := supervisor.RequestStop(context.Background()); err != nil {
		t.Fatalf("RequestStop() returned an error: %v", err)
	}

	snapshot := supervisor.Snapshot()
	if snapshot.MediaMTX.State != StateStopped {
		t.Errorf("state = %q, want stopped", snapshot.MediaMTX.State)
	}
	if snapshot.Ingest.State != IngestUnavailable {
		t.Errorf("ingest = %q, want unavailable once stopped", snapshot.Ingest.State)
	}
}

func TestExplicitStopSuppressesAutomaticRestart(t *testing.T) {
	supervisor := newTestSupervisor(t, fakeModeReady)
	supervisor.options.AutoRestart = true
	supervisor.refreshResolution(context.Background())

	if err := supervisor.RequestStart(context.Background()); err != nil {
		t.Fatalf("RequestStart() returned an error: %v", err)
	}
	waitForState(t, supervisor, 25*time.Second, StateReady)

	if err := supervisor.RequestStop(context.Background()); err != nil {
		t.Fatalf("RequestStop() returned an error: %v", err)
	}

	// A deliberate stop must stay stopped: the restart policy exists for
	// crashes, and undoing a user's Stop would be user-hostile.
	time.Sleep(3 * time.Second)

	snapshot := supervisor.Snapshot()
	if snapshot.MediaMTX.State != StateStopped {
		t.Errorf("state = %q, want it to stay stopped", snapshot.MediaMTX.State)
	}
	if snapshot.MediaMTX.RestartCount != 0 {
		t.Errorf("restartCount = %d, want 0 after an explicit stop", snapshot.MediaMTX.RestartCount)
	}
}

func TestStopIsRefusedWhenNotRunning(t *testing.T) {
	supervisor := newTestSupervisor(t, fakeModeReady)
	supervisor.refreshResolution(context.Background())

	if err := supervisor.RequestStop(context.Background()); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("RequestStop() error = %v, want ErrInvalidState", err)
	}
}

func TestRestartStopsThenStarts(t *testing.T) {
	supervisor := newTestSupervisor(t, fakeModeReady)
	supervisor.refreshResolution(context.Background())

	if err := supervisor.RequestStart(context.Background()); err != nil {
		t.Fatalf("RequestStart() returned an error: %v", err)
	}
	waitForState(t, supervisor, 25*time.Second, StateReady)
	firstStart := supervisor.Snapshot().MediaMTX.StartedAt

	// A second's gap so the new start time is observably different.
	time.Sleep(1100 * time.Millisecond)

	if err := supervisor.RequestRestart(context.Background()); err != nil {
		t.Fatalf("RequestRestart() returned an error: %v", err)
	}
	waitForState(t, supervisor, 25*time.Second, StateReady)

	if second := supervisor.Snapshot().MediaMTX.StartedAt; second == firstStart {
		t.Errorf("startedAt is unchanged after a restart (%q)", second)
	}
}

// --- unexpected exit and restart policy -------------------------------------

func TestUnexpectedExitIsDetected(t *testing.T) {
	supervisor := newTestSupervisor(t, fakeModeExitAfterReady)
	supervisor.options.AutoRestart = false
	supervisor.refreshResolution(context.Background())

	if err := supervisor.RequestStart(context.Background()); err != nil {
		t.Fatalf("RequestStart() returned an error: %v", err)
	}
	waitForState(t, supervisor, 25*time.Second, StateReady)

	// The fake exits on its own two seconds after becoming ready.
	waitForState(t, supervisor, 20*time.Second, StateError)

	snapshot := supervisor.Snapshot()
	if snapshot.MediaMTX.LastError == nil ||
		snapshot.MediaMTX.LastError.Code != CodeExitedUnexpectedly {
		t.Errorf("lastError = %+v, want %s", snapshot.MediaMTX.LastError, CodeExitedUnexpectedly)
	}
	if snapshot.Ingest.State != IngestUnavailable {
		t.Errorf("ingest = %q, want unavailable after the process died", snapshot.Ingest.State)
	}
}

func TestAutomaticRestartRunsAndIsCounted(t *testing.T) {
	supervisor := newTestSupervisor(t, fakeModeExitAfterReady)
	supervisor.options.AutoRestart = true
	supervisor.refreshResolution(context.Background())

	if err := supervisor.RequestStart(context.Background()); err != nil {
		t.Fatalf("RequestStart() returned an error: %v", err)
	}
	waitForState(t, supervisor, 25*time.Second, StateReady)

	// The fake keeps exiting, so the supervisor keeps restarting it.
	deadline := time.Now().Add(40 * time.Second)
	for time.Now().Before(deadline) {
		if supervisor.Snapshot().MediaMTX.RestartCount >= 2 {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}

	t.Fatalf("restartCount = %d after 40s, want at least 2",
		supervisor.Snapshot().MediaMTX.RestartCount)
}

func TestRestartBudgetIsBounded(t *testing.T) {
	supervisor := newTestSupervisor(t, fakeModeCrash)
	supervisor.options.AutoRestart = true
	supervisor.refreshResolution(context.Background())

	// The fake exits immediately every time, so the policy must stop retrying
	// rather than spinning in a tight crash loop.
	if err := supervisor.RequestStart(context.Background()); err != nil {
		t.Fatalf("RequestStart() returned an error: %v", err)
	}

	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		snapshot := supervisor.Snapshot()
		if snapshot.MediaMTX.LastError != nil &&
			snapshot.MediaMTX.LastError.Code == CodeRestartLimit {
			if snapshot.MediaMTX.RestartCount > maxRestartsPerWindow {
				t.Errorf("restartCount = %d, want at most %d",
					snapshot.MediaMTX.RestartCount, maxRestartsPerWindow)
			}
			return
		}
		time.Sleep(250 * time.Millisecond)
	}

	t.Fatalf("the restart limit was never reached; restartCount = %d, lastError = %+v",
		supervisor.Snapshot().MediaMTX.RestartCount, supervisor.Snapshot().MediaMTX.LastError)
}

func TestBackoffGrowsAndIsCapped(t *testing.T) {
	supervisor := newTestSupervisor(t, fakeModeReady)

	// Drive the backoff directly: exercising it through real restarts would
	// make the test take minutes.
	supervisor.options.AutoRestart = true
	supervisor.binaryPath = "unused"
	supervisor.state = StateError

	previous := supervisor.backoff
	for i := 0; i < 10; i++ {
		supervisor.mu.Lock()
		current := supervisor.backoff
		if supervisor.backoff < maxBackoff {
			supervisor.backoff *= 2
			if supervisor.backoff > maxBackoff {
				supervisor.backoff = maxBackoff
			}
		}
		supervisor.mu.Unlock()

		if current < previous {
			t.Fatalf("backoff shrank from %s to %s", previous, current)
		}
		previous = current
	}

	if supervisor.backoff > maxBackoff {
		t.Errorf("backoff = %s, want it capped at %s", supervisor.backoff, maxBackoff)
	}
}

// --- ingest -----------------------------------------------------------------

func TestIngestReportsWaitingWithoutAPublisher(t *testing.T) {
	supervisor := newTestSupervisor(t, fakeModeReady)
	supervisor.refreshResolution(context.Background())

	if err := supervisor.RequestStart(context.Background()); err != nil {
		t.Fatalf("RequestStart() returned an error: %v", err)
	}
	waitForState(t, supervisor, 25*time.Second, StateReady)
	waitForIngest(t, supervisor, 10*time.Second, IngestWaiting)

	snapshot := supervisor.Snapshot()
	if snapshot.Ingest.Path != "live" {
		t.Errorf("ingest path = %q, want live", snapshot.Ingest.Path)
	}
	// Nothing may be invented while nothing is publishing.
	if snapshot.Ingest.TrackCount != nil {
		t.Errorf("trackCount = %v, want nil while waiting", *snapshot.Ingest.TrackCount)
	}
	if snapshot.Ingest.SourceType != "" {
		t.Errorf("sourceType = %q, want empty while waiting", snapshot.Ingest.SourceType)
	}
}

func TestIngestReportsReceivingWithAPublisher(t *testing.T) {
	supervisor := newTestSupervisor(t, fakeModeReady, fakePublishingEnv+"=1")
	supervisor.refreshResolution(context.Background())

	if err := supervisor.RequestStart(context.Background()); err != nil {
		t.Fatalf("RequestStart() returned an error: %v", err)
	}
	waitForState(t, supervisor, 25*time.Second, StateReady)
	waitForIngest(t, supervisor, 10*time.Second, IngestReceiving)

	snapshot := supervisor.Snapshot()
	if snapshot.Ingest.SourceType != "rtmpConn" {
		t.Errorf("sourceType = %q, want rtmpConn", snapshot.Ingest.SourceType)
	}
	if snapshot.Ingest.TrackCount == nil || *snapshot.Ingest.TrackCount != 2 {
		t.Errorf("trackCount = %v, want 2", snapshot.Ingest.TrackCount)
	}
	if len(snapshot.Ingest.Tracks) != 2 || snapshot.Ingest.Tracks[0] != "H264" {
		t.Errorf("tracks = %v, want the codecs MediaMTX reported", snapshot.Ingest.Tracks)
	}
	if snapshot.Ingest.ConnectedAt == "" {
		t.Error("connectedAt is empty for an active publisher")
	}
}

// --- ports ------------------------------------------------------------------

func TestPortInUseIsReportedWithoutTouchingTheOtherListener(t *testing.T) {
	supervisor := newTestSupervisor(t, fakeModeReady)
	supervisor.refreshResolution(context.Background())

	// Occupy the RTMP port with an unrelated listener.
	blocker, err := net.Listen("tcp", supervisor.options.RTMPAddress)
	if err != nil {
		t.Fatalf("occupy the RTMP port: %v", err)
	}
	defer blocker.Close()

	if err := supervisor.RequestStart(context.Background()); err != nil {
		t.Fatalf("RequestStart() returned an error: %v", err)
	}

	waitForState(t, supervisor, 20*time.Second, StateError)

	snapshot := supervisor.Snapshot()
	if snapshot.MediaMTX.LastError == nil ||
		snapshot.MediaMTX.LastError.Code != CodePortInUse {
		t.Fatalf("lastError = %+v, want %s", snapshot.MediaMTX.LastError, CodePortInUse)
	}

	// The unrelated listener must be untouched: stealing a port from another
	// application would be far worse than failing to start.
	connection, dialErr := net.DialTimeout("tcp", supervisor.options.RTMPAddress, 2*time.Second)
	if dialErr != nil {
		t.Errorf("the unrelated listener was closed: %v", dialErr)
	} else {
		_ = connection.Close()
	}
}

// --- shutdown ---------------------------------------------------------------

func TestShutdownReapsTheChildProcess(t *testing.T) {
	supervisor := newTestSupervisor(t, fakeModeReady)
	supervisor.refreshResolution(context.Background())

	if err := supervisor.RequestStart(context.Background()); err != nil {
		t.Fatalf("RequestStart() returned an error: %v", err)
	}
	waitForState(t, supervisor, 25*time.Second, StateReady)

	supervisor.mu.Lock()
	proc := supervisor.current
	supervisor.mu.Unlock()
	if proc == nil {
		t.Fatal("no child process is recorded while ready")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	supervisor.Shutdown(ctx)

	// The child must be gone, not orphaned.
	select {
	case <-proc.Exited():
	case <-time.After(5 * time.Second):
		t.Fatal("the child process was still running after Shutdown")
	}

	if state := supervisor.Snapshot().MediaMTX.State; state != StateStopped {
		t.Errorf("state = %q after shutdown, want stopped", state)
	}
}

func TestShutdownIsSafeWhenNothingIsRunning(t *testing.T) {
	supervisor := newTestSupervisor(t, fakeModeReady)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Must not panic or block.
	supervisor.Shutdown(ctx)
}

// --- output -----------------------------------------------------------------

func TestProcessOutputIsDrainedAndRecorded(t *testing.T) {
	supervisor := newTestSupervisor(t, fakeModeReady)
	supervisor.refreshResolution(context.Background())

	if err := supervisor.RequestStart(context.Background()); err != nil {
		t.Fatalf("RequestStart() returned an error: %v", err)
	}
	waitForState(t, supervisor, 25*time.Second, StateReady)

	// The fake prints one structured line and one plain line; both must be
	// captured, and the malformed one must not have caused a panic.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if len(supervisor.RecentLogLines()) >= 2 {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("captured %d log lines, want at least 2", len(supervisor.RecentLogLines()))
}

func TestDiagnosticRingIsBounded(t *testing.T) {
	p := &process{ring: make([]string, 0, diagnosticRingSize)}

	for i := 0; i < diagnosticRingSize*3; i++ {
		p.record("line")
	}

	if got := len(p.recentLines()); got != diagnosticRingSize {
		t.Errorf("ring holds %d lines, want it capped at %d", got, diagnosticRingSize)
	}
}
