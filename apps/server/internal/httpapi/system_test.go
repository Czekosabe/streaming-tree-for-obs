package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newShutdownRouter(t *testing.T) (http.Handler, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	_, cancel := context.WithCancel(context.Background())
	wrapped := context.CancelFunc(func() {
		calls.Add(1)
		cancel()
	})

	handler := NewRouter(Options{
		Logger:         slog.Default(),
		StartedAt:      time.Now(),
		AllowedOrigins: []string{"http://localhost:5173"},
		Shutdown:       wrapped,
	})
	return handler, &calls
}

func TestShutdownSameOriginSucceeds(t *testing.T) {
	handler, calls := newShutdownRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/api/system/shutdown", strings.NewReader(`{"confirm":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("cancel called %d times, want exactly 1", got)
	}
}

func TestShutdownAllowedOriginSucceeds(t *testing.T) {
	handler, calls := newShutdownRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/api/system/shutdown", strings.NewReader(`{"confirm":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://localhost:5173")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("cancel called %d times, want exactly 1", got)
	}
}

func TestShutdownForeignOriginRejected(t *testing.T) {
	handler, calls := newShutdownRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/api/system/shutdown", strings.NewReader(`{"confirm":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
	if got := calls.Load(); got != 0 {
		t.Errorf("cancel called %d times, want 0 for a foreign origin", got)
	}
}

func TestShutdownGETDoesNotShutDown(t *testing.T) {
	handler, calls := newShutdownRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/system/shutdown", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
	if got := calls.Load(); got != 0 {
		t.Errorf("cancel called %d times, want 0 for GET", got)
	}
}

// TestShutdownCannotBeSubmittedByAnHTMLForm proves the core cross-origin
// boundary: an ordinary HTML <form> can only submit
// application/x-www-form-urlencoded, multipart/form-data, or text/plain -
// never application/json - so a request shaped like a form submission is
// rejected outright regardless of Origin.
func TestShutdownCannotBeSubmittedByAnHTMLForm(t *testing.T) {
	handler, calls := newShutdownRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/api/system/shutdown", strings.NewReader("confirm=true"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("a form-encoded body must never be accepted, got 200")
	}
	if got := calls.Load(); got != 0 {
		t.Errorf("cancel called %d times, want 0 for a form-encoded body", got)
	}
}

func TestShutdownMissingConfirmationRejected(t *testing.T) {
	handler, calls := newShutdownRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/api/system/shutdown", strings.NewReader(`{"confirm":false}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 without confirm:true, got %d", rec.Code)
	}
	if got := calls.Load(); got != 0 {
		t.Errorf("cancel called %d times, want 0", got)
	}
}

func TestShutdownUnknownFieldRejected(t *testing.T) {
	handler, calls := newShutdownRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/api/system/shutdown", strings.NewReader(`{"confirm":true,"action":"reboot"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("an unknown field must be rejected - no generic action/command parameter exists, got 200")
	}
	if got := calls.Load(); got != 0 {
		t.Errorf("cancel called %d times, want 0", got)
	}
}

func TestShutdownIsIdempotent(t *testing.T) {
	handler, calls := newShutdownRouter(t)

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/system/shutdown", strings.NewReader(`{"confirm":true}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i, rec.Code)
		}
	}

	if got := calls.Load(); got != 1 {
		t.Errorf("cancel called %d times across 3 requests, want exactly 1", got)
	}
}

func TestShutdownRouteNotRegisteredWhenNil(t *testing.T) {
	handler := NewRouter(Options{Logger: slog.Default(), StartedAt: time.Now()})

	req := httptest.NewRequest(http.MethodPost, "/api/system/shutdown", strings.NewReader(`{"confirm":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Error("expected the shutdown route to be unavailable when Shutdown is nil")
	}
}
