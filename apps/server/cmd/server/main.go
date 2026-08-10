// Command server starts the Streaming Tree REST API: platform configuration,
// MediaMTX supervision, the destination-credential store, destination
// branch supervision, and the connected-account/Twitch/YouTube integrations
// are all wired here. Kick and TikTok account integrations and the
// engagement platform are still separate, later stages.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/streaming-tree/server/internal/alerts"
	"github.com/streaming-tree/server/internal/buildinfo"
	"github.com/streaming-tree/server/internal/chatautomation"
	co "github.com/streaming-tree/server/internal/chatoverlay"
	"github.com/streaming-tree/server/internal/config"
	"github.com/streaming-tree/server/internal/domain/account"
	chatoverlaydomain "github.com/streaming-tree/server/internal/domain/chatoverlay"
	"github.com/streaming-tree/server/internal/domain/credential"
	"github.com/streaming-tree/server/internal/domain/engagementsettings"
	"github.com/streaming-tree/server/internal/domain/operatorchatprefs"
	"github.com/streaming-tree/server/internal/domain/output"
	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/domain/remotetarget"
	bus "github.com/streaming-tree/server/internal/engagement"
	"github.com/streaming-tree/server/internal/httpapi"
	oc "github.com/streaming-tree/server/internal/operatorchat"
	"github.com/streaming-tree/server/internal/outboundchat"
	"github.com/streaming-tree/server/internal/provider/twitch"
	"github.com/streaming-tree/server/internal/provider/twitch/chatassets"
	"github.com/streaming-tree/server/internal/provider/youtube"
	"github.com/streaming-tree/server/internal/runtime/branch"
	"github.com/streaming-tree/server/internal/runtime/deviceflow"
	"github.com/streaming-tree/server/internal/runtime/ffmpeg"
	"github.com/streaming-tree/server/internal/runtime/mediamtx"
	"github.com/streaming-tree/server/internal/runtime/twitchengagement"
	"github.com/streaming-tree/server/internal/runtime/youtubeauth"
	"github.com/streaming-tree/server/internal/secrets"
	"github.com/streaming-tree/server/internal/storage/sqlite"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server terminated with an error", slog.Any("error", err))
		os.Exit(1)
	}
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

	// twitchEngagementManager is constructed below (it needs accountService
	// and deviceFlowManager, both defined further down); onAccountRemoved
	// closes over a pointer set once that manager exists, so Disconnect's
	// hook is wired before accountService itself is constructed without
	// requiring twitchengagement.Manager to exist yet.
	var twitchEngagementManager *twitchengagement.Manager

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
	}
	chatOverlayManager := co.NewManager(co.WrapOperatorChatSource(operatorChatProjection), chatOverlayResolver, logger)
	if err := chatOverlayManager.Start(ctx); err != nil {
		return err
	}

	// Stage 11A: the outbound-chat dispatcher. In-memory only, reset on
	// every restart - see internal/outboundchat's own doc comment. The
	// same twitchAdapter already registered with account.Service also
	// implements outboundchat.Provider (Adapter.SendChatMessage), so no
	// second Twitch client or adapter is constructed here.
	outboundChatManager := outboundchat.NewManager(outboundchat.ManagerOptions{
		Accounts: accountService, Providers: []outboundchat.Provider{twitchAdapter},
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
	alertsDomainService := alerts.NewDomainService(sqlite.NewAlertsRepository(db.DB), accountService)
	// Stage 13A: the shared, provider-independent visual-design domain -
	// one design per alert rule, persisted in its own migration
	// (0015_visual_designs.sql). A nil VisualDesignService would degrade
	// every rule to the Stage 12 legacy fixed renderer rather than
	// panicking (see ManagerOptions's own doc comment), but production
	// always wires a real one.
	visualDesignService := alerts.NewVisualDesignService(sqlite.NewVisualDesignRepository(db.DB))
	alertsManager := alerts.NewManager(alerts.ManagerOptions{
		DomainService:       alertsDomainService,
		VisualDesignService: visualDesignService,
		Bus:                 eventBus,
	})
	if err := alertsManager.Start(ctx); err != nil {
		return err
	}

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

		EngagementBus:        eventBus,
		EngagementSettings:   engagementSettingsService,
		EngagementConnectors: twitchEngagementManager,

		OperatorChatProjection:        operatorChatProjection,
		OperatorChatPrefs:             operatorChatPrefsService,
		OperatorChatAssets:            operatorChatAssets,
		OnOperatorChatBotUsersChanged: chatOverlayManager.RebuildAll,

		ChatOverlayProfiles: chatOverlayProfileService,
		ChatOverlayRuntime:  chatOverlayManager,

		OutboundChat:   outboundChatManager,
		ChatAutomation: chatAutomationManager,
		Alerts:         alertsManager,
	})

	server := &http.Server{
		Addr:              cfg.Address(),
		Handler:           handler,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	serverErrors := make(chan error, 1)

	go func() {
		logger.Info("http server listening",
			slog.String("service", buildinfo.ServiceName),
			slog.String("version", buildinfo.Version),
			slog.String("address", cfg.Address()),
			slog.Any("allowed_origins", cfg.AllowedOrigins),
		)

		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
			return
		}
		serverErrors <- nil
	}()

	select {
	case err := <-serverErrors:
		// The listener failed outright, most often because the port is taken.
		// MediaMTX and any branch may already be running, so both are
		// stopped before returning - branches first, see below.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		branchManager.Shutdown(shutdownCtx)
		deviceFlowManager.Shutdown(shutdownCtx)
		youtubeAuthManager.Shutdown(shutdownCtx)
		twitchEngagementManager.Shutdown(shutdownCtx)
		operatorChatProjection.Shutdown(shutdownCtx)
		chatOverlayManager.Shutdown(shutdownCtx)
		_ = outboundChatManager.Shutdown(shutdownCtx)
		_ = chatAutomationManager.Shutdown(shutdownCtx)
		_ = alertsManager.Shutdown(shutdownCtx)
		eventBus.Shutdown()
		accountService.ShutdownValidationWorker(shutdownCtx)
		supervisor.Shutdown(shutdownCtx)
		cancel()
		return err

	case <-ctx.Done():
		logger.Info("shutdown signal received, draining connections",
			slog.Duration("timeout", cfg.ShutdownTimeout))

		// Stop intercepting signals: a second Ctrl+C should kill the process
		// immediately instead of waiting for the drain to finish.
		stop()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()

		httpErr := server.Shutdown(shutdownCtx)

		// Branches are stopped before MediaMTX, and MediaMTX is stopped
		// after the HTTP server: an in-flight runtime request cannot
		// restart MediaMTX on the way out, and no branch is left trying to
		// reconnect to an input that is itself in the middle of shutting
		// down. This also reaps every child process, so the backend never
		// leaves one behind. The device-flow manager and the account
		// validation worker are stopped alongside branches: both are
		// background loops with no external process to reap, but stopping
		// them before MediaMTX keeps the shutdown order easy to reason
		// about as one group.
		branchManager.Shutdown(shutdownCtx)
		deviceFlowManager.Shutdown(shutdownCtx)
		youtubeAuthManager.Shutdown(shutdownCtx)
		twitchEngagementManager.Shutdown(shutdownCtx)
		operatorChatProjection.Shutdown(shutdownCtx)
		chatOverlayManager.Shutdown(shutdownCtx)
		_ = outboundChatManager.Shutdown(shutdownCtx)
		_ = chatAutomationManager.Shutdown(shutdownCtx)
		_ = alertsManager.Shutdown(shutdownCtx)
		eventBus.Shutdown()
		accountService.ShutdownValidationWorker(shutdownCtx)
		supervisor.Shutdown(shutdownCtx)

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
