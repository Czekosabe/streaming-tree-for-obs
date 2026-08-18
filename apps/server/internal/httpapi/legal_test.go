package httpapi

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
	"time"
)

func testLegalAssets() fstest.MapFS {
	return fstest.MapFS{
		"LICENSE":                {Data: []byte("GNU GENERAL PUBLIC LICENSE test fixture")},
		"PRIVACY.md":             {Data: []byte("privacy test fixture")},
		"LEGAL.md":               {Data: []byte("legal test fixture")},
		"THIRD_PARTY_NOTICES.md": {Data: []byte("third-party notices test fixture")},
	}
}

func newLegalRouter(t *testing.T) http.Handler {
	t.Helper()
	return NewRouter(Options{
		Logger:      slog.Default(),
		StartedAt:   time.Now(),
		LegalAssets: testLegalAssets(),
	})
}

func TestLegalRoutesServeTheFixedAllowlist(t *testing.T) {
	handler := newLegalRouter(t)

	cases := map[string]string{
		"/legal/license":             "GNU GENERAL PUBLIC LICENSE test fixture",
		"/legal/privacy":             "privacy test fixture",
		"/legal/legal":               "legal test fixture",
		"/legal/third-party-notices": "third-party notices test fixture",
	}

	for path, want := range cases {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("%s: expected 200, got %d", path, rec.Code)
			continue
		}
		if got := rec.Body.String(); got != want {
			t.Errorf("%s: body = %q, want %q", path, got, want)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
			t.Errorf("%s: Content-Type = %q, want plain text", path, ct)
		}
	}
}

func TestLegalRouteWrongMethodReturns405(t *testing.T) {
	handler := newLegalRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/legal/license", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

// TestLegalRoutesRejectArbitraryFilenames proves the allowlist is closed:
// there is no {name} path parameter anywhere under /legal/ that could
// resolve an arbitrary file from the embedded filesystem.
func TestLegalRoutesRejectArbitraryFilenames(t *testing.T) {
	handler := newLegalRouter(t)

	for _, path := range []string{
		"/legal/LICENSE",
		"/legal/does-not-exist",
		"/legal/../LICENSE",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code == http.StatusOK {
			t.Errorf("%s: unexpectedly returned 200 - the allowlist must be closed", path)
		}
	}
}

func TestLegalRoutesNotRegisteredWhenNil(t *testing.T) {
	handler := NewRouter(Options{Logger: slog.Default(), StartedAt: time.Now()})

	req := httptest.NewRequest(http.MethodGet, "/legal/license", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Error("expected /legal/license to be unavailable when LegalAssets is nil")
	}
}
