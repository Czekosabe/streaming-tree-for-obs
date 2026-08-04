package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/runtime/mediamtx"
)

// stubRuntime is a controllable RuntimeService for handler tests.
//
// The supervisor's own behaviour is covered by its package tests; these tests
// are about the HTTP contract, so the service is stubbed to return exactly the
// state each case needs.
type stubRuntime struct {
	mu                                        sync.Mutex
	snapshot                                  mediamtx.Snapshot
	installErr, startErr, stopErr, restartErr error
	calls                                     map[string]int
}

func newStubRuntime() *stubRuntime {
	return &stubRuntime{
		snapshot: mediamtx.Snapshot{
			MediaMTX: mediamtx.MediaMTXSnapshot{
				SupportedVersion: mediamtx.SupportedVersion,
				Source:           mediamtx.SourceMissing,
				State:            mediamtx.StateMissing,
				AutoStart:        true,
				AutoRestart:      true,
			},
			Ingest: mediamtx.IngestSnapshot{
				State:  mediamtx.IngestUnavailable,
				Path:   "live",
				Tracks: []string{},
			},
			Connection: mediamtx.ConnectionSnapshot{
				ServerURL:  "rtmp://127.0.0.1:1935",
				StreamKey:  "live",
				PublishURL: "rtmp://127.0.0.1:1935/live",
			},
		},
		calls: map[string]int{},
	}
}

func (s *stubRuntime) Snapshot() mediamtx.Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshot
}

func (s *stubRuntime) record(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls[name]++
}

func (s *stubRuntime) callCount(name string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls[name]
}

func (s *stubRuntime) RequestInstall(context.Context) error {
	s.record("install")
	return s.installErr
}
func (s *stubRuntime) RequestStart(context.Context) error {
	s.record("start")
	return s.startErr
}
func (s *stubRuntime) RequestStop(context.Context) error {
	s.record("stop")
	return s.stopErr
}
func (s *stubRuntime) RequestRestart(context.Context) error {
	s.record("restart")
	return s.restartErr
}

func newRuntimeServer(t *testing.T, service RuntimeService) http.Handler {
	t.Helper()
	return NewRouter(Options{
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		StartedAt: time.Now(),
		Runtime:   service,
	})
}

// --- GET /api/runtime -------------------------------------------------------

func TestGetRuntimeReturnsAVersionedSnapshot(t *testing.T) {
	stub := newStubRuntime()
	handler := newRuntimeServer(t, stub)

	recorder := do(t, handler, http.MethodGet, "/api/runtime", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	assertJSONContentType(t, recorder)

	var body struct {
		Version  int `json:"version"`
		MediaMTX struct {
			SupportedVersion string                 `json:"supportedVersion"`
			State            string                 `json:"state"`
			Source           string                 `json:"source"`
			RestartCount     int                    `json:"restartCount"`
			LastError        *struct{ Code string } `json:"lastError"`
		} `json:"mediaMtx"`
		Ingest struct {
			State  string   `json:"state"`
			Path   string   `json:"path"`
			Tracks []string `json:"tracks"`
		} `json:"ingest"`
		Connection struct {
			ServerURL  string `json:"serverUrl"`
			StreamKey  string `json:"streamKey"`
			PublishURL string `json:"publishUrl"`
		} `json:"connection"`
	}
	decodeBody(t, recorder, &body)

	if body.Version != runtimeSchemaVersion {
		t.Errorf("version = %d, want %d", body.Version, runtimeSchemaVersion)
	}
	if body.MediaMTX.SupportedVersion != mediamtx.SupportedVersion {
		t.Errorf("supportedVersion = %q", body.MediaMTX.SupportedVersion)
	}
	if body.MediaMTX.State != "missing" {
		t.Errorf("state = %q, want missing", body.MediaMTX.State)
	}
	// The connection details must be present even when MediaMTX is missing, so
	// the interface can show where OBS will eventually publish.
	if body.Connection.PublishURL != "rtmp://127.0.0.1:1935/live" {
		t.Errorf("publishUrl = %q", body.Connection.PublishURL)
	}
	if body.Connection.StreamKey != "live" {
		t.Errorf("streamKey = %q, want the ingest path", body.Connection.StreamKey)
	}
	if body.Ingest.Tracks == nil {
		t.Error("tracks is null, want an empty array")
	}
}

func TestGetRuntimeLeaksNoFilesystemPathOrEnvironment(t *testing.T) {
	stub := newStubRuntime()
	stub.snapshot.MediaMTX.State = mediamtx.StateReady
	stub.snapshot.MediaMTX.Source = mediamtx.SourceManaged
	stub.snapshot.MediaMTX.InstalledVersion = mediamtx.SupportedVersion
	handler := newRuntimeServer(t, stub)

	recorder := do(t, handler, http.MethodGet, "/api/runtime", nil)
	body := strings.ToLower(recorder.Body.String())

	// A path or an environment variable tells the browser about the machine and
	// is of no use to the interface.
	for _, forbidden := range []string{
		"c:\\", "/home/", "/users/", "appdata", "streaming_tree_", "mediamtx.exe",
		"executablepath", "pid", "environ", "argv", "goroutine",
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("the runtime payload leaks %q: %s", forbidden, recorder.Body.String())
		}
	}
}

func TestGetRuntimeExposesNoCredentialFields(t *testing.T) {
	stub := newStubRuntime()
	handler := newRuntimeServer(t, stub)

	recorder := do(t, handler, http.MethodGet, "/api/runtime", nil)
	body := strings.ToLower(recorder.Body.String())

	// "streamKey" is the local route identifier and is expected; anything
	// resembling a destination credential is not.
	for _, forbidden := range []string{"password", "secret", "token", "credential", "apikey"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("the runtime payload contains %q", forbidden)
		}
	}
}

func TestGetRuntimeReportsAnIngestPublisher(t *testing.T) {
	stub := newStubRuntime()
	trackCount := 2
	stub.snapshot.MediaMTX.State = mediamtx.StateReady
	stub.snapshot.Ingest = mediamtx.IngestSnapshot{
		State:       mediamtx.IngestReceiving,
		Path:        "live",
		SourceType:  "rtmpConn",
		ConnectedAt: "2026-08-03T12:00:00Z",
		TrackCount:  &trackCount,
		Tracks:      []string{"H264", "MPEG-4 Audio"},
	}
	handler := newRuntimeServer(t, stub)

	recorder := do(t, handler, http.MethodGet, "/api/runtime", nil)

	var body map[string]any
	decodeBody(t, recorder, &body)

	ingest, ok := body["ingest"].(map[string]any)
	if !ok {
		t.Fatalf("the payload has no ingest object: %v", body)
	}
	if ingest["state"] != "receiving" {
		t.Errorf("state = %v, want receiving", ingest["state"])
	}
	if ingest["sourceType"] != "rtmpConn" {
		t.Errorf("sourceType = %v", ingest["sourceType"])
	}

	// Nothing may be invented: no bitrate, resolution or frame rate exists in
	// the MediaMTX API, so none may appear here.
	for _, forbidden := range []string{"bitrate", "resolution", "fps", "frameRate", "width", "height"} {
		if _, present := ingest[forbidden]; present {
			t.Errorf("the ingest payload contains the invented field %q", forbidden)
		}
	}
}

// --- commands ---------------------------------------------------------------

func TestInstallIsAcceptedAsynchronously(t *testing.T) {
	stub := newStubRuntime()
	handler := newRuntimeServer(t, stub)

	recorder := do(t, handler, http.MethodPost, "/api/runtime/mediamtx/install", nil)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", recorder.Code)
	}
	if stub.callCount("install") != 1 {
		t.Errorf("install was called %d times, want 1", stub.callCount("install"))
	}
}

func TestConcurrentInstallReturnsConflict(t *testing.T) {
	stub := newStubRuntime()
	stub.installErr = mediamtx.ErrInstallInProgress
	handler := newRuntimeServer(t, stub)

	recorder := do(t, handler, http.MethodPost, "/api/runtime/mediamtx/install", nil)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", recorder.Code)
	}

	var body ErrorBody
	decodeBody(t, recorder, &body)
	if body.Error != mediamtx.CodeInstallInProgress {
		t.Errorf("error = %q, want %q", body.Error, mediamtx.CodeInstallInProgress)
	}
}

func TestStartStopRestartAreAccepted(t *testing.T) {
	cases := map[string]struct {
		path       string
		call       string
		wantStatus int
	}{
		"start":   {"/api/runtime/mediamtx/start", "start", http.StatusAccepted},
		"stop":    {"/api/runtime/mediamtx/stop", "stop", http.StatusOK},
		"restart": {"/api/runtime/mediamtx/restart", "restart", http.StatusAccepted},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			stub := newStubRuntime()
			handler := newRuntimeServer(t, stub)

			recorder := do(t, handler, http.MethodPost, testCase.path, nil)
			if recorder.Code != testCase.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, testCase.wantStatus)
			}
			if stub.callCount(testCase.call) != 1 {
				t.Errorf("%s was called %d times, want 1", testCase.call, stub.callCount(testCase.call))
			}
			assertJSONContentType(t, recorder)
		})
	}
}

func TestStartOnAMissingBinaryIsUnprocessable(t *testing.T) {
	stub := newStubRuntime()
	stub.startErr = mediamtx.ErrNotInstalled
	handler := newRuntimeServer(t, stub)

	recorder := do(t, handler, http.MethodPost, "/api/runtime/mediamtx/start", nil)
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", recorder.Code)
	}

	var body ErrorBody
	decodeBody(t, recorder, &body)
	if body.Error != mediamtx.CodeNotInstalled {
		t.Errorf("error = %q, want %q", body.Error, mediamtx.CodeNotInstalled)
	}
}

func TestStartOnAnIncompatibleBinaryIsUnprocessable(t *testing.T) {
	stub := newStubRuntime()
	stub.startErr = mediamtx.ErrIncompatibleVersion
	handler := newRuntimeServer(t, stub)

	recorder := do(t, handler, http.MethodPost, "/api/runtime/mediamtx/start", nil)
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", recorder.Code)
	}

	var body ErrorBody
	decodeBody(t, recorder, &body)
	if body.Error != mediamtx.CodeIncompatibleVersion {
		t.Errorf("error = %q, want %q", body.Error, mediamtx.CodeIncompatibleVersion)
	}
}

func TestStartWhileAlreadyRunningIsAConflict(t *testing.T) {
	stub := newStubRuntime()
	stub.startErr = mediamtx.ErrInvalidState
	handler := newRuntimeServer(t, stub)

	recorder := do(t, handler, http.MethodPost, "/api/runtime/mediamtx/start", nil)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", recorder.Code)
	}
}

// --- request shape ----------------------------------------------------------

func TestRuntimeCommandsRejectARequestBody(t *testing.T) {
	// These are commands, not resources. Accepting a body would invite a client
	// to think it could pass a download URL, a checksum or a path.
	for _, path := range []string{
		"/api/runtime/mediamtx/install",
		"/api/runtime/mediamtx/start",
		"/api/runtime/mediamtx/stop",
		"/api/runtime/mediamtx/restart",
	} {
		t.Run(path, func(t *testing.T) {
			stub := newStubRuntime()
			handler := newRuntimeServer(t, stub)

			recorder := do(t, handler, http.MethodPost, path,
				`{"url":"http://evil.example.com/mediamtx.tar.gz"}`)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", recorder.Code)
			}
			var body ErrorBody
			decodeBody(t, recorder, &body)
			if body.Error != "unexpected_body" {
				t.Errorf("error = %q, want unexpected_body", body.Error)
			}
			// The action must not have run.
			if stub.callCount("install")+stub.callCount("start")+
				stub.callCount("stop")+stub.callCount("restart") != 0 {
				t.Error("the command ran despite the rejected body")
			}
		})
	}
}

func TestRuntimeCommandsRejectAnOversizedBody(t *testing.T) {
	stub := newStubRuntime()
	handler := newRuntimeServer(t, stub)

	huge := strings.Repeat("a", maxRequestBodyBytes+1024)
	recorder := do(t, handler, http.MethodPost, "/api/runtime/mediamtx/install", huge)

	// Either rejection is acceptable; what matters is that it is refused and
	// the command did not run.
	if recorder.Code != http.StatusBadRequest && recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 400 or 413", recorder.Code)
	}
	if stub.callCount("install") != 0 {
		t.Error("the install ran despite an oversized body")
	}
}

func TestRuntimeEndpointsRejectWrongMethodsWithAllow(t *testing.T) {
	cases := map[string]struct {
		path, method, allow string
	}{
		"runtime": {"/api/runtime", http.MethodPost, "GET"},
		"install": {"/api/runtime/mediamtx/install", http.MethodGet, "POST"},
		"start":   {"/api/runtime/mediamtx/start", http.MethodDelete, "POST"},
		"stop":    {"/api/runtime/mediamtx/stop", http.MethodPut, "POST"},
		"restart": {"/api/runtime/mediamtx/restart", http.MethodGet, "POST"},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			handler := newRuntimeServer(t, newStubRuntime())

			recorder := do(t, handler, testCase.method, testCase.path, nil)
			if recorder.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status = %d, want 405", recorder.Code)
			}
			if allow := recorder.Header().Get("Allow"); allow != testCase.allow {
				t.Errorf("Allow = %q, want %q", allow, testCase.allow)
			}
			assertJSONContentType(t, recorder)
		})
	}
}

func TestUnknownRuntimePathStillReturns404(t *testing.T) {
	handler := newRuntimeServer(t, newStubRuntime())

	recorder := do(t, handler, http.MethodGet, "/api/runtime/nope", nil)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
}

func TestMediaMTXControlAPIIsNotProxied(t *testing.T) {
	handler := newRuntimeServer(t, newStubRuntime())

	// The browser must never reach the MediaMTX Control API, directly or
	// through a proxy route.
	for _, path := range []string{
		"/api/runtime/mediamtx/v3/paths/list",
		"/api/runtime/mediamtx/api",
		"/api/mediamtx/v3/config/global/get",
		"/v3/paths/list",
	} {
		recorder := do(t, handler, http.MethodGet, path, nil)
		if recorder.Code == http.StatusOK {
			t.Errorf("%s answered 200; the Control API must not be reachable", path)
		}
	}
}

// --- coexistence ------------------------------------------------------------

func TestHealthIsUnaffectedByRuntimeState(t *testing.T) {
	stub := newStubRuntime()
	stub.snapshot.MediaMTX.State = mediamtx.StateError
	stub.snapshot.MediaMTX.LastError = mediamtx.NewRuntimeError(
		mediamtx.CodeExitedUnexpectedly, "MediaMTX stopped unexpectedly.")
	handler := newRuntimeServer(t, stub)

	// The backend is healthy even when the MediaMTX component is degraded:
	// the two are separate concerns.
	recorder := do(t, handler, http.MethodGet, "/api/health", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("health status = %d, want 200 while MediaMTX is failing", recorder.Code)
	}

	var health map[string]any
	decodeBody(t, recorder, &health)
	if health["status"] != "ok" {
		t.Errorf("health status = %v, want ok", health["status"])
	}
}

func TestRuntimeSnapshotSerializesLastErrorAsNullWhenUnset(t *testing.T) {
	handler := newRuntimeServer(t, newStubRuntime())

	recorder := do(t, handler, http.MethodGet, "/api/runtime", nil)

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(recorder.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var mtx map[string]json.RawMessage
	if err := json.Unmarshal(raw["mediaMtx"], &mtx); err != nil {
		t.Fatalf("decode mediaMtx: %v", err)
	}

	// Explicit null rather than an omitted key, so the frontend schema can
	// require the field and distinguish "no error" from "field missing".
	if string(mtx["lastError"]) != "null" {
		t.Errorf("lastError = %s, want null", mtx["lastError"])
	}
}
