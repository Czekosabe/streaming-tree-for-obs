package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/streaming-tree/server/internal/domain/platform"
)

// saveMetadataRequest is a full replacement of the stored metadata.
//
// Every field is sent on every save. Fields the provider does not support must
// be left empty; sending a meaningful value for one of them is a validation
// error rather than being silently dropped, so a client bug surfaces instead of
// data quietly disappearing.
type saveMetadataRequest struct {
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	Category      string   `json:"category"`
	Tags          []string `json:"tags"`
	Language      string   `json:"language"`
	Visibility    string   `json:"visibility"`
	MatureContent bool     `json:"matureContent"`
	DVR           bool     `json:"dvr"`
	LatencyMode   string   `json:"latencyMode"`
}

func handleGetMetadata(logger *slog.Logger, service PlatformService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		metadata, err := service.Metadata(r.Context(), r.PathValue("id"))
		if err != nil {
			writeDomainError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toMetadataResponse(metadata))
	}
}

func handleSaveMetadata(logger *slog.Logger, service PlatformService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body saveMetadataRequest
		if err := decodeJSON(w, r, &body); err != nil {
			writeDecodeError(w, logger, err)
			return
		}

		tags := body.Tags
		if tags == nil {
			tags = []string{}
		}

		saved, err := service.SaveMetadata(r.Context(), r.PathValue("id"), platform.Metadata{
			Title:         body.Title,
			Description:   body.Description,
			Category:      body.Category,
			Tags:          tags,
			Language:      body.Language,
			Visibility:    body.Visibility,
			MatureContent: body.MatureContent,
			DVR:           body.DVR,
			LatencyMode:   body.LatencyMode,
		})
		if err != nil {
			writeDomainError(w, logger, r, err)
			return
		}

		writeJSON(w, logger, http.StatusOK, toMetadataResponse(saved))
	}
}
