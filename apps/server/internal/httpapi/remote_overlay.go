// Stage 20D2C remote-overlay backend defense-in-depth
// (docs/remote-ingest.md §11). This is a second, independent boundary
// behind the reverse-proxy's own overlay/management origin split -
// PRE-20D2C's own lesson was that the proxy configuration must never
// be the ONLY thing enforcing a routing boundary. Every function here
// is a no-op unless RemoteOverlayOptions.Enabled is true.
package httpapi

import (
	"log/slog"
	"net"
	"net/http"
	"strings"
)

// RemoteOverlayOptions groups the Stage 20D2C remote-overlay
// defense-in-depth settings. Present (Enabled true) only when an
// operator has explicitly configured a remote overlay origin
// (docs/remote-ingest.md §10) - independent of whether remote ingest
// itself is enabled.
type RemoteOverlayOptions struct {
	Enabled bool
	// CanonicalOrigin is the "scheme://host[:port]" form of the
	// configured overlay origin (config.CanonicalRemoteManagementOrigin's
	// own output, reused here - it is a generic origin normalizer
	// despite its name).
	CanonicalOrigin string
}

// overlaySurfaceRoots/overlaySurfacePrefixes mirror the corrected
// Caddy exclusion matcher exactly (docs/examples/
// Caddyfile.remote-management, docs/examples/Caddyfile.self-hosted:
// `path /overlay /overlay/* /api/public /api/public/*`) - the same
// four patterns, so the backend's own idea of "the local-only public-
// overlay surface" can never silently drift from the reverse-proxy
// configuration's own idea of it.
var overlaySurfaceRoots = []string{"/overlay", "/api/public"}
var overlaySurfacePrefixes = []string{"/overlay/", "/api/public/"}

func isOverlaySurfacePath(p string) bool {
	for _, root := range overlaySurfaceRoots {
		if p == root {
			return true
		}
	}
	for _, prefix := range overlaySurfacePrefixes {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}

// withRemoteOverlaySecurity is the second boundary docs/remote-
// ingest.md §11 requires. A direct loopback request (no forwarding
// headers at all) to /overlay/* or /api/public/* is never affected -
// the existing local OBS Browser Source contract, unchanged in every
// mode including this one. A FORWARDED request (peer is the trusted
// loopback reverse proxy, forwarding headers present) to one of those
// paths is accepted only when the forwarded scheme is exactly "https"
// and the forwarded host exactly equals the configured overlay
// origin's own host[:port] - a forwarded MANAGEMENT hostname, an
// unrecognized hostname, or malformed/duplicated forwarding headers
// are all rejected the same way remote_management.go's own
// validateForwardedRequest already rejects them for the management
// origin (singleForwardedValue is shared, unmodified, from that file).
// Every other path is entirely unaffected by this middleware - it
// never gates the management API; withRemoteManagementSecurity
// already owns that.
func withRemoteOverlaySecurity(logger *slog.Logger, opts RemoteOverlayOptions) middleware {
	return func(next http.Handler) http.Handler {
		if !opts.Enabled {
			// Zero behavior change when no remote overlay origin is
			// configured - the exact same handler chain as before this
			// middleware existed.
			return next
		}

		wantHost := strings.TrimPrefix(opts.CanonicalOrigin, "https://")

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isOverlaySurfacePath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			peerHost, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				peerHost = r.RemoteAddr
			}
			peerIP := net.ParseIP(peerHost)

			proto, protoOK := singleForwardedValue(r, "X-Forwarded-Proto")
			host, hostOK := singleForwardedValue(r, "X-Forwarded-Host")

			if !protoOK && !hostOK {
				// No forwarding metadata at all - a direct call, whether
				// from the loopback OBS Browser Source (the ordinary
				// local case, every mode) or from something else
				// entirely; the backend's own loopback-only listener
				// binding is what protects it from being reachable
				// off-machine in the first place, exactly as it always
				// has. This middleware only ever rejects a request that
				// carries forwarding metadata disagreeing with the
				// configured overlay origin.
				next.ServeHTTP(w, r)
				return
			}

			if peerIP == nil || !peerIP.IsLoopback() {
				// Forwarding headers present but the direct peer is not
				// the trusted loopback proxy - never trust client-
				// supplied forwarding metadata from an untrusted peer.
				writeError(w, logger, http.StatusForbidden,
					"forwarded_metadata_invalid", "This request's proxy metadata is not permitted.")
				return
			}
			if !protoOK || !hostOK {
				writeError(w, logger, http.StatusForbidden,
					"forwarded_metadata_invalid", "This request's proxy metadata is not permitted.")
				return
			}
			if proto != "https" {
				writeError(w, logger, http.StatusForbidden,
					"forwarded_metadata_invalid", "This request's proxy metadata is not permitted.")
				return
			}
			if host != wantHost {
				// Includes the exact case docs/remote-ingest.md §11
				// calls out by name: the management hostname forwarded
				// against an overlay-surface path is rejected here, the
				// same as any other unrecognized hostname would be.
				writeError(w, logger, http.StatusForbidden,
					"forwarded_host_not_allowed", "This request's forwarded host is not permitted to reach this surface.")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
