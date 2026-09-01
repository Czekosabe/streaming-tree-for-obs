package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/streaming-tree/server/internal/domain/backup"
)

// BackupService is the subset of backup.Service the HTTP layer needs.
type BackupService interface {
	Export(ctx context.Context) ([]byte, error)
	RestorePreview(ctx context.Context, data []byte) (backup.PreviewSession, error)
	CancelPreview(token string)
	Restore(ctx context.Context, token string) (backup.RestoreResult, error)
}

// registerBackupRoutes wires the Stage 23 configuration backup API
// (docs/backup-restore.md). Never registered under /api/public/*: a
// backup can only ever be created/restored through the same
// management-plane route namespace every other configuration-
// mutating/reading endpoint already uses, so it automatically
// inherits withRemoteManagementSecurity whenever remote management is
// enabled - no new auth mechanism (docs/backup-restore.md §9/§10).
func registerBackupRoutes(mux *http.ServeMux, logger *slog.Logger, service BackupService) {
	mux.HandleFunc("POST /api/backup/export", handleExportBackup(logger, service))
	mux.HandleFunc("/api/backup/export", methodNotAllowed(logger, http.MethodPost))

	mux.HandleFunc("POST /api/backup/restore/preview", handleRestorePreview(logger, service))
	mux.HandleFunc("/api/backup/restore/preview", methodNotAllowed(logger, http.MethodPost))

	mux.HandleFunc("DELETE /api/backup/restore/preview/{token}", handleCancelRestorePreview(logger, service))
	mux.HandleFunc("/api/backup/restore/preview/{token}", methodNotAllowed(logger, http.MethodDelete))

	// The destructive commit step (docs/backup-restore.md §7): re-
	// validates the SAME staged bytes RestorePreview already validated,
	// takes a pre-restore safety snapshot, clears, and re-inserts with
	// fresh local identities. Never reachable without first calling
	// RestorePreview to obtain token - there is no way to POST raw
	// archive bytes directly to this route.
	//
	// A distinct "commit" literal segment (rather than
	// /api/backup/restore/{token}) avoids an ambiguous-overlap panic
	// against the catch-all /api/backup/restore/preview registration
	// above, which (having no method prefix) matches every method for
	// that exact path.
	mux.HandleFunc("POST /api/backup/restore/commit/{token}", handleRestoreCommit(logger, service))
	mux.HandleFunc("/api/backup/restore/commit/{token}", methodNotAllowed(logger, http.MethodPost))
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

// readBoundedBackupBody bounds an upload BEFORE fully buffering it -
// the same http.MaxBytesReader-first pattern
// readBoundedPackageBody (visualpackage.go) already established.
func readBoundedBackupBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, backup.MaxPackageBytes)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return nil, fmt.Errorf("%w: request body exceeds %d bytes", backup.ErrTooLarge, backup.MaxPackageBytes)
		}
		return nil, fmt.Errorf("%w: %v", backup.ErrInvalidArchive, err)
	}
	return data, nil
}

type backupManifestResponse struct {
	FormatVersion    int    `json:"formatVersion"`
	Product          string `json:"product"`
	CreatedAt        string `json:"createdAt"`
	SourceAppVersion string `json:"sourceAppVersion"`
	SourcePlatform   string `json:"sourcePlatform"`
}

type restorePreviewResponse struct {
	Token                             string                 `json:"token"`
	Manifest                          backupManifestResponse `json:"manifest"`
	Counts                            backup.ObjectCounts    `json:"counts"`
	AssetCount                        int                    `json:"assetCount"`
	AssetTotalBytes                   int64                  `json:"assetTotalBytes"`
	ExpiresAt                         string                 `json:"expiresAt"`
	ConnectedAccountsRequireReconnect int                    `json:"connectedAccountsRequireReconnect"`
	DestinationsNeedStreamKey         int                    `json:"destinationsNeedStreamKey"`
	DonationSourcesNeedCredential     int                    `json:"donationSourcesNeedCredential"`
}

func toRestorePreviewResponse(p backup.PreviewSession) restorePreviewResponse {
	return restorePreviewResponse{
		Token: p.Token,
		Manifest: backupManifestResponse{
			FormatVersion: p.Manifest.FormatVersion, Product: p.Manifest.Product,
			CreatedAt:        p.Manifest.CreatedAt.UTC().Format(time.RFC3339Nano),
			SourceAppVersion: p.Manifest.SourceAppVersion, SourcePlatform: p.Manifest.SourcePlatform,
		},
		Counts: p.Counts, AssetCount: p.AssetCount, AssetTotalBytes: p.AssetTotalBytes,
		ExpiresAt:                         p.ExpiresAt.UTC().Format(time.RFC3339Nano),
		ConnectedAccountsRequireReconnect: p.ConnectedAccountsRequireReconnect,
		DestinationsNeedStreamKey:         p.DestinationsNeedStreamKey,
		DonationSourcesNeedCredential:     p.DonationSourcesNeedCredential,
	}
}

func handleRestorePreview(logger *slog.Logger, service BackupService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := readBoundedBackupBody(w, r)
		if err != nil {
			writeBackupError(w, logger, r, err)
			return
		}
		preview, err := service.RestorePreview(r.Context(), data)
		if err != nil {
			writeBackupError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toRestorePreviewResponse(preview))
	}
}

func handleCancelRestorePreview(logger *slog.Logger, service BackupService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}
		service.CancelPreview(r.PathValue("token"))
		w.WriteHeader(http.StatusNoContent)
	}
}

type restoreResultResponse struct {
	Counts                            backup.ObjectCounts `json:"counts"`
	ConnectedAccountsRequireReconnect int                 `json:"connectedAccountsRequireReconnect"`
	DestinationsNeedStreamKey         int                 `json:"destinationsNeedStreamKey"`
	DonationSourcesNeedCredential     int                 `json:"donationSourcesNeedCredential"`
	RestartRequired                   bool                `json:"restartRequired"`
}

func toRestoreResultResponse(res backup.RestoreResult) restoreResultResponse {
	return restoreResultResponse{
		Counts:                            res.Counts,
		ConnectedAccountsRequireReconnect: res.ConnectedAccountsRequireReconnect,
		DestinationsNeedStreamKey:         res.DestinationsNeedStreamKey,
		DonationSourcesNeedCredential:     res.DonationSourcesNeedCredential,
		RestartRequired:                   res.RestartRequired,
	}
}

func handleRestoreCommit(logger *slog.Logger, service BackupService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}
		result, err := service.Restore(r.Context(), r.PathValue("token"))
		if err != nil {
			writeBackupError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toRestoreResultResponse(result))
	}
}

func writeBackupError(w http.ResponseWriter, logger *slog.Logger, r *http.Request, err error) {
	switch {
	case errors.Is(err, backup.ErrTooLarge):
		writeError(w, logger, http.StatusRequestEntityTooLarge, "backup_too_large", "The backup exceeds a size limit.")
	case errors.Is(err, backup.ErrTooManyEntries):
		writeError(w, logger, http.StatusUnprocessableEntity, "backup_too_many_entries", "The backup has too many archive entries.")
	case errors.Is(err, backup.ErrDecompressionLimit):
		writeError(w, logger, http.StatusUnprocessableEntity, "backup_decompression_limit", "The backup exceeds its decompression bound.")
	case errors.Is(err, backup.ErrEntryInvalid):
		writeError(w, logger, http.StatusUnprocessableEntity, "backup_entry_invalid", "The backup contains an invalid archive entry.")
	case errors.Is(err, backup.ErrProductMismatch):
		writeError(w, logger, http.StatusUnprocessableEntity, "backup_product_mismatch", "This file was not produced by Streaming Tree for OBS.")
	case errors.Is(err, backup.ErrVersionUnsupported):
		writeError(w, logger, http.StatusUnprocessableEntity, "backup_version_unsupported", "This backup's format version is not supported.")
	case errors.Is(err, backup.ErrManifestInvalid):
		writeError(w, logger, http.StatusUnprocessableEntity, "backup_manifest_invalid", "The backup manifest is invalid.")
	case errors.Is(err, backup.ErrConfigInvalid):
		writeError(w, logger, http.StatusUnprocessableEntity, "backup_config_invalid", "The backup configuration payload is invalid.")
	case errors.Is(err, backup.ErrAssetMissing):
		writeError(w, logger, http.StatusUnprocessableEntity, "backup_asset_missing", "The backup references an asset that is not present in the archive.")
	case errors.Is(err, backup.ErrAssetUnreferenced):
		writeError(w, logger, http.StatusUnprocessableEntity, "backup_asset_unreferenced", "The backup archive contains an asset not referenced by the manifest.")
	case errors.Is(err, backup.ErrAssetHashMismatch):
		writeError(w, logger, http.StatusUnprocessableEntity, "backup_asset_hash_mismatch", "An asset's content does not match its declared hash.")
	case errors.Is(err, backup.ErrInvalidArchive):
		writeError(w, logger, http.StatusUnprocessableEntity, "backup_invalid", "The backup is not a valid archive.")
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
