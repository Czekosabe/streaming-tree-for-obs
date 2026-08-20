// Stage 20D2C remote-overlay capability management API
// (docs/remote-ingest.md §12/§8's own shape). Every route lives under
// /api/remote-overlay/ - never /api/public/* - so it is protected by
// the exact same deny-by-default session/CSRF/Origin middleware
// (withRemoteManagementSecurity) every other authenticated management
// route already uses; nothing in this file adds a second check.
package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/streaming-tree/server/internal/domain/remoteoverlay"
)

// RemoteOverlayCapabilities is the subset of remoteoverlay.Repository
// the management handlers need - the same interface
// *sqlite.RemoteOverlayCapabilityRepository already satisfies, kept
// separate from RemoteOverlayResolver (which only needs Resolve) so
// handler tests can stub exactly what each surface uses.
type RemoteOverlayCapabilities interface {
	Issue(ctx context.Context, domain remoteoverlay.Domain, localSlug string) (remoteoverlay.Capability, error)
	Revoke(ctx context.Context, domain remoteoverlay.Domain, localSlug string) error
	Get(ctx context.Context, domain remoteoverlay.Domain, localSlug string) (remoteoverlay.Capability, bool, error)
}

// remoteOverlayOwnerExists reports whether localSlug names a real,
// existing profile in domain - the check that stops the shared
// capability table from becoming an orphan-capability factory for a
// fabricated slug the operator never actually created. Every branch
// mirrors the exact "any error means not found" convention the
// corresponding public resolvePublic* helper already uses for that
// domain, so this check and the public routes can never disagree
// about what "exists" means.
type RemoteOverlayOwners struct {
	ChatOverlays ChatOverlayProfileService
	Alerts       AlertsService
	Audio        AudioService
	Widgets      GoalsService
}

func (o RemoteOverlayOwners) exists(ctx context.Context, domain remoteoverlay.Domain, localSlug string) bool {
	switch domain {
	case remoteoverlay.DomainChatOverlay:
		if o.ChatOverlays == nil {
			return false
		}
		_, err := o.ChatOverlays.GetProfileByPublicSlug(ctx, localSlug)
		return err == nil
	case remoteoverlay.DomainAlertProfile:
		if o.Alerts == nil {
			return false
		}
		_, err := o.Alerts.GetProfileByPublicSlug(ctx, localSlug)
		return err == nil
	case remoteoverlay.DomainAudio:
		if o.Audio == nil {
			return false
		}
		// Audio has exactly one profile, identified by its own current
		// slug - there is no per-id lookup to call, mirroring how
		// every public audio handler already checks existence
		// (CurrentPublicSlug() != slug).
		return o.Audio.CurrentPublicSlug() == localSlug && localSlug != ""
	case remoteoverlay.DomainWidget:
		if o.Widgets == nil {
			return false
		}
		_, err := o.Widgets.GetWidgetProfileByPublicSlug(ctx, localSlug)
		return err == nil
	default:
		return false
	}
}

// remoteOverlayRoutePrefix is the real, existing frontend Browser
// Source route (apps/web/src/App.tsx) for each domain - never
// invented, never accepting a caller-supplied path. The embedded SPA
// route handler passes whatever value appears here straight through
// to the matching /api/public/* route, so a remote capability token
// works at this layer identically to a local publicSlug - no frontend
// change was required to support it.
var remoteOverlayRoutePrefix = map[remoteoverlay.Domain]string{
	remoteoverlay.DomainChatOverlay:  "/overlay/chat/",
	remoteoverlay.DomainAlertProfile: "/overlay/alerts/",
	remoteoverlay.DomainAudio:        "/overlay/audio/",
	remoteoverlay.DomainWidget:       "/overlay/widgets/",
}

// remoteOverlayURL builds the full remote Browser Source URL from the
// validated configured overlay origin (never a caller-supplied value,
// never constructed from a request's Host/X-Forwarded-Host header),
// the domain's own real route prefix, and a freshly issued token.
func remoteOverlayURL(canonicalOverlayOrigin string, domain remoteoverlay.Domain, token string) string {
	return canonicalOverlayOrigin + remoteOverlayRoutePrefix[domain] + token
}

// registerRemoteOverlayManagementRoutes wires the shared management
// surface for every domain's remote-capability lifecycle. Registered
// only when capabilities is non-nil (only when a remote overlay
// origin is actually configured - see router.go's own NewRouter),
// mirroring every other optional route group's nil-means-not-
// registered convention.
func registerRemoteOverlayManagementRoutes(
	mux *http.ServeMux, logger *slog.Logger,
	capabilities RemoteOverlayCapabilities, owners RemoteOverlayOwners, canonicalOverlayOrigin string,
) {
	mux.HandleFunc("GET /api/remote-overlay/{domain}/{slug}/status", handleRemoteOverlayStatus(logger, capabilities, owners, canonicalOverlayOrigin))
	mux.HandleFunc("/api/remote-overlay/{domain}/{slug}/status", methodNotAllowed(logger, http.MethodGet))

	mux.HandleFunc("POST /api/remote-overlay/{domain}/{slug}/enable", handleRemoteOverlayEnable(logger, capabilities, owners, canonicalOverlayOrigin))
	mux.HandleFunc("/api/remote-overlay/{domain}/{slug}/enable", methodNotAllowed(logger, http.MethodPost))

	mux.HandleFunc("POST /api/remote-overlay/{domain}/{slug}/rotate", handleRemoteOverlayRotate(logger, capabilities, owners, canonicalOverlayOrigin))
	mux.HandleFunc("/api/remote-overlay/{domain}/{slug}/rotate", methodNotAllowed(logger, http.MethodPost))

	mux.HandleFunc("POST /api/remote-overlay/{domain}/{slug}/disable", handleRemoteOverlayDisable(logger, capabilities, owners))
	mux.HandleFunc("/api/remote-overlay/{domain}/{slug}/disable", methodNotAllowed(logger, http.MethodPost))
}

// remoteOverlayStatusSchemaVersion is the current shape of GET
// /api/remote-overlay/{domain}/{slug}/status.
const remoteOverlayStatusSchemaVersion = 1

type remoteOverlayStatusResponse struct {
	Version   int  `json:"version"`
	Available bool `json:"available"`
	// Enabled reports whether a capability currently exists - never
	// the token itself, and never any historical/revoked token value.
	Enabled bool `json:"enabled"`
}

// remoteOverlayURLResponse carries the freshly issued/current remote
// Browser Source URL - security-sensitive capability information,
// never cached.
type remoteOverlayURLResponse struct {
	Version int    `json:"version"`
	URL     string `json:"url"`
}

// parseRemoteOverlayDomain validates the {domain} path parameter
// against the closed set remoteoverlay.ValidDomains - never trusts an
// arbitrary caller-supplied string past this point.
func parseRemoteOverlayDomain(r *http.Request) (remoteoverlay.Domain, bool) {
	d := remoteoverlay.Domain(r.PathValue("domain"))
	return d, d.IsValid()
}

func handleRemoteOverlayStatus(logger *slog.Logger, capabilities RemoteOverlayCapabilities, owners RemoteOverlayOwners, canonicalOverlayOrigin string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		domain, ok := parseRemoteOverlayDomain(r)
		if !ok {
			writeError(w, logger, http.StatusBadRequest, "remote_overlay_domain_invalid", "This overlay domain is not recognized.")
			return
		}
		slug := r.PathValue("slug")
		if !owners.exists(r.Context(), domain, slug) {
			writeError(w, logger, http.StatusNotFound, "remote_overlay_profile_not_found", "This overlay profile does not exist.")
			return
		}

		_, enabled, err := capabilities.Get(r.Context(), domain, slug)
		if err != nil {
			writeRemoteOverlayManagementError(w, logger, r, err)
			return
		}

		writeJSON(w, logger, http.StatusOK, remoteOverlayStatusResponse{
			Version:   remoteOverlayStatusSchemaVersion,
			Available: canonicalOverlayOrigin != "",
			Enabled:   enabled,
		})
	}
}

func handleRemoteOverlayEnable(logger *slog.Logger, capabilities RemoteOverlayCapabilities, owners RemoteOverlayOwners, canonicalOverlayOrigin string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		issueRemoteOverlayCapability(w, r, logger, capabilities, owners, canonicalOverlayOrigin)
	}
}

func handleRemoteOverlayRotate(logger *slog.Logger, capabilities RemoteOverlayCapabilities, owners RemoteOverlayOwners, canonicalOverlayOrigin string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Issue() itself already replaces any previous token
		// atomically (internal/domain/remoteoverlay's own documented
		// contract) - enable and rotate share this one code path, the
		// same "generate-and-apply" convention
		// internal/remoteingest.Manager already established for the
		// ingest credential's own provision/rotate pair.
		issueRemoteOverlayCapability(w, r, logger, capabilities, owners, canonicalOverlayOrigin)
	}
}

func issueRemoteOverlayCapability(w http.ResponseWriter, r *http.Request, logger *slog.Logger, capabilities RemoteOverlayCapabilities, owners RemoteOverlayOwners, canonicalOverlayOrigin string) {
	if !requireEmptyBody(w, r, logger) {
		return
	}
	if canonicalOverlayOrigin == "" {
		writeError(w, logger, http.StatusConflict, "remote_overlay_not_configured", "No remote overlay origin is configured for this deployment.")
		return
	}
	domain, ok := parseRemoteOverlayDomain(r)
	if !ok {
		writeError(w, logger, http.StatusBadRequest, "remote_overlay_domain_invalid", "This overlay domain is not recognized.")
		return
	}
	slug := r.PathValue("slug")
	if !owners.exists(r.Context(), domain, slug) {
		writeError(w, logger, http.StatusNotFound, "remote_overlay_profile_not_found", "This overlay profile does not exist.")
		return
	}

	cap, err := capabilities.Issue(r.Context(), domain, slug)
	if err != nil {
		writeRemoteOverlayManagementError(w, logger, r, err)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, logger, http.StatusOK, remoteOverlayURLResponse{
		Version: remoteOverlayStatusSchemaVersion,
		URL:     remoteOverlayURL(canonicalOverlayOrigin, domain, cap.Token),
	})
}

func handleRemoteOverlayDisable(logger *slog.Logger, capabilities RemoteOverlayCapabilities, owners RemoteOverlayOwners) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}
		domain, ok := parseRemoteOverlayDomain(r)
		if !ok {
			writeError(w, logger, http.StatusBadRequest, "remote_overlay_domain_invalid", "This overlay domain is not recognized.")
			return
		}
		slug := r.PathValue("slug")
		if !owners.exists(r.Context(), domain, slug) {
			writeError(w, logger, http.StatusNotFound, "remote_overlay_profile_not_found", "This overlay profile does not exist.")
			return
		}

		if err := capabilities.Revoke(r.Context(), domain, slug); err != nil {
			writeRemoteOverlayManagementError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, map[string]string{"status": "disabled"})
	}
}

func writeRemoteOverlayManagementError(w http.ResponseWriter, logger *slog.Logger, r *http.Request, err error) {
	switch {
	case errors.Is(err, remoteoverlay.ErrInvalidDomain):
		writeError(w, logger, http.StatusBadRequest, "remote_overlay_domain_invalid", "This overlay domain is not recognized.")
	default:
		logger.Error("unhandled remote overlay management error",
			slog.String("path", r.URL.Path), slog.Any("error", err))
		writeError(w, logger, http.StatusInternalServerError, "internal_error", "The server encountered an unexpected error.")
	}
}
