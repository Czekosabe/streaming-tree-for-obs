// Stage 20D2C remote-ingest credential-management API
// (docs/remote-ingest.md §8). Every route here lives under
// /api/remote-ingest/ - never /api/public/* - so it is protected by
// the exact same deny-by-default session/CSRF/Origin middleware
// (withRemoteManagementSecurity) every other authenticated management
// route already uses; nothing in this file adds a second security
// check, because none is needed beyond what already applies to every
// non-public /api/ route.
package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/streaming-tree/server/internal/remoteingest"
)

// RemoteIngestService is the subset of *remoteingest.Manager the
// handlers need - declared as an interface so handler tests can drive
// a stub instead of a real MediaMTX supervisor and secret store.
type RemoteIngestService interface {
	Status(ctx context.Context) (bool, error)
	IngestReceiving() bool
	Provision(ctx context.Context) (string, error)
	Rotate(ctx context.Context) (string, error)
	Revoke(ctx context.Context) error
}

// registerRemoteIngestRoutes wires the Stage 20D2C credential-
// management API. Registered only when service is non-nil (only when
// --remote-ingest is active - see router.go's own NewRouter), the same
// nil-means-not-registered convention every other optional route group
// already follows.
func registerRemoteIngestRoutes(mux *http.ServeMux, logger *slog.Logger, service RemoteIngestService, rtmpsAddress, ingestPath string) {
	mux.HandleFunc("GET /api/remote-ingest/status", handleGetRemoteIngestStatus(logger, service, rtmpsAddress, ingestPath))
	mux.HandleFunc("/api/remote-ingest/status", methodNotAllowed(logger, http.MethodGet))

	mux.HandleFunc("POST /api/remote-ingest/provision", handleRemoteIngestProvision(logger, service))
	mux.HandleFunc("/api/remote-ingest/provision", methodNotAllowed(logger, http.MethodPost))

	mux.HandleFunc("POST /api/remote-ingest/rotate", handleRemoteIngestRotate(logger, service))
	mux.HandleFunc("/api/remote-ingest/rotate", methodNotAllowed(logger, http.MethodPost))

	mux.HandleFunc("POST /api/remote-ingest/revoke", handleRemoteIngestRevoke(logger, service))
	mux.HandleFunc("/api/remote-ingest/revoke", methodNotAllowed(logger, http.MethodPost))
}

// remoteIngestStatusSchemaVersion is the current shape of GET
// /api/remote-ingest/status.
const remoteIngestStatusSchemaVersion = 1

type remoteIngestStatusResponse struct {
	Version      int    `json:"version"`
	Configured   bool   `json:"configured"`
	Receiving    bool   `json:"receiving"`
	RTMPSAddress string `json:"rtmpsAddress"`
	IngestPath   string `json:"ingestPath"`
}

func handleGetRemoteIngestStatus(logger *slog.Logger, service RemoteIngestService, rtmpsAddress, ingestPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		configured, err := service.Status(r.Context())
		if err != nil {
			writeRemoteIngestError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, remoteIngestStatusResponse{
			Version:      remoteIngestStatusSchemaVersion,
			Configured:   configured,
			Receiving:    service.IngestReceiving(),
			RTMPSAddress: rtmpsAddress,
			IngestPath:   ingestPath,
		})
	}
}

// remoteIngestSecretResponse carries the new plaintext secret exactly
// once (docs/remote-ingest.md §6/§8) - the frontend must display and
// discard it, never persist it client-side beyond the active response
// lifecycle.
type remoteIngestSecretResponse struct {
	Version int    `json:"version"`
	Secret  string `json:"secret"`
}

func handleRemoteIngestProvision(logger *slog.Logger, service RemoteIngestService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}
		secret, err := service.Provision(r.Context())
		if err != nil {
			writeRemoteIngestError(w, logger, r, err)
			return
		}
		writeRemoteIngestSecret(w, logger, secret)
	}
}

func handleRemoteIngestRotate(logger *slog.Logger, service RemoteIngestService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}
		secret, err := service.Rotate(r.Context())
		if err != nil {
			writeRemoteIngestError(w, logger, r, err)
			return
		}
		writeRemoteIngestSecret(w, logger, secret)
	}
}

func handleRemoteIngestRevoke(logger *slog.Logger, service RemoteIngestService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}
		if err := service.Revoke(r.Context()); err != nil {
			writeRemoteIngestError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, map[string]string{"status": "revoked"})
	}
}

// writeRemoteIngestSecret writes the one-time plaintext secret
// response with Cache-Control: no-store (docs/remote-ingest.md §8) -
// never cached by an intermediate proxy or the browser's own disk
// cache.
func writeRemoteIngestSecret(w http.ResponseWriter, logger *slog.Logger, secret string) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, logger, http.StatusOK, remoteIngestSecretResponse{
		Version: remoteIngestStatusSchemaVersion,
		Secret:  secret,
	})
}

// writeRemoteIngestError maps a Manager failure onto the HTTP
// contract - never logs or includes the secret (none of these error
// paths ever have one to leak).
func writeRemoteIngestError(w http.ResponseWriter, logger *slog.Logger, r *http.Request, err error) {
	switch {
	case errors.Is(err, remoteingest.ErrAlreadyProvisioned):
		writeError(w, logger, http.StatusConflict, "already_provisioned",
			"A remote ingest credential is already provisioned. Use rotate instead.")

	case errors.Is(err, remoteingest.ErrStreamingActive):
		writeError(w, logger, http.StatusConflict, "streaming_active",
			"The remote ingest credential cannot change while a stream is actively being received.")

	default:
		logger.Error("unhandled remote ingest error",
			slog.String("path", r.URL.Path), slog.Any("error", err))
		writeError(w, logger, http.StatusInternalServerError, "internal_error",
			"The server encountered an unexpected error.")
	}
}
