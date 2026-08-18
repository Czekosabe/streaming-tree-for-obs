package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/updater"
)

type fakeUpdateService struct {
	status            updater.Status
	setAutoCheckCalls int
	checkCalls        int
	downloadCalls     int
	installCalls      int

	checkErr    error
	downloadErr error
	installErr  error
}

func (f *fakeUpdateService) Status(ctx context.Context) updater.Status { return f.status }

func (f *fakeUpdateService) SetAutoCheck(ctx context.Context, enabled bool) error {
	f.setAutoCheckCalls++
	f.status.AutoCheck = enabled
	return nil
}

func (f *fakeUpdateService) CheckNow(ctx context.Context) error {
	f.checkCalls++
	return f.checkErr
}

func (f *fakeUpdateService) Download(ctx context.Context) error {
	f.downloadCalls++
	return f.downloadErr
}

func (f *fakeUpdateService) Install(ctx context.Context) error {
	f.installCalls++
	return f.installErr
}

func newUpdaterRouter(t *testing.T, svc *fakeUpdateService) http.Handler {
	t.Helper()
	return NewRouter(Options{
		Logger:         slog.Default(),
		StartedAt:      time.Now(),
		AllowedOrigins: []string{"http://localhost:5173"},
		Updater:        svc,
	})
}

func TestUpdateStatusReturnsSnapshot(t *testing.T) {
	svc := &fakeUpdateService{status: updater.Status{ReleaseBuild: true, CurrentVersion: "0.1.0", State: updater.StateIdle}}
	handler := newUpdaterRouter(t, svc)

	req := httptest.NewRequest(http.MethodGet, "/api/updates/status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"currentVersion":"0.1.0"`) {
		t.Fatalf("response body missing expected version: %s", rec.Body.String())
	}
}

func TestUpdateStatusNeverExposesInternalPaths(t *testing.T) {
	// A regression guard: Status's own JSON tags must never grow a
	// staging-path/helper-path/download-URL field (docs/updater.md §30).
	svc := &fakeUpdateService{status: updater.Status{ReleaseBuild: true}}
	handler := newUpdaterRouter(t, svc)

	req := httptest.NewRequest(http.MethodGet, "/api/updates/status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body := rec.Body.String()
	for _, forbidden := range []string{"path", "Path", "url", "Url", "URL", "sha256", "SHA256"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("status response unexpectedly contains %q: %s", forbidden, body)
		}
	}
}

func TestUpdateStatusRouteNotRegisteredWhenNil(t *testing.T) {
	handler := NewRouter(Options{Logger: slog.Default(), StartedAt: time.Now()})

	req := httptest.NewRequest(http.MethodGet, "/api/updates/status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Error("expected the updates route to be unavailable when Updater is nil")
	}
}

func TestUpdatePreferencesPut(t *testing.T) {
	svc := &fakeUpdateService{}
	handler := newUpdaterRouter(t, svc)

	req := httptest.NewRequest(http.MethodPut, "/api/updates/preferences", strings.NewReader(`{"autoCheck":false}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if svc.setAutoCheckCalls != 1 {
		t.Fatalf("SetAutoCheck called %d times, want 1", svc.setAutoCheckCalls)
	}
}

func TestUpdatePreferencesRejectsUnknownField(t *testing.T) {
	svc := &fakeUpdateService{}
	handler := newUpdaterRouter(t, svc)

	req := httptest.NewRequest(http.MethodPut, "/api/updates/preferences", strings.NewReader(`{"autoCheck":true,"channel":"beta"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("an unknown field must be rejected - no channel selector exists, got 200")
	}
	if svc.setAutoCheckCalls != 0 {
		t.Fatalf("SetAutoCheck called %d times, want 0", svc.setAutoCheckCalls)
	}
}

func TestUpdateCheckNoBody(t *testing.T) {
	svc := &fakeUpdateService{}
	handler := newUpdaterRouter(t, svc)

	req := httptest.NewRequest(http.MethodPost, "/api/updates/check", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if svc.checkCalls != 1 {
		t.Fatalf("CheckNow called %d times, want 1", svc.checkCalls)
	}
}

func TestUpdateCheckRejectsBody(t *testing.T) {
	svc := &fakeUpdateService{}
	handler := newUpdaterRouter(t, svc)

	req := httptest.NewRequest(http.MethodPost, "/api/updates/check", strings.NewReader(`{"force":true}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("a body must be rejected on a documented-empty endpoint, got 200")
	}
	if svc.checkCalls != 0 {
		t.Fatalf("CheckNow called %d times, want 0", svc.checkCalls)
	}
}

func TestUpdateDownloadNoBody(t *testing.T) {
	svc := &fakeUpdateService{}
	handler := newUpdaterRouter(t, svc)

	req := httptest.NewRequest(http.MethodPost, "/api/updates/download", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if svc.downloadCalls != 1 {
		t.Fatalf("Download called %d times, want 1", svc.downloadCalls)
	}
}

func TestUpdateInstallSameOriginSucceeds(t *testing.T) {
	svc := &fakeUpdateService{}
	handler := newUpdaterRouter(t, svc)

	req := httptest.NewRequest(http.MethodPost, "/api/updates/install", strings.NewReader(`{"confirm":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if svc.installCalls != 1 {
		t.Fatalf("Install called %d times, want 1", svc.installCalls)
	}
}

func TestUpdateInstallForeignOriginRejected(t *testing.T) {
	svc := &fakeUpdateService{}
	handler := newUpdaterRouter(t, svc)

	req := httptest.NewRequest(http.MethodPost, "/api/updates/install", strings.NewReader(`{"confirm":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
	if svc.installCalls != 0 {
		t.Fatalf("Install called %d times, want 0 for a foreign origin", svc.installCalls)
	}
}

// TestUpdateInstallCannotBeSubmittedByAnHTMLForm mirrors
// TestShutdownCannotBeSubmittedByAnHTMLForm exactly - the same
// cross-origin boundary protects this endpoint (docs/updater.md §29).
func TestUpdateInstallCannotBeSubmittedByAnHTMLForm(t *testing.T) {
	svc := &fakeUpdateService{}
	handler := newUpdaterRouter(t, svc)

	req := httptest.NewRequest(http.MethodPost, "/api/updates/install", strings.NewReader("confirm=true"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("a form-encoded body must never be accepted, got 200")
	}
	if svc.installCalls != 0 {
		t.Fatalf("Install called %d times, want 0", svc.installCalls)
	}
}

func TestUpdateInstallMissingConfirmationRejected(t *testing.T) {
	svc := &fakeUpdateService{}
	handler := newUpdaterRouter(t, svc)

	req := httptest.NewRequest(http.MethodPost, "/api/updates/install", strings.NewReader(`{"confirm":false}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 without confirm:true, got %d", rec.Code)
	}
	if svc.installCalls != 0 {
		t.Fatalf("Install called %d times, want 0", svc.installCalls)
	}
}

func TestUpdateInstallMapsBlockedError(t *testing.T) {
	svc := &fakeUpdateService{
		installErr: updater.ErrInstallBlocked,
		status:     updater.Status{BlockerCode: updater.BlockerStreamingActive},
	}
	handler := newUpdaterRouter(t, svc)

	req := httptest.NewRequest(http.MethodPost, "/api/updates/install", strings.NewReader(`{"confirm":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), updater.BlockerStreamingActive) {
		t.Fatalf("response does not surface the real blocker code: %s", rec.Body.String())
	}
}

// TestSystemShutdownStillWorksAfterOriginRefactor re-confirms the
// shared checkLocalActionOrigin extraction (docs/updater.md §29)
// left POST /api/system/shutdown's own behavior byte-for-byte
// unchanged - system_test.go's own suite already covers this in
// depth; this is a light cross-check that both endpoints are
// independently wired.
func TestSystemShutdownStillWorksAfterOriginRefactor(t *testing.T) {
	var shutdownCalled bool
	handler := NewRouter(Options{
		Logger: slog.Default(), StartedAt: time.Now(),
		AllowedOrigins: []string{"http://localhost:5173"},
		Shutdown:       context.CancelFunc(func() { shutdownCalled = true }),
	})

	req := httptest.NewRequest(http.MethodPost, "/api/system/shutdown", strings.NewReader(`{"confirm":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || !shutdownCalled {
		t.Fatalf("shutdown endpoint regressed: status=%d called=%v", rec.Code, shutdownCalled)
	}
}
