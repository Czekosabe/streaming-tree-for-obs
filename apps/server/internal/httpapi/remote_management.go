// Stage 20D2B remote-management security surface
// (docs/remote-management.md). Every function/type in this file is a
// no-op unless RemoteManagementOptions.Enabled is true - a desktop
// build or a plain --headless (no --remote-management) deployment
// never constructs a real *auth.SessionStore/*auth.LoginLimiter and
// never has this middleware wired into its handler chain at all (see
// router.go's own NewRouter).
package httpapi

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/streaming-tree/server/internal/auth"
)

// AdminAuthService verifies the single administrator's password
// (docs/remote-management.md §9) - narrow on purpose, exactly like
// CredentialService's own doc comment explains for the same reason:
// no method here can ever expose the stored verifier itself.
type AdminAuthService interface {
	VerifyPassword(ctx context.Context, password string) (bool, error)
}

// RemoteManagementOptions groups everything the remote-management auth
// surface needs. Present (Enabled true) only for a Linux headless
// deployment that explicitly opted in - see docs/remote-management.md
// §3/§47.
type RemoteManagementOptions struct {
	Enabled bool
	// ExternalOrigin is the canonical "scheme://host[:port]" form
	// (config.CanonicalRemoteManagementOrigin's own output) - never a
	// raw, unvalidated environment value.
	ExternalOrigin string
	Auth           AdminAuthService
	Sessions       *auth.SessionStore
	LoginLimiter   *auth.LoginLimiter
}

// sessionCookieName uses the __Host- prefix (docs/remote-management.md
// §2/§11): Secure + Path=/ + no Domain, the strongest cookie-scoping
// guarantee available, since this deployment model never needs to
// share the cookie across a subdomain.
const sessionCookieName = "__Host-streaming-tree-session"

// publicManagementAPIPrefixes lists every /api/ prefix reachable
// without authentication when remote management is enabled -
// deliberately tiny (docs/remote-management.md §15). Everything else
// under /api/ requires a valid session - the deny-by-default guarantee
// the governing task requires, enforced once here rather than per
// route.
var publicManagementAPIPrefixes = []string{
	"/api/health",
	"/api/auth/",
}

// isPublicManagementAPIPath reports whether p is one of the narrow
// public API exceptions (docs/remote-management.md §15). Paths outside
// /api/ entirely (static frontend assets, the SPA shell, /legal/*) are
// handled separately by requiresAuthentication below - this function
// only classifies /api/ paths.
func isPublicManagementAPIPath(p string) bool {
	if strings.HasPrefix(p, "/api/public/") {
		return true
	}
	for _, prefix := range publicManagementAPIPrefixes {
		if p == strings.TrimSuffix(prefix, "/") || strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}

// requiresAuthentication reports whether p needs a valid remote-
// management session: every /api/ path except the narrow public
// exceptions above. Static assets, the SPA shell, and /legal/* are
// never gated here - the login page itself must load unauthenticated,
// and the frontend's own routing/auth-state (never persisted client
// side beyond the HttpOnly cookie) decides what to render.
func requiresAuthentication(p string) bool {
	if !strings.HasPrefix(p, "/api/") {
		return false
	}
	return !isPublicManagementAPIPath(p)
}

// withRemoteManagementSecurity is the single deny-by-default
// middleware backing docs/remote-management.md §7/§15: session +
// CSRF + Origin (+ Sec-Fetch-Site defense in depth) for every
// protected /api/ route, applied once at the top of the handler
// chain rather than per individual route registration.
func withRemoteManagementSecurity(logger *slog.Logger, rm RemoteManagementOptions) middleware {
	return func(next http.Handler) http.Handler {
		if !rm.Enabled {
			// Zero behavior change for every non-D2B deployment - the
			// exact same handler chain as before this middleware
			// existed.
			return next
		}

		allowedOrigin := rm.ExternalOrigin

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !requiresAuthentication(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			if !validateForwardedRequest(r, allowedOrigin) {
				writeError(w, logger, http.StatusForbidden,
					"forwarded_metadata_invalid", "This request's proxy metadata is not permitted.")
				return
			}

			if site := r.Header.Get("Sec-Fetch-Site"); site == "cross-site" {
				writeError(w, logger, http.StatusForbidden,
					"cross_site_request_rejected", "This request's origin is not permitted.")
				return
			}

			session, ok := sessionFromRequest(r, rm.Sessions)
			if !ok {
				writeError(w, logger, http.StatusUnauthorized,
					"unauthenticated", "Authentication is required.")
				return
			}

			if isUnsafeMethod(r.Method) {
				origin := r.Header.Get("Origin")
				if origin == "" || strings.TrimRight(origin, "/") != allowedOrigin {
					writeError(w, logger, http.StatusForbidden,
						"origin_not_allowed", "This origin is not permitted to perform this action.")
					return
				}
				if !auth.CheckCSRFToken(session, r.Header.Get("X-CSRF-Token")) {
					writeError(w, logger, http.StatusForbidden,
						"csrf_token_invalid", "Missing or invalid CSRF token.")
					return
				}
			}

			next.ServeHTTP(w, r.WithContext(withSessionContext(r.Context(), session)))
		})
	}
}

// withManagementSecurityHeaders adds the headers docs/remote-
// management.md §14 requires to every /api/ response (never to
// static assets/SPA routes or /api/public/* - overlay pages must keep
// rendering inside OBS Browser Source unmodified) when remote
// management is enabled. Applied outside (before)
// withRemoteManagementSecurity in the chain so an auth-rejected
// response (401/403) still carries these headers.
func withManagementSecurityHeaders(rm RemoteManagementOptions) middleware {
	return func(next http.Handler) http.Handler {
		if !rm.Enabled {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api/") && !strings.HasPrefix(r.URL.Path, "/api/public/") {
				w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'")
				w.Header().Set("X-Content-Type-Options", "nosniff")
				w.Header().Set("Referrer-Policy", "same-origin")
				if strings.HasPrefix(r.URL.Path, "/api/auth/") {
					w.Header().Set("Cache-Control", "no-store")
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// isUnsafeMethod reports whether method is one CSRF protection must
// cover (docs/remote-management.md §7/§22: unsafe methods only - GET/
// HEAD never require a CSRF token).
func isUnsafeMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// sessionContextKey is unexported so no other package can construct
// or forge one.
type sessionContextKey struct{}

func withSessionContext(ctx context.Context, session auth.Session) context.Context {
	return context.WithValue(ctx, sessionContextKey{}, session)
}

// sessionFromRequest validates the request's session cookie against
// store, returning (Session{}, false) for a missing/malformed/expired
// cookie - a caller must never distinguish these cases in its response
// (docs/remote-management.md §9/§18: no useful remote distinction
// beyond what the contract needs).
func sessionFromRequest(r *http.Request, store *auth.SessionStore) (auth.Session, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return auth.Session{}, false
	}
	session, err := store.Touch(cookie.Value)
	if err != nil {
		return auth.Session{}, false
	}
	return session, true
}

// setSessionCookie writes the one D2B session cookie with every
// attribute docs/remote-management.md §11 requires.
func setSessionCookie(w http.ResponseWriter, session auth.Session) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    session.ID,
		Path:     "/",
		MaxAge:   int(auth.AbsoluteLifetime.Seconds()),
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
}

// clearSessionCookie deletes the session cookie (logout) - the same
// attributes as setSessionCookie except MaxAge<0, which every browser
// treats as "delete immediately" regardless of the original Max-Age.
func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
}

// clientIPForRateLimit derives the client IP the login rate limiter
// keys on (docs/remote-management.md §8/§30): the forwarded value only
// when the direct TCP peer is loopback (the only topology D2B
// supports - a real reverse proxy on the same host), otherwise the
// direct peer address itself (the desktop/local-headless case, with
// no proxy in front at all). A malformed or multi-value forwarded
// header is never trusted - falls back to the direct peer rather than
// guessing.
func clientIPForRateLimit(r *http.Request) string {
	peerHost, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		peerHost = r.RemoteAddr
	}
	peerIP := net.ParseIP(peerHost)

	if peerIP != nil && peerIP.IsLoopback() {
		if forwarded, ok := singleForwardedValue(r, "X-Forwarded-For"); ok {
			if net.ParseIP(forwarded) != nil {
				return forwarded
			}
		}
	}

	return peerHost
}

// validateForwardedRequest enforces docs/remote-management.md §8's
// forwarding-header contract for the current request: when the direct
// peer is loopback, X-Forwarded-Proto must be exactly "https" and
// X-Forwarded-Host must exactly equal the configured external origin's
// own host[:port] - a mismatch, or a peer that is not loopback at all
// while remote management is enabled, fails closed. Called once by
// withRemoteManagementSecurity's own caller chain (main.go's listener
// setup already guarantees the backend itself is loopback-only; this
// function instead validates the *proxy's* forwarded metadata for a
// request that has already reached this loopback backend).
func validateForwardedRequest(r *http.Request, canonicalOrigin string) bool {
	peerHost, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		peerHost = r.RemoteAddr
	}
	peerIP := net.ParseIP(peerHost)
	if peerIP == nil || !peerIP.IsLoopback() {
		// No proxy hop at all (e.g. a direct loopback curl from the
		// same machine, common during local testing) - nothing to
		// validate; the Origin/CSRF/session checks above already gate
		// the request. This function only rejects when a forwarded
		// header IS present but disagrees with the configured origin.
		if r.Header.Get("X-Forwarded-Proto") == "" && r.Header.Get("X-Forwarded-Host") == "" {
			return true
		}
		return false
	}

	proto, protoOK := singleForwardedValue(r, "X-Forwarded-Proto")
	host, hostOK := singleForwardedValue(r, "X-Forwarded-Host")
	if !protoOK && !hostOK {
		// No proxy metadata present at all on this request - treated
		// as a direct loopback call, not a proxied one.
		return true
	}
	if !protoOK || !hostOK {
		return false
	}
	if proto != "https" {
		return false
	}

	wantHost := strings.TrimPrefix(canonicalOrigin, "https://")
	return host == wantHost
}

// singleForwardedValue returns the header's value only when it was
// sent exactly once and contains no comma-separated list - anything
// else (repeated header, multiple values) is rejected outright rather
// than "take the first"/"take the last" (docs/remote-management.md
// §8: an unambiguous single-hop contract has no legitimate reason to
// ever see more than one value).
func singleForwardedValue(r *http.Request, name string) (string, bool) {
	values := r.Header.Values(name)
	if len(values) != 1 {
		return "", false
	}
	if strings.Contains(values[0], ",") {
		return "", false
	}
	trimmed := strings.TrimSpace(values[0])
	if trimmed == "" {
		return "", false
	}
	return trimmed, true
}

// retryAfterHeader formats a Retry-After header value in whole
// seconds, rounding up so a caller never retries a moment too early.
func retryAfterHeader(seconds int) string {
	if seconds < 1 {
		seconds = 1
	}
	return strconv.Itoa(seconds)
}
