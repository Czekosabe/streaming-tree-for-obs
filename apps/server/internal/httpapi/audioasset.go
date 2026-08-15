package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/streaming-tree/server/internal/domain/audioasset"
)

// AudioAssetService is the subset of audioasset.Service the HTTP layer
// needs (Stage 17B, docs/alert-audio.md §7).
type AudioAssetService interface {
	Upload(ctx context.Context, data []byte, ext, declaredMediaType, displayName, source string) (audioasset.Asset, error)
	Get(ctx context.Context, id string) (audioasset.Asset, error)
	List(ctx context.Context) ([]audioasset.Asset, error)
	Delete(ctx context.Context, id string) error
	ReferenceCount(ctx context.Context, id string) (int, error)
}

// maxAudioAssetUploadBytes bounds the whole multipart request body -
// generous relative to audioasset's own single bound (8 MiB,
// docs/alert-audio.md §5.3) so a well-formed upload is never rejected
// purely by multipart framing overhead.
const maxAudioAssetUploadBytes = 12 << 20

// maxAudioAssetMetadataFieldBytes bounds the displayName form field read
// from the multipart body, before audioasset.Service's own code-point
// bound runs.
const maxAudioAssetMetadataFieldBytes = 4 * 1024

// registerAudioAssetRoutes wires the Stage 17B managed audio asset
// management API (/api/audio-assets/...). There is no separate public
// content route - audio bytes are always served through internal/audio's
// own existing /api/public/audio/{slug}/bytes/{token} route
// (docs/alert-audio.md §5.2), never a second public byte-serving surface.
func registerAudioAssetRoutes(mux *http.ServeMux, logger *slog.Logger, svc AudioAssetService) {
	mux.HandleFunc("GET /api/audio-assets", handleListAudioAssets(logger, svc))
	mux.HandleFunc("POST /api/audio-assets", handleUploadAudioAsset(logger, svc))
	mux.HandleFunc("/api/audio-assets", methodNotAllowed(logger, http.MethodGet, http.MethodPost))

	mux.HandleFunc("GET /api/audio-assets/{id}", handleGetAudioAsset(logger, svc))
	mux.HandleFunc("DELETE /api/audio-assets/{id}", handleDeleteAudioAsset(logger, svc))
	mux.HandleFunc("/api/audio-assets/{id}", methodNotAllowed(logger, http.MethodGet, http.MethodDelete))
}

// --- wire DTOs --------------------------------------------------------

type audioAssetDTO struct {
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	MediaType      string `json:"mediaType"`
	SizeBytes      int64  `json:"sizeBytes"`
	DurationMs     int64  `json:"durationMs"`
	DisplayName    string `json:"displayName"`
	Source         string `json:"source"`
	ReferenceCount int    `json:"referenceCount"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
}

// audioAssetToDTO renders asset for a management response - never a
// local storage path, never a filesystem detail (docs/alert-audio.md
// §5.4/§13). There is deliberately no content URL field: unlike a visual
// asset, an audio asset's bytes are never served directly by local ID or
// blob hash - only ever indirectly, through whichever alert instance's
// current internal/audio item happens to reference it.
func audioAssetToDTO(asset audioasset.Asset, refCount int) audioAssetDTO {
	dto := audioAssetDTO{
		ID: asset.ID, Kind: string(asset.Kind),
		DisplayName: asset.DisplayName, Source: asset.Source, ReferenceCount: refCount,
		CreatedAt: asset.CreatedAt.Format(rfc3339Milli), UpdatedAt: asset.UpdatedAt.Format(rfc3339Milli),
	}
	if asset.Blob != nil {
		dto.MediaType = string(asset.Blob.MediaType)
		dto.SizeBytes = asset.Blob.ByteSize
		dto.DurationMs = asset.Blob.DurationMS
	}
	return dto
}

// --- management handlers ----------------------------------------------

func handleListAudioAssets(logger *slog.Logger, svc AudioAssetService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		assets, err := svc.List(r.Context())
		if err != nil {
			writeAudioAssetError(w, logger, err)
			return
		}
		out := make([]audioAssetDTO, 0, len(assets))
		for _, a := range assets {
			count, _ := svc.ReferenceCount(r.Context(), a.ID)
			out = append(out, audioAssetToDTO(a, count))
		}
		writeJSON(w, logger, http.StatusOK, out)
	}
}

func handleGetAudioAsset(logger *slog.Logger, svc AudioAssetService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		asset, err := svc.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			writeAudioAssetError(w, logger, err)
			return
		}
		count, _ := svc.ReferenceCount(r.Context(), asset.ID)
		writeJSON(w, logger, http.StatusOK, audioAssetToDTO(asset, count))
	}
}

func handleDeleteAudioAsset(logger *slog.Logger, svc AudioAssetService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := svc.Delete(r.Context(), r.PathValue("id")); err != nil {
			writeAudioAssetError(w, logger, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// handleUploadAudioAsset implements the same strict streaming multipart
// contract docs/visual-template-packages.md §17 established, reused here
// verbatim (docs/alert-audio.md §7): http.MaxBytesReader first,
// multipart.Reader streaming (never unbounded ParseMultipartForm temp-file
// spilling), exactly one binary file part, one bounded displayName text
// field, and rejection of any unrecognized part.
func handleUploadAudioAsset(logger *slog.Logger, svc AudioAssetService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxAudioAssetUploadBytes)

		mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || !strings.HasPrefix(mediaType, "multipart/") || params["boundary"] == "" {
			writeError(w, logger, http.StatusUnsupportedMediaType, "unsupported_media_type", "Request body must be multipart/form-data.")
			return
		}

		mr := multipart.NewReader(r.Body, params["boundary"])

		var (
			fileData               []byte
			fileSeen               bool
			ext, declaredMediaType string
			displayName            string
		)
		for {
			part, err := mr.NextPart()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				writeError(w, logger, http.StatusBadRequest, "malformed_multipart", "Request body could not be read as multipart/form-data.")
				return
			}

			switch part.FormName() {
			case "file":
				if fileSeen {
					part.Close()
					writeError(w, logger, http.StatusBadRequest, "audio_asset_invalid", "Only one file part is accepted.")
					return
				}
				fileSeen = true
				declaredMediaType = part.Header.Get("Content-Type")
				ext = strings.TrimPrefix(strings.ToLower(filepath.Ext(part.FileName())), ".")
				data, readErr := io.ReadAll(part)
				part.Close()
				if readErr != nil {
					writeError(w, logger, http.StatusRequestEntityTooLarge, "audio_asset_too_large", "The uploaded file is too large.")
					return
				}
				fileData = data
			case "displayName":
				data, readErr := io.ReadAll(io.LimitReader(part, maxAudioAssetMetadataFieldBytes+1))
				part.Close()
				if readErr != nil || len(data) > maxAudioAssetMetadataFieldBytes {
					writeError(w, logger, http.StatusBadRequest, "audio_asset_invalid", "The display name field is too large.")
					return
				}
				displayName = string(data)
			default:
				part.Close()
				writeError(w, logger, http.StatusBadRequest, "audio_asset_invalid", "Unrecognized form field.")
				return
			}
		}

		if !fileSeen {
			writeError(w, logger, http.StatusBadRequest, "audio_asset_invalid", "No file was uploaded.")
			return
		}

		asset, err := svc.Upload(r.Context(), fileData, ext, declaredMediaType, displayName, audioasset.SourceUpload)
		if err != nil {
			writeAudioAssetError(w, logger, err)
			return
		}
		writeJSON(w, logger, http.StatusCreated, audioAssetToDTO(asset, 0))
	}
}

// --- error mapping ------------------------------------------------------

func writeAudioAssetError(w http.ResponseWriter, logger *slog.Logger, err error) {
	switch {
	case errors.Is(err, audioasset.ErrNotFound):
		writeError(w, logger, http.StatusNotFound, "audio_asset_not_found", "The requested audio asset does not exist.")
	case errors.Is(err, audioasset.ErrInUse):
		writeError(w, logger, http.StatusConflict, "audio_asset_in_use", "This audio asset is still referenced by a saved alert rule or template.")
	case errors.Is(err, audioasset.ErrTooLarge):
		writeError(w, logger, http.StatusRequestEntityTooLarge, "audio_asset_too_large", "The audio file exceeds the size or duration limit.")
	case errors.Is(err, audioasset.ErrUnsupported):
		writeError(w, logger, http.StatusUnprocessableEntity, "audio_asset_unsupported", "This audio file type is not supported. Only 16-bit PCM WAV is accepted.")
	case errors.Is(err, audioasset.ErrInvalid):
		writeError(w, logger, http.StatusUnprocessableEntity, "audio_asset_invalid", "The audio asset metadata failed validation.")
	case errors.Is(err, audioasset.ErrStorage):
		writeError(w, logger, http.StatusInternalServerError, "audio_asset_storage_error", "The audio asset could not be read or written.")
	default:
		writeError(w, logger, http.StatusInternalServerError, "internal_error", "An unexpected error occurred.")
	}
}
