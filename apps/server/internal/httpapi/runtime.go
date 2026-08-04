package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/streaming-tree/server/internal/runtime/mediamtx"
)

// RuntimeService is the subset of the supervisor the handlers need.
// Declared as an interface so handler tests can drive a stub.
type RuntimeService interface {
	Snapshot() mediamtx.Snapshot
	RequestInstall(ctx context.Context) error
	RequestStart(ctx context.Context) error
	RequestStop(ctx context.Context) error
	RequestRestart(ctx context.Context) error
}

// runtimeResponse is the public runtime contract.
//
// It is versioned so the frontend can reject a payload it does not understand
// instead of rendering half a screen. The version changes when a field is
// removed or its meaning changes, not when one is added.
type runtimeResponse struct {
	Version int `json:"version"`
	mediamtx.Snapshot
}

// runtimeSchemaVersion is the current shape of GET /api/runtime.
const runtimeSchemaVersion = 1

func handleGetRuntime(logger *slog.Logger, service RuntimeService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, logger, http.StatusOK, runtimeResponse{
			Version:  runtimeSchemaVersion,
			Snapshot: service.Snapshot(),
		})
	}
}

// requireEmptyBody rejects a body on endpoints that document none.
//
// These are commands, not resources: accepting a body would invite a client to
// think it could pass a download URL, a checksum or a path.
func requireEmptyBody(w http.ResponseWriter, r *http.Request, logger *slog.Logger) bool {
	if !hasRequestBody(w, r) {
		return true
	}

	writeError(w, logger, http.StatusBadRequest, "unexpected_body",
		"This endpoint does not accept a request body.")
	return false
}

func handleInstallMediaMTX(logger *slog.Logger, service RuntimeService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}

		// Installation runs in the background: downloading and verifying tens of
		// megabytes takes far longer than a browser request should stay open.
		// Progress is observed through GET /api/runtime.
		if err := service.RequestInstall(r.Context()); err != nil {
			writeRuntimeError(w, logger, r, err)
			return
		}

		writeJSON(w, logger, http.StatusAccepted, map[string]string{
			"status": "installing",
		})
	}
}

func handleStartMediaMTX(logger *slog.Logger, service RuntimeService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}

		if err := service.RequestStart(r.Context()); err != nil {
			writeRuntimeError(w, logger, r, err)
			return
		}

		writeJSON(w, logger, http.StatusAccepted, map[string]string{"status": "starting"})
	}
}

func handleStopMediaMTX(logger *slog.Logger, service RuntimeService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}

		if err := service.RequestStop(r.Context()); err != nil {
			writeRuntimeError(w, logger, r, err)
			return
		}

		writeJSON(w, logger, http.StatusOK, map[string]string{"status": "stopped"})
	}
}

func handleRestartMediaMTX(logger *slog.Logger, service RuntimeService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}

		if err := service.RequestRestart(r.Context()); err != nil {
			writeRuntimeError(w, logger, r, err)
			return
		}

		writeJSON(w, logger, http.StatusAccepted, map[string]string{"status": "starting"})
	}
}

// writeRuntimeError maps a supervisor failure onto the HTTP contract.
//
// The stable code travels to the frontend, which localizes it; the English
// message is the fallback for a code it does not know.
func writeRuntimeError(w http.ResponseWriter, logger *slog.Logger, r *http.Request, err error) {
	switch {
	case errors.Is(err, mediamtx.ErrInstallInProgress):
		writeError(w, logger, http.StatusConflict, mediamtx.CodeInstallInProgress,
			"A MediaMTX installation is already running.")

	case errors.Is(err, mediamtx.ErrInvalidState):
		// 409: the request is well formed but impossible right now.
		writeError(w, logger, http.StatusConflict, mediamtx.CodeInvalidState,
			"MediaMTX is not in a state where that action is possible.")

	case errors.Is(err, mediamtx.ErrNotInstalled):
		writeError(w, logger, http.StatusUnprocessableEntity, mediamtx.CodeNotInstalled,
			"MediaMTX is not installed yet.")

	case errors.Is(err, mediamtx.ErrIncompatibleVersion):
		writeError(w, logger, http.StatusUnprocessableEntity, mediamtx.CodeIncompatibleVersion,
			"The installed MediaMTX version is not supported.")

	default:
		logger.Error("unhandled runtime error",
			slog.String("path", r.URL.Path), slog.Any("error", err))
		writeError(w, logger, http.StatusInternalServerError, "internal_error",
			"The server encountered an unexpected error.")
	}
}
