package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/streaming-tree/server/internal/domain/platform"
)

// PlatformService is the subset of the domain service the handlers need.
// Declared here as an interface so handler tests can use a stub.
type PlatformService interface {
	Definitions() []platform.ProviderDefinition
	List(ctx context.Context) ([]platform.Platform, error)
	Get(ctx context.Context, id string) (platform.Platform, error)
	Create(ctx context.Context, input platform.CreateInput) (platform.Platform, error)
	Update(ctx context.Context, id string, input platform.UpdateInput) (platform.Platform, error)
	Delete(ctx context.Context, id string) error
	Metadata(ctx context.Context, id string) (platform.Metadata, error)
	SaveMetadata(ctx context.Context, id string, m platform.Metadata) (platform.Metadata, error)
}

// platformResponse is the API view of a configured platform.
//
// It deliberately differs from the database row: the provider definition is
// inlined so the dashboard can render a card from one response, and no internal
// column, database path or runtime process state is exposed.
type platformResponse struct {
	ID          string                       `json:"id"`
	ProviderID  platform.ProviderID          `json:"providerId"`
	DisplayName string                       `json:"displayName"`
	Enabled     bool                         `json:"enabled"`
	SortOrder   int                          `json:"sortOrder"`
	CreatedAt   string                       `json:"createdAt"`
	UpdatedAt   string                       `json:"updatedAt"`
	Provider    *platform.ProviderDefinition `json:"provider,omitempty"`
	Metadata    metadataResponse             `json:"metadata"`
}

type metadataResponse struct {
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	Category      string   `json:"category"`
	CategoryID    string   `json:"categoryId"`
	Tags          []string `json:"tags"`
	Language      string   `json:"language"`
	Visibility    string   `json:"visibility"`
	MatureContent bool     `json:"matureContent"`
	DVR           bool     `json:"dvr"`
	LatencyMode   string   `json:"latencyMode"`
	UpdatedAt     string   `json:"updatedAt"`
}

func toMetadataResponse(m platform.Metadata) metadataResponse {
	tags := m.Tags
	if tags == nil {
		tags = []string{}
	}
	return metadataResponse{
		Title:         m.Title,
		Description:   m.Description,
		Category:      m.Category,
		CategoryID:    m.CategoryID,
		Tags:          tags,
		Language:      m.Language,
		Visibility:    m.Visibility,
		MatureContent: m.MatureContent,
		DVR:           m.DVR,
		LatencyMode:   m.LatencyMode,
		UpdatedAt:     formatTime(m.UpdatedAt),
	}
}

func toPlatformResponse(p platform.Platform) platformResponse {
	response := platformResponse{
		ID:          p.ID,
		ProviderID:  p.ProviderID,
		DisplayName: p.DisplayName,
		Enabled:     p.Enabled,
		SortOrder:   p.SortOrder,
		CreatedAt:   formatTime(p.CreatedAt),
		UpdatedAt:   formatTime(p.UpdatedAt),
		Metadata:    toMetadataResponse(p.Metadata),
	}

	if def, ok := platform.Definition(p.ProviderID); ok {
		response.Provider = &def
	}

	return response
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// --- definitions ------------------------------------------------------------

func handleListDefinitions(logger *slog.Logger, service PlatformService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, logger, http.StatusOK, map[string]any{
			"definitions": service.Definitions(),
		})
	}
}

// --- platform collection ----------------------------------------------------

func handleListPlatforms(logger *slog.Logger, service PlatformService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		platforms, err := service.List(r.Context())
		if err != nil {
			writeDomainError(w, logger, r, err)
			return
		}

		items := make([]platformResponse, 0, len(platforms))
		for _, p := range platforms {
			items = append(items, toPlatformResponse(p))
		}

		writeJSON(w, logger, http.StatusOK, map[string]any{"platforms": items})
	}
}

// createPlatformRequest is the accepted create payload. There is intentionally
// no field for a stream key or any other credential.
type createPlatformRequest struct {
	ProviderID  string `json:"providerId"`
	DisplayName string `json:"displayName"`
	Enabled     bool   `json:"enabled"`
	SortOrder   *int   `json:"sortOrder"`
}

func handleCreatePlatform(logger *slog.Logger, service PlatformService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body createPlatformRequest
		if err := decodeJSON(w, r, &body); err != nil {
			writeDecodeError(w, logger, err)
			return
		}

		created, err := service.Create(r.Context(), platform.CreateInput{
			ProviderID:  platform.ProviderID(body.ProviderID),
			DisplayName: body.DisplayName,
			Enabled:     body.Enabled,
			SortOrder:   body.SortOrder,
		})
		if err != nil {
			writeDomainError(w, logger, r, err)
			return
		}

		w.Header().Set("Location", "/api/platforms/"+created.ID)
		writeJSON(w, logger, http.StatusCreated, toPlatformResponse(created))
	}
}

// --- single platform --------------------------------------------------------

func handleGetPlatform(logger *slog.Logger, service PlatformService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := service.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			writeDomainError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toPlatformResponse(p))
	}
}

// updatePlatformRequest is a full replacement of the mutable fields: every
// field must be sent, which avoids the ambiguity of partial PATCH semantics.
type updatePlatformRequest struct {
	DisplayName string `json:"displayName"`
	Enabled     bool   `json:"enabled"`
	SortOrder   int    `json:"sortOrder"`
}

func handleUpdatePlatform(logger *slog.Logger, service PlatformService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body updatePlatformRequest
		if err := decodeJSON(w, r, &body); err != nil {
			writeDecodeError(w, logger, err)
			return
		}

		updated, err := service.Update(r.Context(), r.PathValue("id"), platform.UpdateInput{
			DisplayName: body.DisplayName,
			Enabled:     body.Enabled,
			SortOrder:   body.SortOrder,
		})
		if err != nil {
			writeDomainError(w, logger, r, err)
			return
		}

		writeJSON(w, logger, http.StatusOK, toPlatformResponse(updated))
	}
}

func handleDeletePlatform(logger *slog.Logger, service PlatformService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := service.Delete(r.Context(), r.PathValue("id")); err != nil {
			writeDomainError(w, logger, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
