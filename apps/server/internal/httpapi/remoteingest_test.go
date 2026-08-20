package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/remoteingest"
)

// stubRemoteIngest is a controllable RemoteIngestService for handler
// tests - mirrors stubRuntime's own shape and reasoning: the real
// Manager's coordination logic is covered by its own package tests,
// so these tests are about the HTTP contract only.
type stubRemoteIngest struct {
	mu                                            sync.Mutex
	configured                                    bool
	receiving                                     bool
	nextSecret                                    string
	provisionErr, rotateErr, revokeErr, statusErr error
	calls                                         map[string]int
}

func newStubRemoteIngest() *stubRemoteIngest {
	return &stubRemoteIngest{nextSecret: "generated-secret-value", calls: map[string]int{}}
}

func (s *stubRemoteIngest) record(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls[name]++
}

func (s *stubRemoteIngest) callCount(name string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls[name]
}

func (s *stubRemoteIngest) Status(context.Context) (bool, error) {
	s.record("status")
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.configured, s.statusErr
}

func (s *stubRemoteIngest) IngestReceiving() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.receiving
}

func (s *stubRemoteIngest) Provision(context.Context) (string, error) {
	s.record("provision")
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.provisionErr != nil {
		return "", s.provisionErr
	}
	s.configured = true
	return s.nextSecret, nil
}

func (s *stubRemoteIngest) Rotate(context.Context) (string, error) {
	s.record("rotate")
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rotateErr != nil {
		return "", s.rotateErr
	}
	s.configured = true
	return s.nextSecret, nil
}

func (s *stubRemoteIngest) Revoke(context.Context) error {
	s.record("revoke")
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.revokeErr != nil {
		return s.revokeErr
	}
	s.configured = false
	return nil
}

func newRemoteIngestServer(t *testing.T, service RemoteIngestService) http.Handler {
	t.Helper()
	return NewRouter(Options{
		Logger:                   slog.New(slog.NewTextHandler(io.Discard, nil)),
		StartedAt:                time.Now(),
		RemoteIngest:             service,
		RemoteIngestRTMPSAddress: "0.0.0.0:1936",
		RemoteIngestPath:         "live",
	})
}

// --- GET /api/remote-ingest/status ------------------------------------------

func TestGetRemoteIngestStatusReturnsAVersionedSnapshot(t *testing.T) {
	stub := newStubRemoteIngest()
	stub.configured = true
	stub.receiving = true
	handler := newRemoteIngestServer(t, stub)

	recorder := do(t, handler, http.MethodGet, "/api/remote-ingest/status", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	assertJSONContentType(t, recorder)

	var body remoteIngestStatusResponse
	decodeBody(t, recorder, &body)

	if body.Version != remoteIngestStatusSchemaVersion {
		t.Errorf("version = %d, want %d", body.Version, remoteIngestStatusSchemaVersion)
	}
	if !body.Configured {
		t.Error("configured = false, want true")
	}
	if !body.Receiving {
		t.Error("receiving = false, want true")
	}
	if body.RTMPSAddress != "0.0.0.0:1936" {
		t.Errorf("rtmpsAddress = %q", body.RTMPSAddress)
	}
	if body.IngestPath != "live" {
		t.Errorf("ingestPath = %q", body.IngestPath)
	}
}

func TestRemoteIngestRoutesNotRegisteredWhenServiceIsNil(t *testing.T) {
	handler := NewRouter(Options{
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		StartedAt: time.Now(),
	})

	for _, path := range []string{
		"/api/remote-ingest/status",
		"/api/remote-ingest/provision",
		"/api/remote-ingest/rotate",
		"/api/remote-ingest/revoke",
	} {
		recorder := do(t, handler, http.MethodGet, path, nil)
		if recorder.Code == http.StatusOK {
			t.Errorf("%s: status = 200 with RemoteIngest nil, want not-found/not-registered", path)
		}
	}
}

// --- POST /api/remote-ingest/provision --------------------------------------

func TestPostRemoteIngestProvisionReturnsTheSecretOnce(t *testing.T) {
	stub := newStubRemoteIngest()
	handler := newRemoteIngestServer(t, stub)

	recorder := do(t, handler, http.MethodPost, "/api/remote-ingest/provision", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}

	var body remoteIngestSecretResponse
	decodeBody(t, recorder, &body)
	if body.Secret != stub.nextSecret {
		t.Errorf("secret = %q, want %q", body.Secret, stub.nextSecret)
	}
	if stub.callCount("provision") != 1 {
		t.Errorf("provision called %d times, want 1", stub.callCount("provision"))
	}
}

func TestPostRemoteIngestProvisionRejectsANonEmptyBody(t *testing.T) {
	stub := newStubRemoteIngest()
	handler := newRemoteIngestServer(t, stub)

	recorder := do(t, handler, http.MethodPost, "/api/remote-ingest/provision", `{"x":1}`)
	if recorder.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", recorder.Code)
	}
	if stub.callCount("provision") != 0 {
		t.Error("Provision was called despite the rejected body")
	}
}

func TestPostRemoteIngestProvisionAlreadyProvisionedReturns409(t *testing.T) {
	stub := newStubRemoteIngest()
	stub.provisionErr = remoteingest.ErrAlreadyProvisioned
	handler := newRemoteIngestServer(t, stub)

	recorder := do(t, handler, http.MethodPost, "/api/remote-ingest/provision", nil)
	if recorder.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", recorder.Code)
	}
}

func TestPostRemoteIngestProvisionStreamingActiveReturns409(t *testing.T) {
	stub := newStubRemoteIngest()
	stub.provisionErr = remoteingest.ErrStreamingActive
	handler := newRemoteIngestServer(t, stub)

	recorder := do(t, handler, http.MethodPost, "/api/remote-ingest/provision", nil)
	if recorder.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", recorder.Code)
	}
}

// --- POST /api/remote-ingest/rotate -----------------------------------------

func TestPostRemoteIngestRotateReturnsANewSecretOnce(t *testing.T) {
	stub := newStubRemoteIngest()
	stub.configured = true
	handler := newRemoteIngestServer(t, stub)

	recorder := do(t, handler, http.MethodPost, "/api/remote-ingest/rotate", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	if stub.callCount("rotate") != 1 {
		t.Errorf("rotate called %d times, want 1", stub.callCount("rotate"))
	}
}

func TestPostRemoteIngestRotateStreamingActiveReturns409(t *testing.T) {
	stub := newStubRemoteIngest()
	stub.rotateErr = remoteingest.ErrStreamingActive
	handler := newRemoteIngestServer(t, stub)

	recorder := do(t, handler, http.MethodPost, "/api/remote-ingest/rotate", nil)
	if recorder.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", recorder.Code)
	}
}

// --- POST /api/remote-ingest/revoke -----------------------------------------

func TestPostRemoteIngestRevokeSucceeds(t *testing.T) {
	stub := newStubRemoteIngest()
	stub.configured = true
	handler := newRemoteIngestServer(t, stub)

	recorder := do(t, handler, http.MethodPost, "/api/remote-ingest/revoke", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if stub.callCount("revoke") != 1 {
		t.Errorf("revoke called %d times, want 1", stub.callCount("revoke"))
	}
}

func TestPostRemoteIngestRevokeStreamingActiveReturns409(t *testing.T) {
	stub := newStubRemoteIngest()
	stub.revokeErr = remoteingest.ErrStreamingActive
	handler := newRemoteIngestServer(t, stub)

	recorder := do(t, handler, http.MethodPost, "/api/remote-ingest/revoke", nil)
	if recorder.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", recorder.Code)
	}
}

// --- method handling ---------------------------------------------------------

func TestRemoteIngestStatusRejectsPost(t *testing.T) {
	stub := newStubRemoteIngest()
	handler := newRemoteIngestServer(t, stub)

	recorder := do(t, handler, http.MethodPost, "/api/remote-ingest/status", nil)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", recorder.Code)
	}
}

func TestRemoteIngestProvisionRejectsGet(t *testing.T) {
	stub := newStubRemoteIngest()
	handler := newRemoteIngestServer(t, stub)

	recorder := do(t, handler, http.MethodGet, "/api/remote-ingest/provision", nil)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", recorder.Code)
	}
}
