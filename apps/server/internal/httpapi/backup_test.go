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
}

func (s *stubBackupService) Export(context.Context) ([]byte, error) {
	return s.data, s.err
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
