package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/streaming-tree/server/internal/updater"
)

// UpdateService serves the Stage 20B application-updater API
// (docs/updater.md §28). Implemented by *updater.Manager.
type UpdateService interface {
	Status(ctx context.Context) updater.Status
	SetAutoCheck(ctx context.Context, enabled bool) error
	CheckNow(ctx context.Context) error
	Download(ctx context.Context) error
	Install(ctx context.Context) error
}

// updatePreferencesRequest is the one accepted body shape for PUT
// /api/updates/preferences.
type updatePreferencesRequest struct {
	AutoCheck bool `json:"autoCheck"`
}

// updateInstallRequestBodyLimit/updateInstallRequest mirror
// system.go's shutdownRequestBodyLimit/shutdownRequest exactly
// (docs/updater.md §29) - the only valid body is {"confirm":true}, for
// the same reason: an ordinary HTML <form> cannot submit
// application/json, so this strict shape independently defeats a
// cross-origin form-based attempt.
const updateInstallRequestBodyLimit = 256

type updateInstallRequest struct {
	Confirm bool `json:"confirm"`
}

// registerUpdaterRoutes wires the /api/updates/* API.
func registerUpdaterRoutes(mux *http.ServeMux, logger *slog.Logger, service UpdateService, allowedOrigins []string) {
	mux.HandleFunc("GET /api/updates/status", handleGetUpdateStatus(logger, service))
	mux.HandleFunc("/api/updates/status", methodNotAllowed(logger, http.MethodGet))

	mux.HandleFunc("PUT /api/updates/preferences", handlePutUpdatePreferences(logger, service))
	mux.HandleFunc("/api/updates/preferences", methodNotAllowed(logger, http.MethodPut))

	mux.HandleFunc("POST /api/updates/check", handleCheckForUpdate(logger, service))
	mux.HandleFunc("/api/updates/check", methodNotAllowed(logger, http.MethodPost))

	mux.HandleFunc("POST /api/updates/download", handleDownloadUpdate(logger, service))
	mux.HandleFunc("/api/updates/download", methodNotAllowed(logger, http.MethodPost))

	allowed := buildOriginAllowlist(allowedOrigins)
	mux.HandleFunc("POST /api/updates/install", handleInstallUpdate(logger, service, allowed))
	mux.HandleFunc("/api/updates/install", methodNotAllowed(logger, http.MethodPost))
}

func handleGetUpdateStatus(logger *slog.Logger, service UpdateService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, logger, http.StatusOK, service.Status(r.Context()))
	}
}

func handlePutUpdatePreferences(logger *slog.Logger, service UpdateService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body updatePreferencesRequest
		if err := decodeJSON(w, r, &body); err != nil {
			writeDecodeError(w, logger, err)
			return
		}

		if err := service.SetAutoCheck(r.Context(), body.AutoCheck); err != nil {
			writeUpdateError(w, logger, service, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, service.Status(r.Context()))
	}
}

func handleCheckForUpdate(logger *slog.Logger, service UpdateService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}
		if err := service.CheckNow(r.Context()); err != nil {
			writeUpdateError(w, logger, service, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, service.Status(r.Context()))
	}
}

func handleDownloadUpdate(logger *slog.Logger, service UpdateService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}
		if err := service.Download(r.Context()); err != nil {
			writeUpdateError(w, logger, service, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, service.Status(r.Context()))
	}
}

func handleInstallUpdate(logger *slog.Logger, service UpdateService, allowed originAllowlist) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !checkLocalActionOrigin(w, logger, r, allowed) {
			return
		}

		var body updateInstallRequest
		if err := decodeJSONWithLimit(w, r, &body, updateInstallRequestBodyLimit); err != nil {
			writeDecodeError(w, logger, err)
			return
		}
		if !body.Confirm {
			writeError(w, logger, http.StatusBadRequest,
				"confirmation_required", `The request body must be {"confirm":true}.`)
			return
		}

		if err := service.Install(r.Context()); err != nil {
			writeUpdateError(w, logger, service, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, map[string]string{"status": "installing"})
	}
}

// writeUpdateError maps an updater.Manager error onto the HTTP
// contract (docs/updater.md §28/§30) - never a raw error string.
func writeUpdateError(w http.ResponseWriter, logger *slog.Logger, service UpdateService, r *http.Request, err error) {
	switch {
	case errors.Is(err, updater.ErrDisabled):
		writeError(w, logger, http.StatusConflict, "updater_disabled",
			"Updates are available in packaged release builds.")

	case errors.Is(err, updater.ErrInstallBlocked):
		status := service.Status(r.Context())
		code := status.BlockerCode
		if code == "" {
			code = "install_blocked"
		}
		writeError(w, logger, http.StatusConflict, code,
			"The update cannot be installed right now.")

	default:
		writeError(w, logger, http.StatusBadGateway, "update_action_failed",
			"That updater action could not be completed. Check the update status for details.")
	}
}
