package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func decodeAboutBody(t *testing.T, body []byte) AboutResponse {
	t.Helper()
	var resp AboutResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decoding /api/about response failed: %v\nbody: %s", err, body)
	}
	return resp
}

// TestAboutReturnsFixedProductIdentity proves the canonical product-identity
// fields come back exactly as documented in docs/product-identity-legal.md,
// with no service/database dependency required.
func TestAboutReturnsFixedProductIdentity(t *testing.T) {
	handler := NewRouter(Options{Logger: slog.Default(), StartedAt: time.Now()})

	req := httptest.NewRequest(http.MethodGet, "/api/about", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeAboutBody(t, rec.Body.Bytes())

	if resp.ProductName != "Streaming Tree for OBS" {
		t.Errorf("productName = %q, want %q", resp.ProductName, "Streaming Tree for OBS")
	}
	if resp.CreatorName != "Czekosabe" {
		t.Errorf("creatorName = %q, want exactly %q", resp.CreatorName, "Czekosabe")
	}
	if resp.RepositoryURL != "https://github.com/Czekosabe/streaming-tree-for-obs" {
		t.Errorf("repositoryUrl = %q, want the canonical repository URL", resp.RepositoryURL)
	}
	if resp.CreatorURL != "https://github.com/Czekosabe" {
		t.Errorf("creatorUrl = %q, want the canonical creator profile URL", resp.CreatorURL)
	}
	if resp.SupportURL != "https://streamelements.com/czekosabe/tip" {
		t.Errorf("supportUrl = %q, want the canonical support URL", resp.SupportURL)
	}
	if resp.ApplicationLicenceStatus != "unselected" {
		t.Errorf("applicationLicenceStatus = %q, want %q (no licence has been invented)", resp.ApplicationLicenceStatus, "unselected")
	}
	if resp.IsReleaseBuild {
		t.Error("isReleaseBuild = true, want false: no release-version injection exists yet (stage 20A)")
	}
	if resp.Version == "" {
		t.Error("version is empty, want a non-empty internal identifier")
	}
}

// TestAboutNeverExposesPersonalOrLocalMetadata is a belt-and-braces scan: no
// field name or value in the raw JSON may look like a Git email, an OS
// username, a filesystem path, or a real personal name. The public creator
// identity is exactly "Czekosabe" - see docs/product-identity-legal.md.
func TestAboutNeverExposesPersonalOrLocalMetadata(t *testing.T) {
	handler := NewRouter(Options{Logger: slog.Default(), StartedAt: time.Now()})

	req := httptest.NewRequest(http.MethodGet, "/api/about", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body := rec.Body.String()

	forbidden := []string{
		"@",                         // no email address of any kind belongs in this response
		"C:\\", "/home/", "/Users/", // no filesystem path
		"tlen.pl", "kacper", // the real repository-local Git identity must never leak
	}
	lowerBody := strings.ToLower(body)
	for _, marker := range forbidden {
		if strings.Contains(lowerBody, strings.ToLower(marker)) {
			t.Errorf("GET /api/about body unexpectedly contains %q - personal/local metadata leaked: %s", marker, body)
		}
	}

	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decoding response failed: %v", err)
	}
	for _, forbiddenField := range []string{"email", "gitEmail", "username", "osUser", "hostname", "path", "dbPath", "token", "credential"} {
		if _, present := raw[forbiddenField]; present {
			t.Errorf("GET /api/about response unexpectedly contains a %q field", forbiddenField)
		}
	}
}

// TestAboutWrongMethodReturns405 matches the repository-wide convention:
// every route rejects an unsupported method with 405 and an Allow header.
func TestAboutWrongMethodReturns405(t *testing.T) {
	handler := NewRouter(Options{Logger: slog.Default(), StartedAt: time.Now()})

	req := httptest.NewRequest(http.MethodPost, "/api/about", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
	if allow := rec.Header().Get("Allow"); allow != http.MethodGet {
		t.Errorf("Allow header = %q, want %q", allow, http.MethodGet)
	}
}
