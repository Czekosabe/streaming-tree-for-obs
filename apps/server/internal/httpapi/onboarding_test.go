package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/onboarding"
)

// stubOnboarding is a controllable OnboardingService for handler tests -
// the domain service's own behaviour is covered by its package tests, so
// this is only about the HTTP contract.
type stubOnboarding struct {
	state     onboarding.State
	stateErr  error
	setErr    error
	lastSetTo onboarding.Status
}

func newStubOnboarding() *stubOnboarding {
	return &stubOnboarding{state: onboarding.Default()}
}

func (s *stubOnboarding) State(ctx context.Context) (onboarding.State, error) {
	if s.stateErr != nil {
		return onboarding.State{}, s.stateErr
	}
	return s.state, nil
}

func (s *stubOnboarding) SetStatus(ctx context.Context, status onboarding.Status) (onboarding.State, error) {
	s.lastSetTo = status
	if s.setErr != nil {
		return onboarding.State{}, s.setErr
	}
	if !status.Valid() {
		return onboarding.State{}, onboarding.ErrInvalidStatus
	}
	s.state.Status = status
	return s.state, nil
}

func newOnboardingServer(t *testing.T, service OnboardingService) http.Handler {
	t.Helper()
	return NewRouter(Options{
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		StartedAt:  time.Now(),
		Onboarding: service,
	})
}

// --- GET /api/onboarding -----------------------------------------------

func TestGetOnboardingReturnsAVersionedState(t *testing.T) {
	stub := newStubOnboarding()
	handler := newOnboardingServer(t, stub)

	recorder := do(t, handler, http.MethodGet, "/api/onboarding", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	assertJSONContentType(t, recorder)

	var body struct {
		Version       int    `json:"version"`
		Status        string `json:"status"`
		SchemaVersion int    `json:"schemaVersion"`
	}
	decodeBody(t, recorder, &body)

	if body.Version != onboardingSchemaResponseVersion {
		t.Errorf("version = %d, want %d", body.Version, onboardingSchemaResponseVersion)
	}
	if body.Status != string(onboarding.StatusPending) {
		t.Errorf("status = %q, want %q", body.Status, onboarding.StatusPending)
	}
	if body.SchemaVersion != onboarding.CurrentSchemaVersion {
		t.Errorf("schemaVersion = %d, want %d", body.SchemaVersion, onboarding.CurrentSchemaVersion)
	}
}

func TestGetOnboardingMapsServiceErrorTo500(t *testing.T) {
	stub := newStubOnboarding()
	stub.stateErr = onboarding.ErrStorage
	handler := newOnboardingServer(t, stub)

	recorder := do(t, handler, http.MethodGet, "/api/onboarding", nil)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}
}

func TestOnboardingWrongMethodIsRejected(t *testing.T) {
	stub := newStubOnboarding()
	handler := newOnboardingServer(t, stub)

	recorder := do(t, handler, http.MethodPost, "/api/onboarding", nil)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", recorder.Code)
	}
}

// --- PUT /api/onboarding -------------------------------------------------

func TestPutOnboardingSetsStatus(t *testing.T) {
	stub := newStubOnboarding()
	handler := newOnboardingServer(t, stub)

	recorder := do(t, handler, http.MethodPut, "/api/onboarding", map[string]string{"status": "completed"})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", recorder.Code, recorder.Body.String())
	}
	if stub.lastSetTo != onboarding.StatusCompleted {
		t.Errorf("SetStatus called with %q, want %q", stub.lastSetTo, onboarding.StatusCompleted)
	}

	var body struct {
		Status string `json:"status"`
	}
	decodeBody(t, recorder, &body)
	if body.Status != string(onboarding.StatusCompleted) {
		t.Errorf("status = %q, want %q", body.Status, onboarding.StatusCompleted)
	}
}

func TestPutOnboardingRejectsUnknownStatus(t *testing.T) {
	stub := newStubOnboarding()
	handler := newOnboardingServer(t, stub)

	recorder := do(t, handler, http.MethodPut, "/api/onboarding", map[string]string{"status": "bogus"})
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestPutOnboardingRejectsUnknownField(t *testing.T) {
	stub := newStubOnboarding()
	handler := newOnboardingServer(t, stub)

	recorder := do(t, handler, http.MethodPut, "/api/onboarding", map[string]string{"status": "completed", "extra": "nope"})
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", recorder.Code, recorder.Body.String())
	}
}
