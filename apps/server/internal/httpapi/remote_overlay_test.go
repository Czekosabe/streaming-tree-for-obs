package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

// --- isOverlaySurfacePath ----------------------------------------------------

func TestIsOverlaySurfacePathMatchesExactRootsAndDescendants(t *testing.T) {
	// Mirrors docs/examples/Caddyfile.remote-management's own hardened
	// matcher exactly: /overlay, /overlay/*, /api/public, /api/public/*.
	// Caddy's own documentation confirms /foo/* does not match the bare
	// /foo, so the exact-root cases are not redundant with the prefix
	// cases - both must be covered.
	matches := []string{
		"/overlay",
		"/overlay/",
		"/overlay/chat/abc123",
		"/api/public",
		"/api/public/",
		"/api/public/chat-overlays/abc123/config",
	}
	for _, path := range matches {
		if !isOverlaySurfacePath(path) {
			t.Errorf("isOverlaySurfacePath(%q) = false, want true", path)
		}
	}

	nonMatches := []string{
		"/",
		"/api/health",
		"/api/auth/session",
		"/api/platforms",
		"/overlays",          // similar but not the same root
		"/api/publications",  // similar but not the same root
		"/api/public-assets", // shares the prefix text but not the path segment
	}
	for _, path := range nonMatches {
		if isOverlaySurfacePath(path) {
			t.Errorf("isOverlaySurfacePath(%q) = true, want false", path)
		}
	}
}

// --- withRemoteOverlaySecurity ------------------------------------------------

func newOverlayTestHandler(t *testing.T, opts RemoteOverlayOptions) http.Handler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return withRemoteOverlaySecurity(logger, opts)(inner)
}

func TestWithRemoteOverlaySecurityIsANoOpWhenDisabled(t *testing.T) {
	handler := newOverlayTestHandler(t, RemoteOverlayOptions{Enabled: false})

	req := httptest.NewRequest(http.MethodGet, "/overlay/chat/abc", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "evil.example.com")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (disabled means zero behavior change)", recorder.Code)
	}
}

func TestWithRemoteOverlaySecurityIgnoresNonOverlayPaths(t *testing.T) {
	handler := newOverlayTestHandler(t, RemoteOverlayOptions{Enabled: true, CanonicalOrigin: "https://overlay.example.com"})

	req := httptest.NewRequest(http.MethodGet, "/api/platforms", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "stream.example.com") // the management hostname, not overlay

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (this middleware never gates non-overlay-surface paths)", recorder.Code)
	}
}

func TestWithRemoteOverlaySecurityAllowsDirectLoopbackWithNoForwardedHeaders(t *testing.T) {
	handler := newOverlayTestHandler(t, RemoteOverlayOptions{Enabled: true, CanonicalOrigin: "https://overlay.example.com"})

	req := httptest.NewRequest(http.MethodGet, "/overlay/chat/abc", nil)
	req.RemoteAddr = "127.0.0.1:54321"

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (direct loopback, the existing local Browser Source contract)", recorder.Code)
	}
}

func TestWithRemoteOverlaySecurityAcceptsTheConfiguredOverlayOrigin(t *testing.T) {
	handler := newOverlayTestHandler(t, RemoteOverlayOptions{Enabled: true, CanonicalOrigin: "https://overlay.example.com"})

	for _, path := range []string{"/overlay/chat/abc", "/api/public/chat-overlays/abc/config"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.RemoteAddr = "127.0.0.1:54321"
		req.Header.Set("X-Forwarded-Proto", "https")
		req.Header.Set("X-Forwarded-Host", "overlay.example.com")

		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200 for the correctly forwarded overlay origin", path, recorder.Code)
		}
	}
}

func TestWithRemoteOverlaySecurityRejectsTheManagementHostname(t *testing.T) {
	// docs/remote-ingest.md §11's own named case: a forwarded management
	// hostname attempting to reach the overlay surface must be rejected,
	// exactly like any other unrecognized hostname.
	handler := newOverlayTestHandler(t, RemoteOverlayOptions{Enabled: true, CanonicalOrigin: "https://overlay.example.com"})

	req := httptest.NewRequest(http.MethodGet, "/overlay/chat/abc", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "stream.example.com")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 for the management hostname forwarded against the overlay surface", recorder.Code)
	}
}

func TestWithRemoteOverlaySecurityRejectsAnUnrecognizedHostname(t *testing.T) {
	handler := newOverlayTestHandler(t, RemoteOverlayOptions{Enabled: true, CanonicalOrigin: "https://overlay.example.com"})

	req := httptest.NewRequest(http.MethodGet, "/api/public/chat-overlays/abc/config", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "evil.example.com")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 for an unrecognized forwarded hostname", recorder.Code)
	}
}

func TestWithRemoteOverlaySecurityRejectsHTTPScheme(t *testing.T) {
	handler := newOverlayTestHandler(t, RemoteOverlayOptions{Enabled: true, CanonicalOrigin: "https://overlay.example.com"})

	req := httptest.NewRequest(http.MethodGet, "/overlay/chat/abc", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	req.Header.Set("X-Forwarded-Proto", "http")
	req.Header.Set("X-Forwarded-Host", "overlay.example.com")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 for X-Forwarded-Proto: http", recorder.Code)
	}
}

func TestWithRemoteOverlaySecurityRejectsNonLoopbackPeerWithForwardedHeaders(t *testing.T) {
	handler := newOverlayTestHandler(t, RemoteOverlayOptions{Enabled: true, CanonicalOrigin: "https://overlay.example.com"})

	req := httptest.NewRequest(http.MethodGet, "/overlay/chat/abc", nil)
	req.RemoteAddr = "203.0.113.5:54321"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "overlay.example.com")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 for forwarded headers from a non-loopback peer", recorder.Code)
	}
}

func TestWithRemoteOverlaySecurityRejectsMalformedForwardedHeaders(t *testing.T) {
	handler := newOverlayTestHandler(t, RemoteOverlayOptions{Enabled: true, CanonicalOrigin: "https://overlay.example.com"})

	req := httptest.NewRequest(http.MethodGet, "/overlay/chat/abc", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	req.Header.Add("X-Forwarded-Proto", "https")
	req.Header.Add("X-Forwarded-Proto", "http")
	req.Header.Set("X-Forwarded-Host", "overlay.example.com")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 for a repeated X-Forwarded-Proto header", recorder.Code)
	}
}

func TestWithRemoteOverlaySecurityRejectsOnlyOneForwardedHeaderPresent(t *testing.T) {
	handler := newOverlayTestHandler(t, RemoteOverlayOptions{Enabled: true, CanonicalOrigin: "https://overlay.example.com"})

	req := httptest.NewRequest(http.MethodGet, "/overlay/chat/abc", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	req.Header.Set("X-Forwarded-Proto", "https")
	// X-Forwarded-Host deliberately absent.

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 when only one of the two forwarded headers is present", recorder.Code)
	}
}
