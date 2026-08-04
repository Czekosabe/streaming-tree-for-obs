//go:build integration

// Command testserver is a build-tag-gated twin of cmd/server, used only by
// scripts/verify-ffmpeg-branches.mjs.
//
// It differs from the real server in exactly one way: the destination
// credential store is internal/secrets/secretstest's in-memory fake instead of
// the real OS keychain (internal/secrets.NewKeyringStore). Everything else -
// routing, MediaMTX supervision, branch supervision, SQLite - is identical.
//
// The "integration" build tag is the safety boundary the task requires: this
// file is invisible to `go build ./...`, `go vet ./...`, `go test ./...` and a
// normal `go build ./cmd/server`. It only exists in a binary built with
// `go build -tags integration ./cmd/testserver`, which the verification
// script does itself, in a temporary directory, and only for that script's
// own use. A production environment variable was deliberately not used for
// this: an env var can be set by accident, a build tag cannot.
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

	"github.com/streaming-tree/server/internal/buildinfo"
	"github.com/streaming-tree/server/internal/config"
	"github.com/streaming-tree/server/internal/domain/account"
	"github.com/streaming-tree/server/internal/domain/credential"
	"github.com/streaming-tree/server/internal/domain/output"
	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/httpapi"
	"github.com/streaming-tree/server/internal/provider/twitch"
	"github.com/streaming-tree/server/internal/runtime/branch"
	"github.com/streaming-tree/server/internal/runtime/deviceflow"
	"github.com/streaming-tree/server/internal/runtime/ffmpeg"
	"github.com/streaming-tree/server/internal/runtime/mediamtx"
	"github.com/streaming-tree/server/internal/secrets/secretstest"
	"github.com/streaming-tree/server/internal/storage/sqlite"
)

func main() {
	if err := run(); err != nil {
		slog.Error("test server terminated with an error", slog.Any("error", err))
		os.Exit(1)
	}
}

func run() error {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)
	logger.Warn("running the integration TEST server: credentials are held in an in-memory fake store, not the OS keychain")

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	startedAt := time.Now()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := sqlite.Open(ctx, cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			logger.Error("failed to close the database", slog.Any("error", closeErr))
		}
	}()

	appliedVersions, err := sqlite.Migrate(ctx, db.DB)
	if err != nil {
		return err
	}
	if len(appliedVersions) > 0 {
		logger.Info("applied database migrations", slog.Any("versions", appliedVersions))
	}

	platformService := platform.NewService(sqlite.NewPlatformRepository(db.DB))

	// The one deliberate difference from cmd/server: a single in-memory
	// fake store backs both destination stream keys and connected-account
	// OAuth token bundles, never the real OS keychain.
	secretStore := secretstest.New()
	credentialService := credential.NewService(secretStore)

	outputService := output.NewService(sqlite.NewOutputRepository(db.DB))

	// STREAMING_TREE_TEST_TWITCH_*_BASE_URL exist only in this build-tag-gated
	// binary, read directly rather than through internal/config, so there is
	// no risk of a production config path ever recognizing them. Unset
	// means the real Twitch endpoints, exactly like cmd/server.
	requiredScopes := map[account.ProviderID][]string{account.ProviderTwitch: {twitch.RequiredScope}}
	envClientIDs := map[account.ProviderID]string{}
	if cfg.TwitchClientID != "" {
		envClientIDs[account.ProviderTwitch] = cfg.TwitchClientID
	}
	twitchClient := twitch.New(twitch.Options{
		OAuthBaseURL: os.Getenv("STREAMING_TREE_TEST_TWITCH_OAUTH_BASE_URL"),
		APIBaseURL:   os.Getenv("STREAMING_TREE_TEST_TWITCH_API_BASE_URL"),
	})
	providers := map[account.ProviderID]account.Provider{account.ProviderTwitch: twitch.NewAdapter(twitchClient)}

	accountService := account.NewService(account.Options{
		Repository: sqlite.NewAccountRepository(db.DB), Secrets: secretStore, Providers: providers,
		EnvClientIDs: envClientIDs, RequiredScopes: requiredScopes, Logger: logger,
	})
	accountService.StartValidationWorker(ctx)

	deviceFlowManager := deviceflow.NewManager(deviceflow.Options{
		Accounts: accountService, Providers: providers, RequiredScopes: requiredScopes, Logger: logger,
	})
	deviceFlowManager.Start(ctx)

	twitchMetadataService := twitch.NewMetadataService(accountService, twitchClient)

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

	branchManager := branch.NewManager(branch.Options{
		Platforms:   platformService,
		Outputs:     outputService,
		Credentials: credentialService,
		FFmpeg:      ffmpeg.NewResolver(cfg.FFmpeg.ExecutablePath),
		Ingest:      supervisor,
		Logger:      logger,
	})
	branchManager.Start(ctx)

	handler := httpapi.NewRouter(httpapi.Options{
		Logger:         logger,
		AllowedOrigins: cfg.AllowedOrigins,
		StartedAt:      startedAt,
		Platforms:      platformService,
		Runtime:        supervisor,
		Credentials:    credentialService,
		Outputs:        outputService,
		FFmpegRuntime:  branchManager,
		Branches:       branchManager,
		Accounts:       accountService,
		DeviceFlow:     deviceFlowManager,
		TwitchMetadata: twitchMetadataService,
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
		)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
			return
		}
		serverErrors <- nil
	}()

	select {
	case err := <-serverErrors:
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		branchManager.Shutdown(shutdownCtx)
		deviceFlowManager.Shutdown(shutdownCtx)
		accountService.ShutdownValidationWorker(shutdownCtx)
		supervisor.Shutdown(shutdownCtx)
		cancel()
		return err

	case <-ctx.Done():
		stop()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()

		httpErr := server.Shutdown(shutdownCtx)
		branchManager.Shutdown(shutdownCtx)
		deviceFlowManager.Shutdown(shutdownCtx)
		accountService.ShutdownValidationWorker(shutdownCtx)
		supervisor.Shutdown(shutdownCtx)

		if httpErr != nil {
			if closeErr := server.Close(); closeErr != nil {
				return closeErr
			}
			return httpErr
		}
		return nil
	}
}
