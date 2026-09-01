package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/backup"
)

type stubBackupService struct {
	data []byte
	err  error

	previewResult   backup.PreviewSession
	previewErr      error
	lastPreviewData []byte

	lastCancelledToken string

	restoreResult    backup.RestoreResult
	restoreErr       error
	lastRestoreToken string
}

func (s *stubBackupService) Export(context.Context) ([]byte, error) {
	return s.data, s.err
}

func (s *stubBackupService) RestorePreview(_ context.Context, data []byte) (backup.PreviewSession, error) {
	s.lastPreviewData = data
	if s.previewErr != nil {
		return backup.PreviewSession{}, s.previewErr
	}
	return s.previewResult, nil
}

func (s *stubBackupService) CancelPreview(token string) {
	s.lastCancelledToken = token
}

func (s *stubBackupService) Restore(_ context.Context, token string) (backup.RestoreResult, error) {
	s.lastRestoreToken = token
	if s.restoreErr != nil {
		return backup.RestoreResult{}, s.restoreErr
	}
	return s.restoreResult, nil
}

func newBackupServer(t *testing.T, service BackupService) http.Handler {
	t.Helper()
	return NewRouter(Options{
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		StartedAt: time.Now(),
		Backup:    service,
	})
}

func TestExportBackupReturnsAZipDownload(t *testing.T) {
	stub := &stubBackupService{data: []byte("fake-zip-bytes")}
	handler := newBackupServer(t, stub)

	recorder := do(t, handler, http.MethodPost, "/api/backup/export", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", recorder.Code, recorder.Body.String())
	}
	if ct := recorder.Header().Get("Content-Type"); ct != "application/zip" {
		t.Errorf("Content-Type = %q, want application/zip", ct)
	}
	if cd := recorder.Header().Get("Content-Disposition"); cd == "" {
		t.Error("Content-Disposition header is empty, want an attachment filename")
	}
	if recorder.Body.String() != "fake-zip-bytes" {
		t.Errorf("body = %q, want the exported bytes verbatim", recorder.Body.String())
	}
}

func TestExportBackupRejectsBody(t *testing.T) {
	stub := &stubBackupService{data: []byte("x")}
	handler := newBackupServer(t, stub)

	recorder := do(t, handler, http.MethodPost, "/api/backup/export", map[string]string{"unexpected": "body"})
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}

func TestExportBackupWrongMethodIsRejected(t *testing.T) {
	stub := &stubBackupService{}
	handler := newBackupServer(t, stub)

	recorder := do(t, handler, http.MethodGet, "/api/backup/export", nil)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", recorder.Code)
	}
}

func TestExportBackupTooLargeIsRejected(t *testing.T) {
	stub := &stubBackupService{err: backup.ErrTooLarge}
	handler := newBackupServer(t, stub)

	recorder := do(t, handler, http.MethodPost, "/api/backup/export", nil)
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", recorder.Code)
	}
}

func TestExportBackupInternalErrorIsRejected(t *testing.T) {
	stub := &stubBackupService{err: context.DeadlineExceeded}
	handler := newBackupServer(t, stub)

	recorder := do(t, handler, http.MethodPost, "/api/backup/export", nil)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}
}

func TestRestorePreviewReturnsTheUploadedBytesAndTheSummary(t *testing.T) {
	stub := &stubBackupService{previewResult: backup.PreviewSession{
		Token: "rst_1", Counts: backup.ObjectCounts{Platforms: 3}, AssetCount: 2, AssetTotalBytes: 4096,
		ConnectedAccountsRequireReconnect: 1, DestinationsNeedStreamKey: 3,
	}}
	handler := newBackupServer(t, stub)

	recorder := do(t, handler, http.MethodPost, "/api/backup/restore/preview", "raw-zip-bytes")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", recorder.Code, recorder.Body.String())
	}
	if string(stub.lastPreviewData) != "raw-zip-bytes" {
		t.Errorf("RestorePreview received %q, want the raw uploaded bytes verbatim", stub.lastPreviewData)
	}

	var body restorePreviewResponse
	decodeBody(t, recorder, &body)
	if body.Token != "rst_1" || body.Counts.Platforms != 3 || body.AssetCount != 2 {
		t.Fatalf("body = %+v", body)
	}
}

func TestRestorePreviewRejectsAnInvalidPackage(t *testing.T) {
	stub := &stubBackupService{previewErr: backup.ErrInvalidArchive}
	handler := newBackupServer(t, stub)

	recorder := do(t, handler, http.MethodPost, "/api/backup/restore/preview", "not a zip")
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestRestorePreviewWrongMethodIsRejected(t *testing.T) {
	stub := &stubBackupService{}
	handler := newBackupServer(t, stub)

	recorder := do(t, handler, http.MethodGet, "/api/backup/restore/preview", nil)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", recorder.Code)
	}
}

func TestCancelRestorePreviewCancelsTheNamedToken(t *testing.T) {
	stub := &stubBackupService{}
	handler := newBackupServer(t, stub)

	recorder := do(t, handler, http.MethodDelete, "/api/backup/restore/preview/rst_abc", nil)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204, body=%s", recorder.Code, recorder.Body.String())
	}
	if stub.lastCancelledToken != "rst_abc" {
		t.Errorf("cancelled token = %q, want rst_abc", stub.lastCancelledToken)
	}
}

func TestCancelRestorePreviewRejectsBody(t *testing.T) {
	stub := &stubBackupService{}
	handler := newBackupServer(t, stub)

	recorder := do(t, handler, http.MethodDelete, "/api/backup/restore/preview/rst_abc", map[string]string{"unexpected": "body"})
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}

func TestRestoreCommitRestoresTheNamedTokenAndReturnsTheSummary(t *testing.T) {
	stub := &stubBackupService{restoreResult: backup.RestoreResult{
		Counts: backup.ObjectCounts{Platforms: 2}, ConnectedAccountsRequireReconnect: 1,
		DestinationsNeedStreamKey: 2, DonationSourcesNeedCredential: 1, RestartRequired: true,
	}}
	handler := newBackupServer(t, stub)

	recorder := do(t, handler, http.MethodPost, "/api/backup/restore/commit/rst_abc", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", recorder.Code, recorder.Body.String())
	}
	if stub.lastRestoreToken != "rst_abc" {
		t.Errorf("Restore received token %q, want rst_abc", stub.lastRestoreToken)
	}

	var body restoreResultResponse
	decodeBody(t, recorder, &body)
	if body.Counts.Platforms != 2 || body.ConnectedAccountsRequireReconnect != 1 || body.DonationSourcesNeedCredential != 1 || !body.RestartRequired {
		t.Fatalf("body = %+v", body)
	}
}

func TestRestoreCommitUnknownTokenIsNotFound(t *testing.T) {
	stub := &stubBackupService{restoreErr: backup.ErrNotFound}
	handler := newBackupServer(t, stub)

	recorder := do(t, handler, http.MethodPost, "/api/backup/restore/commit/rst_does_not_exist", nil)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestRestoreCommitRefusesWhileStreamingIsActive(t *testing.T) {
	stub := &stubBackupService{restoreErr: backup.ErrStreamingActive}
	handler := newBackupServer(t, stub)

	recorder := do(t, handler, http.MethodPost, "/api/backup/restore/commit/rst_abc", nil)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestRestoreCommitRejectsBody(t *testing.T) {
	stub := &stubBackupService{}
	handler := newBackupServer(t, stub)

	recorder := do(t, handler, http.MethodPost, "/api/backup/restore/commit/rst_abc", map[string]string{"unexpected": "body"})
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}

func TestRestoreCommitWrongMethodIsRejected(t *testing.T) {
	stub := &stubBackupService{}
	handler := newBackupServer(t, stub)

	recorder := do(t, handler, http.MethodGet, "/api/backup/restore/commit/rst_abc", nil)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", recorder.Code)
	}
}
