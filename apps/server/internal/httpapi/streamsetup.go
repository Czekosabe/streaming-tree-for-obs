package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/streaming-tree/server/internal/domain/metadatapreset"
	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/domain/streamsetup"
)

// StreamSetupService is the subset of streamsetup.Service the HTTP
// layer needs (docs/stream-setup-profiles.md §10).
type StreamSetupService interface {
	List(ctx context.Context) ([]streamsetup.Profile, error)
	Get(ctx context.Context, id string) (streamsetup.Profile, error)
	Create(ctx context.Context, input streamsetup.CreateInput) (streamsetup.Profile, error)
	Update(ctx context.Context, id string, input streamsetup.UpdateInput) (streamsetup.Profile, error)
	Delete(ctx context.Context, id string) error
	Duplicate(ctx context.Context, id, newName string) (streamsetup.Profile, error)
	SaveCurrent(ctx context.Context, name, note string, metadataPresetID *string) (streamsetup.Profile, error)
	Preview(ctx context.Context, profileID string) (streamsetup.Preview, error)
	Apply(ctx context.Context, profileID string) (streamsetup.ApplyResult, error)
}

type streamSetupDestinationResponse struct {
	PlatformID  *string `json:"platformId"`
	ProviderID  string  `json:"providerId"`
	DisplayName string  `json:"displayName"`
}

type streamSetupProfileResponse struct {
	ID                    string                           `json:"id"`
	Name                  string                           `json:"name"`
	Note                  string                           `json:"note"`
	Destinations          []streamSetupDestinationResponse `json:"destinations"`
	MetadataPresetID      *string                          `json:"metadataPresetId"`
	MetadataPresetName    string                           `json:"metadataPresetName"`
	MetadataPresetMissing bool                             `json:"metadataPresetMissing"`
	CreatedAt             string                           `json:"createdAt"`
	UpdatedAt             string                           `json:"updatedAt"`
}

func toStreamSetupProfileResponse(p streamsetup.Profile) streamSetupProfileResponse {
	dests := make([]streamSetupDestinationResponse, 0, len(p.Destinations))
	for _, d := range p.Destinations {
		dests = append(dests, streamSetupDestinationResponse{
			PlatformID: d.PlatformID, ProviderID: d.ProviderID, DisplayName: d.DisplayName,
		})
	}
	return streamSetupProfileResponse{
		ID: p.ID, Name: p.Name, Note: p.Note, Destinations: dests,
		MetadataPresetID: p.MetadataPresetID, MetadataPresetName: p.MetadataPresetName,
		MetadataPresetMissing: p.MetadataPresetMissing(),
		CreatedAt:             platform.FormatTimestamp(p.CreatedAt),
		UpdatedAt:             platform.FormatTimestamp(p.UpdatedAt),
	}
}

// registerStreamSetupRoutes wires the Stage 25 stream setup profile
// CRUD/duplicate/save-current/preview/apply API (docs/stream-setup-
// profiles.md §10). Never registered under /api/public/*: management
// only, exactly like Stage 24's stream-session routes.
func registerStreamSetupRoutes(mux *http.ServeMux, logger *slog.Logger, service StreamSetupService) {
	mux.HandleFunc("GET /api/stream-setups", handleListStreamSetups(logger, service))
	mux.HandleFunc("POST /api/stream-setups", handleCreateStreamSetup(logger, service))
	mux.HandleFunc("/api/stream-setups", methodNotAllowed(logger, http.MethodGet, http.MethodPost))

	// No bare any-method 405 catch-all is registered for .../save-
	// current: it would panic at startup as an ambiguous overlap
	// against the GET-only .../{id} wildcard below (neither pattern is
	// more specific than the other in both the method and path
	// dimensions at once - the same conflict class streamsession.go's
	// own .../settings route hit). A genuinely wrong method against
	// .../save-current still gets Go's own built-in automatic 405
	// (with an Allow header), just without this package's usual
	// custom JSON 405 body.
	mux.HandleFunc("POST /api/stream-setups/save-current", handleSaveCurrentStreamSetup(logger, service))

	mux.HandleFunc("GET /api/stream-setups/{id}", handleGetStreamSetup(logger, service))
	mux.HandleFunc("PUT /api/stream-setups/{id}", handleUpdateStreamSetup(logger, service))
	mux.HandleFunc("DELETE /api/stream-setups/{id}", handleDeleteStreamSetup(logger, service))

	mux.HandleFunc("POST /api/stream-setups/{id}/duplicate", handleDuplicateStreamSetup(logger, service))
	mux.HandleFunc("/api/stream-setups/{id}/duplicate", methodNotAllowed(logger, http.MethodPost))

	mux.HandleFunc("GET /api/stream-setups/{id}/preview", handlePreviewStreamSetup(logger, service))
	mux.HandleFunc("/api/stream-setups/{id}/preview", methodNotAllowed(logger, http.MethodGet))

	mux.HandleFunc("POST /api/stream-setups/{id}/apply", handleApplyStreamSetup(logger, service))
	mux.HandleFunc("/api/stream-setups/{id}/apply", methodNotAllowed(logger, http.MethodPost))
}

func handleListStreamSetups(logger *slog.Logger, service StreamSetupService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		profiles, err := service.List(r.Context())
		if err != nil {
			writeStreamSetupError(w, logger, r, err)
			return
		}
		out := make([]streamSetupProfileResponse, 0, len(profiles))
		for _, p := range profiles {
			out = append(out, toStreamSetupProfileResponse(p))
		}
		writeJSON(w, logger, http.StatusOK, out)
	}
}

func handleGetStreamSetup(logger *slog.Logger, service StreamSetupService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := service.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			writeStreamSetupError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toStreamSetupProfileResponse(p))
	}
}

// streamSetupRequest is the accepted create/update payload. Every
// field is sent on every write, matching presetRequest's own
// full-replacement convention.
type streamSetupRequest struct {
	Name             string   `json:"name"`
	Note             string   `json:"note"`
	DestinationIDs   []string `json:"destinationIds"`
	MetadataPresetID *string  `json:"metadataPresetId"`
}

func handleCreateStreamSetup(logger *slog.Logger, service StreamSetupService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body streamSetupRequest
		if err := decodeJSON(w, r, &body); err != nil {
			writeDecodeError(w, logger, err)
			return
		}
		created, err := service.Create(r.Context(), streamsetup.CreateInput{
			Name: body.Name, Note: body.Note, DestinationIDs: body.DestinationIDs, MetadataPresetID: body.MetadataPresetID,
		})
		if err != nil {
			writeStreamSetupError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusCreated, toStreamSetupProfileResponse(created))
	}
}

func handleUpdateStreamSetup(logger *slog.Logger, service StreamSetupService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body streamSetupRequest
		if err := decodeJSON(w, r, &body); err != nil {
			writeDecodeError(w, logger, err)
			return
		}
		updated, err := service.Update(r.Context(), r.PathValue("id"), streamsetup.UpdateInput{
			Name: body.Name, Note: body.Note, DestinationIDs: body.DestinationIDs, MetadataPresetID: body.MetadataPresetID,
		})
		if err != nil {
			writeStreamSetupError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toStreamSetupProfileResponse(updated))
	}
}

func handleDeleteStreamSetup(logger *slog.Logger, service StreamSetupService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}
		if err := service.Delete(r.Context(), r.PathValue("id")); err != nil {
			writeStreamSetupError(w, logger, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

type duplicateStreamSetupRequest struct {
	Name string `json:"name"`
}

func handleDuplicateStreamSetup(logger *slog.Logger, service StreamSetupService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body duplicateStreamSetupRequest
		if err := decodeJSON(w, r, &body); err != nil {
			writeDecodeError(w, logger, err)
			return
		}
		dup, err := service.Duplicate(r.Context(), r.PathValue("id"), body.Name)
		if err != nil {
			writeStreamSetupError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusCreated, toStreamSetupProfileResponse(dup))
	}
}

type saveCurrentStreamSetupRequest struct {
	Name             string  `json:"name"`
	Note             string  `json:"note"`
	MetadataPresetID *string `json:"metadataPresetId"`
}

func handleSaveCurrentStreamSetup(logger *slog.Logger, service StreamSetupService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body saveCurrentStreamSetupRequest
		if err := decodeJSON(w, r, &body); err != nil {
			writeDecodeError(w, logger, err)
			return
		}
		p, err := service.SaveCurrent(r.Context(), body.Name, body.Note, body.MetadataPresetID)
		if err != nil {
			writeStreamSetupError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusCreated, toStreamSetupProfileResponse(p))
	}
}

type streamSetupDestinationPreviewResponse struct {
	PlatformID       string `json:"platformId"`
	ProviderID       string `json:"providerId"`
	DisplayName      string `json:"displayName"`
	CurrentlyEnabled bool   `json:"currentlyEnabled"`
	Change           string `json:"change"`
	Active           bool   `json:"active"`
}

type streamSetupPreviewResponse struct {
	Profile                     streamSetupProfileResponse              `json:"profile"`
	Destinations                []streamSetupDestinationPreviewResponse `json:"destinations"`
	MetadataPresetReferenced    bool                                    `json:"metadataPresetReferenced"`
	MetadataPresetMissing       bool                                    `json:"metadataPresetMissing"`
	MetadataPresetName          string                                  `json:"metadataPresetName"`
	MetadataDestinationPreviews []applyDestinationResponse              `json:"metadataDestinationPreviews"`
	Blocked                     bool                                    `json:"blocked"`
	BlockedDestinationIDs       []string                                `json:"blockedDestinationIds"`
}

func toStreamSetupPreviewResponse(p streamsetup.Preview) streamSetupPreviewResponse {
	dests := make([]streamSetupDestinationPreviewResponse, 0, len(p.Destinations))
	for _, d := range p.Destinations {
		dests = append(dests, streamSetupDestinationPreviewResponse{
			PlatformID: d.PlatformID, ProviderID: d.ProviderID, DisplayName: d.DisplayName,
			CurrentlyEnabled: d.CurrentlyEnabled, Change: string(d.Change), Active: d.Active,
		})
	}
	metadataPreviews := make([]applyDestinationResponse, 0, len(p.MetadataDestinationPreviews))
	for _, mp := range p.MetadataDestinationPreviews {
		metadataPreviews = append(metadataPreviews, toDestinationPreviewResponse(mp))
	}
	blocked := p.BlockedDestinationIDs
	if blocked == nil {
		blocked = []string{}
	}
	return streamSetupPreviewResponse{
		Profile: toStreamSetupProfileResponse(p.Profile), Destinations: dests,
		MetadataPresetReferenced: p.MetadataPresetReferenced, MetadataPresetMissing: p.MetadataPresetMissing,
		MetadataPresetName: p.MetadataPresetName, MetadataDestinationPreviews: metadataPreviews,
		Blocked: p.Blocked, BlockedDestinationIDs: blocked,
	}
}

func handlePreviewStreamSetup(logger *slog.Logger, service StreamSetupService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		preview, err := service.Preview(r.Context(), r.PathValue("id"))
		if err != nil {
			writeStreamSetupError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toStreamSetupPreviewResponse(preview))
	}
}

type applyStreamSetupResponse struct {
	DestinationsChanged   int    `json:"destinationsChanged"`
	MetadataApplied       bool   `json:"metadataApplied"`
	MetadataSkippedReason string `json:"metadataSkippedReason,omitempty"`
}

func handleApplyStreamSetup(logger *slog.Logger, service StreamSetupService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}
		result, err := service.Apply(r.Context(), r.PathValue("id"))
		if err != nil {
			writeStreamSetupError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, applyStreamSetupResponse{
			DestinationsChanged: result.DestinationsChanged, MetadataApplied: result.MetadataApplied,
			MetadataSkippedReason: result.MetadataSkippedReason,
		})
	}
}

func writeStreamSetupError(w http.ResponseWriter, logger *slog.Logger, r *http.Request, err error) {
	switch {
	case errors.Is(err, streamsetup.ErrNotFound):
		writeError(w, logger, http.StatusNotFound, "not_found", "The requested stream setup profile does not exist.")

	case errors.Is(err, streamsetup.ErrDuplicateName):
		writeError(w, logger, http.StatusConflict, "duplicate_name", "A stream setup profile with this name already exists.")

	case errors.Is(err, platform.ErrNotFound):
		writeError(w, logger, http.StatusUnprocessableEntity, "unknown_destination", "One or more selected destinations do not exist.")

	case errors.Is(err, metadatapreset.ErrNotFound):
		writeError(w, logger, http.StatusUnprocessableEntity, "unknown_metadata_preset", "The referenced metadata preset does not exist.")

	case errors.Is(err, streamsetup.ErrActiveStreamBlocksApply):
		writeError(w, logger, http.StatusConflict, "active_stream_blocks_apply",
			"Stop streaming on the affected destination(s) before changing this setup.")

	default:
		logger.Error("unhandled stream setup error",
			slog.String("path", r.URL.Path), slog.String("method", r.Method), slog.Any("error", err))
		writeError(w, logger, http.StatusInternalServerError, "internal_error", "The server encountered an unexpected error.")
	}
}
