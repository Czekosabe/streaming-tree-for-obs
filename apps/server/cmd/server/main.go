// Command server starts the Streaming Tree REST API: platform configuration,
// MediaMTX supervision, the destination-credential store, destination
// branch supervision, and the connected-account/Twitch/YouTube integrations
// are all wired here. Kick and TikTok account integrations and the
// engagement platform are still separate, later stages.
package main

import (
	"context"
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
	"syscall"
	"time"

	"github.com/streaming-tree/server/internal/alerts"
	audiort "github.com/streaming-tree/server/internal/audio"
	"github.com/streaming-tree/server/internal/buildinfo"
	"github.com/streaming-tree/server/internal/chatautomation"
	co "github.com/streaming-tree/server/internal/chatoverlay"
	"github.com/streaming-tree/server/internal/config"
	"github.com/streaming-tree/server/internal/domain/account"
	audiodomain "github.com/streaming-tree/server/internal/domain/audio"
	"github.com/streaming-tree/server/internal/domain/audioasset"
	chatoverlaydomain "github.com/streaming-tree/server/internal/domain/chatoverlay"
	"github.com/streaming-tree/server/internal/domain/credential"
	"github.com/streaming-tree/server/internal/domain/donationsource"
	"github.com/streaming-tree/server/internal/domain/engagementsettings"
	domaingoals "github.com/streaming-tree/server/internal/domain/goals"
	"github.com/streaming-tree/server/internal/domain/operatorchatprefs"
	"github.com/streaming-tree/server/internal/domain/output"
	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/domain/remotetarget"
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
	"github.com/streaming-tree/server/internal/runtime/branch"
	"github.com/streaming-tree/server/internal/runtime/browserlaunch"
	"github.com/streaming-tree/server/internal/runtime/deviceflow"
	"github.com/streaming-tree/server/internal/runtime/ffmpeg"
	"github.com/streaming-tree/server/internal/runtime/mediamtx"
	"github.com/streaming-tree/server/internal/runtime/nativealert"
	"github.com/streaming-tree/server/internal/runtime/singleinstance"
	"github.com/streaming-tree/server/internal/runtime/streamelementsengagement"
	"github.com/streaming-tree/server/internal/runtime/twitchengagement"
	"github.com/streaming-tree/server/internal/runtime/youtubeauth"
	"github.com/streaming-tree/server/internal/runtime/youtubeengagement"
	"github.com/streaming-tree/server/internal/secrets"
	"github.com/streaming-tree/server/internal/storage/sqlite"
	supporterwidgetsrt "github.com/streaming-tree/server/internal/supporterwidgets"
	"github.com/streaming-tree/server/internal/webassets"
)

func main() {
	if handled := handleVersionFlag(); handled {
		return
	}

	if err := run(); err != nil {
		slog.Error("server terminated with an error", slog.Any("error", err))
		if buildinfo.Packaged() {
			// The release binary has no console window (docs/windows-
			// packaging.md §7/§13) - a fatal startup error must not simply
			// disappear. err's own text follows this codebase's existing
			// convention of never including a secret/token/credential.
			nativealert.ShowFatalError(buildinfo.ProductName, err.Error())
		}
		os.Exit(1)
	}
}

// handleVersionFlag implements `--version`: prints product identity and
// exits 0 without starting any application service (no database open, no
// MediaMTX supervisor, no HTTP listener) - safe to run against an
// installed release binary as a smoke-test check.
func handleVersionFlag() bool {
	versionFlag := flag.Bool("version", false, "print version information and exit")
	flag.Parse()
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

// run holds the real main so that every exit path can return an error and still
// let deferred cleanup happen (os.Exit in main would skip it).
func run() error {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		return err
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
			if cfg.TestNoUI {
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

	// Opening the OS credential store is deferred to first use (see
	// secrets.NewKeyringStore), so constructing it here never blocks startup
	// or prompts, even on a system where no credential store is available.
	// One shared store instance backs both destination stream keys and
	// connected-account OAuth token bundles - different SecretType
	// namespaces, same underlying OS credential store.
	secretStore := secrets.NewKeyringStore()
	credentialService := credential.NewService(secretStore)

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

	handler := httpapi.NewRouter(httpapi.Options{
		Logger:          logger,
		AllowedOrigins:  cfg.AllowedOrigins,
		StartedAt:       startedAt,
		Platforms:       platformService,
		Runtime:         supervisor,
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
	shutdownRuntime := func(shutdownCtx context.Context) {
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
		if cfg.TestNoUI {
			logger.Info("browser launch suppressed (test mode)", slog.String("url", managementURL))
		} else if openErr := browserlaunch.Open(managementURL); openErr != nil {
			logger.Warn("failed to open the default browser",
				slog.Any("error", openErr), slog.String("url", managementURL))
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
