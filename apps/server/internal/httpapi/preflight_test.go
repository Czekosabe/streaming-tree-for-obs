package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/preflight"
	"github.com/streaming-tree/server/internal/domain/streamsetup"
)

type stubPreflightService struct {
	report        preflight.Report
	err           error
	lastProfileID *string
}

func (s *stubPreflightService) Evaluate(_ context.Context, profileID *string) (preflight.Report, error) {
	s.lastProfileID = profileID
	if s.err != nil {
		return preflight.Report{}, s.err
	}
	return s.report, nil
}

func newPreflightServer(t *testing.T, service PreflightService) http.Handler {
	t.Helper()
	return NewRouter(Options{
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		StartedAt: time.Now(),
		Preflight: service,
	})
}

func TestGetPreflightReturnsTheReport(t *testing.T) {
	stub := &stubPreflightService{report: preflight.Report{
		Status: preflight.StatusReady,
		Destinations: []preflight.DestinationReadiness{
			{PlatformID: "pf_1", ProviderID: "twitch", DisplayName: "Main Twitch"},
		},
	}}
	handler := newPreflightServer(t, stub)

	recorder := do(t, handler, http.MethodGet, "/api/preflight", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", recorder.Code, recorder.Body.String())
	}
	var body preflightReportResponse
	decodeBody(t, recorder, &body)
	if body.Status != "ready" || len(body.Destinations) != 1 || body.Destinations[0].PlatformID != "pf_1" {
		t.Fatalf("body = %+v", body)
	}
	if stub.lastProfileID != nil {
		t.Errorf("lastProfileID = %v, want nil for no query param", stub.lastProfileID)
	}
}

func TestGetPreflightPassesTheProfileIDQueryParamThrough(t *testing.T) {
	stub := &stubPreflightService{report: preflight.Report{Status: preflight.StatusReady}}
	handler := newPreflightServer(t, stub)

	recorder := do(t, handler, http.MethodGet, "/api/preflight?profileId=setup_1", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", recorder.Code, recorder.Body.String())
	}
	if stub.lastProfileID == nil || *stub.lastProfileID != "setup_1" {
		t.Errorf("lastProfileID = %v, want setup_1", stub.lastProfileID)
	}
}

func TestGetPreflightIncludesFindingsAndActions(t *testing.T) {
	stub := &stubPreflightService{report: preflight.Report{
		Status: preflight.StatusNotReady,
		Findings: []preflight.Finding{
			{Code: "stream_key_missing", Severity: preflight.SeverityBlocker, PlatformID: "pf_1",
				Action: &preflight.Action{Code: preflight.ActionAddStreamKey, PlatformID: "pf_1"}},
		},
	}}
	handler := newPreflightServer(t, stub)

	recorder := do(t, handler, http.MethodGet, "/api/preflight", nil)
	var body preflightReportResponse
	decodeBody(t, recorder, &body)
	if len(body.Findings) != 1 || body.Findings[0].Code != "stream_key_missing" {
		t.Fatalf("Findings = %+v", body.Findings)
	}
	if body.Findings[0].Action == nil || body.Findings[0].Action.Code != "add_stream_key" {
		t.Errorf("Action = %+v, want add_stream_key", body.Findings[0].Action)
	}
}

func TestGetPreflightUnknownProfileReturns404(t *testing.T) {
	stub := &stubPreflightService{err: streamsetup.ErrNotFound}
	handler := newPreflightServer(t, stub)

	recorder := do(t, handler, http.MethodGet, "/api/preflight?profileId=setup_missing", nil)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
}

func TestPreflightWrongMethodReturns405(t *testing.T) {
	stub := &stubPreflightService{}
	handler := newPreflightServer(t, stub)

	recorder := do(t, handler, http.MethodPost, "/api/preflight", nil)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", recorder.Code)
	}
}
