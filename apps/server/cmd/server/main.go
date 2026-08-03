// Command server starts the Streaming Tree REST API.
//
// The current build exposes a single endpoint (GET /api/health). Stream
// routing, MediaMTX supervision, FFmpeg branches, persistence and credential
// storage are separate, later stages - none of them are started here.
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
	"github.com/streaming-tree/server/internal/httpapi"
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

	handler := httpapi.NewRouter(httpapi.Options{
		Logger:         logger,
		AllowedOrigins: cfg.AllowedOrigins,
		StartedAt:      startedAt,
	})

	server := &http.Server{
		Addr:              cfg.Address(),
		Handler:           handler,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	// Cancelled on Ctrl+C or SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

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
		return err

	case <-ctx.Done():
		logger.Info("shutdown signal received, draining connections",
			slog.Duration("timeout", cfg.ShutdownTimeout))

		// Stop intercepting signals: a second Ctrl+C should kill the process
		// immediately instead of waiting for the drain to finish.
		stop()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("graceful shutdown failed, closing forcefully",
				slog.Any("error", err))
			if closeErr := server.Close(); closeErr != nil {
				return closeErr
			}
			return err
		}

		logger.Info("server stopped cleanly")
		return nil
	}
}
