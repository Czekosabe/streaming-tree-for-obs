package httpapi

import (
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"
)

const indexHTMLName = "index.html"

// registerProductionRoutes serves the embedded production frontend build
// for every path this router does not already claim - /api/ and /legal/
// are both registered earlier in NewRouter and are never reachable here.
// See docs/windows-packaging.md §4.
func registerProductionRoutes(mux *http.ServeMux, logger *slog.Logger, frontend fs.FS) {
	fileServer := http.FileServer(http.FS(frontend))

	// Registered as a bare "/" (every method) rather than "GET /": Go's
	// ServeMux refuses to register a method-restricted broad-path pattern
	// alongside the already-registered "/api/" (every method, narrower
	// path) - the two would have an ambiguous specificity ordering. The
	// method check happens inside the handler instead.
	mux.Handle("/", productionHandler(logger, frontend, fileServer))
}

// productionHandler distinguishes three cases for any GET path outside
// /api/ and /legal/:
//
//  1. the path looks like a static asset (its last segment has a file
//     extension) - served verbatim if it exists, a real 404 if it does not;
//     never falls back to index.html, so a missing/renamed hashed asset is
//     never silently masked as a successful page load.
//  2. the path exists as a real file with no extension (rare) - served
//     verbatim.
//  3. anything else - a React Router client-side route (management or
//     public overlay, present or future) - receives index.html, exactly
//     like Vite's dev server already does for every one of these routes.
func productionHandler(logger *slog.Logger, frontend fs.FS, fileServer http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		requestPath, ok := cleanRequestPath(r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}

		if requestPath == "" {
			serveIndexHTML(w, logger, frontend)
			return
		}

		if isStaticAssetLikePath(requestPath) {
			if !fileExists(frontend, requestPath) {
				http.NotFound(w, r)
				return
			}
			applyAssetCaching(w, requestPath)
			fileServer.ServeHTTP(w, r)
			return
		}

		if fileExists(frontend, requestPath) {
			fileServer.ServeHTTP(w, r)
			return
		}

		serveIndexHTML(w, logger, frontend)
	})
}

// cleanRequestPath normalizes a URL path to a slash-free relative path
// suitable for fs.FS.Open, rejecting any traversal attempt outright before
// any filesystem lookup happens - defense in depth on top of fs.FS's own
// refusal to resolve outside its root.
func cleanRequestPath(raw string) (string, bool) {
	if strings.Contains(raw, "\\") {
		return "", false
	}

	cleaned := path.Clean(raw)
	trimmed := strings.TrimPrefix(cleaned, "/")
	if trimmed == "." {
		trimmed = ""
	}
	if trimmed == ".." || strings.HasPrefix(trimmed, "../") {
		return "", false
	}

	return trimmed, true
}

// isStaticAssetLikePath reports whether the final path segment has a file
// extension (contains a '.') - the same heuristic every common SPA static
// server uses to decide "attempt a real file, and if missing say so
// honestly" versus "this is a client-side route name."
func isStaticAssetLikePath(p string) bool {
	return strings.Contains(path.Base(p), ".")
}

// fileExists reports whether p names a real, readable, non-directory file
// in frontend.
func fileExists(frontend fs.FS, p string) bool {
	f, err := frontend.Open(p)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// applyAssetCaching sets a long, immutable cache lifetime for Vite's own
// content-hashed asset directory - safe because the filename itself
// changes whenever the content does. Every other path (starting with
// index.html) is left at its own explicit no-cache header instead.
func applyAssetCaching(w http.ResponseWriter, p string) {
	if strings.HasPrefix(p, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
}

// serveIndexHTML writes the SPA entry point. index.html deliberately never
// becomes long-cacheable: it is the one file whose content can legitimately
// change between releases while its own URL (every non-asset path) never
// does.
func serveIndexHTML(w http.ResponseWriter, logger *slog.Logger, frontend fs.FS) {
	data, err := fs.ReadFile(frontend, indexHTMLName)
	if err != nil {
		writeError(w, logger, http.StatusInternalServerError,
			"frontend_unavailable", "The application frontend is unavailable.")
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
