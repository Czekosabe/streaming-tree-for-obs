package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/streaming-tree/server/internal/domain/audioasset"
	"github.com/streaming-tree/server/internal/domain/visualasset"
	"github.com/streaming-tree/server/internal/domain/visualpackage"
	"github.com/streaming-tree/server/internal/domain/visualtemplate"
)

// VisualPackageService is the subset of visualpackage.Service the HTTP
// layer needs (Stage 14B, docs/visual-template-packages.md §19/§20/§43).
type VisualPackageService interface {
	ImportPreview(ctx context.Context, raw []byte) (visualpackage.PreviewSession, error)
	CancelPreview(token string) error
	Import(ctx context.Context, raw []byte) (visualtemplate.Template, error)
	ExportTemplate(ctx context.Context, templateID string) ([]byte, error)
}

// maxVisualTemplatePackageBytes mirrors visualpackage.MaxPackageBytes -
// duplicated here as an int64 the same way maxVisualTemplateImportBytes
// already mirrors visualtemplate.MaxImportBytes, purely because
// http.MaxBytesReader wants its own literal at the httpapi boundary.
const maxVisualTemplatePackageBytes = 96 << 20

// registerVisualPackageRoutes wires the Stage 14B package import/
// preview/preview-asset/cancel routes. Export is registered alongside
// the existing Stage 14A template routes (see registerVisualTemplateRoutes)
// as GET /api/visual-templates/{id}/export-package, since it extends
// that same resource rather than introducing a new one.
func registerVisualPackageRoutes(mux *http.ServeMux, logger *slog.Logger, svc VisualPackageService, assets VisualAssetService) {
	importPreviewNotAllowed := methodNotAllowed(logger, http.MethodPost)
	mux.HandleFunc("POST /api/visual-template-packages/import/preview", handleImportVisualTemplatePackagePreview(logger, svc))
	for _, m := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		mux.HandleFunc(m+" /api/visual-template-packages/import/preview", importPreviewNotAllowed)
	}

	importNotAllowed := methodNotAllowed(logger, http.MethodPost)
	mux.HandleFunc("POST /api/visual-template-packages/import", handleImportVisualTemplatePackage(logger, svc))
	for _, m := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		mux.HandleFunc(m+" /api/visual-template-packages/import", importNotAllowed)
	}

	mux.HandleFunc("DELETE /api/visual-template-packages/preview/{token}", handleCancelVisualTemplatePackagePreview(logger, svc))
	mux.HandleFunc("/api/visual-template-packages/preview/{token}", methodNotAllowed(logger, http.MethodDelete))

	mux.HandleFunc("GET /api/visual-template-packages/preview/{token}/assets/{name}", handleVisualTemplatePackagePreviewAsset(logger, assets))
	mux.HandleFunc("/api/visual-template-packages/preview/{token}/assets/{name}", methodNotAllowed(logger, http.MethodGet))
}

// --- wire DTOs ----------------------------------------------------------

type visualTemplatePackagePreviewAssetDTO struct {
	PackageAssetID string `json:"packageAssetId"`
	Kind           string `json:"kind"`
	MediaType      string `json:"mediaType"`
	SizeBytes      int64  `json:"sizeBytes"`
	DisplayName    string `json:"displayName"`
	Author         string `json:"author"`
	License        string `json:"license"`
	Notice         string `json:"notice"`
	URL            string `json:"url"`
}

type visualTemplatePackagePreviewDTO struct {
	Token       string                                 `json:"token"`
	Target      string                                 `json:"target"`
	Name        string                                 `json:"name"`
	Description string                                 `json:"description"`
	Author      string                                 `json:"author"`
	License     string                                 `json:"license"`
	Document    visualDesignDocumentDTO                `json:"document"`
	Assets      []visualTemplatePackagePreviewAssetDTO `json:"assets"`
	// AlertAudio describes the package's own optional alert-audio
	// preset (docs/alert-audio.md §12: "package preview identifies
	// audio") - nil when the package carries none. Informational only;
	// preview never stages or plays the sound bytes themselves.
	AlertAudio *visualTemplatePackagePreviewAudioDTO `json:"alertAudio,omitempty"`
	ExpiresAt  string                                `json:"expiresAt"`
}

type visualTemplatePackagePreviewAudioDTO struct {
	SoundEnabled     bool   `json:"soundEnabled"`
	SoundDisplayName string `json:"soundDisplayName,omitempty"`
	SoundDurationMS  int64  `json:"soundDurationMs,omitempty"`
	TTSEnabled       bool   `json:"ttsEnabled"`
	TTSTemplate      string `json:"ttsTemplate,omitempty"`
}

func previewToDTO(p visualpackage.PreviewSession) visualTemplatePackagePreviewDTO {
	assets := make([]visualTemplatePackagePreviewAssetDTO, 0, len(p.Assets))
	for _, a := range p.Assets {
		assets = append(assets, visualTemplatePackagePreviewAssetDTO{
			PackageAssetID: a.PackageAssetID, Kind: string(a.Kind), MediaType: string(a.MediaType), SizeBytes: a.SizeBytes,
			DisplayName: a.DisplayName, Author: a.Author, License: a.License, Notice: a.Notice,
			URL: "/api/visual-template-packages/preview/" + p.Token + "/assets/" + a.LogicalName,
		})
	}
	dto := visualTemplatePackagePreviewDTO{
		Token: p.Token, Target: string(p.Target), Name: p.Name, Description: p.Description,
		Author: p.Author, License: p.License, Document: documentToDTO(p.Document),
		Assets: assets, ExpiresAt: p.ExpiresAt.Format(rfc3339Milli),
	}
	if p.AlertAudio != nil {
		dto.AlertAudio = &visualTemplatePackagePreviewAudioDTO{
			SoundEnabled: p.AlertAudio.SoundEnabled, SoundDisplayName: p.AlertAudio.SoundDisplayName, SoundDurationMS: p.AlertAudio.SoundDurationMS,
			TTSEnabled: p.AlertAudio.TTSEnabled, TTSTemplate: p.AlertAudio.TTSTemplate,
		}
	}
	return dto
}

// --- handlers -------------------------------------------------------------

func readBoundedPackageBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxVisualTemplatePackageBytes)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return nil, fmt.Errorf("%w: request body exceeds %d bytes", visualpackage.ErrTooLarge, maxVisualTemplatePackageBytes)
		}
		return nil, fmt.Errorf("%w: %v", visualpackage.ErrInvalidArchive, err)
	}
	return data, nil
}

func handleImportVisualTemplatePackagePreview(logger *slog.Logger, svc VisualPackageService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := readBoundedPackageBody(w, r)
		if err != nil {
			writeVisualPackageError(w, logger, err)
			return
		}
		preview, err := svc.ImportPreview(r.Context(), data)
		if err != nil {
			writeVisualPackageError(w, logger, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, previewToDTO(preview))
	}
}

func handleImportVisualTemplatePackage(logger *slog.Logger, svc VisualPackageService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := readBoundedPackageBody(w, r)
		if err != nil {
			writeVisualPackageError(w, logger, err)
			return
		}
		tmpl, err := svc.Import(r.Context(), data)
		if err != nil {
			writeVisualPackageError(w, logger, err)
			return
		}
		writeJSON(w, logger, http.StatusCreated, templateToDTO(tmpl))
	}
}

func handleCancelVisualTemplatePackagePreview(logger *slog.Logger, svc VisualPackageService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := svc.CancelPreview(r.PathValue("token")); err != nil {
			writeVisualPackageError(w, logger, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// handleVisualTemplatePackagePreviewAsset streams one staged preview
// asset's bytes - management-only (never reachable from the public OBS
// surface, docs/visual-template-packages.md §44), never cached
// immutably (a preview asset is not yet a real, permanent blob).
func handleVisualTemplatePackagePreviewAsset(logger *slog.Logger, assets VisualAssetService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f, err := assets.OpenPreviewAsset(r.PathValue("token"), r.PathValue("name"))
		if err != nil {
			writeVisualAssetError(w, logger, err)
			return
		}
		defer f.Close()

		header := make([]byte, 64)
		n, _ := f.Read(header)
		if mt, ok := visualasset.DetectSignature(header[:n]); ok {
			w.Header().Set("Content-Type", string(mt))
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			writeVisualAssetError(w, logger, err)
			return
		}
		io.Copy(w, f)
	}
}

func handleExportVisualTemplatePackage(logger *slog.Logger, packages VisualPackageService, templates VisualTemplateService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t, err := templates.Export(r.Context(), r.PathValue("id"))
		if err != nil {
			writeVisualTemplateError(w, logger, err)
			return
		}
		data, err := packages.ExportTemplate(r.Context(), t.ID)
		if err != nil {
			writeVisualPackageError(w, logger, err)
			return
		}
		filename := safePackageExportFilename(t.Name)
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	}
}

// --- error mapping ------------------------------------------------------

func writeVisualPackageError(w http.ResponseWriter, logger *slog.Logger, err error) {
	switch {
	case errors.Is(err, visualpackage.ErrTooLarge):
		writeError(w, logger, http.StatusRequestEntityTooLarge, "visual_template_package_too_large", "The package exceeds the size limit.")
	case errors.Is(err, visualpackage.ErrTooManyEntries):
		writeError(w, logger, http.StatusUnprocessableEntity, "visual_template_package_too_many_entries", "The package has too many archive entries.")
	case errors.Is(err, visualpackage.ErrTooManyAssets):
		writeError(w, logger, http.StatusUnprocessableEntity, "visual_template_package_too_many_entries", "The package has too many assets.")
	case errors.Is(err, visualpackage.ErrDecompressionLimit):
		writeError(w, logger, http.StatusUnprocessableEntity, "visual_template_package_decompression_limit", "The package exceeds its decompression bound.")
	case errors.Is(err, visualpackage.ErrEntryInvalid):
		writeError(w, logger, http.StatusUnprocessableEntity, "visual_template_package_entry_invalid", "The package contains an invalid archive entry.")
	case errors.Is(err, visualpackage.ErrVersionUnsupported):
		writeError(w, logger, http.StatusUnprocessableEntity, "visual_template_package_version_unsupported", "This package's schema version is not supported.")
	case errors.Is(err, visualpackage.ErrManifestInvalid):
		writeError(w, logger, http.StatusUnprocessableEntity, "visual_template_package_manifest_invalid", "The package manifest is invalid.")
	case errors.Is(err, visualpackage.ErrAssetMissing):
		writeError(w, logger, http.StatusUnprocessableEntity, "visual_template_package_asset_missing", "The package references an asset that is not present in the archive.")
	case errors.Is(err, visualpackage.ErrAssetUnreferenced):
		writeError(w, logger, http.StatusUnprocessableEntity, "visual_template_package_asset_unreferenced", "The package archive contains an asset that is not referenced by the manifest or document.")
	case errors.Is(err, visualpackage.ErrAssetHashMismatch):
		writeError(w, logger, http.StatusUnprocessableEntity, "visual_template_package_asset_hash_mismatch", "An asset's content does not match its declared hash.")
	case errors.Is(err, visualpackage.ErrAssetTypeMismatch):
		writeError(w, logger, http.StatusUnprocessableEntity, "visual_template_package_asset_type_mismatch", "An asset's type does not match its declared kind or media type.")
	case errors.Is(err, visualpackage.ErrAssetUnsupported):
		writeError(w, logger, http.StatusUnprocessableEntity, "visual_template_package_asset_unsupported", "This asset type is not supported.")
	case errors.Is(err, visualpackage.ErrPreviewExpired):
		writeError(w, logger, http.StatusGone, "visual_template_package_preview_expired", "This preview session has expired.")
	case errors.Is(err, visualpackage.ErrPreviewNotFound):
		writeError(w, logger, http.StatusNotFound, "visual_template_package_preview_expired", "This preview session was not found.")
	case errors.Is(err, visualpackage.ErrInvalidArchive):
		writeError(w, logger, http.StatusUnprocessableEntity, "visual_template_package_invalid", "The package is not a valid archive.")
	case errors.Is(err, visualasset.ErrUnsupported):
		writeError(w, logger, http.StatusUnprocessableEntity, "visual_template_package_asset_unsupported", "This asset type is not supported.")
	case errors.Is(err, visualasset.ErrTooLarge):
		writeError(w, logger, http.StatusRequestEntityTooLarge, "visual_template_package_too_large", "An asset in the package exceeds its size limit.")
	case errors.Is(err, visualpackage.ErrAudioTargetInvalid):
		writeError(w, logger, http.StatusUnprocessableEntity, "visual_template_package_audio_target_invalid", "Alert-audio presets are only valid for an alert-target template.")
	case errors.Is(err, audioasset.ErrUnsupported):
		writeError(w, logger, http.StatusUnprocessableEntity, "visual_template_package_asset_unsupported", "This audio asset type is not supported.")
	case errors.Is(err, audioasset.ErrTooLarge):
		writeError(w, logger, http.StatusRequestEntityTooLarge, "visual_template_package_too_large", "An audio asset in the package exceeds its size limit.")
	case errors.Is(err, audioasset.ErrInvalid):
		writeError(w, logger, http.StatusUnprocessableEntity, "visual_template_package_manifest_invalid", "An audio asset in the package failed validation.")
	case errors.Is(err, visualtemplate.ErrAudioAssetNotFound):
		writeError(w, logger, http.StatusUnprocessableEntity, "visual_template_package_asset_missing", "The package's alert-audio preset references an audio asset that was not found.")
	case errors.Is(err, visualtemplate.ErrAudioNotAllowedForTarget):
		writeError(w, logger, http.StatusUnprocessableEntity, "visual_template_package_audio_target_invalid", "Alert-audio presets are only valid for an alert-target template.")
	case errors.Is(err, visualtemplate.ErrValidation):
		writeError(w, logger, http.StatusUnprocessableEntity, "visual_template_invalid", "The imported template failed validation.")
	case errors.Is(err, visualtemplate.ErrNotFound):
		writeError(w, logger, http.StatusNotFound, "visual_template_not_found", "The requested visual template does not exist.")
	default:
		writeError(w, logger, http.StatusInternalServerError, "internal_error", "An unexpected error occurred.")
	}
}

// safePackageExportFilename mirrors safeTemplateExportFilename exactly,
// with the package's own fixed extension (docs/visual-template-
// packages.md §20's "safe filename... fixed .streaming-tree-template
// extension").
func safePackageExportFilename(name string) string {
	base := safeExportBaseName(name)
	return base + ".streaming-tree-template"
}
