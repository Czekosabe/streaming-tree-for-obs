package httpapi

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/streaming-tree/server/internal/auth"
)

// maxLoginRequestBodyBytes: the only accepted body is {"password":
// "..."}, bounded well below the general request-body ceiling
// (docs/remote-management.md §18/§20).
const maxLoginRequestBodyBytes = auth.MaxPasswordLength + 256

type loginRequest struct {
	Password string `json:"password"`
}

// sessionBootstrapResponse is the one response shape both GET
// /api/auth/session and a successful POST /api/auth/login return -
// the frontend never has to make a second round trip for the CSRF
// token after logging in (docs/remote-management.md §18).
type sessionBootstrapResponse struct {
	Authenticated bool   `json:"authenticated"`
	CSRFToken     string `json:"csrfToken,omitempty"`
}

// registerAuthRoutes wires the Stage 20D2B authentication API
// (docs/remote-management.md §18) - registered only when remote
// management is enabled (see router.go's own NewRouter).
func registerAuthRoutes(mux *http.ServeMux, logger *slog.Logger, rm RemoteManagementOptions) {
	mux.HandleFunc("GET /api/auth/session", handleAuthSession(logger, rm))
	mux.HandleFunc("/api/auth/session", methodNotAllowed(logger, http.MethodGet))

	mux.HandleFunc("POST /api/auth/login", handleLogin(logger, rm))
	mux.HandleFunc("/api/auth/login", methodNotAllowed(logger, http.MethodPost))

	mux.HandleFunc("POST /api/auth/logout", handleLogout(logger, rm))
	mux.HandleFunc("/api/auth/logout", methodNotAllowed(logger, http.MethodPost))
}

// handleAuthSession bootstraps the frontend's auth state on load -
// always 200, distinguishing authenticated/not in the body rather
// than via status code, since an unauthenticated visitor asking "am I
// authenticated?" is not itself an error (docs/remote-management.md
// §18).
func handleAuthSession(logger *slog.Logger, rm RemoteManagementOptions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")

		session, ok := sessionFromRequest(r, rm.Sessions)
		if !ok {
			writeJSON(w, logger, http.StatusOK, sessionBootstrapResponse{Authenticated: false})
			return
		}
		writeJSON(w, logger, http.StatusOK, sessionBootstrapResponse{
			Authenticated: true,
			CSRFToken:     session.CSRFToken,
		})
	}
}

// handleLogin authenticates the single administrator
// (docs/remote-management.md §9/§18/§20/§21). No username field: there
// is exactly one identity, so nothing to enumerate.
func handleLogin(logger *slog.Logger, rm RemoteManagementOptions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")

		// Origin is checked even before authentication exists - a
		// cross-origin browser cannot submit credentials to this
		// endpoint at all (docs/remote-management.md §21).
		origin := r.Header.Get("Origin")
		if origin != "" && strings.TrimRight(origin, "/") != rm.ExternalOrigin {
			writeError(w, logger, http.StatusForbidden,
				"origin_not_allowed", "This origin is not permitted to perform this action.")
			return
		}
		if origin == "" {
			writeError(w, logger, http.StatusForbidden,
				"origin_not_allowed", "This origin is not permitted to perform this action.")
			return
		}
		if site := r.Header.Get("Sec-Fetch-Site"); site == "cross-site" {
			writeError(w, logger, http.StatusForbidden,
				"cross_site_request_rejected", "This request's origin is not permitted.")
			return
		}

		clientIP := clientIPForRateLimit(r)
		if allow, retryAfter := rm.LoginLimiter.Allow(clientIP); !allow {
			w.Header().Set("Retry-After", retryAfterHeader(int(retryAfter.Seconds())+1))
			writeError(w, logger, http.StatusTooManyRequests,
				"rate_limited", "Too many failed login attempts. Try again later.")
			return
		}

		var body loginRequest
		if err := decodeJSONWithLimit(w, r, &body, maxLoginRequestBodyBytes); err != nil {
			writeDecodeError(w, logger, err)
			return
		}

		ok, err := rm.Auth.VerifyPassword(r.Context(), body.Password)
		if err != nil {
			logger.Error("admin password verification failed", slog.Any("error", err))
			writeError(w, logger, http.StatusInternalServerError,
				"internal_error", "The server encountered an unexpected error.")
			return
		}
		if !ok {
			rm.LoginLimiter.RecordFailure(clientIP)
			logger.Warn("remote login failed")
			writeError(w, logger, http.StatusUnauthorized,
				"invalid_credentials", "Incorrect password.")
			return
		}

		session, err := rm.Sessions.Create()
		if err != nil {
			logger.Error("failed to create session", slog.Any("error", err))
			writeError(w, logger, http.StatusInternalServerError,
				"internal_error", "The server encountered an unexpected error.")
			return
		}

		setSessionCookie(w, session)
		logger.Info("remote login succeeded")
		writeJSON(w, logger, http.StatusOK, sessionBootstrapResponse{
			Authenticated: true,
			CSRFToken:     session.CSRFToken,
		})
	}
}

// handleLogout requires an existing valid session and CSRF token - it
// is itself a state-changing action (docs/remote-management.md §18).
func handleLogout(logger *slog.Logger, rm RemoteManagementOptions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")

		session, ok := sessionFromRequest(r, rm.Sessions)
		if !ok {
			writeError(w, logger, http.StatusUnauthorized,
				"unauthenticated", "Authentication is required.")
			return
		}
		if !auth.CheckCSRFToken(session, r.Header.Get("X-CSRF-Token")) {
			writeError(w, logger, http.StatusForbidden,
				"csrf_token_invalid", "Missing or invalid CSRF token.")
			return
		}

		rm.Sessions.Delete(session.ID)
		clearSessionCookie(w)
		logger.Info("logout")
		writeJSON(w, logger, http.StatusOK, map[string]string{"status": "logged_out"})
	}
}
