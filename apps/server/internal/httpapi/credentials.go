package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/streaming-tree/server/internal/domain/credential"
	"github.com/streaming-tree/server/internal/domain/platform"
)

// maxCredentialRequestBodyBytes is deliberately far smaller than
// maxRequestBodyBytes: a stream key is never remotely as large as platform
// metadata, so a small explicit ceiling makes an oversized body an obvious
// mistake or probe rather than something that has to track future metadata
// growth.
const maxCredentialRequestBodyBytes = 8 * 1024

// CredentialService is the subset of the credential domain service the HTTP
// layer needs.
//
// It is declared narrowly on purpose: it has no method that returns a secret
// value, so this layer cannot expose one even by accident. See
// credential.Service.RetrieveForProcessStart, which exists on the concrete
// service for the future FFmpeg stage and is deliberately absent here.
type CredentialService interface {
	Status(ctx context.Context, platformID string) (credential.Status, credential.StoreStatus, error)
	SetStreamKey(ctx context.Context, platformID, rawValue string) error
	DeleteStreamKey(ctx context.Context, platformID string) error
	DeletePlatformCredentials(ctx context.Context, platformID string) error
}

// credentialStatusResponse is the only shape a credential ever takes in an
// API response: presence, never value. Fields are grouped by credential type
// so more secret types (an OAuth token, eventually) can be added as siblings
// of streamKey without changing this envelope.
type credentialStatusResponse struct {
	StreamKey streamKeyStatusResponse `json:"streamKey"`
	Store     storeStatusResponse     `json:"store"`
}

type streamKeyStatusResponse struct {
	Configured bool `json:"configured"`
}

type storeStatusResponse struct {
	Available bool `json:"available"`
}

// requirePlatform answers 404 platform_not_found and reports false when the
// platform does not exist, so no credential handler ever runs a store
// operation against an ID that was never configured. It reports true only
// when the caller may proceed.
func requirePlatform(w http.ResponseWriter, logger *slog.Logger, r *http.Request, platforms PlatformService, id string) bool {
	if _, err := platforms.Get(r.Context(), id); err != nil {
		if errors.Is(err, platform.ErrNotFound) {
			writeError(w, logger, http.StatusNotFound,
				"platform_not_found", "The requested platform does not exist.")
			return false
		}
		writeDomainError(w, logger, r, err)
		return false
	}
	return true
}

// --- credential status --------------------------------------------------

func handleGetCredentials(logger *slog.Logger, platforms PlatformService, credentials CredentialService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !requirePlatform(w, logger, r, platforms, id) {
			return
		}

		status, store, err := credentials.Status(r.Context(), id)
		if err != nil {
			writeCredentialError(w, logger, r, err)
			return
		}

		writeJSON(w, logger, http.StatusOK, credentialStatusResponse{
			StreamKey: streamKeyStatusResponse{Configured: status.Configured},
			Store:     storeStatusResponse{Available: store.Available},
		})
	}
}

// --- stream key -----------------------------------------------------------

// setStreamKeyRequest is the accepted payload. There is exactly one field on
// purpose: this endpoint sets one secret, nothing else.
type setStreamKeyRequest struct {
	StreamKey string `json:"streamKey"`
}

func handleSetStreamKey(logger *slog.Logger, platforms PlatformService, credentials CredentialService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !requirePlatform(w, logger, r, platforms, id) {
			return
		}

		var body setStreamKeyRequest
		if err := decodeJSONWithLimit(w, r, &body, maxCredentialRequestBodyBytes); err != nil {
			writeDecodeError(w, logger, err)
			return
		}

		if err := credentials.SetStreamKey(r.Context(), id, body.StreamKey); err != nil {
			writeCredentialError(w, logger, r, err)
			return
		}

		// SetStreamKey only returns nil once the store accepted the write,
		// so the resulting status is known without a second store round
		// trip - and without ever echoing the value back.
		writeJSON(w, logger, http.StatusOK, credentialStatusResponse{
			StreamKey: streamKeyStatusResponse{Configured: true},
			Store:     storeStatusResponse{Available: true},
		})
	}
}

func handleDeleteStreamKey(logger *slog.Logger, platforms PlatformService, credentials CredentialService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !requirePlatform(w, logger, r, platforms, id) {
			return
		}

		if err := credentials.DeleteStreamKey(r.Context(), id); err != nil {
			writeCredentialError(w, logger, r, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// --- platform deletion cascade --------------------------------------------

// handleDeletePlatformWithCredentials deletes a platform's branch (if any is
// tracked) and its credentials before deleting the platform row itself, so a
// platform is never removed while cleanup of its own credential is in a
// confirmed-failed state, and no branch entry lingers for an id that no
// longer exists.
//
// branches may be nil (no branch manager wired, e.g. in tests that do not
// need it); credentials may not, since this function is only ever registered
// when it is non-nil - see router.go.
//
// See credential.Service.DeletePlatformCredentials for the ordering and
// failure policy this depends on, including the one case (an unreachable
// store) where cleanup is treated as best-effort and platform deletion
// proceeds anyway.
func handleDeletePlatformWithCredentials(
	logger *slog.Logger, platforms PlatformService, credentials CredentialService, branches BranchRuntimeService,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !requirePlatform(w, logger, r, platforms, id) {
			return
		}

		if branches != nil {
			branches.Forget(r.Context(), id)
		}

		if err := credentials.DeletePlatformCredentials(r.Context(), id); err != nil {
			writeCredentialError(w, logger, r, err)
			return
		}

		if err := platforms.Delete(r.Context(), id); err != nil {
			writeDomainError(w, logger, r, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// --- error mapping ----------------------------------------------------------

// writeCredentialError maps a credential domain error onto the HTTP
// contract. Store-failure causes are logged server-side only: the client
// response never carries more than a stable code and a generic message.
func writeCredentialError(w http.ResponseWriter, logger *slog.Logger, r *http.Request, err error) {
	if verr, ok := platform.AsValidationError(err); ok {
		writeValidationError(w, logger, verr)
		return
	}

	switch {
	case errors.Is(err, credential.ErrCredentialNotFound):
		writeError(w, logger, http.StatusNotFound,
			"credential_not_found", "No credential is stored for this platform.")

	case errors.Is(err, credential.ErrStoreUnavailable):
		writeError(w, logger, http.StatusServiceUnavailable,
			"credential_store_unavailable", "The secure credential store is not available right now.")

	case errors.Is(err, credential.ErrStoreFailure):
		logger.Error("credential store failure",
			slog.String("path", r.URL.Path),
			slog.String("method", r.Method),
			slog.Any("error", err),
		)
		writeError(w, logger, http.StatusInternalServerError,
			"credential_store_failure", "The credential store could not complete the request.")

	default:
		logger.Error("unhandled credential error",
			slog.String("path", r.URL.Path),
			slog.String("method", r.Method),
			slog.Any("error", err),
		)
		writeError(w, logger, http.StatusInternalServerError,
			"internal_error", "The server encountered an unexpected error.")
	}
}
