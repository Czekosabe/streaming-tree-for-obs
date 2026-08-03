// Package httpapi wires the REST API: routes, middleware and JSON responses.
//
// Route handlers stay thin. Domain logic (stream routing, platform metadata,
// process supervision) will live in dedicated packages and be injected here.
package httpapi

import (
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Options carries everything the router needs from the caller.
type Options struct {
	Logger         *slog.Logger
	AllowedOrigins []string
	// StartedAt is used to report uptime on the health endpoint.
	StartedAt time.Time
}

// NewRouter builds the fully decorated HTTP handler.
func NewRouter(opts Options) http.Handler {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	mux := http.NewServeMux()

	// Method-aware pattern (Go 1.22+).
	mux.HandleFunc("GET /api/health", healthHandler(logger, opts.StartedAt))
	// Same path without a method: matched only when the method did not match
	// above, so a wrong verb yields 405 instead of being swallowed by the
	// /api/ catch-all below.
	mux.HandleFunc("/api/health", methodNotAllowed(logger, http.MethodGet))

	// Anything else under /api is an explicit, JSON-shaped 404 rather than the
	// default plain-text response, so the frontend can parse every failure.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, logger, http.StatusNotFound,
			"not_found", "This API endpoint does not exist.")
	})

	// The root is a tiny liveness hint for someone opening the port in a browser.
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, logger, http.StatusOK, map[string]string{
			"service": "streaming-tree-server",
			"health":  "/api/health",
		})
	})

	return chain(mux,
		withRecovery(logger),
		withLogging(logger),
		withCORS(opts.AllowedOrigins),
	)
}

// methodNotAllowed returns a 405 carrying the Allow header, as required by the
// HTTP specification.
func methodNotAllowed(logger *slog.Logger, allowed ...string) http.HandlerFunc {
	allowHeader := strings.Join(allowed, ", ")

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Allow", allowHeader)
		writeError(w, logger, http.StatusMethodNotAllowed,
			"method_not_allowed", "This endpoint only accepts: "+allowHeader+".")
	}
}
