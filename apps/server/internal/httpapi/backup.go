package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/streaming-tree/server/internal/domain/backup"
)

// BackupService is the subset of backup.Service the HTTP layer needs.
type BackupService interface {
	Export(ctx context.Context) ([]byte, error)
}

// registerBackupRoutes wires the Stage 23 configuration backup API
// (docs/backup-restore.md). Never registered under /api/public/*: a
// backup can only ever be created through the same management-plane
// route namespace every other configuration-mutating/reading endpoint
// already uses, so it automatically inherits
// withRemoteManagementSecurity whenever remote management is enabled -
// no new auth mechanism (docs/backup-restore.md §9/§10).
func registerBackupRoutes(mux *http.ServeMux, logger *slog.Logger, service BackupService) {
	mux.HandleFunc("POST /api/backup/export", handleExportBackup(logger, service))
	mux.HandleFunc("/api/backup/export", methodNotAllowed(logger, http.MethodPost))
}

func handleExportBackup(logger *slog.Logger, service BackupService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}

		data, err := service.Export(r.Context())
		if err != nil {
			writeBackupError(w, logger, r, err)
			return
		}

		filename := fmt.Sprintf("streaming-tree-backup-%s%s", time.Now().UTC().Format("2006-01-02-150405"), backup.Extension)
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	}
}

func writeBackupError(w http.ResponseWriter, logger *slog.Logger, r *http.Request, err error) {
	switch {
	case errors.Is(err, backup.ErrTooLarge):
		writeError(w, logger, http.StatusRequestEntityTooLarge, "backup_too_large", "The backup exceeds a size limit.")
	case errors.Is(err, backup.ErrTooManyEntries):
		writeError(w, logger, http.StatusUnprocessableEntity, "backup_too_many_entries", "The backup has too many archive entries.")
	case errors.Is(err, backup.ErrNotFound):
		writeError(w, logger, http.StatusNotFound, "not_found", "The requested backup session does not exist.")
	case errors.Is(err, backup.ErrStreamingActive):
		writeError(w, logger, http.StatusConflict, "restore_blocked_streaming_active", "Streaming is active. Stop streaming before restoring a backup.")
	default:
		logger.Error("unhandled backup error",
			slog.String("path", r.URL.Path), slog.String("method", r.Method), slog.Any("error", err))
		writeError(w, logger, http.StatusInternalServerError, "internal_error", "The server encountered an unexpected error.")
	}
}
