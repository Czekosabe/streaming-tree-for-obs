package httpapi

import (
	"io/fs"
	"log/slog"
	"net/http"
)

// legalRoutes is the fixed, closed allowlist of local legal-document
// routes. No path parameter ever resolves an arbitrary filename - the
// installed application must be able to show its own licence, privacy
// notice, disclaimer, and third-party notices fully offline (see
// docs/windows-packaging.md §16) without a Markdown renderer, so each is
// served as plain text at a name-stable route.
var legalRoutes = map[string]string{
	"/legal/license":             "LICENSE",
	"/legal/privacy":             "PRIVACY.md",
	"/legal/legal":               "LEGAL.md",
	"/legal/third-party-notices": "THIRD_PARTY_NOTICES.md",
}

// registerLegalRoutes wires the fixed legal-document allowlist above. Each
// path is registered twice, matching every other route in this package:
// once with its allowed method, once without one so a wrong verb produces
// a 405 with an Allow header.
func registerLegalRoutes(mux *http.ServeMux, logger *slog.Logger, legal fs.FS) {
	for path, file := range legalRoutes {
		mux.HandleFunc("GET "+path, handleLegalDocument(logger, legal, file))
		mux.HandleFunc(path, methodNotAllowed(logger, http.MethodGet))
	}
}

// handleLegalDocument serves exactly one fixed, closed-over file from the
// embedded legal filesystem - the request never supplies any part of the
// path.
func handleLegalDocument(logger *slog.Logger, legal fs.FS, file string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := fs.ReadFile(legal, file)
		if err != nil {
			writeError(w, logger, http.StatusNotFound,
				"not_found", "This legal document is unavailable.")
			return
		}

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}
}
