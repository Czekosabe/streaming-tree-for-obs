package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/streaming-tree/server/internal/domain/metadatapreset"
	"github.com/streaming-tree/server/internal/domain/platform"
)

// MetadataPresetService is the subset of metadatapreset.Service the
// HTTP layer needs.
type MetadataPresetService interface {
	List(ctx context.Context) ([]metadatapreset.Preset, error)
	Get(ctx context.Context, id string) (metadatapreset.Preset, error)
	Create(ctx context.Context, input metadatapreset.CreateInput) (metadatapreset.Preset, error)
	Update(ctx context.Context, id string, input metadatapreset.UpdateInput) (metadatapreset.Preset, error)
	Delete(ctx context.Context, id string) error
	ApplyPreview(ctx context.Context, id string, platformIDs []string) ([]metadatapreset.DestinationPreview, error)
	Apply(ctx context.Context, id string, platformIDs []string) (map[string]platform.Metadata, error)
}

// providerMetadataDTO is the wire shape of one provider-scoped category
// entry - never applied outside the exact provider it is keyed under.
type providerMetadataDTO struct {
	Category   string `json:"category"`
	CategoryID string `json:"categoryId"`
}

// presetRequest is the accepted create/update payload. Every field is
// sent on every write, matching saveMetadataRequest's own convention.
type presetRequest struct {
	Name          string                         `json:"name"`
	Note          string                         `json:"note"`
	Title         string                         `json:"title"`
	Description   string                         `json:"description"`
	Tags          []string                       `json:"tags"`
	Language      string                         `json:"language"`
	Visibility    string                         `json:"visibility"`
	MatureContent bool                           `json:"matureContent"`
	DVR           bool                           `json:"dvr"`
	LatencyMode   string                         `json:"latencyMode"`
	Providers     map[string]providerMetadataDTO `json:"providers"`
}

type presetResponse struct {
	ID            string                         `json:"id"`
	Name          string                         `json:"name"`
	Note          string                         `json:"note"`
	Title         string                         `json:"title"`
	Description   string                         `json:"description"`
	Tags          []string                       `json:"tags"`
	Language      string                         `json:"language"`
	Visibility    string                         `json:"visibility"`
	MatureContent bool                           `json:"matureContent"`
	DVR           bool                           `json:"dvr"`
	LatencyMode   string                         `json:"latencyMode"`
	Providers     map[string]providerMetadataDTO `json:"providers"`
	CreatedAt     string                         `json:"createdAt"`
	UpdatedAt     string                         `json:"updatedAt"`
}

func toPresetResponse(p metadatapreset.Preset) presetResponse {
	providers := make(map[string]providerMetadataDTO, len(p.Providers))
	for id, pm := range p.Providers {
		providers[string(id)] = providerMetadataDTO{Category: pm.Category, CategoryID: pm.CategoryID}
	}
	tags := p.Common.Tags
	if tags == nil {
		tags = []string{}
	}
	return presetResponse{
		ID: p.ID, Name: p.Name, Note: p.Note,
		Title: p.Common.Title, Description: p.Common.Description, Tags: tags,
		Language: p.Common.Language, Visibility: p.Common.Visibility,
		MatureContent: p.Common.MatureContent, DVR: p.Common.DVR, LatencyMode: p.Common.LatencyMode,
		Providers: providers,
		CreatedAt: platform.FormatTimestamp(p.CreatedAt), UpdatedAt: platform.FormatTimestamp(p.UpdatedAt),
	}
}

func fromPresetRequest(body presetRequest) (metadatapreset.CommonMetadata, map[platform.ProviderID]metadatapreset.ProviderMetadata) {
	tags := body.Tags
	if tags == nil {
		tags = []string{}
	}
	common := metadatapreset.CommonMetadata{
		Title: body.Title, Description: body.Description, Tags: tags,
		Language: body.Language, Visibility: body.Visibility,
		MatureContent: body.MatureContent, DVR: body.DVR, LatencyMode: body.LatencyMode,
	}
	providers := make(map[platform.ProviderID]metadatapreset.ProviderMetadata, len(body.Providers))
	for id, pm := range body.Providers {
		providers[platform.ProviderID(id)] = metadatapreset.ProviderMetadata{Category: pm.Category, CategoryID: pm.CategoryID}
	}
	return common, providers
}

// registerMetadataPresetRoutes wires the Stage 22 metadata-preset CRUD
// and apply API (docs/metadata-presets.md).
func registerMetadataPresetRoutes(mux *http.ServeMux, logger *slog.Logger, service MetadataPresetService) {
	mux.HandleFunc("GET /api/metadata-presets", handleListPresets(logger, service))
	mux.HandleFunc("POST /api/metadata-presets", handleCreatePreset(logger, service))
	mux.HandleFunc("/api/metadata-presets", methodNotAllowed(logger, http.MethodGet, http.MethodPost))

	mux.HandleFunc("GET /api/metadata-presets/{id}", handleGetPreset(logger, service))
	mux.HandleFunc("PUT /api/metadata-presets/{id}", handleUpdatePreset(logger, service))
	mux.HandleFunc("DELETE /api/metadata-presets/{id}", handleDeletePreset(logger, service))
	mux.HandleFunc("/api/metadata-presets/{id}",
		methodNotAllowed(logger, http.MethodGet, http.MethodPut, http.MethodDelete))

	mux.HandleFunc("GET /api/metadata-presets/{id}/apply-preview", handleApplyPreviewPreset(logger, service))
	mux.HandleFunc("/api/metadata-presets/{id}/apply-preview", methodNotAllowed(logger, http.MethodGet))

	mux.HandleFunc("POST /api/metadata-presets/{id}/apply", handleApplyPreset(logger, service))
	mux.HandleFunc("/api/metadata-presets/{id}/apply", methodNotAllowed(logger, http.MethodPost))
}

func handleListPresets(logger *slog.Logger, service MetadataPresetService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		presets, err := service.List(r.Context())
		if err != nil {
			writeMetadataPresetError(w, logger, r, err)
			return
		}
		out := make([]presetResponse, 0, len(presets))
		for _, p := range presets {
			out = append(out, toPresetResponse(p))
		}
		writeJSON(w, logger, http.StatusOK, out)
	}
}

func handleGetPreset(logger *slog.Logger, service MetadataPresetService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := service.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			writeMetadataPresetError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toPresetResponse(p))
	}
}

func handleCreatePreset(logger *slog.Logger, service MetadataPresetService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body presetRequest
		if err := decodeJSON(w, r, &body); err != nil {
			writeDecodeError(w, logger, err)
			return
		}
		common, providers := fromPresetRequest(body)
		created, err := service.Create(r.Context(), metadatapreset.CreateInput{
			Name: body.Name, Note: body.Note, Common: common, Providers: providers,
		})
		if err != nil {
			writeMetadataPresetError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusCreated, toPresetResponse(created))
	}
}

func handleUpdatePreset(logger *slog.Logger, service MetadataPresetService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body presetRequest
		if err := decodeJSON(w, r, &body); err != nil {
			writeDecodeError(w, logger, err)
			return
		}
		common, providers := fromPresetRequest(body)
		updated, err := service.Update(r.Context(), r.PathValue("id"), metadatapreset.UpdateInput{
			Name: body.Name, Note: body.Note, Common: common, Providers: providers,
		})
		if err != nil {
			writeMetadataPresetError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toPresetResponse(updated))
	}
}

func handleDeletePreset(logger *slog.Logger, service MetadataPresetService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}
		if err := service.Delete(r.Context(), r.PathValue("id")); err != nil {
			writeMetadataPresetError(w, logger, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// applyFieldResponse is one field's compatibility classification for
// one destination (docs/metadata-presets.md §6).
type applyFieldResponse struct {
	Field  string `json:"field"`
	Status string `json:"status"`
}

// applyDestinationResponse is one destination's apply-preview result.
// Errors, when present, uses the same plain field->message shape
// ErrorBody.Fields already does, scoped to this one destination.
type applyDestinationResponse struct {
	PlatformID string               `json:"platformId"`
	ProviderID string               `json:"providerId"`
	Valid      bool                 `json:"valid"`
	Fields     []applyFieldResponse `json:"fields"`
	Errors     map[string]string    `json:"errors,omitempty"`
}

func toDestinationPreviewResponse(p metadatapreset.DestinationPreview) applyDestinationResponse {
	fields := make([]applyFieldResponse, 0, len(p.Fields))
	for _, f := range p.Fields {
		fields = append(fields, applyFieldResponse{Field: f.Field, Status: string(f.Status)})
	}

	var errs map[string]string
	if len(p.Errors) > 0 {
		errs = make(map[string]string, len(p.Errors))
		for _, v := range p.Errors {
			if _, seen := errs[v.Field]; seen {
				continue
			}
			errs[v.Field] = v.Message
		}
	}

	return applyDestinationResponse{
		PlatformID: p.PlatformID, ProviderID: string(p.ProviderID),
		Valid: p.Valid, Fields: fields, Errors: errs,
	}
}

// parsePlatformIDs reads a comma-separated "platformIds" query
// parameter, dropping blank entries left by a stray comma.
func parsePlatformIDs(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		id := strings.TrimSpace(part)
		if id != "" {
			out = append(out, id)
		}
	}
	return out
}

func handleApplyPreviewPreset(logger *slog.Logger, service MetadataPresetService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		platformIDs := parsePlatformIDs(r.URL.Query().Get("platformIds"))
		previews, err := service.ApplyPreview(r.Context(), r.PathValue("id"), platformIDs)
		if err != nil {
			writeMetadataPresetError(w, logger, r, err)
			return
		}
		out := make([]applyDestinationResponse, 0, len(previews))
		for _, p := range previews {
			out = append(out, toDestinationPreviewResponse(p))
		}
		writeJSON(w, logger, http.StatusOK, out)
	}
}

type applyPresetRequest struct {
	PlatformIDs []string `json:"platformIds"`
}

type applyPresetResponse struct {
	Platforms map[string]metadataResponse `json:"platforms"`
}

func handleApplyPreset(logger *slog.Logger, service MetadataPresetService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body applyPresetRequest
		if err := decodeJSON(w, r, &body); err != nil {
			writeDecodeError(w, logger, err)
			return
		}

		updated, err := service.Apply(r.Context(), r.PathValue("id"), body.PlatformIDs)
		if err != nil {
			writeMetadataPresetError(w, logger, r, err)
			return
		}

		platforms := make(map[string]metadataResponse, len(updated))
		for id, m := range updated {
			platforms[id] = toMetadataResponse(m)
		}
		writeJSON(w, logger, http.StatusOK, applyPresetResponse{Platforms: platforms})
	}
}

// writeMetadataPresetError maps a metadatapreset domain failure onto
// the HTTP contract - metadatapreset defines its own sentinel errors
// (ErrNotFound/ErrDuplicateName/ErrTooMany), so writeDomainError's own
// platform-package-specific errors.Is checks do not apply here.
func writeMetadataPresetError(w http.ResponseWriter, logger *slog.Logger, r *http.Request, err error) {
	if verr, ok := platform.AsValidationError(err); ok {
		writeValidationError(w, logger, verr)
		return
	}

	// Apply is all-or-nothing across possibly several destinations
	// (docs/metadata-presets.md §6/§15/§23): flatten every destination's
	// own violations into the one shared ErrorBody.Fields/Details
	// contract, prefixing each field with its platform ID so two
	// destinations' violations on the same field name (e.g. both
	// failing "title") never collide.
	if aerr, ok := metadatapreset.AsApplyValidationError(err); ok {
		verr := &platform.ValidationError{}
		for platformID, violations := range aerr.Destinations {
			for _, v := range violations {
				verr.Add(platformID+"."+v.Field, v.Rule, v.Message, v.Params)
			}
		}
		writeValidationError(w, logger, verr)
		return
	}

	switch {
	case errors.Is(err, metadatapreset.ErrNotFound):
		writeError(w, logger, http.StatusNotFound, "not_found", "The requested preset does not exist.")

	case errors.Is(err, metadatapreset.ErrDuplicateName):
		writeError(w, logger, http.StatusConflict, "duplicate_name", "A preset with this name already exists.")

	case errors.Is(err, metadatapreset.ErrTooMany):
		writeError(w, logger, http.StatusUnprocessableEntity, "too_many_presets",
			"The maximum number of presets has been reached.")

	default:
		logger.Error("unhandled metadata preset error",
			slog.String("path", r.URL.Path), slog.String("method", r.Method), slog.Any("error", err))
		writeError(w, logger, http.StatusInternalServerError, "internal_error",
			"The server encountered an unexpected error.")
	}
}
