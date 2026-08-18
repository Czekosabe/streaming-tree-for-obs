package httpapi

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func testFrontend() fstest.MapFS {
	return fstest.MapFS{
		"index.html":              {Data: []byte("<!doctype html><html><body>Streaming Tree</body></html>")},
		"assets/index-abc123.js":  {Data: []byte("console.log('app')")},
		"assets/index-abc123.css": {Data: []byte("body{color:red}")},
		"favicon.ico":             {Data: []byte("icon-bytes")},
	}
}

func newProductionRouter(t *testing.T) http.Handler {
	t.Helper()
	return NewRouter(Options{
		Logger:    slog.Default(),
		StartedAt: time.Now(),
		WebAssets: testFrontend(),
	})
}

func TestProductionRootServesIndexHTML(t *testing.T) {
	handler := newProductionRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Body.String(); got == "" || !strings.Contains(got, "Streaming Tree") {
		t.Errorf("body = %q, want it to contain the index.html content", got)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache for index.html", cc)
	}
}

func TestProductionHashedAssetServedWithLongCache(t *testing.T) {
	handler := newProductionRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/assets/index-abc123.js", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); got != "console.log('app')" {
		t.Errorf("body = %q, want the real asset bytes, not index.html", got)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
		t.Errorf("Cache-Control = %q, want the long-lived immutable directive", cc)
	}
}

func TestProductionMissingStaticAssetIsRealNotFound(t *testing.T) {
	handler := newProductionRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/assets/does-not-exist-xyz.js", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "Streaming Tree") {
		t.Error("a missing hashed asset must never silently fall back to index.html")
	}
}

func TestProductionClientRouteFallsBackToIndexHTML(t *testing.T) {
	handler := newProductionRouter(t)

	for _, route := range []string{
		"/goals",
		"/settings/about",
		"/overlay/chat/some-public-slug",
		"/overlay/alerts/some-public-slug",
		"/overlay/audio/some-public-slug",
		"/overlay/widgets/some-public-slug",
	} {
		req := httptest.NewRequest(http.MethodGet, route, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("%s: expected 200 (SPA fallback), got %d", route, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "Streaming Tree") {
			t.Errorf("%s: expected the index.html body, got %q", route, rec.Body.String())
		}
	}
}

func TestProductionAPIRoutesAreNeverShadowed(t *testing.T) {
	handler := newProductionRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected /api/health to still work with WebAssets set, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/this-does-not-exist", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected the existing JSON 404 for an unknown /api/ path, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" && !strings.Contains(ct, "json") {
		t.Errorf("Content-Type = %q, want the API's own JSON 404, not index.html", ct)
	}
}

func TestProductionPathTraversalRejected(t *testing.T) {
	handler := newProductionRouter(t)

	for _, raw := range []string{
		"/../../../../etc/passwd",
		"/assets/../../../etc/passwd",
	} {
		req := httptest.NewRequest(http.MethodGet, raw, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code == http.StatusOK && strings.Contains(rec.Body.String(), "root:") {
			t.Fatalf("%s: traversal attempt leaked filesystem content", raw)
		}
	}
}

func TestProductionUnaffectedWhenWebAssetsNil(t *testing.T) {
	// Every existing development/test build - unchanged liveness behavior.
	handler := NewRouter(Options{Logger: slog.Default(), StartedAt: time.Now()})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "Streaming Tree</body>") {
		t.Error("no WebAssets means the tiny liveness JSON, not the frontend")
	}
}
