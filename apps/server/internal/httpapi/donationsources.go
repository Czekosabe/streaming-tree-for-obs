package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/streaming-tree/server/internal/domain/donationsource"
	"github.com/streaming-tree/server/internal/runtime/streamelementsengagement"
)

// maxDonationCredentialRequestBodyBytes mirrors maxCredentialRequestBodyBytes
// - a StreamElements JWT is a few hundred bytes; this is a conservative
// ceiling against an accidental oversized paste, well above
// donationsource.MaxCredentialBytes so that bound (not this one) is always
// the one that actually rejects an oversized token.
const maxDonationCredentialRequestBodyBytes = 16 * 1024

// DonationSourceService is the subset of donationsource.Service the HTTP
// layer needs. Declared narrowly on purpose: no method returns a
// credential value, so this layer cannot expose one even by accident.
type DonationSourceService interface {
	List(ctx context.Context) ([]donationsource.Source, error)
	Get(ctx context.Context, id string) (donationsource.Source, bool, error)
	Create(ctx context.Context, in donationsource.CreateInput) (donationsource.Source, error)
	UpdateMetadata(ctx context.Context, id string, in donationsource.UpdateInput) (donationsource.Source, error)
	ReplaceCredential(ctx context.Context, id, token string) error
	CredentialConfigured(ctx context.Context, id string) (bool, error)
	Delete(ctx context.Context, id string) error
}

// DonationEngagementConnectorService is the subset of
// streamelementsengagement.Manager the HTTP layer needs - the donation-
// source twin of EngagementConnectorService/YouTubeEngagementConnectorService.
type DonationEngagementConnectorService interface {
	Enable(ctx context.Context, sourceID string) (streamelementsengagement.Snapshot, error)
	Disable(ctx context.Context, sourceID string) (streamelementsengagement.Snapshot, error)
	Restart(ctx context.Context, sourceID string) (streamelementsengagement.Snapshot, error)
	Snapshot(sourceID string) (streamelementsengagement.Snapshot, bool)
}

// registerDonationSourceRoutes wires the external-donation-source
// management API: safe-metadata CRUD, credential replacement (status-only
// response, mirrors registerCredentialRoutes), and per-source engagement
// connector management (mirrors the /api/connected-accounts/{id}/engagement
// sibling routes engagement.go registers for Twitch/YouTube).
func registerDonationSourceRoutes(mux *http.ServeMux, logger *slog.Logger, sources DonationSourceService, connectors DonationEngagementConnectorService) {
	mux.HandleFunc("GET /api/donation-sources", handleListDonationSources(logger, sources))
	mux.HandleFunc("POST /api/donation-sources", handleCreateDonationSource(logger, sources))
	mux.HandleFunc("/api/donation-sources", methodNotAllowed(logger, http.MethodGet, http.MethodPost))

	mux.HandleFunc("GET /api/donation-sources/{id}", handleGetDonationSource(logger, sources))
	mux.HandleFunc("PUT /api/donation-sources/{id}", handleUpdateDonationSource(logger, sources))
	mux.HandleFunc("DELETE /api/donation-sources/{id}", handleDeleteDonationSource(logger, sources))
	mux.HandleFunc("/api/donation-sources/{id}", methodNotAllowed(logger, http.MethodGet, http.MethodPut, http.MethodDelete))

	mux.HandleFunc("PUT /api/donation-sources/{id}/credential", handleReplaceDonationSourceCredential(logger, sources))
	mux.HandleFunc("/api/donation-sources/{id}/credential", methodNotAllowed(logger, http.MethodPut))

	mux.HandleFunc("GET /api/donation-sources/{id}/engagement", handleGetDonationSourceEngagement(logger, sources, connectors))
	mux.HandleFunc("PUT /api/donation-sources/{id}/engagement", handlePutDonationSourceEngagement(logger, sources, connectors))
	mux.HandleFunc("/api/donation-sources/{id}/engagement", methodNotAllowed(logger, http.MethodGet, http.MethodPut))

	mux.HandleFunc("POST /api/donation-sources/{id}/engagement/restart", handleRestartDonationSourceEngagement(logger, sources, connectors))
	mux.HandleFunc("/api/donation-sources/{id}/engagement/restart", methodNotAllowed(logger, http.MethodPost))
}

// --- response DTOs -------------------------------------------------------

// donationSourceResponse is the API view of a donation source: safe
// metadata plus whether a credential is currently stored - never the
// credential itself. Runtime connection status lives at the sibling
// .../engagement route, mirroring connected accounts.
type donationSourceResponse struct {
	ID                   string `json:"id"`
	ProviderID           string `json:"providerId"`
	Label                string `json:"label"`
	Enabled              bool   `json:"enabled"`
	RemoteChannelID      string `json:"remoteChannelId"`
	CredentialConfigured bool   `json:"credentialConfigured"`
	CreatedAt            string `json:"createdAt"`
	UpdatedAt            string `json:"updatedAt"`
}

func toDonationSourceResponse(src donationsource.Source, credentialConfigured bool) donationSourceResponse {
	return donationSourceResponse{
		ID: src.ID, ProviderID: string(src.ProviderID), Label: src.Label, Enabled: src.Enabled,
		RemoteChannelID: src.RemoteChannelID, CredentialConfigured: credentialConfigured,
		CreatedAt: formatTime(src.CreatedAt), UpdatedAt: formatTime(src.UpdatedAt),
	}
}

type donationConnectorResponse struct {
	SourceID string `json:"sourceId"`
	Enabled  bool   `json:"enabled"`
	State    string `json:"state"`

	ConnectedAt   string `json:"connectedAt,omitempty"`
	LastEventAt   string `json:"lastEventAt,omitempty"`
	LastDataGapAt string `json:"lastDataGapAt,omitempty"`

	ReconnectCount   int `json:"reconnectCount"`
	PossibleGapCount int `json:"possibleGapCount"`

	LastError string `json:"lastError,omitempty"`
}

func toDonationConnectorResponse(s streamelementsengagement.Snapshot) donationConnectorResponse {
	return donationConnectorResponse{
		SourceID: s.SourceID, Enabled: s.Enabled, State: string(s.State),
		ConnectedAt: formatOptionalTime(s.ConnectedAt), LastEventAt: formatOptionalTime(s.LastEventAt),
		LastDataGapAt:  formatOptionalTime(s.LastDataGapAt),
		ReconnectCount: s.ReconnectCount, PossibleGapCount: s.PossibleGapCount,
		LastError: s.LastError,
	}
}

// --- collection ------------------------------------------------------------

func handleListDonationSources(logger *slog.Logger, sources DonationSourceService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := sources.List(r.Context())
		if err != nil {
			writeDonationSourceError(w, logger, r, err)
			return
		}

		items := make([]donationSourceResponse, 0, len(list))
		for _, src := range list {
			configured, err := sources.CredentialConfigured(r.Context(), src.ID)
			if err != nil {
				writeDonationSourceError(w, logger, r, err)
				return
			}
			items = append(items, toDonationSourceResponse(src, configured))
		}
		writeJSON(w, logger, http.StatusOK, map[string]any{"items": items})
	}
}

// createDonationSourceRequest is the accepted create payload. Token is the
// sensitive credential (a StreamElements personal JWT, pasted verbatim) -
// never echoed back, never persisted in the row itself; see
// donationsource.Service.Create's own credential-then-row ordering.
type createDonationSourceRequest struct {
	ProviderID      string `json:"providerId"`
	Label           string `json:"label"`
	RemoteChannelID string `json:"remoteChannelId"`
	Token           string `json:"token"`
}

func handleCreateDonationSource(logger *slog.Logger, sources DonationSourceService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body createDonationSourceRequest
		if err := decodeJSONWithLimit(w, r, &body, maxDonationCredentialRequestBodyBytes); err != nil {
			writeDecodeError(w, logger, err)
			return
		}

		created, err := sources.Create(r.Context(), donationsource.CreateInput{
			ProviderID: donationsource.ProviderID(body.ProviderID), Label: body.Label,
			RemoteChannelID: body.RemoteChannelID, Token: body.Token,
		})
		if err != nil {
			writeDonationSourceError(w, logger, r, err)
			return
		}

		w.Header().Set("Location", "/api/donation-sources/"+created.ID)
		writeJSON(w, logger, http.StatusCreated, toDonationSourceResponse(created, true))
	}
}

// --- single source -----------------------------------------------------------

func requireDonationSource(w http.ResponseWriter, logger *slog.Logger, r *http.Request, sources DonationSourceService, id string) (donationsource.Source, bool) {
	src, found, err := sources.Get(r.Context(), id)
	if err != nil {
		writeDonationSourceError(w, logger, r, err)
		return donationsource.Source{}, false
	}
	if !found {
		writeError(w, logger, http.StatusNotFound, "donation_source_not_found", "No donation source exists with this id.")
		return donationsource.Source{}, false
	}
	return src, true
}

func handleGetDonationSource(logger *slog.Logger, sources DonationSourceService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		src, ok := requireDonationSource(w, logger, r, sources, r.PathValue("id"))
		if !ok {
			return
		}
		configured, err := sources.CredentialConfigured(r.Context(), src.ID)
		if err != nil {
			writeDonationSourceError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toDonationSourceResponse(src, configured))
	}
}

// updateDonationSourceRequest replaces the mutable safe-metadata fields
// only - never the credential (see handleReplaceDonationSourceCredential)
// and never Enabled (see handlePutDonationSourceEngagement), each a
// deliberately separate operation mirroring donationsource.Service's own
// method split.
type updateDonationSourceRequest struct {
	Label           string `json:"label"`
	RemoteChannelID string `json:"remoteChannelId"`
}

func handleUpdateDonationSource(logger *slog.Logger, sources DonationSourceService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var body updateDonationSourceRequest
		if err := decodeJSON(w, r, &body); err != nil {
			writeDecodeError(w, logger, err)
			return
		}

		updated, err := sources.UpdateMetadata(r.Context(), id, donationsource.UpdateInput{
			Label: body.Label, RemoteChannelID: body.RemoteChannelID,
		})
		if err != nil {
			writeDonationSourceError(w, logger, r, err)
			return
		}
		configured, err := sources.CredentialConfigured(r.Context(), id)
		if err != nil {
			writeDonationSourceError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toDonationSourceResponse(updated, configured))
	}
}

func handleDeleteDonationSource(logger *slog.Logger, sources DonationSourceService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := sources.Delete(r.Context(), r.PathValue("id")); err != nil {
			writeDonationSourceError(w, logger, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// --- credential --------------------------------------------------------------

// replaceDonationCredentialRequest is the accepted payload. There is
// exactly one field on purpose: this endpoint rotates one secret, nothing
// else - mirrors setStreamKeyRequest exactly.
type replaceDonationCredentialRequest struct {
	Token string `json:"token"`
}

func handleReplaceDonationSourceCredential(logger *slog.Logger, sources DonationSourceService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if _, ok := requireDonationSource(w, logger, r, sources, id); !ok {
			return
		}

		var body replaceDonationCredentialRequest
		if err := decodeJSONWithLimit(w, r, &body, maxDonationCredentialRequestBodyBytes); err != nil {
			writeDecodeError(w, logger, err)
			return
		}

		if err := sources.ReplaceCredential(r.Context(), id, body.Token); err != nil {
			writeDonationSourceError(w, logger, r, err)
			return
		}

		// ReplaceCredential only returns nil once the store accepted the
		// write, so the resulting status is known without a second store
		// round trip - and without ever echoing the value back.
		writeJSON(w, logger, http.StatusOK, map[string]bool{"configured": true})
	}
}

// --- engagement connector ----------------------------------------------------

func handleGetDonationSourceEngagement(logger *slog.Logger, sources DonationSourceService, connectors DonationEngagementConnectorService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		src, ok := requireDonationSource(w, logger, r, sources, id)
		if !ok {
			return
		}

		snap, ok := connectors.Snapshot(id)
		if !ok {
			state := streamelementsengagement.StateDisabled
			snap = streamelementsengagement.Snapshot{SourceID: id, Enabled: src.Enabled, State: state}
		}
		writeJSON(w, logger, http.StatusOK, toDonationConnectorResponse(snap))
	}
}

type putDonationSourceEngagementRequest struct {
	Enabled bool `json:"enabled"`
}

func handlePutDonationSourceEngagement(logger *slog.Logger, sources DonationSourceService, connectors DonationEngagementConnectorService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if _, ok := requireDonationSource(w, logger, r, sources, id); !ok {
			return
		}

		var body putDonationSourceEngagementRequest
		if err := decodeJSON(w, r, &body); err != nil {
			writeDecodeError(w, logger, err)
			return
		}

		var snap streamelementsengagement.Snapshot
		var err error
		if body.Enabled {
			snap, err = connectors.Enable(r.Context(), id)
		} else {
			snap, err = connectors.Disable(r.Context(), id)
		}
		if err != nil {
			writeDonationEngagementError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toDonationConnectorResponse(snap))
	}
}

func handleRestartDonationSourceEngagement(logger *slog.Logger, sources DonationSourceService, connectors DonationEngagementConnectorService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}
		id := r.PathValue("id")
		if _, ok := requireDonationSource(w, logger, r, sources, id); !ok {
			return
		}

		snap, err := connectors.Restart(r.Context(), id)
		if err != nil {
			writeDonationEngagementError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toDonationConnectorResponse(snap))
	}
}

// --- error mapping ----------------------------------------------------------

func writeDonationSourceError(w http.ResponseWriter, logger *slog.Logger, r *http.Request, err error) {
	switch {
	case errors.Is(err, donationsource.ErrNotFound):
		writeError(w, logger, http.StatusNotFound, "donation_source_not_found", "No donation source exists with this id.")
	case errors.Is(err, donationsource.ErrInvalidProvider):
		writeError(w, logger, http.StatusUnprocessableEntity, "invalid_provider", "This donation service provider is not supported.")
	case errors.Is(err, donationsource.ErrInvalidLabel):
		writeError(w, logger, http.StatusUnprocessableEntity, "invalid_label", "Label is required and must be a reasonable length.")
	case errors.Is(err, donationsource.ErrInvalidRemoteChannelID):
		writeError(w, logger, http.StatusUnprocessableEntity, "invalid_remote_channel_id", "Remote channel id is required and must be a reasonable length.")
	case errors.Is(err, donationsource.ErrCredentialRequired):
		writeError(w, logger, http.StatusUnprocessableEntity, "credential_required", "A credential is required.")
	case errors.Is(err, donationsource.ErrCredentialTooLong):
		writeError(w, logger, http.StatusUnprocessableEntity, "credential_too_long", "The supplied credential is too long.")
	case errors.Is(err, donationsource.ErrConflict):
		writeError(w, logger, http.StatusConflict, "conflict", "The request conflicts with the current state of the resource.")
	case errors.Is(err, donationsource.ErrSecretStoreUnavailable):
		writeError(w, logger, http.StatusServiceUnavailable, "secret_store_unavailable", "The secure credential store is not available right now.")
	default:
		logger.Error("unhandled donation source error",
			slog.String("path", r.URL.Path), slog.String("method", r.Method), slog.Any("error", err))
		writeError(w, logger, http.StatusInternalServerError, "internal_error", "The server encountered an unexpected error.")
	}
}

func writeDonationEngagementError(w http.ResponseWriter, logger *slog.Logger, r *http.Request, err error) {
	switch {
	case errors.Is(err, streamelementsengagement.ErrUnsupportedProvider):
		writeError(w, logger, http.StatusUnprocessableEntity, "engagement_not_supported", "Only StreamElements donation sources support engagement in this stage.")
	case errors.Is(err, streamelementsengagement.ErrNotFound):
		writeError(w, logger, http.StatusNotFound, "engagement_connector_not_found", "No engagement connector is configured for this donation source.")
	case errors.Is(err, donationsource.ErrNotFound):
		writeError(w, logger, http.StatusNotFound, "donation_source_not_found", "No donation source exists with this id.")
	default:
		writeDonationSourceError(w, logger, r, err)
	}
}
