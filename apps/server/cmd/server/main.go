// Command server starts the Streaming Tree REST API: platform configuration,
// MediaMTX supervision, the destination-credential store, destination
// branch supervision, and the connected-account/Twitch/YouTube integrations
// are all wired here. Kick and TikTok account integrations and the
// engagement platform are still separate, later stages.
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/streaming-tree/server/internal/alerts"
	audiort "github.com/streaming-tree/server/internal/audio"
	"github.com/streaming-tree/server/internal/auth"
	"github.com/streaming-tree/server/internal/buildinfo"
	"github.com/streaming-tree/server/internal/chatautomation"
	co "github.com/streaming-tree/server/internal/chatoverlay"
	"github.com/streaming-tree/server/internal/config"
	"github.com/streaming-tree/server/internal/diagnostics"
	"github.com/streaming-tree/server/internal/domain/account"
	audiodomain "github.com/streaming-tree/server/internal/domain/audio"
	"github.com/streaming-tree/server/internal/domain/audioasset"
	backupdomain "github.com/streaming-tree/server/internal/domain/backup"
	chatoverlaydomain "github.com/streaming-tree/server/internal/domain/chatoverlay"
	"github.com/streaming-tree/server/internal/domain/credential"
	"github.com/streaming-tree/server/internal/domain/donationsource"
	"github.com/streaming-tree/server/internal/domain/engagementsettings"
	domaingoals "github.com/streaming-tree/server/internal/domain/goals"
	"github.com/streaming-tree/server/internal/domain/metadatapreset"
	"github.com/streaming-tree/server/internal/domain/onboarding"
	"github.com/streaming-tree/server/internal/domain/operatorchatprefs"
	"github.com/streaming-tree/server/internal/domain/output"
	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/domain/remotetarget"
	"github.com/streaming-tree/server/internal/domain/updatersettings"
	"github.com/streaming-tree/server/internal/domain/visualasset"
	"github.com/streaming-tree/server/internal/domain/visualpackage"
	"github.com/streaming-tree/server/internal/domain/visualtemplate"
	bus "github.com/streaming-tree/server/internal/engagement"
	goalsrt "github.com/streaming-tree/server/internal/goals"
	"github.com/streaming-tree/server/internal/httpapi"
	oc "github.com/streaming-tree/server/internal/operatorchat"
	"github.com/streaming-tree/server/internal/outboundchat"
	"github.com/streaming-tree/server/internal/provider/streamelements"
	"github.com/streaming-tree/server/internal/provider/tts"
	"github.com/streaming-tree/server/internal/provider/twitch"
	"github.com/streaming-tree/server/internal/provider/twitch/chatassets"
	"github.com/streaming-tree/server/internal/provider/youtube"
	"github.com/streaming-tree/server/internal/remoteingest"
	"github.com/streaming-tree/server/internal/runtime/branch"
	"github.com/streaming-tree/server/internal/runtime/browserlaunch"
	"github.com/streaming-tree/server/internal/runtime/deviceflow"
	"github.com/streaming-tree/server/internal/runtime/ffmpeg"
	"github.com/streaming-tree/server/internal/runtime/mediamtx"
	"github.com/streaming-tree/server/internal/runtime/nativealert"
	"github.com/streaming-tree/server/internal/runtime/singleinstance"
	"github.com/streaming-tree/server/internal/runtime/streamelementsengagement"
	"github.com/streaming-tree/server/internal/runtime/tray"
	"github.com/streaming-tree/server/internal/runtime/twitchengagement"
	"github.com/streaming-tree/server/internal/runtime/youtubeauth"
	"github.com/streaming-tree/server/internal/runtime/youtubeengagement"
	"github.com/streaming-tree/server/internal/secrets"
	"github.com/streaming-tree/server/internal/storage/sqlite"
	"github.com/streaming-tree/server/internal/support"
	supporterwidgetsrt "github.com/streaming-tree/server/internal/supporterwidgets"
	"github.com/streaming-tree/server/internal/sysresources"
	"github.com/streaming-tree/server/internal/updater"
	"github.com/streaming-tree/server/internal/updater/manifest"
	"github.com/streaming-tree/server/internal/userdatapurge"
	"github.com/streaming-tree/server/internal/webassets"
)

// headlessMode is set once, from the real command-line flag, inside
// handleEarlyFlags - never inferred from runtime.GOOS, a missing
// DISPLAY, or being launched by systemd (docs/linux-headless-server.md
// §5). Read only after handleEarlyFlags has run.
var headlessMode bool

// remoteManagementFlag is set once, from the real command-line flag,
// inside handleEarlyFlags - never inferred from --headless alone
// (docs/remote-management.md §3). Read only after handleEarlyFlags has
// run.
var remoteManagementFlag bool

// remoteIngestFlag is set once, from the real command-line flag,
// inside handleEarlyFlags - never inferred from --headless/
// --remote-management alone (docs/remote-ingest.md §3). Read only
// after handleEarlyFlags has run. Mirrors remoteManagementFlag's own
// naming/lifecycle exactly for interface consistency with the
// established --headless/--remote-management convention; the actual
// enablement gate run() checks is cfg.RemoteIngest.Enabled
// (STREAMING_TREE_REMOTE_INGEST), the same env-var-driven mechanism
// every other RemoteManagement/RemoteIngest setting already uses.
var remoteIngestFlag bool

// remoteIngestPublisherUser is the fixed, non-secret remote-publisher
// service identity (docs/remote-ingest.md §5) - not a secret, unlike
// the per-deployment generated password/verifier.
const remoteIngestPublisherUser = "streaming-tree-obs"

func main() {
	if handled := handleEarlyFlags(); handled {
		return
	}

	if err := run(); err != nil {
		slog.Error("server terminated with an error", slog.Any("error", err))
		if headlessMode {
			// docs/linux-headless-server.md §5/§23: a headless fatal
			// startup error is a structured log line (already written
			// above, captured by journald under the real systemd unit)
			// plus this nonzero exit - never zenity/kdialog/a desktop
			// alert of any kind, and no dependency on DISPLAY.
		} else if buildinfo.Packaged() {
			// The release binary has no console window (docs/windows-
			// packaging.md §7/§13) - a fatal startup error must not simply
			// disappear. err's own text follows this codebase's existing
			// convention of never including a secret/token/credential.
			nativealert.ShowFatalError(buildinfo.ProductName, err.Error())
		}
		os.Exit(1)
	}
}

// handleEarlyFlags parses every command-line flag exactly once and
// handles the two modes that must run before any normal application
// startup: `--version` (unchanged since Stage 20A) and Stage 20B's
// internal `-update-helper` mode (docs/updater.md §22) - detected here,
// before SQLite/MediaMTX/providers/the HTTP server/the single-instance
// mutex/TTS are ever touched, exactly like `--version` already is. Also
// captures `--headless` (docs/linux-headless-server.md §5) into the
// package-level headlessMode for run() to read afterward.
// Returns true when either mode fully handled the process (main should
// simply return), false when normal startup should proceed.
func handleEarlyFlags() bool {
	versionFlag := flag.Bool("version", false, "print version information and exit")
	headlessFlag := flag.Bool("headless", false, "run without a browser/desktop UI, as an unattended Linux service (docs/linux-headless-server.md)")
	remoteManagementCLIFlag := flag.Bool("remote-management", false, "enable the Stage 20D2B remote management/control plane - requires --headless (docs/remote-management.md)")
	remoteIngestCLIFlag := flag.Bool("remote-ingest", false, "enable the Stage 20D2C authenticated/encrypted remote OBS ingest plane - requires --headless and --remote-management (docs/remote-ingest.md)")
	updateHelperFlag := flag.Bool(updater.FlagUpdateHelper, false, "internal: run in update-helper mode")
	parentPID := flag.Int(updater.FlagParentPID, 0, "internal: update-helper only")
	candidate := flag.String(updater.FlagCandidate, "", "internal: update-helper only")
	targetExe := flag.String(updater.FlagTargetExe, "", "internal: update-helper only")
	expectedVersion := flag.String(updater.FlagExpectedVersion, "", "internal: update-helper only")
	provisionAdminPasswordFlag := flag.Bool("provision-admin-password", false,
		"local-only: read a new administrator password from stdin and store its verifier, then exit (docs/remote-management.md §9.2)")
	forceProvision := flag.Bool("force", false, "with -provision-admin-password: overwrite an existing verifier")
	purgeUserDataFlag := flag.Bool("purge-user-data", false,
		"internal: invoked only by the Windows uninstaller's explicit opt-in checkbox - permanently deletes the database, managed assets, and every stored credential, then exits; refuses to run while the application is still running (docs/windows-packaging.md §26)")
	flag.Parse()
	headlessMode = *headlessFlag
	remoteManagementFlag = *remoteManagementCLIFlag
	remoteIngestFlag = *remoteIngestCLIFlag

	if *provisionAdminPasswordFlag {
		runProvisionAdminPassword(*forceProvision)
		return true
	}

	if *purgeUserDataFlag {
		runPurgeUserData()
		return true
	}

	if *updateHelperFlag {
		runUpdateHelper(*parentPID, *candidate, *targetExe, *expectedVersion)
		return true
	}

	if !*versionFlag {
		return false
	}

	fmt.Printf("%s %s\n", buildinfo.ProductName, buildinfo.EffectiveVersion())
	if commit, _, ok := buildinfo.CommitInfo(); ok {
		fmt.Printf("commit %s\n", commit)
	}
	fmt.Printf("licence %s\n", buildinfo.ApplicationLicenseSPDX)
	return true
}

// runUpdateHelper validates the closed argument set and runs the real
// handoff (docs/updater.md §21/§22) - a strict, self-contained mode
// that starts no normal application service. Failure here has no
// operator-facing UI of its own (the original application, if it is
// still running, already reported the error state that would have led
// here); a bounded stderr line is the only diagnostic surface.
func runUpdateHelper(parentPID int, candidate, targetExe, expectedVersion string) {
	args, err := updater.ValidateHelperArgs(parentPID, candidate, targetExe, expectedVersion)
	if err != nil {
		fmt.Fprintf(os.Stderr, "update-helper: %v\n", err)
		os.Exit(1)
	}
	result := updater.RunHelper(args)
	if result.Outcome != updater.OutcomeOK {
		os.Exit(1)
	}
}

// runProvisionAdminPassword implements `--provision-admin-password`
// (docs/remote-management.md §9.2) - a local-only mode, never reachable
// through any HTTP route. It reuses the exact same
// secrets.LoadHeadlessMasterKey/secrets.NewHeadlessStore construction
// run() itself uses, so it requires the same $CREDENTIALS_DIRECTORY
// input a real systemd unit invocation provides - operators run this
// via scripts/provision-admin-password.sh, which wraps it in a
// `systemd-run` invocation carrying the shipped unit's own
// LoadCredential=/DynamicUser=/StateDirectory=/Environment= properties,
// rather than inventing a second, parallel identity/state path. The
// password is read from stdin only - never a command-line argument,
// never an environment variable - and is hashed immediately; nothing
// derived from it is ever written to a temporary file.
func runProvisionAdminPassword(force bool) {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "provision-admin-password: %v\n", err)
		os.Exit(1)
	}

	masterKey, err := secrets.LoadHeadlessMasterKey()
	if err != nil {
		fmt.Fprintf(os.Stderr, "provision-admin-password: %v\n", err)
		os.Exit(1)
	}
	store, err := secrets.NewHeadlessStore(filepath.Join(cfg.DataDir, "secrets.json"), masterKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "provision-admin-password: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	if !force {
		provisioned, err := auth.AdminPasswordProvisioned(ctx, store)
		if err != nil {
			fmt.Fprintf(os.Stderr, "provision-admin-password: %v\n", err)
			os.Exit(1)
		}
		if provisioned {
			fmt.Fprintln(os.Stderr,
				"provision-admin-password: an administrator password is already provisioned - pass -force to deliberately overwrite it (this invalidates every active remote-management session)")
			os.Exit(1)
		}
	}

	password, err := readProvisioningPassword()
	if err != nil {
		fmt.Fprintf(os.Stderr, "provision-admin-password: %v\n", err)
		os.Exit(1)
	}

	if err := auth.SetAdminPassword(ctx, store, password); err != nil {
		fmt.Fprintf(os.Stderr, "provision-admin-password: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintln(os.Stderr, "Administrator password provisioned.")
}

// runPurgeUserData implements `-purge-user-data` (docs/windows-
// packaging.md §26): a narrowly-scoped mode invoked only by the
// Windows uninstaller's own [UninstallRun] entry, itself gated on the
// operator's explicit, unchecked-by-default "also remove all Streaming
// Tree settings, local data, and saved credentials" checkbox
// (scripts/installer/streaming-tree.iss). The real deletion logic
// lives in internal/userdatapurge, where it is unit-testable; this is
// only the thin CLI wrapper, following the same pattern as
// runUpdateHelper above.
//
// This refuses to run at all while another instance holds the single-
// instance mutex (docs/windows-packaging.md §9's own mechanism,
// reused unchanged): deleting a SQLite database or a credential-store
// entry a live process might still have open is never attempted, per
// the operator's own explicit "purge must require the application to
// be stopped" requirement. The Inno Setup script's own cooperative-
// shutdown request (see PrepareToInstall/InitializeUninstall in the
// .iss) is what makes that precondition actually true in the normal
// case before this ever runs.
func runPurgeUserData() {
	acquired, release, err := singleinstance.Acquire()
	if err != nil {
		fmt.Fprintf(os.Stderr, "purge-user-data: %v\n", err)
		os.Exit(1)
	}
	if !acquired {
		fmt.Fprintln(os.Stderr,
			"purge-user-data: Streaming Tree for OBS is still running - stop it before removing its data")
		os.Exit(1)
	}
	// Held only long enough to prove no other instance is running -
	// released immediately so a normal launch right after this command
	// finishes is never blocked by it.
	release()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "purge-user-data: %v\n", err)
		os.Exit(1)
	}

	if err := userdatapurge.Purge(context.Background(), cfg.DataDir, cfg.DatabasePath, secrets.NewKeyringStore()); err != nil {
		fmt.Fprintf(os.Stderr, "purge-user-data: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintln(os.Stderr, "Streaming Tree data removed.")
}

// newUpdaterClient builds the updater's GitHub API client. The default
// here - the only one a normal `go build ./cmd/server` (no tags) can
// ever produce - always talks to the real, canonical GitHub host
// (docs/updater.md §1/§15). A companion file gated behind the
// `integration` build tag (updater_testhook_integration.go) may
// override this variable at init time; that file, and the
// updater.NewTestClient symbol it depends on, do not exist at all
// unless the whole binary was built with `-tags integration` - see
// that file's own doc comment for the full reasoning.
var newUpdaterClient = func(installedVersion string) *updater.Client {
	return updater.NewClient(installedVersion)
}

// trayIngestStatusLabel maps a runtime snapshot onto the tray's own
// concise status item (docs/windows-tray.md) - the same underlying
// truth internal/httpapi's /api/runtime response and the web
// dashboard's SystemStatusPill already read, just condensed into one
// short native-menu line instead of a translated web component.
func trayIngestStatusLabel(snapshot mediamtx.Snapshot) string {
	if snapshot.MediaMTX.State == mediamtx.StateMissing {
		return "Ingest: Not installed"
	}
	switch snapshot.Ingest.State {
	case mediamtx.IngestReceiving:
		return "Ingest: Receiving"
	case mediamtx.IngestWaiting:
		return "Ingest: Waiting"
	case mediamtx.IngestError:
		return "Ingest: Error"
	default:
		return "Ingest: Not ready"
	}
}

// trayUpdatesLabel reports the tray's "Check for updates" item text and
// whether it should be enabled - disabled (grayed, per
// docs/windows-tray.md) for exactly the three permanent, non-
// actionable updater states (docs/updater.md §11/§35/§43), so the
// tray never offers an action the updater would just refuse.
func trayUpdatesLabel(state updater.State) (string, bool) {
	switch state {
	case updater.StateDisabled, updater.StateManualBuild, updater.StatePlatformUnsupported:
		return "Check for updates", false
	default:
		return "Check for updates", true
	}
}

// run holds the real main so that every exit path can return an error and still
// let deferred cleanup happen (os.Exit in main would skip it).
func run() error {
	// Stage 20E (docs/final-hardening.md §A): the real stdout handler
	// stays exactly as before - headless/journald output is
	// byte-for-byte unaffected - but every record additionally lands,
	// redacted, in a bounded in-memory ring buffer that backs the new
	// GET /api/logs API and the support bundle below. This is the one
	// seam; nothing else about logging changes.
	diagnosticsRecorder := diagnostics.NewRecorder()
	logger := slog.New(diagnostics.NewHandler(
		slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}),
		diagnosticsRecorder,
	))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// docs/linux-headless-server.md §6: the one real gap in the
	// existing address validation - MediaMTX's own two addresses are
	// already unconditionally loopback-validated by config.Load()
	// regardless of mode, but the management HTTP listener has no such
	// restriction by default. Headless mode fails closed here, before
	// any listener of any kind is created, rather than silently
	// reinterpreting a requested non-loopback bind as loopback.
	if headlessMode {
		if err := config.ValidateHeadlessListenAddress(cfg); err != nil {
			return err
		}
	}

	// docs/remote-management.md §3/§5: remote management is Linux
	// headless deployment functionality only - never inferred from
	// --headless alone, and refused outright if requested without it.
	// Every other precondition (a valid HTTPS origin, a provisioned
	// administrator credential) is checked further below, once the
	// secret store this depends on has been constructed - a headless
	// service whose remote-management preconditions are not all met
	// must fail before serving any traffic, not silently fall back to
	// an unauthenticated or desktop-only mode.
	remoteManagementEnabled := cfg.RemoteManagement.Enabled
	if remoteManagementEnabled && !headlessMode {
		return fmt.Errorf("--remote-management requires --headless (docs/remote-management.md §3)")
	}
	var remoteManagementOrigin string
	if remoteManagementEnabled {
		if err := config.ValidateRemoteManagementOrigin(cfg.RemoteManagement.ExternalOrigin); err != nil {
			return err
		}
		remoteManagementOrigin = config.CanonicalRemoteManagementOrigin(cfg.RemoteManagement.ExternalOrigin)
	}

	// docs/remote-ingest.md §3: remote ingest requires both --headless
	// and --remote-management, checked explicitly (never inferred from
	// one another, GOOS, TLS file presence, or environment alone) -
	// mirrors the remote-management-requires-headless check immediately
	// above. Every other precondition (a valid RTMPS address distinct
	// from the loopback MediaMTX addresses, a readable TLS key/
	// certificate pair) is checked further below, before the MediaMTX
	// supervisor is constructed - a deployment whose remote-ingest
	// preconditions are not all met must fail before MediaMTX starts,
	// never silently fall back to the loopback-only D2B behavior.
	remoteIngestEnabled := cfg.RemoteIngest.Enabled
	if remoteIngestEnabled && !remoteManagementEnabled {
		return fmt.Errorf("--remote-ingest requires --remote-management (docs/remote-ingest.md §3)")
	}
	var remoteIngestOptions *mediamtx.RemoteIngestOptions
	if remoteIngestEnabled {
		if err := config.ValidateRemoteIngestPreconditions(cfg); err != nil {
			return err
		}
		// docs/remote-ingest.md §8: the operator supplies the RTMPS
		// certificate/key; this application never generates or issues
		// one. Read once, here, and fail closed - the same "fail
		// loudly at startup" philosophy the D2A master key already
		// applies, rather than letting MediaMTX itself discover a
		// missing/malformed file later with a less actionable error.
		if _, err := os.Stat(cfg.RemoteIngest.ServerKeyPath); err != nil {
			return fmt.Errorf("STREAMING_TREE_REMOTE_INGEST_TLS_KEY_PATH: %q is not readable: %w", cfg.RemoteIngest.ServerKeyPath, err)
		}
		if _, err := os.Stat(cfg.RemoteIngest.ServerCertPath); err != nil {
			return fmt.Errorf("STREAMING_TREE_REMOTE_INGEST_TLS_CERT_PATH: %q is not readable: %w", cfg.RemoteIngest.ServerCertPath, err)
		}
		if _, err := tls.LoadX509KeyPair(cfg.RemoteIngest.ServerCertPath, cfg.RemoteIngest.ServerKeyPath); err != nil {
			return fmt.Errorf("remote ingest TLS material does not form a valid key/certificate pair: %w", err)
		}

		remoteIngestOptions = &mediamtx.RemoteIngestOptions{
			RTMPSAddress:   cfg.RemoteIngest.RTMPSAddress,
			ServerKeyPath:  cfg.RemoteIngest.ServerKeyPath,
			ServerCertPath: cfg.RemoteIngest.ServerCertPath,
			PublisherUser:  remoteIngestPublisherUser,
			// PublisherPassVerifier is deliberately left empty here:
			// credential generation/persistence is not implemented
			// yet (docs/remote-ingest.md §6/§8, tracked as this
			// stage's next commit). RenderConfig's own documented
			// behavior for an empty verifier is to omit the
			// publisher entry entirely, so the RTMPS listener and
			// local read/api identity come up correctly while
			// nothing can yet publish.
		}
	}

	// docs/remote-ingest.md §10: the remote overlay origin is an
	// independent opt-in from remote ingest (an operator may want one
	// without the other), but it still requires remote management to
	// be enabled - it is validated against, and must differ in
	// hostname from, the management origin.
	var remoteOverlayOptions httpapi.RemoteOverlayOptions
	if cfg.RemoteIngest.OverlayOrigin != "" {
		if !remoteManagementEnabled {
			return fmt.Errorf("STREAMING_TREE_REMOTE_INGEST_OVERLAY_ORIGIN requires --remote-management (docs/remote-ingest.md §10)")
		}
		if err := config.ValidateRemoteOverlayOrigin(cfg.RemoteIngest.OverlayOrigin, remoteManagementOrigin); err != nil {
			return err
		}
		remoteOverlayOptions = httpapi.RemoteOverlayOptions{
			Enabled: true,
			// CanonicalRemoteManagementOrigin is a generic
			// "scheme://host[:port]" normalizer despite its name - the
			// exact form withRemoteOverlaySecurity's own forwarded-host
			// comparison needs, the same normalization the management
			// origin already uses for the same purpose.
			CanonicalOrigin: config.CanonicalRemoteManagementOrigin(cfg.RemoteIngest.OverlayOrigin),
		}
		logger.Info("remote overlay origin configured", slog.String("overlay_origin", remoteOverlayOptions.CanonicalOrigin))
	}

	// Packaged mode only (docs/windows-packaging.md §9): a second launch
	// while an instance is already running must not open a second backend,
	// bind the port again, or touch the database - it focuses the existing
	// instance's management URL and exits cleanly instead.
	if buildinfo.Packaged() {
		acquired, release, instErr := singleinstance.Acquire()
		if instErr != nil {
			return instErr
		}
		if !acquired {
			managementURL := "http://" + cfg.Address() + "/"
			if headlessMode {
				// docs/linux-headless-server.md §5/§24: never a browser
				// launch in headless mode, and no zenity/kdialog/native
				// UI either - a second headless instance detecting the
				// first is simply a structured log line, exactly what a
				// service operator inspects via journald.
				logger.Info("another instance is already running (headless mode, no browser to open)",
					slog.String("url", managementURL))
			} else if cfg.TestNoUI {
				logger.Info("another instance is already running (test mode, browser launch suppressed)",
					slog.String("url", managementURL))
			} else if openErr := browserlaunch.Open(managementURL); openErr != nil {
				logger.Warn("another instance is already running, and opening its browser tab failed",
					slog.Any("error", openErr), slog.String("url", managementURL))
			} else {
				logger.Info("another instance is already running; focused it and exiting",
					slog.String("url", managementURL))
			}
			return nil
		}
		defer release()
	}

	startedAt := time.Now()

	// Cancelled on Ctrl+C or SIGTERM. Created before the database so a signal
	// during startup still unwinds cleanly.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := sqlite.Open(ctx, cfg.DatabasePath)
	if err != nil {
		return err
	}
	// Closed on every exit path, including a failed migration.
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			logger.Error("failed to close the database", slog.Any("error", closeErr))
		}
	}()

	logger.Info("database ready",
		// The path holds no credentials: those live in the OS credential
		// store, never in SQLite.
		slog.String("path", db.Path()),
		slog.String("journal_mode", db.JournalMode()),
	)

	appliedVersions, err := sqlite.Migrate(ctx, db.DB)
	if err != nil {
		return err
	}
	if len(appliedVersions) > 0 {
		logger.Info("applied database migrations", slog.Any("versions", appliedVersions))
	} else {
		logger.Info("database schema is up to date")
	}

	platformService := platform.NewService(sqlite.NewPlatformRepository(db.DB))

	// Secret-store backend selection is mode-driven, never GOOS-driven
	// (docs/linux-headless-server.md §10): a normal Linux desktop
	// package still uses Secret Service unconditionally; only an
	// explicit --headless run ever selects the encrypted headless
	// store, and it never falls through to KeyringStore (which would
	// try to open a desktop D-Bus session that does not exist for a
	// service account). One shared store instance backs both
	// destination stream keys and connected-account OAuth token
	// bundles either way - different SecretType namespaces, same
	// underlying backend.
	var secretStore secrets.SecretStore
	if headlessMode {
		// docs/linux-headless-server.md §13: fail closed. A headless
		// service whose mandatory secret backend cannot be
		// initialized must not report itself healthy while every
		// configured provider credential is silently unusable - so
		// this is a startup error, not a deferred-to-first-use
		// condition the way KeyringStore's desktop path is.
		masterKey, keyErr := secrets.LoadHeadlessMasterKey()
		if keyErr != nil {
			return keyErr
		}
		headlessStore, storeErr := secrets.NewHeadlessStore(filepath.Join(cfg.DataDir, "secrets.json"), masterKey)
		if storeErr != nil {
			return storeErr
		}
		secretStore = headlessStore
	} else {
		// Opening the OS credential store is deferred to first use (see
		// secrets.NewKeyringStore), so constructing it here never blocks
		// startup or prompts, even on a system where no credential store
		// is available.
		secretStore = secrets.NewKeyringStore()
	}
	credentialService := credential.NewService(secretStore)

	// docs/remote-management.md §5: fail closed before any listener is
	// created if remote management is enabled but no administrator
	// credential has been provisioned - a service must never appear
	// healthy while remote authentication is impossible.
	var remoteManagementOptions httpapi.RemoteManagementOptions
	if remoteManagementEnabled {
		provisioned, provisionErr := auth.AdminPasswordProvisioned(ctx, secretStore)
		if provisionErr != nil {
			return provisionErr
		}
		if !provisioned {
			return fmt.Errorf(
				"remote management is enabled but no administrator password is provisioned - run --provision-admin-password first (docs/remote-management.md §9.2)")
		}
		remoteManagementOptions = httpapi.RemoteManagementOptions{
			Enabled:        true,
			ExternalOrigin: remoteManagementOrigin,
			Auth:           auth.AdminAuthenticator{Store: secretStore},
			Sessions:       auth.NewSessionStore(auth.RealClock),
			LoginLimiter:   auth.NewLoginLimiter(auth.RealClock),
		}
		logger.Info("remote management enabled", slog.String("external_origin", remoteManagementOrigin))
	}

	// docs/remote-ingest.md §6: a previously-provisioned remote-ingest
	// credential must survive a service restart - read the stored
	// verifier now (secretStore was just constructed above) and thread
	// it into remoteIngestOptions before the MediaMTX supervisor is
	// constructed further below, instead of always starting with no
	// credential provisioned. A missing verifier is not an error here
	// (first boot, or the operator has not provisioned one yet) -
	// mediamtx.RenderConfig's own documented behavior for an empty
	// verifier already handles that state correctly.
	if remoteIngestEnabled {
		verifier, _, verifierErr := auth.RemoteIngestPublisherVerifier(ctx, secretStore)
		if verifierErr != nil {
			return fmt.Errorf("reading the remote ingest publisher credential: %w", verifierErr)
		}
		remoteIngestOptions.PublisherPassVerifier = verifier
	}

	outputService := output.NewService(sqlite.NewOutputRepository(db.DB))

	// Twitch and YouTube both have connected-account adapters in this
	// stage; Kick and TikTok remain configuration-only destinations.
	requiredScopes := map[account.ProviderID][]string{
		account.ProviderTwitch:  {twitch.RequiredScope},
		account.ProviderYouTube: {youtube.RequiredScope},
	}
	envClientIDs := map[account.ProviderID]string{}
	if cfg.TwitchClientID != "" {
		envClientIDs[account.ProviderTwitch] = cfg.TwitchClientID
	}
	if cfg.YouTubeClientID != "" {
		envClientIDs[account.ProviderYouTube] = cfg.YouTubeClientID
	}
	twitchClient := twitch.New(twitch.Options{})
	twitchAdapter := twitch.NewAdapter(twitchClient)
	youtubeClient := youtube.New(youtube.Options{})
	youtubeAdapter := youtube.NewAdapter(youtubeClient)
	providers := map[account.ProviderID]account.Provider{
		account.ProviderTwitch:  twitchAdapter,
		account.ProviderYouTube: youtubeAdapter,
	}
	// Only Twitch uses Device Code Grant Flow; deviceflow.Manager depends on
	// the narrower DeviceFlowProvider interface, so it gets its own map -
	// see account.DeviceFlowProvider's own doc comment for why YouTube's
	// adapter is never a candidate for this one. YouTube's own OAuth
	// attempts are orchestrated separately by youtubeauth.Manager below.
	deviceFlowProviders := map[account.ProviderID]account.DeviceFlowProvider{
		account.ProviderTwitch: twitchAdapter,
	}

	// Constructed before accountService so Disconnect can clear a YouTube
	// destination's remote-target association (its selected broadcast)
	// when the account backing it is removed - see account.Options.
	// OnAccountDisconnected's own doc comment for why this is a plain
	// callback rather than an import of internal/domain/remotetarget from
	// internal/domain/account itself.
	remoteTargetService := remotetarget.NewService(sqlite.NewRemoteTargetRepository(db.DB), nil)

	// Stage 8A: the Engagement Event Bus and its Twitch connector manager.
	// eventBus is constructed here (not inside twitchengagement.Manager)
	// because it is also handed to the HTTP router directly - the SSE and
	// snapshot endpoints read it without going through the connector
	// manager at all.
	eventBus := bus.New(bus.Options{Capacity: cfg.EngagementBufferSize})
	engagementSettingsService := engagementsettings.NewService(sqlite.NewEngagementSettingsRepository(db.DB), nil)

	// twitchEngagementManager/youtubeEngagementManager are constructed
	// below (each needs accountService and deviceFlowManager, both
	// defined further down); onAccountRemoved closes over pointers set
	// once those managers exist, so Disconnect's hook is wired before
	// accountService itself is constructed without requiring either
	// manager to exist yet.
	var twitchEngagementManager *twitchengagement.Manager
	var youtubeEngagementManager *youtubeengagement.Manager

	accountService := account.NewService(account.Options{
		Repository:     sqlite.NewAccountRepository(db.DB),
		Secrets:        secretStore,
		Providers:      providers,
		EnvClientIDs:   envClientIDs,
		RequiredScopes: requiredScopes,
		Logger:         logger,
		OnAccountDisconnected: func(cbCtx context.Context, platformID string) error {
			return remoteTargetService.DeleteTarget(cbCtx, platformID)
		},
		OnAccountRemoved: func(cbCtx context.Context, accountID string) {
			if twitchEngagementManager != nil {
				twitchEngagementManager.StopAndRemove(accountID)
			}
			if youtubeEngagementManager != nil {
				youtubeEngagementManager.StopAndRemove(accountID)
			}
		},
	})
	// Runs Twitch's required hourly re-validation in the background; a
	// Twitch or credential-store outage here only affects account status,
	// never HTTP server startup.
	accountService.StartValidationWorker(ctx)

	deviceFlowManager := deviceflow.NewManager(deviceflow.Options{
		Accounts:       accountService,
		Providers:      deviceFlowProviders,
		RequiredScopes: requiredScopes,
		Logger:         logger,
	})
	deviceFlowManager.Start(ctx)

	twitchMetadataService := twitch.NewMetadataService(accountService, twitchClient)

	// destinationLookup attaches DestinationID to a normalized event only
	// when the account is linked to exactly one configured destination -
	// never guessed when there is more than one (see
	// account.Service.LinkedPlatforms's own doc comment).
	destinationLookup := func(accountID string) (string, bool) {
		links, err := accountService.LinkedPlatforms(ctx, accountID)
		if err != nil || len(links) != 1 {
			return "", false
		}
		return links[0].PlatformID, true
	}
	twitchEngagementManager = twitchengagement.NewManager(twitchengagement.Options{
		Accounts: accountService, Settings: engagementSettingsService, Bus: eventBus, Client: twitchClient,
		Logger: logger, DestinationLookup: destinationLookup,
	})
	if err := twitchEngagementManager.Start(ctx); err != nil {
		logger.Warn("could not restore enabled Twitch engagement connectors at startup", slog.Any("error", err))
	}

	// broadcastLookup resolves a destination's currently-selected YouTube
	// live-broadcast id, reusing Stage 7B's own remote-target selection -
	// shared by the Stage 15A engagement connector and the YouTube
	// outbound-chat adapter below, never a second, invented selector. Only
	// a YouTube-provider, live_broadcast-typed target is ever returned.
	broadcastLookup := func(platformID string) (string, bool) {
		target, found, err := remoteTargetService.GetTarget(ctx, platformID)
		if err != nil || !found {
			return "", false
		}
		if target.ProviderID != string(account.ProviderYouTube) || target.ResourceType != remotetarget.ResourceTypeLiveBroadcast {
			return "", false
		}
		return target.ResourceID, true
	}
	youtubeEngagementManager = youtubeengagement.NewManager(youtubeengagement.Options{
		Accounts: accountService, Settings: engagementSettingsService, Bus: eventBus, Client: youtubeClient,
		Logger: logger, DestinationLookup: destinationLookup, BroadcastLookup: broadcastLookup,
	})
	if err := youtubeEngagementManager.Start(ctx); err != nil {
		logger.Warn("could not restore enabled YouTube engagement connectors at startup", slog.Any("error", err))
	}

	// Stage 16A: external donation sources (StreamElements first) -
	// donationSourceService is a deliberately separate domain from
	// accountService (see internal/domain/donationsource's own doc
	// comment: a StreamElements personal JWT has none of account.Account's
	// OAuth shape). streamElementsEngagementManager is forward-declared
	// exactly the way twitchEngagementManager/youtubeEngagementManager are
	// above, so OnSourceRemoved can close over it before it exists yet.
	var streamElementsEngagementManager *streamelementsengagement.Manager
	donationSourceService := donationsource.NewService(donationsource.Options{
		Repository: sqlite.NewDonationSourceRepository(db.DB),
		Secrets:    secretStore,
		OnSourceRemoved: func(sourceID string) {
			if streamElementsEngagementManager != nil {
				streamElementsEngagementManager.StopAndRemove(sourceID)
			}
		},
	})
	streamElementsEngagementManager = streamelementsengagement.NewManager(streamelementsengagement.Options{
		Sources: donationSourceService, Secrets: secretStore, Bus: eventBus,
		Client: streamelements.New(streamelements.Options{}), Logger: logger,
	})
	if err := streamElementsEngagementManager.Start(ctx); err != nil {
		logger.Warn("could not restore enabled StreamElements donation connectors at startup", slog.Any("error", err))
	}

	// Stage 9: the unified-operator-chat projection consumes the same
	// Event Bus, begins empty regardless of what the bus already retains
	// (see operatorchat.Projection.Start's own doc comment), and is
	// independently bounded from it - see cfg.OperatorChatBufferSize.
	operatorChatProjection := oc.New(oc.Options{
		Source: eventBus, Capacity: cfg.OperatorChatBufferSize, Logger: logger, Destinations: destinationLookup,
	})
	if err := operatorChatProjection.Start(ctx); err != nil {
		return err
	}
	operatorChatPrefsService := operatorchatprefs.NewService(sqlite.NewOperatorChatPrefsRepository(db.DB), nil, nil)
	operatorChatAssets := chatassets.NewResolver(twitchClient, accountService, nil)

	// Stage 13A/13B: the shared, provider-independent visual-design
	// service - one design per (owner_kind, owner_id), persisted in its
	// own migration (0015_visual_designs.sql, widened by
	// 0016_visual_design_chat_overlay_owner.sql). Constructed once, here,
	// and reused unchanged by both the chat-overlay wiring below and the
	// alert wiring further down - the same shared table serves both
	// owner kinds through this one generic Service. A nil
	// VisualDesignService anywhere below would degrade that owner to its
	// legacy fixed renderer rather than panicking; production always
	// wires a real one.
	visualDesignService := alerts.NewVisualDesignService(sqlite.NewVisualDesignRepository(db.DB))

	// Stage 14B: the managed visual asset store (docs/visual-template-
	// packages.md §13/§14) - a sibling of internal/runtime/mediamtx's
	// own "<DataDir>/runtime" convention. Reconcile runs once, here, on
	// every clean startup, before any request can reach it: it removes
	// every leftover package-import preview session (none from a
	// previous process can still be legitimate) and any truly orphaned
	// blob - never fatal, only logged, since a broken individual asset
	// must never prevent the rest of the database from being read.
	visualAssetStore := visualasset.NewFileStore(filepath.Join(cfg.DataDir, "assets", "visual"))
	if err := visualAssetStore.EnsureDirs(); err != nil {
		return err
	}
	visualAssetService := visualasset.NewService(sqlite.NewVisualAssetRepository(db.DB), visualAssetStore, nil)
	if reconciled, err := visualAssetService.Reconcile(ctx); err != nil {
		logger.Error("visual asset store reconciliation failed", slog.Any("error", err))
	} else {
		logger.Info("visual asset store reconciled",
			slog.Int("orphan_blob_files_removed", reconciled.OrphanBlobFilesRemoved),
			slog.Int("orphan_blob_rows_removed", reconciled.OrphanBlobRowsRemoved),
			slog.Int("missing_blob_files", len(reconciled.MissingBlobFiles)),
		)
	}

	// Stage 17B: the managed persistent alert-audio asset store
	// (docs/alert-audio.md §5) - a second, independent
	// *visualasset.FileStore instance rooted at a sibling directory,
	// reusing that type's own generic content-addressed blob primitive
	// directly rather than duplicating it (docs/alert-audio.md §5.1).
	// Reconcile runs once, here, on every clean startup, exactly like
	// the visual asset store above.
	audioAssetStore := visualasset.NewFileStore(filepath.Join(cfg.DataDir, "assets", "audio"))
	if err := audioAssetStore.EnsureDirs(); err != nil {
		return err
	}
	audioAssetService := audioasset.NewService(sqlite.NewAudioAssetRepository(db.DB), audioAssetStore, nil)
	if reconciled, err := audioAssetService.Reconcile(ctx); err != nil {
		logger.Error("audio asset store reconciliation failed", slog.Any("error", err))
	} else {
		logger.Info("audio asset store reconciled",
			slog.Int("orphan_blob_files_removed", reconciled.OrphanBlobFilesRemoved),
			slog.Int("orphan_blob_rows_removed", reconciled.OrphanBlobRowsRemoved),
			slog.Int("missing_blob_files", len(reconciled.MissingBlobFiles)),
		)
	}

	// Stage 10: the chat-overlay profile store and its live public
	// projection. The projection's own bounded revision buffer is
	// independent from both the Event Bus's and operator-chat's own -
	// see internal/chatoverlay.DefaultRevisionCapacity's own doc
	// comment.
	chatOverlayProfileService := chatoverlaydomain.NewService(sqlite.NewChatOverlayRepository(db.DB), nil)

	// docs/remote-ingest.md §12: one shared repository backs every
	// overlay domain's remote capability tokens - cheap to construct
	// unconditionally; the real gating is withRemoteOverlaySecurity's
	// own RemoteOverlay.Enabled check (no forwarded request can ever
	// reach a handler's resolution path unless that is true).
	remoteOverlayCapabilities := sqlite.NewRemoteOverlayCapabilityRepository(db.DB)
	chatOverlayAccountLabel := func(connectedAccountID string) (string, bool) {
		acct, err := accountService.GetAccount(ctx, connectedAccountID)
		if err != nil || acct.DisplayName == "" {
			return "", false
		}
		return acct.DisplayName, true
	}
	chatOverlayResolver := &co.DefaultSettingsResolver{
		Profiles: chatOverlayProfileService, OperatorPrefs: operatorChatPrefsService, AccountLabel: chatOverlayAccountLabel,
		VisualDesigns: visualDesignService,
	}
	chatOverlayManager := co.NewManager(co.WrapOperatorChatSource(operatorChatProjection), chatOverlayResolver, visualDesignService, logger)
	chatOverlayManager.SetAssetService(visualAssetService)
	if err := chatOverlayManager.Start(ctx); err != nil {
		return err
	}

	// Stage 11A/15A: the outbound-chat dispatcher. In-memory only, reset on
	// every restart - see internal/outboundchat's own doc comment. The
	// same twitchAdapter already registered with account.Service also
	// implements outboundchat.Provider (Adapter.SendChatMessage), so no
	// second Twitch client or adapter is constructed here. The YouTube
	// adapter is its own type (youtube.OutboundChatAdapter, not
	// youtube.Adapter) since sending needs the same broadcastLookup
	// dependency the engagement connector above already uses.
	youtubeOutboundAdapter := youtube.NewOutboundChatAdapter(youtubeClient, destinationLookup, broadcastLookup)
	outboundChatManager := outboundchat.NewManager(outboundchat.ManagerOptions{
		Accounts: accountService, Providers: []outboundchat.Provider{twitchAdapter, youtubeOutboundAdapter},
	})

	youtubeAuthManager := youtubeauth.NewManager(youtubeauth.Options{
		Accounts: accountService, Client: youtubeClient, RequiredScopes: []string{youtube.RequiredScope}, Logger: logger,
	})
	youtubeAuthManager.Start(ctx)

	youtubeRegionRepo := sqlite.NewYouTubeRegionRepository(db.DB)
	youtubeMetadataService := youtube.NewMetadataService(accountService, youtubeRegionRepo, youtubeClient)

	// The MediaMTX supervisor holds runtime state only, in memory. A missing or
	// failed MediaMTX must never stop the Go API: platform configuration stays
	// readable and writable regardless.
	supervisor := mediamtx.NewSupervisor(mediamtx.Options{
		DataDir:        cfg.DataDir,
		RTMPAddress:    cfg.MediaMTX.RTMPAddress,
		APIAddress:     cfg.MediaMTX.APIAddress,
		IngestPath:     cfg.MediaMTX.IngestPath,
		AutoStart:      cfg.MediaMTX.AutoStart,
		AutoRestart:    cfg.MediaMTX.AutoRestart,
		ExecutablePath: cfg.MediaMTX.ExecutablePath,
		RemoteIngest:   remoteIngestOptions,
		Logger:         logger,
	})
	supervisor.Start(ctx)

	snapshot := supervisor.Snapshot()
	logger.Info("mediamtx runtime",
		slog.String("state", string(snapshot.MediaMTX.State)),
		slog.String("source", string(snapshot.MediaMTX.Source)),
		slog.String("supported_version", snapshot.MediaMTX.SupportedVersion),
		slog.String("rtmp", snapshot.Connection.ServerURL),
	)

	// docs/remote-ingest.md §8: constructed only when --remote-ingest is
	// active, coordinating the secret store and the supervisor for
	// provision/rotate/revoke (internal/remoteingest.Manager) - see
	// router.go's own nil-means-not-registered convention for why a nil
	// value here is sufficient to keep every route unregistered.
	var remoteIngestService httpapi.RemoteIngestService
	if remoteIngestEnabled {
		remoteIngestService = &remoteingest.Manager{
			Store:        secretStore,
			Supervisor:   supervisor,
			RTMPSAddress: cfg.RemoteIngest.RTMPSAddress,
			IngestPath:   cfg.MediaMTX.IngestPath,
		}
	}

	// Stage 11B: the automation runtime (scheduled messages, chat
	// commands). Reuses outboundChatManager - no second outbound
	// pipeline - and shares one Event Bus subscription for both command
	// matching and activity counting. Persisted definitions live in
	// their own migration; runtime state (next-run times, cooldowns,
	// activity counters) is in-memory only, exactly like every other
	// automation runtime manager above.
	chatAutomationDomainService := chatautomation.NewDomainService(
		sqlite.NewChatAutomationRepository(db.DB), accountService, platformService,
	)
	chatAutomationManager := chatautomation.NewManager(chatautomation.ManagerOptions{
		DomainService: chatAutomationDomainService,
		Outbound:      outboundChatManager,
		Bus:           eventBus,
		Ingest:        chatautomation.MediaMTXIngestChecker{Supervisor: supervisor},
		Accounts:      accountService,
		Platforms:     platformService,
		BotUsers:      chatautomation.BotUserCheckerAdapter{Prefs: operatorChatPrefsService},
	})
	if err := chatAutomationManager.Start(ctx); err != nil {
		return err
	}

	// Stage 12A: the alert runtime (rule matching, bounded per-profile
	// queues, playback, the public Browser Source SSE protocol).
	// Consumes the same Event Bus as every other engagement consumer
	// above, through its own single shared subscription - never a
	// second EventSub connection, never a direct call into
	// internal/provider/twitch. Persisted profile/rule definitions live
	// in their own migration; every queue/playback runtime value stays
	// in memory only.
	alertsDomainService := alerts.NewDomainService(sqlite.NewAlertsRepository(db.DB), accountService, donationSourceService, audioAssetService)

	// Stage 17A: the shared audio/text-to-speech runtime - the ONE
	// Engagement Event Bus subscription for TTS-eligible events.
	// audioSettingsService persists only operator settings
	// (docs/audio-tts.md §12); every queue/cooldown/playback value stays
	// in memory only. audioSelfLookup mirrors internal/chatautomation's
	// own self-message identity check exactly (a connected account's own
	// ProviderUserID). Constructed before alertsManager below (Stage
	// 17B) since alertsManager links to it as its AudioLink - internal/
	// audio never depends on internal/alerts, only the reverse.
	audioSettingsService := audiodomain.NewService(audiodomain.Options{
		Repository: sqlite.NewAudioSettingsRepository(db.DB),
	})
	audioSelfLookup := func(connectedAccountID string) (string, bool) {
		acc, err := accountService.GetAccount(context.Background(), connectedAccountID)
		if err != nil {
			return "", false
		}
		return acc.ProviderUserID, true
	}
	audioManager := audiort.NewManager(audiort.Options{
		SettingsService:    audioSettingsService,
		Bus:                eventBus,
		Provider:           tts.NewSystemProvider(),
		OperatorChatPrefs:  operatorChatPrefsService,
		SelfLookup:         audioSelfLookup,
		AudioAssetResolver: audioAssetService,
	})
	if err := audioManager.Start(ctx); err != nil {
		return err
	}

	// visualDesignService (the same shared instance the chat-overlay
	// wiring above already received) is reused here unchanged - one
	// design per alert rule, in the same shared visual_designs table.
	// AudioLink wires Stage 17B rule-owned sound/TTS playback through
	// the same audioManager instance above - never a second engine.
	alertsManager := alerts.NewManager(alerts.ManagerOptions{
		DomainService:       alertsDomainService,
		VisualDesignService: visualDesignService,
		AssetService:        visualAssetService,
		Bus:                 eventBus,
		AudioLink:           audioManager,
	})
	if err := alertsManager.Start(ctx); err != nil {
		return err
	}

	// Stage 18A: the persistent goals/counters foundation
	// (docs/goals-widgets.md). goalsDomainService owns configuration/
	// accumulated-state persistence; goalsManager is the ONE Engagement
	// Event Bus subscription that applies real contributions to it -
	// never a second accumulation engine, never a direct call into any
	// provider package.
	goalsDomainService := domaingoals.NewService(
		sqlite.NewGoalsRepository(db.DB),
		goalsrt.SourceLookupAdapter{Accounts: accountService, DonationSources: donationSourceService},
		nil,
	)
	goalsManager := goalsrt.NewManager(goalsrt.ManagerOptions{
		DomainService: goalsDomainService,
		Bus:           eventBus,
	})
	if err := goalsManager.Start(ctx); err != nil {
		return err
	}

	// Stage 18B: supporter/activity widgets, richer counters, and
	// bounded multi-widget dashboards (docs/supporter-widgets.md §4).
	// One provider-independent runtime manager, one Event Bus
	// subscription at current position - never a second engine, never
	// one subscription per widget profile. goalsDomainService already
	// satisfies WidgetProfileLister directly, exactly like it already
	// satisfies GoalsService below.
	supporterWidgetsManager := supporterwidgetsrt.NewManager(supporterwidgetsrt.ManagerOptions{
		Profiles: goalsDomainService,
		Bus:      eventBus,
	})
	if err := supporterWidgetsManager.Start(ctx); err != nil {
		return err
	}

	// Stage 14A: the reusable, portable visual-design template library -
	// an independent management surface from visual_designs above; a
	// template is never linked to any specific alert rule or chat
	// overlay (see docs/visual-templates.md). Built-ins are validated
	// once here so a malformed one fails startup loudly rather than
	// reaching an operator.
	visualTemplateService, err := visualtemplate.NewService(sqlite.NewVisualTemplateRepository(db.DB), visualtemplate.DefaultBuiltins(), nil)
	if err != nil {
		return err
	}
	visualTemplateService.SetAssetService(visualAssetService)
	visualTemplateService.SetAudioAssetService(audioAssetService)

	// Stage 14B: the portable, secure `.streaming-tree-template` package
	// import/preview/export domain - bridges visualAssetService and
	// visualTemplateService (docs/visual-template-packages.md §20/§43).
	visualPackageService := visualpackage.NewService(visualAssetService, audioAssetService, visualTemplateService, nil)

	// Every branch begins with desiredRunning false: a backend restart never
	// resumes a broadcast on its own, so nothing is started here.
	branchManager := branch.NewManager(branch.Options{
		Platforms:   platformService,
		Outputs:     outputService,
		Credentials: credentialService,
		FFmpeg:      ffmpeg.NewResolver(cfg.FFmpeg.ExecutablePath),
		Ingest:      supervisor,
		Logger:      logger,
	})
	branchManager.Start(ctx)

	ffmpegStatus := branchManager.FFmpegStatus()
	logger.Info("ffmpeg dependency",
		// Never the path: see ffmpeg.Resolution.Path's own doc comment.
		slog.String("source", string(ffmpegStatus.Source)),
		slog.String("detected_version", ffmpegStatus.Version),
		slog.Bool("compatible", ffmpegStatus.Compatible),
	)

	// Packaged/release builds only (docs/windows-packaging.md §1/§2/§8/§16):
	// the embedded production frontend/legal documents and the real
	// graceful-shutdown endpoint. Every development/test build leaves all
	// three nil, exactly matching every prior stage's behavior.
	var webAssets, legalAssets fs.FS
	var shutdownCancel context.CancelFunc
	if buildinfo.Packaged() {
		webAssets = webassets.Frontend()
		legalAssets = webassets.Legal()
		// The same CancelFunc signal.NotifyContext already returned above -
		// POST /api/system/shutdown reuses the exact existing
		// <-ctx.Done() graceful-shutdown path rather than duplicating it.
		shutdownCancel = stop
	}

	// Stage 20B: the application updater (docs/updater.md). Wired in
	// every build, not only packaged/release ones - a development build
	// simply reports itself honestly disabled (docs/updater.md §35)
	// rather than the /api/updates/* routes not existing at all. The
	// updater's own release-build/streaming/installed-context guards
	// (not this conditional) are what actually prevent any real GitHub
	// traffic or install action outside a packaged release build.
	updateSettingsService := updatersettings.NewService(sqlite.NewUpdateSettingsRepository(db.DB), nil)

	// Stage 21: the first-run onboarding-state preference
	// (docs/onboarding.md §4). Wired unconditionally, exactly like every
	// other singleton-preference domain in this codebase - it is UI-flow
	// state, not a release-build-gated capability.
	onboardingService := onboarding.NewService(sqlite.NewOnboardingRepository(db.DB), nil)

	// Stage 22: reusable stream metadata presets (docs/metadata-presets.md).
	metadataPresetService := metadatapreset.NewService(sqlite.NewMetadataPresetRepository(db.DB), platformService)

	// Stage 23: safe configuration backup/restore (docs/backup-restore.md).
	// backupStaging holds an uploaded package's raw bytes between
	// RestorePreview and Restore - its own directory, never the
	// directory a real backup file the operator saved lives in, and
	// never reachable from any public/overlay route (docs/backup-
	// restore.md §28).
	backupStaging, err := backupdomain.NewFileStaging(filepath.Join(cfg.DataDir, "backup-staging"), backupdomain.PreviewTTL)
	if err != nil {
		return err
	}

	// Every Sources field below is a FRESH repository instance
	// constructed directly against db.DB, exactly like
	// internal/userdatapurge's own cross-domain sweep already does
	// (never through another domain's own Service, which may apply
	// business-rule side effects a read-only export must not trigger).
	backupService := backupdomain.NewService(backupdomain.Sources{
		Platforms:          sqlite.NewPlatformRepository(db.DB),
		Output:             sqlite.NewOutputRepository(db.DB),
		Accounts:           sqlite.NewAccountRepository(db.DB),
		YouTubeRegion:      sqlite.NewYouTubeRegionRepository(db.DB),
		EngagementSettings: sqlite.NewEngagementSettingsRepository(db.DB),
		OperatorChatPrefs:  sqlite.NewOperatorChatPrefsRepository(db.DB),
		ChatOverlays:       sqlite.NewChatOverlayRepository(db.DB),
		ChatAutomation:     sqlite.NewChatAutomationRepository(db.DB),
		Alerts:             sqlite.NewAlertsRepository(db.DB),
		VisualDesigns:      sqlite.NewVisualDesignRepository(db.DB),
		VisualTemplates:    sqlite.NewVisualTemplateRepository(db.DB),
		VisualAssets:       sqlite.NewVisualAssetRepository(db.DB),
		AudioAssets:        sqlite.NewAudioAssetRepository(db.DB),
		AudioSettings:      sqlite.NewAudioSettingsRepository(db.DB),
		Goals:              sqlite.NewGoalsRepository(db.DB),
		MetadataPresets:    sqlite.NewMetadataPresetRepository(db.DB),
		DonationSources:    sqlite.NewDonationSourceRepository(db.DB),
		UpdatePreferences:  sqlite.NewUpdateSettingsRepository(db.DB),
	}, visualAssetStore, audioAssetStore, backupStaging, buildinfo.EffectiveVersion(), runtime.GOOS)
	updateManager := updater.NewManager(updater.Options{
		Client:            newUpdaterClient(buildinfo.EffectiveVersion()),
		Settings:          updateSettingsService,
		Branches:          branchManager,
		Handoff:           updater.NewPlatformHandoff(cfg.DataDir),
		DataDir:           cfg.DataDir,
		ReleaseBuild:      buildinfo.IsReleaseBuild(),
		CurrentVersion:    buildinfo.EffectiveVersion(),
		ProductionVersion: buildinfo.IsStrictProductionVersion(),
		Identity: manifest.Identity{
			OS: manifest.OS(runtime.GOOS), Arch: manifest.Arch(runtime.GOARCH), Kind: manifest.KindInstaller,
		},
		// The same CancelFunc signal.NotifyContext already returned above -
		// a successful install handoff reuses the exact existing
		// <-ctx.Done() graceful-shutdown path rather than duplicating it
		// (docs/updater.md §24).
		OnHandoffBegun: stop,
		Logger:         logger,
	})
	updateManager.Start(ctx)

	// Stage 20E support bundle (docs/final-hardening.md §C): every
	// field gathered here is already a non-secret fact this process
	// has in scope - the snapshot function never reaches into the
	// database, the secret store, or any credential/token value.
	supportBundleSnapshot := func(ctx context.Context) (support.Snapshot, error) {
		mediamtxSnapshot := supervisor.Snapshot()
		ffmpegStatus := branchManager.FFmpegStatus()
		commit, dirty, _ := buildinfo.CommitInfo()

		snap := support.Snapshot{
			Version:          buildinfo.EffectiveVersion(),
			Commit:           commit,
			CommitDirty:      dirty,
			Packaged:         buildinfo.Packaged(),
			OS:               runtime.GOOS,
			Arch:             runtime.GOARCH,
			GoRuntimeVersion: runtime.Version(),
			Headless:         headlessMode,
			RemoteManagement: remoteManagementOptions.Enabled,
			RemoteIngest:     remoteIngestEnabled,
			RemoteOverlay:    remoteOverlayOptions.Enabled,
			MediaMTXVersion:  mediamtxSnapshot.MediaMTX.SupportedVersion,
			FFmpegAvailable:  ffmpegStatus.Compatible,
			FFmpegVersion:    ffmpegStatus.Version,
			SubsystemStates: map[string]string{
				"mediamtx": string(mediamtxSnapshot.MediaMTX.State),
			},
			UpdaterStatus: string(updateManager.Status(ctx).State),
		}
		return snap, nil
	}
	diagnosticsBundleBuilder := support.NewBuilder(diagnosticsRecorder, supportBundleSnapshot)

	// Local-only host-resource snapshot (CPU/memory/disk of cfg.DataDir's
	// own volume) for the Dashboard's "System resources" card - sampled in
	// the background on a bounded, low-frequency tick; never persisted,
	// never transmitted anywhere beyond this process's own local HTTP API.
	resourcesCollector := sysresources.NewCollector(cfg.DataDir, logger, 5*time.Second)
	resourcesCollector.Start()

	handler := httpapi.NewRouter(httpapi.Options{
		Logger:          logger,
		AllowedOrigins:  cfg.AllowedOrigins,
		StartedAt:       startedAt,
		Resources:       resourcesCollector,
		Platforms:       platformService,
		Runtime:         supervisor,
		Onboarding:      onboardingService,
		MetadataPresets: metadataPresetService,
		Backup:          backupService,
		Credentials:     credentialService,
		Outputs:         outputService,
		FFmpegRuntime:   branchManager,
		Branches:        branchManager,
		Accounts:        accountService,
		DeviceFlow:      deviceFlowManager,
		TwitchMetadata:  twitchMetadataService,
		YouTubeAuth:     youtubeAuthManager,
		YouTubeMetadata: youtubeMetadataService,
		RemoteTargets:   remoteTargetService,

		EngagementBus:               eventBus,
		EngagementSettings:          engagementSettingsService,
		EngagementConnectors:        twitchEngagementManager,
		YouTubeEngagementConnectors: youtubeEngagementManager,

		OperatorChatProjection:        operatorChatProjection,
		OperatorChatPrefs:             operatorChatPrefsService,
		OperatorChatAssets:            operatorChatAssets,
		OnOperatorChatBotUsersChanged: chatOverlayManager.RebuildAll,

		ChatOverlayProfiles: chatOverlayProfileService,
		ChatOverlayRuntime:  chatOverlayManager,

		OutboundChat:   outboundChatManager,
		ChatAutomation: chatAutomationManager,
		Alerts:         alertsManager,

		VisualTemplates: visualTemplateService,
		VisualAssets:    visualAssetService,
		VisualPackages:  visualPackageService,

		DonationSources:    donationSourceService,
		DonationConnectors: streamElementsEngagementManager,

		Audio:       audioManager,
		AudioAssets: audioAssetService,

		Goals:            goalsDomainService,
		SupporterWidgets: supporterWidgetsManager,

		WebAssets:   webAssets,
		LegalAssets: legalAssets,
		Shutdown:    shutdownCancel,
		Updater:     updateManager,

		RemoteManagement:      remoteManagementOptions,
		RemoteOverlay:         remoteOverlayOptions,
		RemoteOverlayResolver: remoteOverlayCapabilities,

		// docs/remote-ingest.md §12: the management API is registered
		// whenever a remote overlay origin is configured, independent
		// of whether remote ingest itself is enabled - an operator may
		// want remote overlays without remote RTMPS publishing.
		RemoteOverlayCapabilities: remoteOverlayCapabilities,
		RemoteOverlayOwners: httpapi.RemoteOverlayOwners{
			ChatOverlays: chatOverlayProfileService,
			Alerts:       alertsManager,
			Audio:        audioManager,
			Widgets:      goalsDomainService,
		},
		RemoteOverlayCanonicalOrigin: remoteOverlayOptions.CanonicalOrigin,

		RemoteIngest:             remoteIngestService,
		RemoteIngestRTMPSAddress: cfg.RemoteIngest.RTMPSAddress,
		RemoteIngestPath:         cfg.MediaMTX.IngestPath,

		Diagnostics:       diagnosticsRecorder,
		DiagnosticsBundle: diagnosticsBundleBuilder,
	})

	server := &http.Server{
		Addr:              cfg.Address(),
		Handler:           handler,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	// shutdownRuntime stops every runtime subsystem started above, in the
	// same order regardless of which of the three paths below triggers it -
	// branches before MediaMTX (an in-flight runtime request cannot restart
	// MediaMTX on the way out, and no branch is left trying to reconnect to
	// an input that is itself mid-shutdown), reaping every child process so
	// the backend never leaves one behind.
	// Set below, only in desktop packaged mode - referenced here (not
	// assigned yet) so shutdownRuntime always stops the tray icon on
	// every shutdown path (web UI Quit, tray Quit, Ctrl+C/SIGTERM, and
	// the updater's install handoff all converge on this one function),
	// exactly like every manager below it (docs/windows-tray.md).
	var trayHandle tray.Handle

	shutdownRuntime := func(shutdownCtx context.Context) {
		if trayHandle != nil {
			trayHandle.Stop()
		}
		updateManager.Shutdown(shutdownCtx)
		branchManager.Shutdown(shutdownCtx)
		deviceFlowManager.Shutdown(shutdownCtx)
		youtubeAuthManager.Shutdown(shutdownCtx)
		twitchEngagementManager.Shutdown(shutdownCtx)
		youtubeEngagementManager.Shutdown(shutdownCtx)
		streamElementsEngagementManager.Shutdown(shutdownCtx)
		operatorChatProjection.Shutdown(shutdownCtx)
		chatOverlayManager.Shutdown(shutdownCtx)
		_ = outboundChatManager.Shutdown(shutdownCtx)
		_ = chatAutomationManager.Shutdown(shutdownCtx)
		_ = alertsManager.Shutdown(shutdownCtx)
		_ = audioManager.Shutdown(shutdownCtx)
		_ = goalsManager.Shutdown(shutdownCtx)
		_ = supporterWidgetsManager.Shutdown(shutdownCtx)
		eventBus.Shutdown()
		accountService.ShutdownValidationWorker(shutdownCtx)
		supervisor.Shutdown(shutdownCtx)
		// Last, deliberately: no real user-visible state to flush (no active
		// stream, no in-flight OAuth handoff, nothing durable), so any
		// manager above it with actual state to flush gets first claim on
		// the shared shutdownCtx deadline.
		resourcesCollector.Shutdown(shutdownCtx)
	}

	// The listener is created synchronously, before Serve, so packaged mode
	// only ever opens the browser once the server is actually able to
	// accept connections (docs/windows-packaging.md §6) - not merely once
	// ListenAndServe has been called.
	listener, listenErr := net.Listen("tcp", cfg.Address())
	if listenErr != nil {
		// Most often because the port is already taken. MediaMTX and any
		// branch may already be running, so both are stopped before
		// returning.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		shutdownRuntime(shutdownCtx)
		cancel()
		return listenErr
	}

	if buildinfo.Packaged() {
		managementURL := "http://" + listener.Addr().String() + "/"
		if headlessMode {
			// docs/linux-headless-server.md §5/§23: no browser launch,
			// no xdg-open, ever, in headless mode - just a log line an
			// operator can read via journald.
			logger.Info("headless mode: no browser will be opened", slog.String("url", managementURL))
		} else if cfg.TestNoUI {
			logger.Info("browser launch suppressed (test mode)", slog.String("url", managementURL))
		} else if openErr := browserlaunch.Open(managementURL); openErr != nil {
			logger.Warn("failed to open the default browser",
				slog.Any("error", openErr), slog.String("url", managementURL))
		}

		// Desktop mode only (never --headless), and either normal
		// operation or a test that explicitly asked to keep the tray
		// alive despite suppressing the browser (cfg.TestKeepTray -
		// docs/windows-packaging.md §26's own manual-running-app
		// upgrade integration test needs the real tray window's
		// cooperative-shutdown IPC mechanism to exist). the Stage 20E
		// tray icon (docs/windows-tray.md): closing the browser tab
		// does not stop the backend, and this is the one persistent
		// piece of desktop UI that lets an operator reopen it, see its
		// status, or quit it without Task Manager. A failure to create
		// it (e.g. a non-Windows packaged build, or a real Windows API
		// failure) is logged and never fatal - the rest of the
		// application runs identically without a tray.
		if !headlessMode && (!cfg.TestNoUI || cfg.TestKeepTray) {
			logsURL := managementURL + "logs"
			trayOpts := tray.Options{
				Tooltip: buildinfo.ProductName,
				IconICO: tray.IconICO,
				OnOpenDashboard: func() {
					if openErr := browserlaunch.Open(managementURL); openErr != nil {
						logger.Warn("tray: failed to open the dashboard", slog.Any("error", openErr))
					}
				},
				OnOpenLogs: func() {
					if openErr := browserlaunch.Open(logsURL); openErr != nil {
						logger.Warn("tray: failed to open logs & diagnostics", slog.Any("error", openErr))
					}
				},
				StatusLabel: func() string {
					return trayIngestStatusLabel(supervisor.Snapshot())
				},
				UpdatesLabel: func() (string, bool) {
					return trayUpdatesLabel(updateManager.Status(context.Background()).State)
				},
				OnCheckForUpdates: func() {
					if checkErr := updateManager.CheckNow(context.Background()); checkErr != nil {
						logger.Info("tray: check for updates did not start", slog.Any("error", checkErr))
					}
				},
				OnQuit: stop,
			}
			if handle, trayErr := tray.Run(trayOpts); trayErr != nil {
				logger.Info("tray icon not available", slog.Any("error", trayErr))
			} else {
				trayHandle = handle
			}
		}
	}

	serverErrors := make(chan error, 1)

	go func() {
		logger.Info("http server listening",
			slog.String("service", buildinfo.ServiceName),
			slog.String("version", buildinfo.EffectiveVersion()),
			slog.String("address", cfg.Address()),
			slog.Any("allowed_origins", cfg.AllowedOrigins),
		)

		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
			return
		}
		serverErrors <- nil
	}()

	select {
	case err := <-serverErrors:
		// Serve failed after the listener was already accepting connections
		// (rare) - same cleanup as the bind-failure path above.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		shutdownRuntime(shutdownCtx)
		cancel()
		return err

	case <-ctx.Done():
		logger.Info("shutdown signal received, draining connections",
			slog.Duration("timeout", cfg.ShutdownTimeout))

		// Stop intercepting signals: a second Ctrl+C should kill the process
		// immediately instead of waiting for the drain to finish. Also the
		// same CancelFunc POST /api/system/shutdown calls, so both paths
		// converge here - see httpapi.Options.Shutdown's own doc comment.
		stop()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()

		httpErr := server.Shutdown(shutdownCtx)
		shutdownRuntime(shutdownCtx)

		if httpErr != nil {
			logger.Error("graceful shutdown failed, closing forcefully",
				slog.Any("error", httpErr))
			if closeErr := server.Close(); closeErr != nil {
				return closeErr
			}
			return httpErr
		}

		logger.Info("server stopped cleanly")
		return nil
	}
}
