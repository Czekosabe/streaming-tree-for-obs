package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
)

// shutdownRequestBodyLimit is deliberately tiny - the only valid body is
// {"confirm":true}.
const shutdownRequestBodyLimit = 256

// shutdownRequest is the one accepted body shape for POST
// /api/system/shutdown. There is no generic "action"/"command" parameter -
// this endpoint does exactly one thing.
type shutdownRequest struct {
	Confirm bool `json:"confirm"`
}

// registerShutdownRoute wires POST /api/system/shutdown - the packaged
// application's real "Quit Streaming Tree" action. See
// docs/windows-packaging.md §8 for the full security-boundary reasoning:
// an ordinary HTML <form> cannot submit application/json at all (only
// x-www-form-urlencoded, multipart/form-data, or text/plain), so the
// strict Content-Type/body-shape check below already defeats a
// cross-origin form-based attempt independent of the Origin check.
func registerShutdownRoute(mux *http.ServeMux, logger *slog.Logger, cancel context.CancelFunc, allowedOrigins []string) {
	mux.HandleFunc("POST /api/system/shutdown", handleShutdown(logger, cancel, allowedOrigins))
	mux.HandleFunc("/api/system/shutdown", methodNotAllowed(logger, http.MethodPost))
}

func handleShutdown(logger *slog.Logger, cancel context.CancelFunc, allowedOrigins []string) http.HandlerFunc {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		allowed[strings.TrimRight(origin, "/")] = struct{}{}
	}

	// Shutdown must happen exactly once: cancel() itself is safe to call
	// repeatedly, but this flag also makes a repeat request return
	// immediately with the same success response instead of re-logging a
	// second "shutdown requested" line.
	var triggered atomic.Bool

	return func(w http.ResponseWriter, r *http.Request) {
		if origin := strings.TrimRight(r.Header.Get("Origin"), "/"); origin != "" {
			if _, ok := allowed[origin]; !ok {
				writeError(w, logger, http.StatusForbidden,
					"origin_not_allowed", "This origin is not permitted to shut down the application.")
				return
			}
		}

		var body shutdownRequest
		if err := decodeJSONWithLimit(w, r, &body, shutdownRequestBodyLimit); err != nil {
			writeDecodeError(w, logger, err)
			return
		}
		if !body.Confirm {
			writeError(w, logger, http.StatusBadRequest,
				"confirmation_required", `The request body must be {"confirm":true}.`)
			return
		}

		if triggered.CompareAndSwap(false, true) {
			logger.Info("shutdown requested through the application UI")
			cancel()
		}

		writeJSON(w, logger, http.StatusOK, map[string]string{"status": "shutting_down"})
	}
}
