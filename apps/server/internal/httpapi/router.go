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
	// Platforms serves the configuration API. When nil those routes are not
	// registered, which keeps the health-only server usable in tests.
	Platforms PlatformService
	// Runtime serves the MediaMTX runtime API. When nil those routes are not
	// registered.
	Runtime RuntimeService
	// Credentials serves the destination-credential API. When nil those
	// routes are not registered, and DELETE /api/platforms/{id} falls back
	// to deleting the platform with no credential-cleanup step.
	Credentials CredentialService
	// Outputs serves the destination output-settings API (RTMP/RTMPS server
	// address, restart preference). When nil those routes are not registered.
	Outputs OutputService
	// FFmpegRuntime reports the resolved FFmpeg dependency. When nil,
	// GET /api/runtime/ffmpeg is not registered.
	FFmpegRuntime FFmpegRuntimeService
	// Branches serves the destination-branch runtime API. When nil, none of
	// the /api/runtime/branches routes are registered.
	Branches BranchRuntimeService
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

	if opts.Platforms != nil {
		registerPlatformRoutes(mux, logger, opts.Platforms, opts.Credentials, opts.Outputs, opts.Branches)
	}

	if opts.Runtime != nil {
		registerRuntimeRoutes(mux, logger, opts.Runtime)
	}

	if opts.FFmpegRuntime != nil {
		mux.HandleFunc("GET /api/runtime/ffmpeg", handleGetFFmpegStatus(logger, opts.FFmpegRuntime))
		mux.HandleFunc("/api/runtime/ffmpeg", methodNotAllowed(logger, http.MethodGet))
	}

	if opts.Branches != nil {
		registerBranchRoutes(mux, logger, opts.Branches)
	}

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

// registerPlatformRoutes wires the platform configuration API.
//
// Each path is registered twice: once with its allowed methods, and once
// without a method so a wrong verb produces a 405 with an Allow header instead
// of falling through to the /api/ catch-all as a 404. Go's ServeMux prefers the
// more specific method-aware pattern, so the bare pattern only ever matches
// when no method pattern did.
func registerPlatformRoutes(mux *http.ServeMux, logger *slog.Logger, service PlatformService, credentials CredentialService, outputs OutputService, branches BranchRuntimeService) {
	mux.HandleFunc("GET /api/platform-definitions", handleListDefinitions(logger, service))
	mux.HandleFunc("/api/platform-definitions", methodNotAllowed(logger, http.MethodGet))

	mux.HandleFunc("GET /api/platforms", handleListPlatforms(logger, service))
	mux.HandleFunc("POST /api/platforms", handleCreatePlatform(logger, service))
	mux.HandleFunc("/api/platforms", methodNotAllowed(logger, http.MethodGet, http.MethodPost))

	mux.HandleFunc("GET /api/platforms/{id}", handleGetPlatform(logger, service))
	mux.HandleFunc("PUT /api/platforms/{id}", handleUpdatePlatform(logger, service))
	if credentials != nil {
		// A platform's credentials are deleted before the platform row
		// itself; see handleDeletePlatformWithCredentials.
		mux.HandleFunc("DELETE /api/platforms/{id}", handleDeletePlatformWithCredentials(logger, service, credentials, branches))
	} else {
		mux.HandleFunc("DELETE /api/platforms/{id}", handleDeletePlatform(logger, service))
	}
	mux.HandleFunc("/api/platforms/{id}",
		methodNotAllowed(logger, http.MethodGet, http.MethodPut, http.MethodDelete))

	mux.HandleFunc("GET /api/platforms/{id}/metadata", handleGetMetadata(logger, service))
	mux.HandleFunc("PUT /api/platforms/{id}/metadata", handleSaveMetadata(logger, service))
	mux.HandleFunc("/api/platforms/{id}/metadata",
		methodNotAllowed(logger, http.MethodGet, http.MethodPut))

	if credentials != nil {
		registerCredentialRoutes(mux, logger, service, credentials)
	}

	if outputs != nil {
		mux.HandleFunc("GET /api/platforms/{id}/output", handleGetOutputSettings(logger, service, outputs))
		mux.HandleFunc("PUT /api/platforms/{id}/output", handleUpdateOutputSettings(logger, service, outputs))
		mux.HandleFunc("/api/platforms/{id}/output", methodNotAllowed(logger, http.MethodGet, http.MethodPut))
	}
}

// registerCredentialRoutes wires the destination-credential API: status,
// setting and deleting a stream key. The secret value itself is never part
// of any response - see CredentialService and credentialStatusResponse in
// credentials.go.
func registerCredentialRoutes(mux *http.ServeMux, logger *slog.Logger, service PlatformService, credentials CredentialService) {
	mux.HandleFunc("GET /api/platforms/{id}/credentials", handleGetCredentials(logger, service, credentials))
	mux.HandleFunc("/api/platforms/{id}/credentials", methodNotAllowed(logger, http.MethodGet))

	mux.HandleFunc("PUT /api/platforms/{id}/credentials/stream-key", handleSetStreamKey(logger, service, credentials))
	mux.HandleFunc("DELETE /api/platforms/{id}/credentials/stream-key", handleDeleteStreamKey(logger, service, credentials))
	mux.HandleFunc("/api/platforms/{id}/credentials/stream-key",
		methodNotAllowed(logger, http.MethodPut, http.MethodDelete))
}

// registerRuntimeRoutes wires the MediaMTX runtime API.
//
// The MediaMTX Control API is deliberately not proxied: the browser never talks
// to it, directly or indirectly. Only these curated endpoints are exposed.
func registerRuntimeRoutes(mux *http.ServeMux, logger *slog.Logger, service RuntimeService) {
	mux.HandleFunc("GET /api/runtime", handleGetRuntime(logger, service))
	mux.HandleFunc("/api/runtime", methodNotAllowed(logger, http.MethodGet))

	for path, handler := range map[string]http.HandlerFunc{
		"/api/runtime/mediamtx/install": handleInstallMediaMTX(logger, service),
		"/api/runtime/mediamtx/start":   handleStartMediaMTX(logger, service),
		"/api/runtime/mediamtx/stop":    handleStopMediaMTX(logger, service),
		"/api/runtime/mediamtx/restart": handleRestartMediaMTX(logger, service),
	} {
		mux.HandleFunc("POST "+path, handler)
		mux.HandleFunc(path, methodNotAllowed(logger, http.MethodPost))
	}
}

// registerBranchRoutes wires the destination-branch runtime API.
//
// start-enabled and stop-all are registered as their own literal paths,
// distinct in segment count from /api/runtime/branches/{id}/start and
// .../{id}/stop, so there is no pattern overlap for ServeMux to resolve.
func registerBranchRoutes(mux *http.ServeMux, logger *slog.Logger, service BranchRuntimeService) {
	mux.HandleFunc("GET /api/runtime/branches", handleGetBranches(logger, service))
	mux.HandleFunc("/api/runtime/branches", methodNotAllowed(logger, http.MethodGet))

	mux.HandleFunc("POST /api/runtime/branches/start-enabled", handleStartEnabledBranches(logger, service))
	mux.HandleFunc("/api/runtime/branches/start-enabled", methodNotAllowed(logger, http.MethodPost))

	mux.HandleFunc("POST /api/runtime/branches/stop-all", handleStopAllBranches(logger, service))
	mux.HandleFunc("/api/runtime/branches/stop-all", methodNotAllowed(logger, http.MethodPost))

	for path, handler := range map[string]http.HandlerFunc{
		"/api/runtime/branches/{id}/start":   handleStartBranch(logger, service),
		"/api/runtime/branches/{id}/stop":    handleStopBranch(logger, service),
		"/api/runtime/branches/{id}/restart": handleRestartBranch(logger, service),
	} {
		mux.HandleFunc("POST "+path, handler)
		mux.HandleFunc(path, methodNotAllowed(logger, http.MethodPost))
	}
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
