package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"unicode"

	alertsdomain "github.com/streaming-tree/server/internal/domain/alerts"
	chatoverlaydomain "github.com/streaming-tree/server/internal/domain/chatoverlay"
	"github.com/streaming-tree/server/internal/domain/visualdesign"
	"github.com/streaming-tree/server/internal/domain/visualtemplate"
)

// maxVisualTemplateImportBytes mirrors visualtemplate.MaxImportBytes -
// duplicated as an int64 constant here purely because
// decodeJSONWithLimit wants an int64 and the domain package's own
// constant is untyped-int; the two are asserted equal in this file's
// own test.
const maxVisualTemplateImportBytes = int64(visualtemplate.MaxImportBytes)

// VisualTemplateService is the subset of *visualtemplate.Service the
// HTTP layer depends on (Stage 14A task Part 26/27).
type VisualTemplateService interface {
	Builtins() []visualtemplate.Template
	List(ctx context.Context) ([]visualtemplate.Template, error)
	Get(ctx context.Context, id string) (visualtemplate.Template, error)
	Create(ctx context.Context, target visualtemplate.Target, name, description, author, license string, doc visualdesign.Document) (visualtemplate.Template, error)
	UpdateMetadata(ctx context.Context, id, name, description, author, license string) (visualtemplate.Template, error)
	Delete(ctx context.Context, id string) error
	ImportPreview(candidate visualtemplate.Template) (visualtemplate.Template, error)
	Import(ctx context.Context, candidate visualtemplate.Template) (visualtemplate.Template, error)
	Export(ctx context.Context, id string) (visualtemplate.Template, error)
}

// registerVisualTemplateRoutes wires Stage 14A's visual-template
// library management API - a management/editor surface only, never
// exposed on the public OBS Browser Source API (Stage 14A task Part
// 27). alerts/chatOverlays are used only to resolve an optional
// ?ownerId= query parameter into a real owner-instance compatibility
// check (internal/domain/alerts.ValidateDesignBindingsForEventType /
// internal/domain/chatoverlay.ValidateDesignBindingsForChatOverlay) -
// either may be nil, in which case compatibility falls back to
// target-level only.
func registerVisualTemplateRoutes(mux *http.ServeMux, logger *slog.Logger, svc VisualTemplateService, alerts AlertsService, chatOverlays ChatOverlayProfileService) {
	mux.HandleFunc("GET /api/visual-templates", handleListVisualTemplates(logger, svc, alerts, chatOverlays))
	mux.HandleFunc("POST /api/visual-templates", handleCreateVisualTemplate(logger, svc))
	mux.HandleFunc("/api/visual-templates", methodNotAllowed(logger, http.MethodGet, http.MethodPost))

	// Deliberately no bare (any-method) fallback registration for these
	// two literal paths: a bare pattern here would be genuinely
	// ambiguous against "GET /api/visual-templates/{id}" (id could be
	// literally "import") and net/http's ServeMux rejects that
	// combination at registration time. A non-POST request to either
	// path instead falls through to the {id} wildcard (treating
	// "import"/"import" as a template id, itself falling through to a
	// 404) - still a client error, never a security concern.
	mux.HandleFunc("POST /api/visual-templates/import/preview", handleImportVisualTemplatePreview(logger, svc, alerts, chatOverlays))
	mux.HandleFunc("POST /api/visual-templates/import", handleImportVisualTemplate(logger, svc))

	mux.HandleFunc("GET /api/visual-templates/{id}", handleGetVisualTemplate(logger, svc))
	mux.HandleFunc("PUT /api/visual-templates/{id}", handleUpdateVisualTemplateMetadata(logger, svc))
	mux.HandleFunc("DELETE /api/visual-templates/{id}", handleDeleteVisualTemplate(logger, svc))
	mux.HandleFunc("/api/visual-templates/{id}", methodNotAllowed(logger, http.MethodGet, http.MethodPut, http.MethodDelete))

	mux.HandleFunc("GET /api/visual-templates/{id}/export", handleExportVisualTemplate(logger, svc))
	mux.HandleFunc("/api/visual-templates/{id}/export", methodNotAllowed(logger, http.MethodGet))
}

// --- wire DTOs --------------------------------------------------------

// visualTemplateDTO is the management-API shape - includes local
// identity/timestamps/source, and an optional Compatibility block when
// an owner context was supplied. Never the shape sent/received for
// portable import/export (see visualTemplateFileDTO).
type visualTemplateDTO struct {
	ID                    string                    `json:"id"`
	Target                string                    `json:"target"`
	Source                string                    `json:"source"`
	Name                  string                    `json:"name"`
	Description           string                    `json:"description"`
	Author                string                    `json:"author"`
	License               string                    `json:"license"`
	TemplateSchemaVersion int                       `json:"templateSchemaVersion"`
	Document              visualDesignDocumentDTO   `json:"document"`
	CreatedAt             string                    `json:"createdAt,omitempty"`
	UpdatedAt             string                    `json:"updatedAt,omitempty"`
	Compatibility         *templateCompatibilityDTO `json:"compatibility,omitempty"`
}

type templateCompatibilityDTO struct {
	Compatible bool     `json:"compatible"`
	Blockers   []string `json:"blockers,omitempty"`
}

// visualTemplateFileDTO is the Stage 14A portable, asset-free JSON
// template-interchange shape (docs/visual-templates.md) - used for
// both import (request body) and export (response body). Never
// includes a local database id, timestamps, or any owner id (Stage
// 14A task Part 10/22).
type visualTemplateFileDTO struct {
	Format        string                  `json:"format"`
	SchemaVersion int                     `json:"schemaVersion"`
	Target        string                  `json:"target"`
	Name          string                  `json:"name"`
	Description   string                  `json:"description"`
	Author        string                  `json:"author"`
	License       string                  `json:"license"`
	VisualDesign  visualDesignDocumentDTO `json:"visualDesign"`
}

func templateToDTO(t visualtemplate.Template) visualTemplateDTO {
	dto := visualTemplateDTO{
		ID: t.ID, Target: string(t.Target), Source: string(t.Source),
		Name: t.Name, Description: t.Description, Author: t.Author, License: t.License,
		TemplateSchemaVersion: t.TemplateSchemaVersion,
		Document:              documentToDTO(t.Document),
	}
	if !t.CreatedAt.IsZero() {
		dto.CreatedAt = t.CreatedAt.Format("2006-01-02T15:04:05.000Z")
	}
	if !t.UpdatedAt.IsZero() {
		dto.UpdatedAt = t.UpdatedAt.Format("2006-01-02T15:04:05.000Z")
	}
	return dto
}

func templateToFileDTO(t visualtemplate.Template) visualTemplateFileDTO {
	return visualTemplateFileDTO{
		Format: visualtemplate.Format, SchemaVersion: t.TemplateSchemaVersion, Target: string(t.Target),
		Name: t.Name, Description: t.Description, Author: t.Author, License: t.License,
		VisualDesign: documentToDTO(t.Document),
	}
}

func templateFromFileDTO(dto visualTemplateFileDTO) (visualtemplate.Template, error) {
	if dto.Format != visualtemplate.Format {
		return visualtemplate.Template{}, fmt.Errorf("%w: unrecognized format %q", visualtemplate.ErrValidation, dto.Format)
	}
	return visualtemplate.Template{
		Target: visualtemplate.Target(dto.Target), Source: visualtemplate.SourceUser,
		Name: dto.Name, Description: dto.Description, Author: dto.Author, License: dto.License,
		TemplateSchemaVersion: dto.SchemaVersion,
		Document:              documentFromDTO(dto.VisualDesign),
	}, nil
}

// --- owner-instance compatibility resolution --------------------------

// resolveOwnerCheck turns an optional (target, ownerID) pair from a
// query parameter into a visualtemplate.OwnerBindingCheck - nil, nil
// when ownerID is empty (no owner context requested). Returns a
// *decodeError-shaped sentinel via the second value when ownerID was
// given but does not resolve to a real owner, so the caller can answer
// 404 rather than silently ignoring it.
func resolveOwnerCheck(ctx context.Context, target visualtemplate.Target, ownerID string, alerts AlertsService, chatOverlays ChatOverlayProfileService) (visualtemplate.OwnerBindingCheck, error) {
	if ownerID == "" {
		return nil, nil
	}
	switch target {
	case visualtemplate.TargetAlert:
		if alerts == nil {
			return nil, nil
		}
		rule, err := alerts.GetRule(ctx, ownerID)
		if err != nil {
			return nil, err
		}
		eventType := rule.EventType
		return func(doc visualdesign.Document) error {
			return alertsdomain.ValidateDesignBindingsForEventType(doc, eventType)
		}, nil
	case visualtemplate.TargetChat:
		if chatOverlays == nil {
			return nil, nil
		}
		if _, err := chatOverlays.GetProfile(ctx, ownerID); err != nil {
			return nil, err
		}
		return func(doc visualdesign.Document) error {
			return chatoverlaydomain.ValidateDesignBindingsForChatOverlay(doc)
		}, nil
	default:
		return nil, nil
	}
}

func compatibilityDTOFor(ctx context.Context, t visualtemplate.Template, target, ownerID string, alerts AlertsService, chatOverlays ChatOverlayProfileService) *templateCompatibilityDTO {
	if target == "" {
		return nil
	}
	check, err := resolveOwnerCheck(ctx, visualtemplate.Target(target), ownerID, alerts, chatOverlays)
	if err != nil {
		return nil
	}
	c := visualtemplate.AssessCompatibility(t, visualtemplate.Target(target), check)
	return &templateCompatibilityDTO{Compatible: c.Compatible, Blockers: c.Blockers}
}

// --- handlers -----------------------------------------------------------

func handleListVisualTemplates(logger *slog.Logger, svc VisualTemplateService, alerts AlertsService, chatOverlays ChatOverlayProfileService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		target := r.URL.Query().Get("target")
		ownerID := r.URL.Query().Get("ownerId")

		list, err := svc.List(r.Context())
		if err != nil {
			writeVisualTemplateError(w, logger, err)
			return
		}
		out := make([]visualTemplateDTO, 0, len(list))
		for _, t := range list {
			dto := templateToDTO(t)
			dto.Compatibility = compatibilityDTOFor(r.Context(), t, target, ownerID, alerts, chatOverlays)
			out = append(out, dto)
		}
		writeJSON(w, logger, http.StatusOK, out)
	}
}

func handleGetVisualTemplate(logger *slog.Logger, svc VisualTemplateService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t, err := svc.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			writeVisualTemplateError(w, logger, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, templateToDTO(t))
	}
}

type createVisualTemplateRequest struct {
	Target      string                  `json:"target"`
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	Author      string                  `json:"author"`
	License     string                  `json:"license"`
	Document    visualDesignDocumentDTO `json:"document"`
}

func handleCreateVisualTemplate(logger *slog.Logger, svc VisualTemplateService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body createVisualTemplateRequest
		if err := decodeJSONWithLimit(w, r, &body, maxVisualTemplateImportBytes); err != nil {
			writeDecodeError(w, logger, err)
			return
		}
		t, err := svc.Create(r.Context(), visualtemplate.Target(body.Target), body.Name, body.Description, body.Author, body.License, documentFromDTO(body.Document))
		if err != nil {
			writeVisualTemplateError(w, logger, err)
			return
		}
		writeJSON(w, logger, http.StatusCreated, templateToDTO(t))
	}
}

type updateVisualTemplateMetadataRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Author      string `json:"author"`
	License     string `json:"license"`
}

func handleUpdateVisualTemplateMetadata(logger *slog.Logger, svc VisualTemplateService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body updateVisualTemplateMetadataRequest
		if err := decodeJSONWithLimit(w, r, &body, maxRequestBodyBytes); err != nil {
			writeDecodeError(w, logger, err)
			return
		}
		t, err := svc.UpdateMetadata(r.Context(), r.PathValue("id"), body.Name, body.Description, body.Author, body.License)
		if err != nil {
			writeVisualTemplateError(w, logger, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, templateToDTO(t))
	}
}

func handleDeleteVisualTemplate(logger *slog.Logger, svc VisualTemplateService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := svc.Delete(r.Context(), r.PathValue("id")); err != nil {
			writeVisualTemplateError(w, logger, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleImportVisualTemplatePreview(logger *slog.Logger, svc VisualTemplateService, alerts AlertsService, chatOverlays ChatOverlayProfileService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body visualTemplateFileDTO
		if err := decodeJSONWithLimit(w, r, &body, maxVisualTemplateImportBytes); err != nil {
			writeDecodeError(w, logger, err)
			return
		}
		candidate, err := templateFromFileDTO(body)
		if err != nil {
			writeVisualTemplateError(w, logger, err)
			return
		}
		previewed, err := svc.ImportPreview(candidate)
		if err != nil {
			writeVisualTemplateError(w, logger, err)
			return
		}
		dto := templateToDTO(previewed)
		dto.Compatibility = compatibilityDTOFor(r.Context(), previewed, r.URL.Query().Get("target"), r.URL.Query().Get("ownerId"), alerts, chatOverlays)
		writeJSON(w, logger, http.StatusOK, dto)
	}
}

func handleImportVisualTemplate(logger *slog.Logger, svc VisualTemplateService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body visualTemplateFileDTO
		if err := decodeJSONWithLimit(w, r, &body, maxVisualTemplateImportBytes); err != nil {
			writeDecodeError(w, logger, err)
			return
		}
		candidate, err := templateFromFileDTO(body)
		if err != nil {
			writeVisualTemplateError(w, logger, err)
			return
		}
		imported, err := svc.Import(r.Context(), candidate)
		if err != nil {
			writeVisualTemplateError(w, logger, err)
			return
		}
		writeJSON(w, logger, http.StatusCreated, templateToDTO(imported))
	}
}

func handleExportVisualTemplate(logger *slog.Logger, svc VisualTemplateService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t, err := svc.Export(r.Context(), r.PathValue("id"))
		if err != nil {
			writeVisualTemplateError(w, logger, err)
			return
		}
		filename := safeTemplateExportFilename(t.Name)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		writeJSON(w, logger, http.StatusOK, templateToFileDTO(t))
	}
}

// safeTemplateExportFilename derives a safe download filename from an
// operator-authored template name (Stage 14A task Part 22): every path
// separator and control character (including CR/LF, which could
// otherwise inject extra response headers) is stripped, the result is
// bounded, and a fixed, non-Stage-14B extension is always appended.
// Never derived from any local path, and never a bare user string
// passed into the header unescaped.
func safeTemplateExportFilename(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r == '/' || r == '\\' || r == ':':
			b.WriteRune('-')
		case unicode.IsControl(r):
			// drop entirely - never re-introduce a byte into the header
		default:
			b.WriteRune(r)
		}
	}
	safe := strings.TrimSpace(b.String())
	const maxBase = 80
	if len(safe) > maxBase {
		safe = safe[:maxBase]
	}
	if safe == "" {
		safe = "template"
	}
	return safe + ".streaming-tree-template.json"
}

func writeVisualTemplateError(w http.ResponseWriter, logger *slog.Logger, err error) {
	switch {
	case errors.Is(err, visualtemplate.ErrNotFound):
		writeError(w, logger, http.StatusNotFound, "visual_template_not_found", "The requested visual template does not exist.")
	case errors.Is(err, visualtemplate.ErrImmutable):
		writeError(w, logger, http.StatusConflict, "visual_template_immutable", "Built-in templates cannot be modified or deleted.")
	case errors.Is(err, visualtemplate.ErrTargetMismatch):
		writeError(w, logger, http.StatusUnprocessableEntity, "visual_template_target_mismatch", "This template's target does not match the requested owner.")
	case errors.Is(err, visualtemplate.ErrUnsupportedTemplateVersion):
		writeError(w, logger, http.StatusUnprocessableEntity, "visual_template_version_unsupported", "This template file's own schema version is not supported.")
	case errors.Is(err, visualtemplate.ErrUnsupportedDesignVersion):
		writeError(w, logger, http.StatusUnprocessableEntity, "visual_template_design_version_unsupported", "The embedded visual design's own version is not supported.")
	case errors.Is(err, visualtemplate.ErrTooLarge):
		writeError(w, logger, http.StatusRequestEntityTooLarge, "visual_template_import_too_large", "The imported template file is too large.")
	case errors.Is(err, visualtemplate.ErrValidation):
		writeError(w, logger, http.StatusUnprocessableEntity, "visual_template_invalid", "The template failed validation.")
	case errors.Is(err, alertsdomain.ErrRuleNotFound):
		writeError(w, logger, http.StatusNotFound, "alert_rule_not_found", "The requested alert rule does not exist.")
	case errors.Is(err, chatoverlaydomain.ErrNotFound):
		writeError(w, logger, http.StatusNotFound, "chat_overlay_not_found", "The requested chat overlay does not exist.")
	default:
		writeError(w, logger, http.StatusInternalServerError, "internal_error", "An unexpected error occurred.")
	}
}
