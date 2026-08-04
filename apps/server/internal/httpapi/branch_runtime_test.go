package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/runtime/branch"
	"github.com/streaming-tree/server/internal/runtime/ffmpeg"
)

// --- stubs -----------------------------------------------------------

type stubFFmpegRuntime struct {
	resolution ffmpeg.Resolution
}

func (s *stubFFmpegRuntime) FFmpegStatus() ffmpeg.Resolution { return s.resolution }

type stubBranches struct {
	snapshots     []branch.Snapshot
	snapshotErr   error
	startOutcome  branch.Outcome
	startErr      error
	stopErr       error
	restartOut    branch.Outcome
	restartErr    error
	startEnabled  []branch.StartEnabledResult
	stopAllCalled bool
	forgotten     []string
	calls         []string
}

func (s *stubBranches) Snapshot(context.Context) ([]branch.Snapshot, error) {
	return s.snapshots, s.snapshotErr
}

func (s *stubBranches) StartBranch(_ context.Context, id string) (branch.Outcome, error) {
	s.calls = append(s.calls, "start:"+id)
	return s.startOutcome, s.startErr
}

func (s *stubBranches) StopBranch(_ context.Context, id string) error {
	s.calls = append(s.calls, "stop:"+id)
	return s.stopErr
}

func (s *stubBranches) RestartBranch(_ context.Context, id string) (branch.Outcome, error) {
	s.calls = append(s.calls, "restart:"+id)
	return s.restartOut, s.restartErr
}

func (s *stubBranches) StartEnabled(context.Context) []branch.StartEnabledResult {
	return s.startEnabled
}

func (s *stubBranches) StopAll(context.Context) {
	s.stopAllCalled = true
}

func (s *stubBranches) Forget(_ context.Context, id string) {
	s.forgotten = append(s.forgotten, id)
}

func newBranchTestServer(t *testing.T, ffmpegSvc FFmpegRuntimeService, branchSvc BranchRuntimeService) http.Handler {
	t.Helper()
	return NewRouter(Options{
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		StartedAt:     time.Now(),
		FFmpegRuntime: ffmpegSvc,
		Branches:      branchSvc,
	})
}

// --- FFmpeg status --------------------------------------------------------

func TestGetFFmpegStatusReportsReady(t *testing.T) {
	stub := &stubFFmpegRuntime{resolution: ffmpeg.Resolution{
		Source: ffmpeg.SourcePath, Path: "/usr/bin/ffmpeg", Version: "8.1", Compatible: true,
		Capabilities: ffmpeg.Capabilities{RTMPInput: true, RTMPOutput: true, RTMPSOutput: true, FLVMuxer: true, Progress: true},
	}}
	handler := newBranchTestServer(t, stub, &stubBranches{})

	recorder := do(t, handler, http.MethodGet, "/api/runtime/ffmpeg", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", recorder.Code, recorder.Body.String())
	}

	var body struct {
		Version int `json:"version"`
		FFmpeg  struct {
			State           string `json:"state"`
			Source          string `json:"source"`
			DetectedVersion string `json:"detectedVersion"`
			MinimumVersion  string `json:"minimumVersion"`
		} `json:"ffmpeg"`
	}
	decodeBody(t, recorder, &body)

	if body.FFmpeg.State != "ready" {
		t.Errorf("State = %q, want ready", body.FFmpeg.State)
	}
	if body.FFmpeg.Source != "path" {
		t.Errorf("Source = %q, want path", body.FFmpeg.Source)
	}
	if body.FFmpeg.MinimumVersion != ffmpeg.MinimumVersion {
		t.Errorf("MinimumVersion = %q, want %q", body.FFmpeg.MinimumVersion, ffmpeg.MinimumVersion)
	}
}

func TestGetFFmpegStatusNeverExposesTheExecutablePath(t *testing.T) {
	stub := &stubFFmpegRuntime{resolution: ffmpeg.Resolution{
		Source: ffmpeg.SourcePath, Path: "/very/specific/local/path/to/ffmpeg", Compatible: true,
	}}
	handler := newBranchTestServer(t, stub, &stubBranches{})

	recorder := do(t, handler, http.MethodGet, "/api/runtime/ffmpeg", nil)
	if strings.Contains(recorder.Body.String(), "/very/specific/local/path/to/ffmpeg") {
		t.Fatalf("response leaked the executable path: %s", recorder.Body.String())
	}
}

func TestGetFFmpegStatusReportsMissing(t *testing.T) {
	stub := &stubFFmpegRuntime{resolution: ffmpeg.Resolution{Source: ffmpeg.SourceMissing}}
	handler := newBranchTestServer(t, stub, &stubBranches{})

	recorder := do(t, handler, http.MethodGet, "/api/runtime/ffmpeg", nil)
	var body struct {
		FFmpeg struct {
			State string `json:"state"`
		} `json:"ffmpeg"`
	}
	decodeBody(t, recorder, &body)
	if body.FFmpeg.State != "missing" {
		t.Errorf("State = %q, want missing", body.FFmpeg.State)
	}
}

func TestFFmpegStatusEndpointRejectsWrongMethod(t *testing.T) {
	handler := newBranchTestServer(t, &stubFFmpegRuntime{}, &stubBranches{})

	recorder := do(t, handler, http.MethodPost, "/api/runtime/ffmpeg", nil)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", recorder.Code)
	}
	if got := recorder.Header().Get("Allow"); got != "GET" {
		t.Errorf("Allow = %q, want GET", got)
	}
}

// --- branch list --------------------------------------------------------

func TestGetBranchesReturnsAVersionedList(t *testing.T) {
	stub := &stubBranches{snapshots: []branch.Snapshot{
		{PlatformID: "pf_1", State: branch.StateIdle, Blockers: []string{branch.BlockerOutputServerMissing}},
	}}
	handler := newBranchTestServer(t, &stubFFmpegRuntime{}, stub)

	recorder := do(t, handler, http.MethodGet, "/api/runtime/branches", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", recorder.Code, recorder.Body.String())
	}

	var body struct {
		Version  int               `json:"version"`
		Branches []branch.Snapshot `json:"branches"`
	}
	decodeBody(t, recorder, &body)
	if body.Version != 1 {
		t.Errorf("Version = %d, want 1", body.Version)
	}
	if len(body.Branches) != 1 || body.Branches[0].PlatformID != "pf_1" {
		t.Errorf("Branches = %+v", body.Branches)
	}
}

func TestGetBranchesNeverExposesASecretLookingField(t *testing.T) {
	stub := &stubBranches{snapshots: []branch.Snapshot{
		{PlatformID: "pf_1", State: branch.StateLive, Blockers: []string{}},
	}}
	handler := newBranchTestServer(t, &stubFFmpegRuntime{}, stub)

	recorder := do(t, handler, http.MethodGet, "/api/runtime/branches", nil)
	for _, forbidden := range []string{"streamKey", "destinationUrl", "commandLine", "pid", "processId"} {
		if strings.Contains(recorder.Body.String(), forbidden) {
			t.Errorf("response contains forbidden field %q: %s", forbidden, recorder.Body.String())
		}
	}
}

func TestBranchesEndpointRejectsWrongMethod(t *testing.T) {
	handler := newBranchTestServer(t, &stubFFmpegRuntime{}, &stubBranches{})

	recorder := do(t, handler, http.MethodPost, "/api/runtime/branches", nil)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", recorder.Code)
	}
}

// --- single-branch commands -----------------------------------------------

func TestStartBranchAccepted(t *testing.T) {
	stub := &stubBranches{startOutcome: branch.Outcome{Accepted: true}}
	handler := newBranchTestServer(t, &stubFFmpegRuntime{}, stub)

	recorder := do(t, handler, http.MethodPost, "/api/runtime/branches/pf_1/start", nil)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202, body: %s", recorder.Code, recorder.Body.String())
	}
	if len(stub.calls) != 1 || stub.calls[0] != "start:pf_1" {
		t.Errorf("calls = %v", stub.calls)
	}
}

func TestStartBranchReturnsBlockersAsAStructuredOutcome(t *testing.T) {
	// Blocked is a normal, structured outcome, not an HTTP error: the caller
	// asked "can this start" and the answer is "no, here is why" - the
	// response is 200 with a blockers list, not a 4xx error envelope, so
	// the frontend can read it through its ordinary success path.
	stub := &stubBranches{startOutcome: branch.Outcome{Blockers: []string{branch.BlockerStreamKeyMissing}}}
	handler := newBranchTestServer(t, &stubFFmpegRuntime{}, stub)

	recorder := do(t, handler, http.MethodPost, "/api/runtime/branches/pf_1/start", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", recorder.Code, recorder.Body.String())
	}

	var body struct {
		Blockers []string `json:"blockers"`
	}
	decodeBody(t, recorder, &body)
	if len(body.Blockers) != 1 || body.Blockers[0] != branch.BlockerStreamKeyMissing {
		t.Errorf("Blockers = %v", body.Blockers)
	}
}

func TestStartBranchConflictReturns409(t *testing.T) {
	stub := &stubBranches{startOutcome: branch.Outcome{Conflict: true}}
	handler := newBranchTestServer(t, &stubFFmpegRuntime{}, stub)

	recorder := do(t, handler, http.MethodPost, "/api/runtime/branches/pf_1/start", nil)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", recorder.Code)
	}
}

func TestStartBranchOnAnUnknownPlatformIs404(t *testing.T) {
	stub := &stubBranches{startErr: branch.ErrNotFound}
	handler := newBranchTestServer(t, &stubFFmpegRuntime{}, stub)

	recorder := do(t, handler, http.MethodPost, "/api/runtime/branches/pf_missing/start", nil)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
}

func TestStartBranchRejectsARequestBody(t *testing.T) {
	stub := &stubBranches{startOutcome: branch.Outcome{Accepted: true}}
	handler := newBranchTestServer(t, &stubFFmpegRuntime{}, stub)

	recorder := do(t, handler, http.MethodPost, "/api/runtime/branches/pf_1/start", map[string]string{"foo": "bar"})
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}

func TestStopBranchSucceeds(t *testing.T) {
	stub := &stubBranches{}
	handler := newBranchTestServer(t, &stubFFmpegRuntime{}, stub)

	recorder := do(t, handler, http.MethodPost, "/api/runtime/branches/pf_1/stop", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", recorder.Code, recorder.Body.String())
	}
}

func TestStopBranchNotRunningReturns409(t *testing.T) {
	stub := &stubBranches{stopErr: branch.ErrNotRunning}
	handler := newBranchTestServer(t, &stubFFmpegRuntime{}, stub)

	recorder := do(t, handler, http.MethodPost, "/api/runtime/branches/pf_1/stop", nil)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", recorder.Code)
	}
}

func TestRestartBranchAccepted(t *testing.T) {
	stub := &stubBranches{restartOut: branch.Outcome{Accepted: true}}
	handler := newBranchTestServer(t, &stubFFmpegRuntime{}, stub)

	recorder := do(t, handler, http.MethodPost, "/api/runtime/branches/pf_1/restart", nil)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", recorder.Code)
	}
}

func TestBranchCommandsRejectWrongMethodWithAllow(t *testing.T) {
	handler := newBranchTestServer(t, &stubFFmpegRuntime{}, &stubBranches{})

	for _, path := range []string{
		"/api/runtime/branches/pf_1/start",
		"/api/runtime/branches/pf_1/stop",
		"/api/runtime/branches/pf_1/restart",
	} {
		recorder := do(t, handler, http.MethodGet, path, nil)
		if recorder.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: status = %d, want 405", path, recorder.Code)
		}
		if got := recorder.Header().Get("Allow"); got != "POST" {
			t.Errorf("%s: Allow = %q, want POST", path, got)
		}
	}
}

// --- bulk commands ------------------------------------------------------

func TestStartEnabledReturnsPerPlatformResults(t *testing.T) {
	stub := &stubBranches{startEnabled: []branch.StartEnabledResult{
		{PlatformID: "pf_1", Outcome: branch.Outcome{Accepted: true}},
		{PlatformID: "pf_2", Outcome: branch.Outcome{Blockers: []string{branch.BlockerStreamKeyMissing}}},
	}}
	handler := newBranchTestServer(t, &stubFFmpegRuntime{}, stub)

	recorder := do(t, handler, http.MethodPost, "/api/runtime/branches/start-enabled", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", recorder.Code, recorder.Body.String())
	}

	var body struct {
		Results []struct {
			PlatformID string   `json:"platformId"`
			Accepted   bool     `json:"accepted"`
			Blockers   []string `json:"blockers"`
		} `json:"results"`
	}
	decodeBody(t, recorder, &body)
	if len(body.Results) != 2 {
		t.Fatalf("got %d results, want 2", len(body.Results))
	}
	if !body.Results[0].Accepted {
		t.Error("pf_1 was not accepted")
	}
	if len(body.Results[1].Blockers) == 0 {
		t.Error("pf_2 should report its blocker")
	}
}

func TestStartEnabledRejectsARequestBody(t *testing.T) {
	handler := newBranchTestServer(t, &stubFFmpegRuntime{}, &stubBranches{})

	recorder := do(t, handler, http.MethodPost, "/api/runtime/branches/start-enabled", map[string]string{"x": "y"})
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}

func TestStopAllCallsTheManager(t *testing.T) {
	stub := &stubBranches{}
	handler := newBranchTestServer(t, &stubFFmpegRuntime{}, stub)

	recorder := do(t, handler, http.MethodPost, "/api/runtime/branches/stop-all", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if !stub.stopAllCalled {
		t.Error("StopAll was not called")
	}
}

func TestBulkEndpointsRejectWrongMethod(t *testing.T) {
	handler := newBranchTestServer(t, &stubFFmpegRuntime{}, &stubBranches{})

	for _, path := range []string{
		"/api/runtime/branches/start-enabled",
		"/api/runtime/branches/stop-all",
	} {
		recorder := do(t, handler, http.MethodGet, path, nil)
		if recorder.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: status = %d, want 405", path, recorder.Code)
		}
	}
}

// --- health is unaffected -------------------------------------------------

func TestHealthEndpointUnaffectedByBranchRoutes(t *testing.T) {
	handler := newBranchTestServer(t, &stubFFmpegRuntime{}, &stubBranches{})

	recorder := do(t, handler, http.MethodGet, "/api/health", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
}
