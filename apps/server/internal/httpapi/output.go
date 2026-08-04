package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/streaming-tree/server/internal/domain/output"
	"github.com/streaming-tree/server/internal/domain/platform"
)

// OutputService is the subset of the output domain service the handlers
// need.
type OutputService interface {
	Get(ctx context.Context, platformID string) (output.Settings, error)
	Update(ctx context.Context, platformID string, input output.UpdateInput) (output.Settings, error)
}

// outputSettingsResponse is the public shape of a platform's output
// configuration. It never carries a stream key or a full destination URL -
// only the non-secret server address and restart preference.
type outputSettingsResponse struct {
	ServerURL   string `json:"serverUrl"`
	AutoRestart bool   `json:"autoRestart"`
	UpdatedAt   string `json:"updatedAt"`
}

func toOutputSettingsResponse(s output.Settings) outputSettingsResponse {
	return outputSettingsResponse{
		ServerURL:   s.ServerURL,
		AutoRestart: s.AutoRestart,
		UpdatedAt:   formatOutputTime(s.UpdatedAt),
	}
}

func formatOutputTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func handleGetOutputSettings(logger *slog.Logger, platforms PlatformService, outputs OutputService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !requirePlatform(w, logger, r, platforms, id) {
			return
		}

		settings, err := outputs.Get(r.Context(), id)
		if err != nil {
			writeOutputError(w, logger, r, err)
			return
		}

		writeJSON(w, logger, http.StatusOK, toOutputSettingsResponse(settings))
	}
}

// setOutputSettingsRequest is a full replacement of the mutable fields, same
// convention as updatePlatformRequest: every field must be sent.
type setOutputSettingsRequest struct {
	ServerURL   string `json:"serverUrl"`
	AutoRestart bool   `json:"autoRestart"`
}

func handleUpdateOutputSettings(logger *slog.Logger, platforms PlatformService, outputs OutputService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !requirePlatform(w, logger, r, platforms, id) {
			return
		}

		var body setOutputSettingsRequest
		if err := decodeJSON(w, r, &body); err != nil {
			writeDecodeError(w, logger, err)
			return
		}

		updated, err := outputs.Update(r.Context(), id, output.UpdateInput{
			ServerURL:   body.ServerURL,
			AutoRestart: body.AutoRestart,
		})
		if err != nil {
			writeOutputError(w, logger, r, err)
			return
		}

		writeJSON(w, logger, http.StatusOK, toOutputSettingsResponse(updated))
	}
}

// writeOutputError maps an output domain error onto the HTTP contract.
func writeOutputError(w http.ResponseWriter, logger *slog.Logger, r *http.Request, err error) {
	if verr, ok := platform.AsValidationError(err); ok {
		writeValidationError(w, logger, verr)
		return
	}

	switch {
	case errors.Is(err, output.ErrNotFound):
		writeError(w, logger, http.StatusNotFound,
			"platform_not_found", "The requested platform does not exist.")

	default:
		logger.Error("unhandled output settings error",
			slog.String("path", r.URL.Path),
			slog.String("method", r.Method),
			slog.Any("error", err),
		)
		writeError(w, logger, http.StatusInternalServerError,
			"internal_error", "The server encountered an unexpected error.")
	}
}
