package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/streamsession"
)

type stubStreamSessionService struct {
	sessions           []streamsession.Session
	byID               map[string]streamsession.Session
	getErr             error
	deleteAllErr       error
	deleteAllCalled    bool
	retentionDays      int
	retentionFound     bool
	setRetentionCalled int
	lastLimit          int
}

func (s *stubStreamSessionService) ListSessions(_ context.Context, limit int) ([]streamsession.Session, error) {
	s.lastLimit = limit
	return s.sessions, nil
}

func (s *stubStreamSessionService) GetSession(_ context.Context, id string) (streamsession.Session, error) {
	if s.getErr != nil {
		return streamsession.Session{}, s.getErr
	}
	sess, ok := s.byID[id]
	if !ok {
		return streamsession.Session{}, streamsession.ErrNotFound
	}
	return sess, nil
}

func (s *stubStreamSessionService) DeleteAllSessions(_ context.Context) error {
	s.deleteAllCalled = true
	return s.deleteAllErr
}

func (s *stubStreamSessionService) GetRetentionDays(_ context.Context) (int, bool, error) {
	return s.retentionDays, s.retentionFound, nil
}

func (s *stubStreamSessionService) SetRetentionDays(_ context.Context, days int, _ time.Time) error {
	s.setRetentionCalled++
	s.retentionDays = days
	s.retentionFound = true
	return nil
}

func newStreamSessionServer(t *testing.T, service StreamSessionService) http.Handler {
	t.Helper()
	return NewRouter(Options{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		StartedAt:      time.Now(),
		StreamSessions: service,
	})
}

func TestListStreamSessionsReturnsTheSessionsWrapped(t *testing.T) {
	now := time.Now()
	stub := &stubStreamSessionService{sessions: []streamsession.Session{{ID: "sess_1", StartedAt: now, LastSeenAt: now}}}
	handler := newStreamSessionServer(t, stub)

	recorder := do(t, handler, http.MethodGet, "/api/stream-sessions", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Sessions []streamSessionResponse `json:"sessions"`
	}
	decodeBody(t, recorder, &body)
	if len(body.Sessions) != 1 || body.Sessions[0].ID != "sess_1" {
		t.Fatalf("body = %+v", body)
	}
	if !body.Sessions[0].Open {
		t.Error("Open = false, want true for a session with no EndedAt")
	}
	if stub.lastLimit != defaultStreamSessionListLimit {
		t.Errorf("ListSessions received limit = %d, want the default %d", stub.lastLimit, defaultStreamSessionListLimit)
	}
}

func TestListStreamSessionsRespectsAndBoundsTheLimitQueryParam(t *testing.T) {
	stub := &stubStreamSessionService{}
	handler := newStreamSessionServer(t, stub)

	do(t, handler, http.MethodGet, "/api/stream-sessions?limit=5", nil)
	if stub.lastLimit != 5 {
		t.Errorf("limit = %d, want 5", stub.lastLimit)
	}

	do(t, handler, http.MethodGet, "/api/stream-sessions?limit=100000", nil)
	if stub.lastLimit != maxStreamSessionListLimit {
		t.Errorf("limit = %d, want it bounded to %d", stub.lastLimit, maxStreamSessionListLimit)
	}
}

func TestListStreamSessionsRejectsAnInvalidLimit(t *testing.T) {
	stub := &stubStreamSessionService{}
	handler := newStreamSessionServer(t, stub)

	recorder := do(t, handler, http.MethodGet, "/api/stream-sessions?limit=not-a-number", nil)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}

func TestGetStreamSessionReturnsItsDestinations(t *testing.T) {
	now := time.Now()
	pid := "pf_1"
	stub := &stubStreamSessionService{byID: map[string]streamsession.Session{
		"sess_1": {
			ID: "sess_1", StartedAt: now, LastSeenAt: now,
			Destinations: []streamsession.Destination{
				{ID: "sessdest_1", SessionID: "sess_1", PlatformID: &pid, ProviderID: "twitch", DisplayName: "Main", StartedAt: now, Outcome: ""},
			},
		},
	}}
	handler := newStreamSessionServer(t, stub)

	recorder := do(t, handler, http.MethodGet, "/api/stream-sessions/sess_1", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", recorder.Code, recorder.Body.String())
	}
	var body streamSessionResponse
	decodeBody(t, recorder, &body)
	if len(body.Destinations) != 1 || body.Destinations[0].DisplayName != "Main" {
		t.Fatalf("body = %+v", body)
	}
	if !body.Destinations[0].Open {
		t.Error("destination Open = false, want true (no EndedAt)")
	}
}

func TestGetStreamSessionUnknownIDIsNotFound(t *testing.T) {
	stub := &stubStreamSessionService{byID: map[string]streamsession.Session{}}
	handler := newStreamSessionServer(t, stub)

	recorder := do(t, handler, http.MethodGet, "/api/stream-sessions/sess_missing", nil)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestClearStreamSessionHistoryRequiresConfirmTrue(t *testing.T) {
	stub := &stubStreamSessionService{}
	handler := newStreamSessionServer(t, stub)

	recorder := do(t, handler, http.MethodDelete, "/api/stream-sessions", map[string]bool{"confirm": false})
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", recorder.Code, recorder.Body.String())
	}
	if stub.deleteAllCalled {
		t.Error("DeleteAllSessions was called despite confirm: false")
	}
}

func TestClearStreamSessionHistorySucceedsWithConfirmTrue(t *testing.T) {
	stub := &stubStreamSessionService{}
	handler := newStreamSessionServer(t, stub)

	recorder := do(t, handler, http.MethodDelete, "/api/stream-sessions", map[string]bool{"confirm": true})
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204, body=%s", recorder.Code, recorder.Body.String())
	}
	if !stub.deleteAllCalled {
		t.Error("DeleteAllSessions was not called")
	}
}

func TestGetStreamSessionSettingsFallsBackToTheDefault(t *testing.T) {
	stub := &stubStreamSessionService{retentionFound: false}
	handler := newStreamSessionServer(t, stub)

	recorder := do(t, handler, http.MethodGet, "/api/stream-sessions/settings", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", recorder.Code, recorder.Body.String())
	}
	var body streamSessionSettingsResponse
	decodeBody(t, recorder, &body)
	if body.RetentionDays != streamsession.DefaultRetentionDays {
		t.Errorf("RetentionDays = %d, want the default %d", body.RetentionDays, streamsession.DefaultRetentionDays)
	}
}

func TestSetStreamSessionSettingsRejectsNonPositiveRetention(t *testing.T) {
	stub := &stubStreamSessionService{}
	handler := newStreamSessionServer(t, stub)

	recorder := do(t, handler, http.MethodPut, "/api/stream-sessions/settings", map[string]int{"retentionDays": 0})
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body=%s", recorder.Code, recorder.Body.String())
	}
	if stub.setRetentionCalled != 0 {
		t.Error("SetRetentionDays was called despite an invalid value")
	}
}

func TestSetStreamSessionSettingsSucceeds(t *testing.T) {
	stub := &stubStreamSessionService{}
	handler := newStreamSessionServer(t, stub)

	recorder := do(t, handler, http.MethodPut, "/api/stream-sessions/settings", map[string]int{"retentionDays": 30})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", recorder.Code, recorder.Body.String())
	}
	if stub.setRetentionCalled != 1 {
		t.Errorf("SetRetentionDays called %d times, want 1", stub.setRetentionCalled)
	}
	var body streamSessionSettingsResponse
	decodeBody(t, recorder, &body)
	if body.RetentionDays != 30 {
		t.Errorf("RetentionDays = %d, want 30", body.RetentionDays)
	}
}

func TestStreamSessionSettingsWrongMethodStillGets405NotAPanic(t *testing.T) {
	stub := &stubStreamSessionService{}
	handler := newStreamSessionServer(t, stub)

	recorder := do(t, handler, http.MethodDelete, "/api/stream-sessions/settings", nil)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405 (Go's own built-in method handling, since no custom catch-all is registered here to avoid an ambiguous-pattern panic)", recorder.Code)
	}
}

func TestStreamSessionRoutesWrongMethodIsRejected(t *testing.T) {
	stub := &stubStreamSessionService{}
	handler := newStreamSessionServer(t, stub)

	recorder := do(t, handler, http.MethodPost, "/api/stream-sessions", nil)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", recorder.Code)
	}
}
