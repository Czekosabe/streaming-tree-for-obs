package httpapi

import (
	"log/slog"
	"net/http"
	"strings"
)

// originAllowlist is a normalized set of allowed browser origins, built
// once per route registration and reused by every request.
type originAllowlist map[string]struct{}

// buildOriginAllowlist normalizes allowedOrigins (trailing slash
// stripped) into a lookup set.
func buildOriginAllowlist(allowedOrigins []string) originAllowlist {
	allowed := make(originAllowlist, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		allowed[strings.TrimRight(origin, "/")] = struct{}{}
	}
	return allowed
}

// checkLocalActionOrigin enforces the Origin allowlist a same-origin,
// non-form-submittable local action requires (docs/windows-packaging.md
// §8) - shared by POST /api/system/shutdown and POST /api/updates/
// install (docs/updater.md §29) so there is exactly one implementation
// of this check, never two that could subtly diverge. An absent Origin
// header (a non-browser client) is allowed through, exactly as the
// original shutdown-only implementation already did. Returns false
// (and has already written the error response) when Origin is present
// and not on allowed.
func checkLocalActionOrigin(w http.ResponseWriter, logger *slog.Logger, r *http.Request, allowed originAllowlist) bool {
	origin := strings.TrimRight(r.Header.Get("Origin"), "/")
	if origin == "" {
		return true
	}
	if _, ok := allowed[origin]; ok {
		return true
	}
	writeError(w, logger, http.StatusForbidden,
		"origin_not_allowed", "This origin is not permitted to perform this action.")
	return false
}
