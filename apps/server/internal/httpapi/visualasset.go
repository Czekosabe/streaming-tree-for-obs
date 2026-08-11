package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/streaming-tree/server/internal/domain/visualasset"
)

// VisualAssetService is the subset of visualasset.Service the HTTP
// layer needs (Stage 14B, docs/visual-template-packages.md §17/§18).
type VisualAssetService interface {
	Upload(ctx context.Context, data []byte, ext, declaredMediaType, displayName, author, license, notice, source string) (visualasset.Asset, error)
	Get(ctx context.Context, id string) (visualasset.Asset, error)
	List(ctx context.Context) ([]visualasset.Asset, error)
	UpdateMetadata(ctx context.Context, id, displayName, author, license, notice string) (visualasset.Asset, error)
	Delete(ctx context.Context, id string) error
	ReferenceCount(ctx context.Context, id string) (int, error)
	PublicBlobByToken(ctx context.Context, token string) (visualasset.Blob, error)
	OpenBlob(sha256Hex string) (*os.File, error)
	OpenPreviewAsset(token, logicalName string) (*os.File, error)
}

// maxVisualAssetUploadBytes bounds the whole multipart request body -
// generous relative to visualasset's own largest single-kind bound
// (64 MiB video, docs/visual-template-packages.md §10) so a well-formed
// upload is never rejected purely by multipart framing overhead.
const maxVisualAssetUploadBytes = 72 << 20

// maxVisualAssetMetadataFieldBytes bounds each individual metadata form
// field read from the multipart body, before visualasset.Service's own
// code-point bound runs.
const maxVisualAssetMetadataFieldBytes = 4 * 1024

// registerVisualAssetRoutes wires the Stage 14B managed-asset management
// API (/api/visual-assets/...) and the public, unauthenticated asset
// content route (/api/public/visual-assets/{token}) an OBS Browser
// Source or the Designer's own preview loads images/video/fonts from.
func registerVisualAssetRoutes(mux *http.ServeMux, logger *slog.Logger, svc VisualAssetService) {
	mux.HandleFunc("GET /api/visual-assets", handleListVisualAssets(logger, svc))
	mux.HandleFunc("POST /api/visual-assets", handleUploadVisualAsset(logger, svc))
	mux.HandleFunc("/api/visual-assets", methodNotAllowed(logger, http.MethodGet, http.MethodPost))

	mux.HandleFunc("GET /api/visual-assets/{id}", handleGetVisualAsset(logger, svc))
	mux.HandleFunc("PUT /api/visual-assets/{id}", handleUpdateVisualAssetMetadata(logger, svc))
	mux.HandleFunc("DELETE /api/visual-assets/{id}", handleDeleteVisualAsset(logger, svc))
	mux.HandleFunc("/api/visual-assets/{id}", methodNotAllowed(logger, http.MethodGet, http.MethodPut, http.MethodDelete))

	mux.HandleFunc("GET /api/public/visual-assets/{token}", handlePublicVisualAsset(logger, svc))
	mux.HandleFunc("HEAD /api/public/visual-assets/{token}", handlePublicVisualAsset(logger, svc))
	mux.HandleFunc("/api/public/visual-assets/{token}", methodNotAllowed(logger, http.MethodGet, http.MethodHead))
}

// --- wire DTOs --------------------------------------------------------

type visualAssetDTO struct {
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	MediaType      string `json:"mediaType"`
	SizeBytes      int64  `json:"sizeBytes"`
	DisplayName    string `json:"displayName"`
	Author         string `json:"author"`
	License        string `json:"license"`
	Notice         string `json:"notice"`
	Source         string `json:"source"`
	URL            string `json:"url"`
	ReferenceCount int    `json:"referenceCount"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
}

// visualAssetToDTO renders asset for a management response. The content
// URL is the same public, app-owned URL a Browser Source would use -
// blob bytes themselves are not sensitive (they are meant to be shown),
// only the mapping from "which owner uses this" is management-only, so
// no separate management-only content endpoint exists (docs/visual-
// template-packages.md §18/§38).
func visualAssetToDTO(asset visualasset.Asset, refCount int) visualAssetDTO {
	dto := visualAssetDTO{
		ID: asset.ID, Kind: string(asset.Kind),
		DisplayName: asset.DisplayName, Author: asset.Author, License: asset.License, Notice: asset.Notice,
		Source: asset.Source, ReferenceCount: refCount,
		CreatedAt: asset.CreatedAt.Format(rfc3339Milli), UpdatedAt: asset.UpdatedAt.Format(rfc3339Milli),
	}
	if asset.Blob != nil {
		dto.MediaType = string(asset.Blob.MediaType)
		dto.SizeBytes = asset.Blob.ByteSize
		dto.URL = "/api/public/visual-assets/" + asset.Blob.PublicToken
	}
	return dto
}

const rfc3339Milli = "2006-01-02T15:04:05.000Z07:00"

// --- management handlers ----------------------------------------------

func handleListVisualAssets(logger *slog.Logger, svc VisualAssetService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		assets, err := svc.List(r.Context())
		if err != nil {
			writeVisualAssetError(w, logger, err)
			return
		}
		out := make([]visualAssetDTO, 0, len(assets))
		for _, a := range assets {
			count, _ := svc.ReferenceCount(r.Context(), a.ID)
			out = append(out, visualAssetToDTO(a, count))
		}
		writeJSON(w, logger, http.StatusOK, out)
	}
}

func handleGetVisualAsset(logger *slog.Logger, svc VisualAssetService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		asset, err := svc.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			writeVisualAssetError(w, logger, err)
			return
		}
		count, _ := svc.ReferenceCount(r.Context(), asset.ID)
		writeJSON(w, logger, http.StatusOK, visualAssetToDTO(asset, count))
	}
}

type updateVisualAssetMetadataRequest struct {
	DisplayName string `json:"displayName"`
	Author      string `json:"author"`
	License     string `json:"license"`
	Notice      string `json:"notice"`
}

func handleUpdateVisualAssetMetadata(logger *slog.Logger, svc VisualAssetService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body updateVisualAssetMetadataRequest
		if err := decodeJSON(w, r, &body); err != nil {
			writeDecodeError(w, logger, err)
			return
		}
		asset, err := svc.UpdateMetadata(r.Context(), r.PathValue("id"), body.DisplayName, body.Author, body.License, body.Notice)
		if err != nil {
			writeVisualAssetError(w, logger, err)
			return
		}
		count, _ := svc.ReferenceCount(r.Context(), asset.ID)
		writeJSON(w, logger, http.StatusOK, visualAssetToDTO(asset, count))
	}
}

func handleDeleteVisualAsset(logger *slog.Logger, svc VisualAssetService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := svc.Delete(r.Context(), r.PathValue("id")); err != nil {
			writeVisualAssetError(w, logger, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// handleUploadVisualAsset implements the strict streaming multipart
// contract docs/visual-template-packages.md §17/§33 requires:
// http.MaxBytesReader first, multipart.Reader streaming (never
// unbounded ParseMultipartForm temp-file spilling), exactly one binary
// file part, bounded metadata text fields, and rejection of any
// unrecognized part.
func handleUploadVisualAsset(logger *slog.Logger, svc VisualAssetService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxVisualAssetUploadBytes)

		mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || !strings.HasPrefix(mediaType, "multipart/") || params["boundary"] == "" {
			writeError(w, logger, http.StatusUnsupportedMediaType, "unsupported_media_type", "Request body must be multipart/form-data.")
			return
		}

		mr := multipart.NewReader(r.Body, params["boundary"])

		var (
			fileData                             []byte
			fileSeen                             bool
			ext, declaredMediaType               string
			displayName, author, license, notice string
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
					writeError(w, logger, http.StatusBadRequest, "visual_asset_invalid", "Only one file part is accepted.")
					return
				}
				fileSeen = true
				declaredMediaType = part.Header.Get("Content-Type")
				ext = strings.TrimPrefix(strings.ToLower(filepath.Ext(part.FileName())), ".")
				data, readErr := io.ReadAll(part)
				part.Close()
				if readErr != nil {
					writeError(w, logger, http.StatusRequestEntityTooLarge, "visual_asset_too_large", "The uploaded file is too large.")
					return
				}
				fileData = data
			case "displayName", "author", "license", "notice":
				data, readErr := io.ReadAll(io.LimitReader(part, maxVisualAssetMetadataFieldBytes+1))
				part.Close()
				if readErr != nil || len(data) > maxVisualAssetMetadataFieldBytes {
					writeError(w, logger, http.StatusBadRequest, "visual_asset_invalid", "A metadata field is too large.")
					return
				}
				switch part.FormName() {
				case "displayName":
					displayName = string(data)
				case "author":
					author = string(data)
				case "license":
					license = string(data)
				case "notice":
					notice = string(data)
				}
			default:
				part.Close()
				writeError(w, logger, http.StatusBadRequest, "visual_asset_invalid", "Unrecognized form field.")
				return
			}
		}

		if !fileSeen {
			writeError(w, logger, http.StatusBadRequest, "visual_asset_invalid", "No file was uploaded.")
			return
		}

		asset, err := svc.Upload(r.Context(), fileData, ext, declaredMediaType, displayName, author, license, notice, visualasset.SourceUpload)
		if err != nil {
			writeVisualAssetError(w, logger, err)
			return
		}
		writeJSON(w, logger, http.StatusCreated, visualAssetToDTO(asset, 0))
	}
}

// --- public content serving --------------------------------------------

// handlePublicVisualAsset serves one blob's raw bytes by its public
// token (docs/visual-template-packages.md §18/§36) - correct Content-
// Type/Content-Length/nosniff/immutable caching, and HTTP Range support
// via the standard library's own http.ServeContent (206/416 handled
// correctly by the stdlib rather than a hand-rolled reimplementation).
func handlePublicVisualAsset(logger *slog.Logger, svc VisualAssetService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.PathValue("token")
		blob, err := svc.PublicBlobByToken(r.Context(), token)
		if err != nil {
			writeVisualAssetError(w, logger, err)
			return
		}
		f, err := svc.OpenBlob(blob.SHA256)
		if err != nil {
			writeVisualAssetError(w, logger, err)
			return
		}
		defer f.Close()

		w.Header().Set("Content-Type", string(blob.MediaType))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		http.ServeContent(w, r, "", blob.CreatedAt, f)
	}
}

// --- error mapping ------------------------------------------------------

func writeVisualAssetError(w http.ResponseWriter, logger *slog.Logger, err error) {
	switch {
	case errors.Is(err, visualasset.ErrNotFound):
		writeError(w, logger, http.StatusNotFound, "visual_asset_not_found", "The requested visual asset does not exist.")
	case errors.Is(err, visualasset.ErrInUse):
		writeError(w, logger, http.StatusConflict, "visual_asset_in_use", "This asset is still referenced by a saved design or template.")
	case errors.Is(err, visualasset.ErrTooLarge):
		writeError(w, logger, http.StatusRequestEntityTooLarge, "visual_asset_too_large", "The asset exceeds the size limit for its kind.")
	case errors.Is(err, visualasset.ErrUnsupported):
		writeError(w, logger, http.StatusUnprocessableEntity, "visual_asset_unsupported", "This file type is not supported.")
	case errors.Is(err, visualasset.ErrInvalid):
		writeError(w, logger, http.StatusUnprocessableEntity, "visual_asset_invalid", "The asset metadata failed validation.")
	case errors.Is(err, visualasset.ErrStorage):
		writeError(w, logger, http.StatusInternalServerError, "visual_asset_storage_error", "The asset could not be read or written.")
	default:
		writeError(w, logger, http.StatusInternalServerError, "internal_error", "An unexpected error occurred.")
	}
}
