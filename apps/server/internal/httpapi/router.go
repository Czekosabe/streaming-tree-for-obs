// Package httpapi wires the REST API: routes, middleware and JSON responses.
//
// Route handlers stay thin. Domain logic (stream routing, platform metadata,
// process supervision) will live in dedicated packages and be injected here.
package httpapi

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/streaming-tree/server/internal/diagnostics"
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
	// Onboarding serves the Stage 21 first-run onboarding-state API. When
	// nil, GET/PUT /api/onboarding is not registered.
	Onboarding OnboardingService
	// MetadataPresets serves the Stage 22 reusable metadata-preset CRUD
	// API. When nil, none of the /api/metadata-presets routes are
	// registered.
	MetadataPresets MetadataPresetService
	// Backup serves the Stage 23 configuration backup API
	// (docs/backup-restore.md). When nil, none of the /api/backup
	// routes are registered.
	Backup BackupService
	// StreamSessions serves the Stage 24 stream session / operational
	// history API (docs/stream-session-history.md). When nil, none of
	// the /api/stream-sessions routes are registered.
	StreamSessions StreamSessionService
	// StreamSetups serves the Stage 25 stream setup profile CRUD/
	// duplicate/save-current/preview/apply API (docs/stream-setup-
	// profiles.md). When nil, none of the /api/stream-setups routes
	// are registered.
	StreamSetups StreamSetupService
	// Resources serves the local host-resource snapshot (CPU/memory/disk)
	// for the Dashboard's "System resources" card. When nil,
	// GET /api/system/resources is not registered.
	Resources ResourcesService
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
	// Accounts serves the connected-account API. When nil, none of the
	// connected-account, device-flow, integration-config or account-link
	// routes are registered.
	Accounts AccountService
	// DeviceFlow serves OAuth device-authorization attempts. Required
	// alongside Accounts for those routes to register.
	DeviceFlow DeviceFlowService
	// TwitchMetadata serves Twitch category search and metadata
	// publish/preview. Required alongside Accounts for those routes to
	// register.
	TwitchMetadata TwitchMetadataService
	// YouTubeAuth serves YouTube OAuth attempts (Authorization Code + PKCE
	// + loopback callback). When nil, none of the YouTube integration or
	// OAuth-attempt routes are registered, and reconnecting a YouTube
	// account falls back to being unavailable.
	YouTubeAuth YouTubeAuthService
	// YouTubeMetadata serves YouTube broadcast listing, category listing,
	// region resolution, and metadata publish/preview. Required alongside
	// YouTubeAuth for the YouTube-specific routes to register; also used by
	// the shared publish-preview/publish endpoints when a destination's
	// provider is YouTube.
	YouTubeMetadata YouTubeMetadataService
	// RemoteTargets serves a YouTube destination's selected live-broadcast
	// target. Required alongside YouTubeAuth/YouTubeMetadata.
	RemoteTargets RemoteTargetService
	// EngagementBus serves the Event Bus snapshot/SSE API
	// (GET /api/engagement/*). When nil, none of the engagement routes are
	// registered.
	EngagementBus EngagementBusService
	// EngagementSettings serves a connected account's persisted
	// engagement-connector enable/disable preference. Required alongside
	// EngagementBus and EngagementConnectors.
	EngagementSettings EngagementSettingsService
	// EngagementConnectors serves the per-account Twitch EventSub connector
	// management API. Required alongside EngagementBus and Accounts/
	// DeviceFlow for the engagement routes to register.
	EngagementConnectors EngagementConnectorService
	// YouTubeEngagementConnectors serves the per-account YouTube Live Chat
	// gRPC streamList connector management API (Stage 15A) - the same
	// account-scoped routes EngagementConnectors already registers also
	// dispatch to this manager for a YouTube account. Required alongside
	// EngagementConnectors for the engagement routes to register.
	YouTubeEngagementConnectors YouTubeEngagementConnectorService
	// OperatorChatProjection serves the Stage 9 unified-operator-chat
	// status/snapshot/SSE API. Required alongside OperatorChatPrefs for the
	// operator-chat routes to register.
	OperatorChatProjection OperatorChatProjectionService
	// OperatorChatPrefs serves persisted operator-chat preferences,
	// per-account visibility, and the hidden-user/bot-user lists.
	OperatorChatPrefs OperatorChatPrefsService
	// OperatorChatAssets resolves Twitch chat badge image URLs at
	// serialization time. May be nil - items still serialize without
	// resolved badge images (see OperatorChatAssetResolver's own doc
	// comment).
	OperatorChatAssets OperatorChatAssetResolver
	// OnOperatorChatBotUsersChanged is called after a bot-user is marked
	// or unmarked, so the chat-overlay runtime (which shares this exact
	// list) can rebuild every running overlay. May be nil.
	OnOperatorChatBotUsersChanged func(ctx context.Context)
	// ChatOverlayProfiles serves the Stage 10 chat-overlay profile
	// management API (/api/chat-overlays/...). Required alongside
	// ChatOverlayRuntime and Accounts for the chat-overlay routes
	// (management and public) to register.
	ChatOverlayProfiles ChatOverlayProfileService
	// ChatOverlayRuntime serves the live per-overlay public projection
	// the management API rebuilds on every settings change and the
	// public API (/api/public/chat-overlays/...) reads from.
	ChatOverlayRuntime ChatOverlayRuntime
	// OutboundChat serves the Stage 11A manual outbound-chat API
	// (per-account status, permission upgrade, sending). Required
	// alongside Accounts and DeviceFlow for those routes to register.
	OutboundChat OutboundChatService
	// ChatAutomation serves the Stage 11B scheduled-message/chat-command
	// automation API (/api/chat-automation/...), built on top of the
	// same Stage 11A outbound dispatcher OutboundChat itself uses.
	// Required alongside Accounts for those routes to register.
	ChatAutomation ChatAutomationService
	// Alerts serves the Stage 12A alert management API
	// (/api/alert-profiles/..., /api/alert-rules/...) and the public
	// alert API (/api/public/alert-profiles/...). When nil, none of
	// those routes are registered.
	Alerts AlertsService
	// VisualTemplates serves the Stage 14A reusable visual-template
	// library (/api/visual-templates/...) - a management/editor
	// surface only, never exposed on any public API. When nil, none of
	// those routes are registered.
	VisualTemplates VisualTemplateService
	// VisualAssets serves the Stage 14B managed-asset management API
	// (/api/visual-assets/...) and the public asset-content route
	// (/api/public/visual-assets/{token}). When nil, none of those
	// routes are registered.
	VisualAssets VisualAssetService
	// VisualPackages serves the Stage 14B portable package import/
	// preview/export API. Required alongside VisualAssets and
	// VisualTemplates for those routes (including the package-export
	// extension of /api/visual-templates/{id}/export-package) to
	// register.
	VisualPackages VisualPackageService
	// DonationSources serves the Stage 16A external-donation-source
	// management API (/api/donation-sources/...): safe-metadata CRUD and
	// credential replacement. Required alongside DonationConnectors for
	// those routes to register.
	DonationSources DonationSourceService
	// DonationConnectors serves the per-source StreamElements Astro
	// WebSocket connector management API - the donation-source twin of
	// EngagementConnectors/YouTubeEngagementConnectors. Required alongside
	// DonationSources for the donation-source routes to register.
	DonationConnectors DonationEngagementConnectorService
	// Audio serves the Stage 17A TTS/audio management API
	// (/api/audio/...) and the public Browser Source audio output API
	// (/api/public/audio/{slug}/...). When nil, none of those routes are
	// registered.
	Audio AudioService
	// AudioAssets serves the Stage 17B managed persistent-audio-asset
	// management API (/api/audio-assets/...) - no public route of its
	// own; audio bytes are always served through Audio's own existing
	// public bytes-URL route (docs/alert-audio.md §5.2/§7). When nil,
	// none of those routes are registered.
	AudioAssets AudioAssetService
	// Goals serves the Stage 18A/18B goal and widget-profile management
	// API (/api/goals/..., /api/widget-profiles/...) and the public
	// widget API (/api/public/widgets/{slug}/...). When nil, none of
	// those routes are registered.
	Goals GoalsService
	// SupporterWidgets serves the Stage 18B runtime-only presentation
	// state (latest/largest/recent/ticker/counter) every non-goal,
	// non-dashboard widget kind needs (docs/supporter-widgets.md §4).
	// When nil, every such kind renders its own well-defined empty state
	// (docs/supporter-widgets.md §12) rather than failing - only a
	// goal widget is unaffected by this being nil.
	SupporterWidgets SupporterWidgetsRuntime
	// WebAssets is the embedded production frontend build
	// (webassets.Frontend()). When nil (every development/test build
	// today), the router keeps its existing tiny liveness JSON at "/" and
	// serves no static files at all - unchanged from every prior stage.
	// When set (packaged/release builds only), "/" and every other
	// non-/api/ path are served from it, with an SPA fallback to
	// index.html for client-side routes - see production.go.
	WebAssets fs.FS
	// LegalAssets is the embedded canonical legal documents
	// (webassets.Legal()). When nil, the fixed /legal/* routes are not
	// registered.
	LegalAssets fs.FS
	// Shutdown, when non-nil, is called by POST /api/system/shutdown to
	// begin the application's real graceful-shutdown sequence - the exact
	// same context.CancelFunc main.go already gets from
	// signal.NotifyContext, so the shutdown endpoint reuses the existing,
	// already-correct shutdown path rather than duplicating it. When nil
	// (every development/test build today), the route is not registered.
	Shutdown context.CancelFunc
	// Updater serves the Stage 20B application-updater API
	// (/api/updates/*, docs/updater.md §28). When nil (every
	// development/test build unless explicitly wired), those routes are
	// not registered.
	Updater UpdateService

	// RemoteManagement carries the Stage 20D2B remote-management
	// security surface (docs/remote-management.md). When
	// RemoteManagement.Enabled is false (every build unless explicitly
	// opted into --remote-management), no auth route is registered, no
	// security middleware runs, and every existing route behaves
	// exactly as it did before this stage - see
	// withRemoteManagementSecurity's own doc comment.
	RemoteManagement RemoteManagementOptions

	// RemoteOverlay carries the Stage 20D2C backend defense-in-depth
	// for the remote overlay origin (docs/remote-ingest.md §11). When
	// RemoteOverlay.Enabled is false (every build unless an overlay
	// origin is explicitly configured), the middleware is a no-op -
	// see withRemoteOverlaySecurity's own doc comment.
	RemoteOverlay RemoteOverlayOptions

	// RemoteOverlayResolver resolves a forwarded overlay request's
	// {slug} path parameter to the real local slug (docs/remote-
	// ingest.md §12). When nil (every build unless remote overlay
	// exposure is explicitly wired), resolvePublicSlug's own nil check
	// means a forwarded overlay request simply cannot resolve any
	// token - not a panic, and not a silent fall-through to treating
	// the token as a local slug.
	RemoteOverlayResolver RemoteOverlayResolver

	// RemoteOverlayCapabilities serves the authenticated management
	// API for issuing/rotating/revoking a profile's remote capability
	// (/api/remote-overlay/{domain}/{slug}/*). When nil, those routes
	// are not registered - the same nil-means-not-registered
	// convention as every other optional route group.
	RemoteOverlayCapabilities RemoteOverlayCapabilities
	// RemoteOverlayOwners validates that a {domain}/{slug} pair names
	// a real, existing profile before the management API issues or
	// checks a capability for it - required whenever
	// RemoteOverlayCapabilities is non-nil.
	RemoteOverlayOwners RemoteOverlayOwners
	// RemoteOverlayCanonicalOrigin is the validated
	// "scheme://host[:port]" overlay origin the management API embeds
	// into every remote Browser Source URL it returns - never a value
	// derived from an incoming request.
	RemoteOverlayCanonicalOrigin string

	// RemoteIngest serves the Stage 20D2C remote-ingest credential-
	// management API (/api/remote-ingest/*, docs/remote-ingest.md §8).
	// When nil (every build unless --remote-ingest is active), those
	// routes are not registered - the same nil-means-not-registered
	// convention as Updater/Runtime/every other optional service.
	RemoteIngest RemoteIngestService
	// RemoteIngestRTMPSAddress / RemoteIngestPath are the static,
	// already-validated deployment facts GET /api/remote-ingest/status
	// reports. Empty unless RemoteIngest is non-nil.
	RemoteIngestRTMPSAddress string
	RemoteIngestPath         string

	// Diagnostics serves the Stage 20E diagnostics API
	// (GET /api/logs, POST /api/diagnostics/support-bundle,
	// docs/final-hardening.md §A/§E). When nil, those routes are not
	// registered - the same nil-means-not-registered convention as
	// every other optional service. Never registered under
	// /api/public/*: local desktop use relies on loopback-only
	// exposure exactly like every other /api/ route, and a remote-
	// management deployment gates it through the same
	// withRemoteManagementSecurity middleware as everything else under
	// /api/.
	Diagnostics *diagnostics.Recorder
	// DiagnosticsBundle builds the Stage 20E support bundle
	// (docs/final-hardening.md §C). Required alongside Diagnostics for
	// POST /api/diagnostics/support-bundle to register.
	DiagnosticsBundle SupportBundleBuilder
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

	// Product-identity/build metadata for the About & Legal UI. Like health,
	// it needs no service dependency, so it is always registered.
	mux.HandleFunc("GET /api/about", aboutHandler(logger))
	mux.HandleFunc("/api/about", methodNotAllowed(logger, http.MethodGet))

	if opts.Platforms != nil {
		registerPlatformRoutes(mux, logger, opts.Platforms, opts.Credentials, opts.Outputs, opts.Branches)
	}

	if opts.Runtime != nil {
		registerRuntimeRoutes(mux, logger, opts.Runtime)
	}

	if opts.Onboarding != nil {
		registerOnboardingRoutes(mux, logger, opts.Onboarding)
	}

	if opts.MetadataPresets != nil {
		registerMetadataPresetRoutes(mux, logger, opts.MetadataPresets)
	}

	if opts.Backup != nil {
		registerBackupRoutes(mux, logger, opts.Backup)
	}

	if opts.StreamSessions != nil {
		registerStreamSessionRoutes(mux, logger, opts.StreamSessions)
	}

	if opts.StreamSetups != nil {
		registerStreamSetupRoutes(mux, logger, opts.StreamSetups)
	}

	// Local host-resource snapshot for the Dashboard's "System resources"
	// card (Stage 20E). When nil (health-only test servers), the route is
	// simply not registered, matching every other optional service here.
	if opts.Resources != nil {
		mux.HandleFunc("GET /api/system/resources", handleGetSystemResources(logger, opts.Resources))
		mux.HandleFunc("/api/system/resources", methodNotAllowed(logger, http.MethodGet))
	}

	if opts.RemoteIngest != nil {
		registerRemoteIngestRoutes(mux, logger, opts.RemoteIngest, opts.RemoteIngestRTMPSAddress, opts.RemoteIngestPath)
	}

	if opts.RemoteOverlayCapabilities != nil {
		registerRemoteOverlayManagementRoutes(mux, logger, opts.RemoteOverlayCapabilities, opts.RemoteOverlayOwners, opts.RemoteOverlayCanonicalOrigin)
	}

	if opts.FFmpegRuntime != nil {
		mux.HandleFunc("GET /api/runtime/ffmpeg", handleGetFFmpegStatus(logger, opts.FFmpegRuntime))
		mux.HandleFunc("/api/runtime/ffmpeg", methodNotAllowed(logger, http.MethodGet))
	}

	if opts.Diagnostics != nil {
		registerDiagnosticsRoutes(mux, logger, opts.Diagnostics, opts.DiagnosticsBundle)
	}

	if opts.Branches != nil {
		registerBranchRoutes(mux, logger, opts.Branches)
	}

	if opts.Accounts != nil && opts.DeviceFlow != nil && opts.TwitchMetadata != nil {
		registerAccountRoutes(mux, logger, opts.Platforms, opts.Accounts, opts.DeviceFlow, opts.TwitchMetadata,
			opts.YouTubeAuth, opts.YouTubeMetadata, opts.RemoteTargets)
	}

	if opts.Accounts != nil && opts.YouTubeAuth != nil && opts.YouTubeMetadata != nil && opts.RemoteTargets != nil {
		registerYouTubeRoutes(mux, logger, opts.Platforms, opts.Accounts, opts.YouTubeAuth, opts.YouTubeMetadata, opts.RemoteTargets)
	}

	if opts.EngagementBus != nil && opts.Accounts != nil && opts.DeviceFlow != nil &&
		opts.EngagementSettings != nil && opts.EngagementConnectors != nil && opts.YouTubeEngagementConnectors != nil {
		registerEngagementRoutes(mux, logger, opts.Accounts, opts.DeviceFlow, opts.EngagementBus, opts.EngagementSettings, opts.EngagementConnectors, opts.YouTubeEngagementConnectors)
	}

	if opts.OperatorChatProjection != nil && opts.OperatorChatPrefs != nil && opts.Accounts != nil {
		registerOperatorChatRoutes(mux, logger, opts.Accounts, opts.OperatorChatProjection, opts.OperatorChatPrefs, opts.OperatorChatAssets, opts.OnOperatorChatBotUsersChanged)
	}

	if opts.ChatOverlayProfiles != nil && opts.ChatOverlayRuntime != nil && opts.Accounts != nil {
		registerChatOverlayRoutes(mux, logger, opts.Accounts, opts.ChatOverlayProfiles, opts.ChatOverlayRuntime, opts.OperatorChatAssets, opts.VisualAssets, opts.RemoteOverlayResolver)
	}

	if opts.Accounts != nil && opts.DeviceFlow != nil && opts.OutboundChat != nil {
		registerOutboundChatRoutes(mux, logger, opts.Accounts, opts.DeviceFlow, opts.OutboundChat)
	}

	if opts.Accounts != nil && opts.ChatAutomation != nil {
		registerChatAutomationRoutes(mux, logger, opts.ChatAutomation)
	}

	if opts.Alerts != nil {
		registerAlertRoutes(mux, logger, opts.Alerts, opts.VisualAssets, opts.RemoteOverlayResolver)
		registerVisualDesignRoutes(mux, logger, opts.Alerts)
	}

	if opts.VisualTemplates != nil {
		registerVisualTemplateRoutes(mux, logger, opts.VisualTemplates, opts.Alerts, opts.ChatOverlayProfiles, opts.VisualPackages)
	}

	if opts.VisualAssets != nil {
		registerVisualAssetRoutes(mux, logger, opts.VisualAssets)
	}

	if opts.VisualPackages != nil && opts.VisualAssets != nil {
		registerVisualPackageRoutes(mux, logger, opts.VisualPackages, opts.VisualAssets)
	}

	if opts.DonationSources != nil && opts.DonationConnectors != nil {
		registerDonationSourceRoutes(mux, logger, opts.DonationSources, opts.DonationConnectors)
	}

	if opts.Audio != nil {
		registerAudioRoutes(mux, logger, opts.Audio, opts.RemoteOverlayResolver)
	}

	if opts.AudioAssets != nil {
		registerAudioAssetRoutes(mux, logger, opts.AudioAssets)
	}

	if opts.Goals != nil {
		registerGoalRoutes(mux, logger, opts.Goals, opts.SupporterWidgets)
		registerPublicWidgetRoutes(mux, logger, opts.Goals, opts.SupporterWidgets, opts.RemoteOverlayResolver)
	}

	// localActionOrigins is opts.AllowedOrigins (the local dev-server
	// allowlist) plus the configured remote-management external origin
	// when enabled - a real gap found and fixed during this milestone's
	// own native CI verification: POST /api/system/shutdown and POST
	// /api/updates/install each carry their own pre-existing
	// checkLocalActionOrigin check (docs/windows-packaging.md §8,
	// predating Stage 20D2B), entirely independent of
	// withRemoteManagementSecurity's own (correctly passing) Origin
	// check - without this, a legitimate remote-management request
	// with the exact right session/CSRF/Origin still failed at this
	// second, older check, since the remote external origin was never
	// added to the allowlist it validates against.
	localActionOrigins := opts.AllowedOrigins
	if opts.RemoteManagement.Enabled && opts.RemoteManagement.ExternalOrigin != "" {
		localActionOrigins = append(append([]string{}, opts.AllowedOrigins...), opts.RemoteManagement.ExternalOrigin)
	}

	if opts.Shutdown != nil {
		registerShutdownRoute(mux, logger, opts.Shutdown, localActionOrigins)
	}

	if opts.Updater != nil {
		registerUpdaterRoutes(mux, logger, opts.Updater, localActionOrigins)
	}

	if opts.LegalAssets != nil {
		registerLegalRoutes(mux, logger, opts.LegalAssets)
	}

	if opts.RemoteManagement.Enabled {
		registerAuthRoutes(mux, logger, opts.RemoteManagement)
	}

	// Anything else under /api is an explicit, JSON-shaped 404 rather than the
	// default plain-text response, so the frontend can parse every failure.
	// Registered before the production/liveness routes below so a real /api/
	// path can never fall through to either of them.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, logger, http.StatusNotFound,
			"not_found", "This API endpoint does not exist.")
	})

	if opts.WebAssets != nil {
		// Packaged/release mode: the embedded production frontend owns "/"
		// and every other non-/api/ path (see production.go) - the tiny
		// liveness JSON below is redundant once the real application is
		// being served there.
		registerProductionRoutes(mux, logger, opts.WebAssets)
	} else {
		// Development/test mode (every build today unless WebAssets was
		// explicitly set): the root stays a tiny liveness hint for someone
		// opening the port in a browser, exactly as before this stage.
		mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, logger, http.StatusOK, map[string]string{
				"service": "streaming-tree-server",
				"health":  "/api/health",
			})
		})
	}

	return chain(mux,
		withRecovery(logger),
		withLogging(logger),
		withCORS(opts.AllowedOrigins),
		withManagementSecurityHeaders(opts.RemoteManagement),
		withRemoteManagementSecurity(logger, opts.RemoteManagement),
		withRemoteOverlaySecurity(logger, opts.RemoteOverlay),
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
