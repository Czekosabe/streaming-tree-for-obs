package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/streaming-tree/server/internal/diagnostics"
)

// SupportBundleBuilder builds the Stage 20E diagnostic support bundle
// (docs/final-hardening.md §C) - a privacy-safe ZIP an operator can
// export on explicit request. The concrete implementation lives
// outside this package (internal/support) so httpapi does not need to
// depend on every subsystem the bundle summarizes; it is wired in via
// Options.DiagnosticsBundle exactly like every other optional service.
type SupportBundleBuilder interface {
	// BuildSupportBundle returns the bundle's bytes and an
	// app-controlled filename (never derived from request input).
	BuildSupportBundle(ctx context.Context) (data []byte, filename string, err error)
}

// registerDiagnosticsRoutes wires the Stage 20E diagnostics API. Never
// registered under /api/public/*: local desktop use relies on the
// same loopback-only exposure every other /api/ route already has,
// and a remote-management deployment gates it through the same
// withRemoteManagementSecurity middleware applied to the whole mux in
// NewRouter - plain session auth for the read-only log retrieval,
// session + CSRF + Origin for the bundle-generation POST, matching
// docs/final-hardening.md §E.
func registerDiagnosticsRoutes(mux *http.ServeMux, logger *slog.Logger, recorder *diagnostics.Recorder, bundler SupportBundleBuilder) {
	mux.HandleFunc("GET /api/logs", handleGetLogs(logger, recorder))
	mux.HandleFunc("/api/logs", methodNotAllowed(logger, http.MethodGet))

	if bundler != nil {
		mux.HandleFunc("POST /api/diagnostics/support-bundle", handlePostSupportBundle(logger, bundler))
		mux.HandleFunc("/api/diagnostics/support-bundle", methodNotAllowed(logger, http.MethodPost))
	}
}

// LogsResponse is the payload of GET /api/logs.
type LogsResponse struct {
	Entries []diagnostics.Entry `json:"entries"`
	// NextCursor, when present, is the value to pass as ?before= to
	// retrieve the next (older) page of matching entries. Absent when
	// the result was not truncated by the limit - i.e. there is
	// nothing older left to page to.
	NextCursor *uint64 `json:"nextCursor,omitempty"`
}

// handleGetLogs serves bounded, filtered retrieval from the ring
// buffer - never the whole buffer unconditionally, never an arbitrary
// filesystem path, never OS-wide system logs (docs/final-hardening.md
// §E/governing task §10).
func handleGetLogs(logger *slog.Logger, recorder *diagnostics.Recorder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		filter := diagnostics.Filter{
			Severity:  strings.ToUpper(strings.TrimSpace(q.Get("severity"))),
			Subsystem: strings.TrimSpace(q.Get("subsystem")),
			Search:    strings.TrimSpace(q.Get("search")),
		}
		if v := q.Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				filter.Limit = n
			}
		}
		if v := q.Get("before"); v != "" {
			if n, err := strconv.ParseUint(v, 10, 64); err == nil {
				filter.Before = n
			}
		}

		entries := recorder.Snapshot(filter)

		resp := LogsResponse{Entries: entries}
		if len(entries) == diagnostics.ClampLimit(filter.Limit) {
			cursor := entries[len(entries)-1].Seq
			resp.NextCursor = &cursor
		}

		writeJSON(w, logger, http.StatusOK, resp)
	}
}

// handlePostSupportBundle generates the support bundle on explicit
// operator request only - never automatic, never uploaded anywhere,
// and the response filename is entirely app-controlled (governing
// task §13).
func handlePostSupportBundle(logger *slog.Logger, bundler SupportBundleBuilder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, filename, err := bundler.BuildSupportBundle(r.Context())
		if err != nil {
			logger.Error("failed to build support bundle", slog.Any("error", err))
			writeError(w, logger, http.StatusInternalServerError,
				"support_bundle_failed", "The support bundle could not be generated.")
			return
		}

		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(data); err != nil {
			logger.Error("failed to write support bundle response", slog.Any("error", err))
		}
	}
}
