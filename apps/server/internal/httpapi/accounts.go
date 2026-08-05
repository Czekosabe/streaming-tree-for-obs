package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/streaming-tree/server/internal/domain/account"
	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/provider/twitch"
	"github.com/streaming-tree/server/internal/runtime/deviceflow"
)

// AccountService is the subset of account.Service the HTTP layer needs.
type AccountService interface {
	IntegrationConfig(ctx context.Context, providerID account.ProviderID) (account.IntegrationConfig, error)
	SetIntegrationClientID(ctx context.Context, providerID account.ProviderID, clientID string) (account.IntegrationConfig, error)
	ListAccounts(ctx context.Context) ([]account.Account, error)
	GetAccount(ctx context.Context, id string) (account.Account, error)
	ValidateNow(ctx context.Context, accountID string) (account.Account, error)
	Disconnect(ctx context.Context, accountID string) error
	GetLink(ctx context.Context, platformID string) (account.Link, bool, error)
	LinkPlatform(ctx context.Context, platformID, platformProviderID, accountID string) (account.Link, error)
	UnlinkPlatform(ctx context.Context, platformID string) error
}

// DeviceFlowService is the subset of deviceflow.Manager the HTTP layer needs.
type DeviceFlowService interface {
	StartAttempt(ctx context.Context, providerID account.ProviderID, reconnectAccountID string) (deviceflow.Snapshot, error)
	// StartAttemptWithScopes is used only by the Stage 8A engagement
	// permission-upgrade endpoint (see engagement.go) - every other caller
	// uses StartAttempt's default per-provider scope set.
	StartAttemptWithScopes(ctx context.Context, providerID account.ProviderID, reconnectAccountID string, scopes []string) (deviceflow.Snapshot, error)
	GetAttempt(attemptID string) (deviceflow.Snapshot, error)
	CancelAttempt(attemptID string) (deviceflow.Snapshot, error)
}

// TwitchMetadataService is the subset of twitch.MetadataService the HTTP
// layer needs.
type TwitchMetadataService interface {
	SearchCategories(ctx context.Context, accountID, query string) ([]twitch.Category, error)
	Preview(ctx context.Context, platformProviderID string, local platform.Metadata, link account.Link, linked bool) (twitch.Preview, error)
	Publish(ctx context.Context, platformProviderID string, local platform.Metadata, link account.Link, linked bool, now time.Time) (twitch.PublishResult, []string, error)
}

// registerAccountRoutes wires the connected-account, device-flow, provider
// integration-config, account-link, category-search and metadata-publish
// API.
func registerAccountRoutes(
	mux *http.ServeMux, logger *slog.Logger,
	platforms PlatformService, accounts AccountService, deviceFlow DeviceFlowService, twitchMetadata TwitchMetadataService,
	youtubeAuth YouTubeAuthService, youtubeMetadata YouTubeMetadataService, remoteTargets RemoteTargetService,
) {
	mux.HandleFunc("GET /api/integrations/twitch/config", handleGetIntegrationConfig(logger, accounts))
	mux.HandleFunc("PUT /api/integrations/twitch/config", handleSetIntegrationConfig(logger, accounts))
	mux.HandleFunc("/api/integrations/twitch/config", methodNotAllowed(logger, http.MethodGet, http.MethodPut))

	mux.HandleFunc("POST /api/integrations/twitch/device-flow", handleStartDeviceFlow(logger, deviceFlow))
	mux.HandleFunc("/api/integrations/twitch/device-flow", methodNotAllowed(logger, http.MethodPost))

	mux.HandleFunc("GET /api/integrations/twitch/device-flow/{id}", handleGetDeviceFlow(logger, deviceFlow))
	mux.HandleFunc("DELETE /api/integrations/twitch/device-flow/{id}", handleCancelDeviceFlow(logger, deviceFlow))
	mux.HandleFunc("/api/integrations/twitch/device-flow/{id}",
		methodNotAllowed(logger, http.MethodGet, http.MethodDelete))

	mux.HandleFunc("GET /api/connected-accounts", handleListAccounts(logger, accounts))
	mux.HandleFunc("/api/connected-accounts", methodNotAllowed(logger, http.MethodGet))

	mux.HandleFunc("GET /api/connected-accounts/{id}", handleGetAccount(logger, accounts))
	mux.HandleFunc("DELETE /api/connected-accounts/{id}", handleDisconnectAccount(logger, accounts))
	mux.HandleFunc("/api/connected-accounts/{id}",
		methodNotAllowed(logger, http.MethodGet, http.MethodDelete))

	mux.HandleFunc("POST /api/connected-accounts/{id}/validate", handleValidateAccount(logger, accounts))
	mux.HandleFunc("/api/connected-accounts/{id}/validate", methodNotAllowed(logger, http.MethodPost))

	mux.HandleFunc("POST /api/connected-accounts/{id}/reconnect", handleReconnectAccount(logger, accounts, deviceFlow, youtubeAuth))
	mux.HandleFunc("/api/connected-accounts/{id}/reconnect", methodNotAllowed(logger, http.MethodPost))

	mux.HandleFunc("GET /api/connected-accounts/{id}/twitch/categories", handleSearchCategories(logger, accounts, twitchMetadata))
	mux.HandleFunc("/api/connected-accounts/{id}/twitch/categories", methodNotAllowed(logger, http.MethodGet))

	mux.HandleFunc("GET /api/platforms/{id}/connected-account", handleGetPlatformAccountLink(logger, platforms, accounts))
	mux.HandleFunc("PUT /api/platforms/{id}/connected-account", handleSetPlatformAccountLink(logger, platforms, accounts))
	mux.HandleFunc("DELETE /api/platforms/{id}/connected-account", handleDeletePlatformAccountLink(logger, platforms, accounts))
	mux.HandleFunc("/api/platforms/{id}/connected-account",
		methodNotAllowed(logger, http.MethodGet, http.MethodPut, http.MethodDelete))

	mux.HandleFunc("GET /api/platforms/{id}/metadata/publish-preview", handlePublishPreview(logger, platforms, accounts, twitchMetadata, youtubeMetadata, remoteTargets))
	mux.HandleFunc("/api/platforms/{id}/metadata/publish-preview", methodNotAllowed(logger, http.MethodGet))

	mux.HandleFunc("POST /api/platforms/{id}/metadata/publish", handlePublishMetadata(logger, platforms, accounts, twitchMetadata, youtubeMetadata, remoteTargets))
	mux.HandleFunc("/api/platforms/{id}/metadata/publish", methodNotAllowed(logger, http.MethodPost))
}

// --- response shapes -----------------------------------------------------

type integrationConfigResponse struct {
	Configured bool   `json:"configured"`
	Source     string `json:"source"`
	ClientID   string `json:"clientId,omitempty"`
}

func toIntegrationConfigResponse(cfg account.IntegrationConfig) integrationConfigResponse {
	resp := integrationConfigResponse{Configured: cfg.Configured, Source: string(cfg.Source)}
	// The Client ID is not a secret and is returned so a database-managed
	// value can be edited - but never when it came from the environment,
	// which the frontend must never be able to overwrite (see the doc
	// comment on account.SourceEnvironment).
	if cfg.Source == account.SourceDatabase {
		resp.ClientID = cfg.ClientID
	}
	return resp
}

type setIntegrationConfigRequest struct {
	ClientID string `json:"clientId"`
}

func handleGetIntegrationConfig(logger *slog.Logger, accounts AccountService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg, err := accounts.IntegrationConfig(r.Context(), account.ProviderTwitch)
		if err != nil {
			writeAccountError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toIntegrationConfigResponse(cfg))
	}
}

func handleSetIntegrationConfig(logger *slog.Logger, accounts AccountService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body setIntegrationConfigRequest
		if err := decodeJSON(w, r, &body); err != nil {
			writeDecodeError(w, logger, err)
			return
		}

		cfg, err := accounts.SetIntegrationClientID(r.Context(), account.ProviderTwitch, body.ClientID)
		if err != nil {
			writeAccountError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toIntegrationConfigResponse(cfg))
	}
}

// --- device flow -----------------------------------------------------------

type deviceFlowResponse struct {
	AttemptID          string `json:"attemptId"`
	ProviderID         string `json:"providerId"`
	State              string `json:"state"`
	UserCode           string `json:"userCode,omitempty"`
	VerificationURI    string `json:"verificationUri,omitempty"`
	CreatedAt          string `json:"createdAt"`
	ExpiresAt          string `json:"expiresAt,omitempty"`
	IntervalSeconds    int    `json:"intervalSeconds,omitempty"`
	ConnectedAccountID string `json:"connectedAccountId,omitempty"`
	ErrorCode          string `json:"errorCode,omitempty"`
	ErrorMessage       string `json:"errorMessage,omitempty"`
}

func toDeviceFlowResponse(s deviceflow.Snapshot) deviceFlowResponse {
	resp := deviceFlowResponse{
		AttemptID: s.AttemptID, ProviderID: string(s.ProviderID), State: string(s.State),
		UserCode: s.UserCode, VerificationURI: s.VerificationURI,
		CreatedAt:          s.CreatedAt.UTC().Format(time.RFC3339Nano),
		ConnectedAccountID: s.ConnectedAccountID, ErrorCode: s.ErrorCode, ErrorMessage: s.ErrorMessage,
	}
	if !s.ExpiresAt.IsZero() {
		resp.ExpiresAt = s.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	if s.Interval > 0 {
		resp.IntervalSeconds = int(s.Interval / time.Second)
	}
	return resp
}

func handleStartDeviceFlow(logger *slog.Logger, deviceFlow DeviceFlowService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}
		snapshot, err := deviceFlow.StartAttempt(r.Context(), account.ProviderTwitch, "")
		if err != nil {
			writeDeviceFlowError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusAccepted, toDeviceFlowResponse(snapshot))
	}
}

func handleGetDeviceFlow(logger *slog.Logger, deviceFlow DeviceFlowService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		snapshot, err := deviceFlow.GetAttempt(r.PathValue("id"))
		if err != nil {
			writeDeviceFlowError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toDeviceFlowResponse(snapshot))
	}
}

func handleCancelDeviceFlow(logger *slog.Logger, deviceFlow DeviceFlowService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}
		snapshot, err := deviceFlow.CancelAttempt(r.PathValue("id"))
		if err != nil {
			writeDeviceFlowError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toDeviceFlowResponse(snapshot))
	}
}

// --- connected accounts ------------------------------------------------

type accountResponse struct {
	ID              string   `json:"id"`
	ProviderID      string   `json:"providerId"`
	Login           string   `json:"login"`
	DisplayName     string   `json:"displayName"`
	AvatarURL       string   `json:"avatarUrl,omitempty"`
	Status          string   `json:"status"`
	Scopes          []string `json:"scopes"`
	LastValidatedAt string   `json:"lastValidatedAt,omitempty"`
	CreatedAt       string   `json:"createdAt"`
	UpdatedAt       string   `json:"updatedAt"`
}

func toAccountResponse(a account.Account) accountResponse {
	scopes := a.Scopes
	if scopes == nil {
		scopes = []string{}
	}
	resp := accountResponse{
		ID: a.ID, ProviderID: string(a.ProviderID), Login: a.Login, DisplayName: a.DisplayName,
		AvatarURL: a.AvatarURL, Status: string(a.Status), Scopes: scopes,
		CreatedAt: a.CreatedAt.UTC().Format(time.RFC3339Nano), UpdatedAt: a.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	if a.LastValidatedAt != nil {
		resp.LastValidatedAt = a.LastValidatedAt.UTC().Format(time.RFC3339Nano)
	}
	return resp
}

func handleListAccounts(logger *slog.Logger, accounts AccountService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := accounts.ListAccounts(r.Context())
		if err != nil {
			writeAccountError(w, logger, r, err)
			return
		}
		out := make([]accountResponse, 0, len(list))
		for _, a := range list {
			out = append(out, toAccountResponse(a))
		}
		writeJSON(w, logger, http.StatusOK, map[string]any{"accounts": out})
	}
}

func handleGetAccount(logger *slog.Logger, accounts AccountService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acc, err := accounts.GetAccount(r.Context(), r.PathValue("id"))
		if err != nil {
			writeAccountError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toAccountResponse(acc))
	}
}

func handleValidateAccount(logger *slog.Logger, accounts AccountService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}
		acc, err := accounts.ValidateNow(r.Context(), r.PathValue("id"))
		if err != nil {
			writeAccountError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toAccountResponse(acc))
	}
}

// handleReconnectAccount dispatches to the provider's own OAuth attempt
// manager: a Twitch account gets a new device-flow attempt, a YouTube
// account gets a new Authorization Code + PKCE attempt - see
// docs/provider-integrations/youtube.md's "Why Twitch's Device Code Flow is
// not reused" section for why these are two different attempt shapes
// rather than one forced into the other's model.
func handleReconnectAccount(logger *slog.Logger, accounts AccountService, deviceFlow DeviceFlowService, youtubeAuth YouTubeAuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}
		id := r.PathValue("id")
		acc, err := accounts.GetAccount(r.Context(), id)
		if err != nil {
			writeAccountError(w, logger, r, err)
			return
		}

		if acc.ProviderID == account.ProviderYouTube && youtubeAuth != nil {
			snapshot, err := youtubeAuth.StartAttempt(r.Context(), id)
			if err != nil {
				writeYouTubeAuthError(w, logger, r, err)
				return
			}
			writeJSON(w, logger, http.StatusAccepted, toOAuthAttemptResponse(snapshot))
			return
		}

		snapshot, err := deviceFlow.StartAttempt(r.Context(), acc.ProviderID, id)
		if err != nil {
			writeDeviceFlowError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusAccepted, toDeviceFlowResponse(snapshot))
	}
}

func handleDisconnectAccount(logger *slog.Logger, accounts AccountService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := accounts.Disconnect(r.Context(), r.PathValue("id")); err != nil {
			writeAccountError(w, logger, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// --- category search -------------------------------------------------------

const (
	categoryQueryMinLength = 2
	categoryQueryMaxLength = 100
)

type categoryItemResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	BoxArtURL string `json:"boxArtUrl,omitempty"`
}

func handleSearchCategories(logger *slog.Logger, accounts AccountService, twitchMetadata TwitchMetadataService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID := r.PathValue("id")
		if _, err := accounts.GetAccount(r.Context(), accountID); err != nil {
			writeAccountError(w, logger, r, err)
			return
		}

		query := strings.TrimSpace(r.URL.Query().Get("query"))
		length := utf8.RuneCountInString(query)
		if length < categoryQueryMinLength || length > categoryQueryMaxLength {
			writeError(w, logger, http.StatusUnprocessableEntity, "invalid_query",
				"The search query must be between 2 and 100 characters.")
			return
		}

		results, err := twitchMetadata.SearchCategories(r.Context(), accountID, query)
		if err != nil {
			writeTwitchError(w, logger, r, err)
			return
		}

		items := make([]categoryItemResponse, 0, len(results))
		for _, c := range results {
			items = append(items, categoryItemResponse{ID: c.ID, Name: c.Name, BoxArtURL: c.BoxArtURL})
		}
		writeJSON(w, logger, http.StatusOK, map[string]any{"items": items})
	}
}

// --- platform account links -------------------------------------------

type linkResponse struct {
	PlatformID string `json:"platformId"`
	AccountID  string `json:"accountId"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
}

func toLinkResponse(l account.Link) linkResponse {
	return linkResponse{
		PlatformID: l.PlatformID, AccountID: l.AccountID,
		CreatedAt: l.CreatedAt.UTC().Format(time.RFC3339Nano), UpdatedAt: l.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func handleGetPlatformAccountLink(logger *slog.Logger, platforms PlatformService, accounts AccountService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !requirePlatform(w, logger, r, platforms, id) {
			return
		}
		link, found, err := accounts.GetLink(r.Context(), id)
		if err != nil {
			writeAccountError(w, logger, r, err)
			return
		}
		if !found {
			writeJSON(w, logger, http.StatusOK, nil)
			return
		}
		writeJSON(w, logger, http.StatusOK, toLinkResponse(link))
	}
}

type setLinkRequest struct {
	AccountID string `json:"accountId"`
}

func handleSetPlatformAccountLink(logger *slog.Logger, platforms PlatformService, accounts AccountService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		p, err := platforms.Get(r.Context(), id)
		if err != nil {
			writeDomainError(w, logger, r, err)
			return
		}

		var body setLinkRequest
		if err := decodeJSON(w, r, &body); err != nil {
			writeDecodeError(w, logger, err)
			return
		}
		if strings.TrimSpace(body.AccountID) == "" {
			writeError(w, logger, http.StatusUnprocessableEntity, "validation_failed", "accountId is required.")
			return
		}

		link, err := accounts.LinkPlatform(r.Context(), id, string(p.ProviderID), body.AccountID)
		if err != nil {
			writeAccountError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toLinkResponse(link))
	}
}

func handleDeletePlatformAccountLink(logger *slog.Logger, platforms PlatformService, accounts AccountService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !requirePlatform(w, logger, r, platforms, id) {
			return
		}
		if err := accounts.UnlinkPlatform(r.Context(), id); err != nil {
			writeAccountError(w, logger, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// --- metadata publish --------------------------------------------------

type fieldDiffResponse struct {
	Field   string `json:"field"`
	Local   string `json:"local"`
	Remote  string `json:"remote"`
	Changed bool   `json:"changed"`
}

type publishPreviewResponse struct {
	ProviderID     string              `json:"providerId"`
	AccountID      string              `json:"accountId,omitempty"`
	AccountLogin   string              `json:"accountLogin,omitempty"`
	BroadcastID    string              `json:"broadcastId,omitempty"`
	BroadcastTitle string              `json:"broadcastTitle,omitempty"`
	Fields         []fieldDiffResponse `json:"fields"`
	Skipped        []string            `json:"skipped"`
	Blockers       []string            `json:"blockers"`
	Warnings       []string            `json:"warnings,omitempty"`
	Allowed        bool                `json:"allowed"`
}

func loadLinkAndMetadataForPublish(r *http.Request, platforms PlatformService, accounts AccountService) (platform.Platform, account.Link, bool, error) {
	id := r.PathValue("id")
	p, err := platforms.Get(r.Context(), id)
	if err != nil {
		return platform.Platform{}, account.Link{}, false, err
	}
	link, found, err := accounts.GetLink(r.Context(), id)
	if err != nil {
		return platform.Platform{}, account.Link{}, false, err
	}
	return p, link, found, nil
}

// handlePublishPreview dispatches to the platform's own provider metadata
// service. A provider with no metadata adapter at all (Kick, TikTok) always
// reports account_not_linked, matching the pre-existing Twitch-only
// behavior for those providers.
func handlePublishPreview(
	logger *slog.Logger, platforms PlatformService, accounts AccountService,
	twitchMetadata TwitchMetadataService, youtubeMetadata YouTubeMetadataService, remoteTargets RemoteTargetService,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, link, found, err := loadLinkAndMetadataForPublish(r, platforms, accounts)
		if err != nil {
			writeDomainError(w, logger, r, err)
			return
		}

		if p.ProviderID == platform.ProviderYouTube && youtubeMetadata != nil && remoteTargets != nil {
			target, hasTarget, err := remoteTargets.GetTarget(r.Context(), p.ID)
			if err != nil {
				writeDomainError(w, logger, r, err)
				return
			}
			preview, err := youtubeMetadata.Preview(r.Context(), string(p.ProviderID), p.Metadata, link, found, target, hasTarget)
			if err != nil {
				writeYouTubeError(w, logger, r, err)
				return
			}
			fields := make([]fieldDiffResponse, 0, len(preview.Fields))
			for _, f := range preview.Fields {
				fields = append(fields, fieldDiffResponse{Field: f.Field, Local: f.Local, Remote: f.Remote, Changed: f.Changed})
			}
			writeJSON(w, logger, http.StatusOK, publishPreviewResponse{
				ProviderID: string(p.ProviderID), AccountID: preview.AccountID, AccountLogin: preview.AccountLogin,
				BroadcastID: preview.BroadcastID, BroadcastTitle: preview.BroadcastTitle,
				Fields: fields, Skipped: preview.Skipped, Blockers: preview.Blockers, Warnings: preview.Warnings, Allowed: preview.Allowed,
			})
			return
		}

		preview, err := twitchMetadata.Preview(r.Context(), string(p.ProviderID), p.Metadata, link, found)
		if err != nil {
			writeTwitchError(w, logger, r, err)
			return
		}

		fields := make([]fieldDiffResponse, 0, len(preview.Fields))
		for _, f := range preview.Fields {
			fields = append(fields, fieldDiffResponse{Field: f.Field, Local: f.Local, Remote: f.Remote, Changed: f.Changed})
		}
		writeJSON(w, logger, http.StatusOK, publishPreviewResponse{
			ProviderID: string(p.ProviderID), AccountID: preview.AccountID, AccountLogin: preview.AccountLogin,
			Fields: fields, Skipped: preview.Skipped, Blockers: preview.Blockers, Allowed: preview.Allowed,
		})
	}
}

type publishResultResponse struct {
	Status        string   `json:"status"`
	AccountID     string   `json:"accountId,omitempty"`
	BroadcastID   string   `json:"broadcastId,omitempty"`
	PublishedAt   string   `json:"publishedAt,omitempty"`
	FieldsChanged []string `json:"fieldsChanged,omitempty"`
	FieldsSkipped []string `json:"fieldsSkipped,omitempty"`
	FieldsFailed  []string `json:"fieldsFailed,omitempty"`
	Warnings      []string `json:"warnings,omitempty"`
	Blockers      []string `json:"blockers,omitempty"`
}

func handlePublishMetadata(
	logger *slog.Logger, platforms PlatformService, accounts AccountService,
	twitchMetadata TwitchMetadataService, youtubeMetadata YouTubeMetadataService, remoteTargets RemoteTargetService,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}
		p, link, found, err := loadLinkAndMetadataForPublish(r, platforms, accounts)
		if err != nil {
			writeDomainError(w, logger, r, err)
			return
		}

		if p.ProviderID == platform.ProviderYouTube && youtubeMetadata != nil && remoteTargets != nil {
			target, hasTarget, err := remoteTargets.GetTarget(r.Context(), p.ID)
			if err != nil {
				writeDomainError(w, logger, r, err)
				return
			}
			result, blockers, err := youtubeMetadata.Publish(r.Context(), string(p.ProviderID), p.Metadata, link, found, target, hasTarget, time.Now().UTC())
			if err != nil {
				writeYouTubeError(w, logger, r, err)
				return
			}
			if len(blockers) > 0 {
				writeJSON(w, logger, http.StatusOK, publishResultResponse{Status: "blocked", Blockers: blockers})
				return
			}
			writeJSON(w, logger, http.StatusOK, publishResultResponse{
				Status: "published", AccountID: result.AccountID, BroadcastID: result.BroadcastID,
				PublishedAt:   result.PublishedAt.UTC().Format(time.RFC3339Nano),
				FieldsChanged: result.FieldsChanged, FieldsSkipped: result.FieldsSkipped, FieldsFailed: result.FieldsFailed, Warnings: result.Warnings,
			})
			return
		}

		result, blockers, err := twitchMetadata.Publish(r.Context(), string(p.ProviderID), p.Metadata, link, found, time.Now().UTC())
		if err != nil {
			writeTwitchError(w, logger, r, err)
			return
		}
		if len(blockers) > 0 {
			writeJSON(w, logger, http.StatusOK, publishResultResponse{Status: "blocked", Blockers: blockers})
			return
		}
		writeJSON(w, logger, http.StatusOK, publishResultResponse{
			Status: "published", AccountID: result.AccountID, PublishedAt: result.PublishedAt.UTC().Format(time.RFC3339Nano),
			FieldsChanged: result.FieldsChanged, FieldsSkipped: result.FieldsSkipped,
		})
	}
}

// --- error mapping ------------------------------------------------------

func writeAccountError(w http.ResponseWriter, logger *slog.Logger, r *http.Request, err error) {
	if verr, ok := platform.AsValidationError(err); ok {
		writeValidationError(w, logger, verr)
		return
	}

	switch {
	case errors.Is(err, account.ErrNotFound):
		writeError(w, logger, http.StatusNotFound, "account_not_found", "The requested connected account does not exist.")
	case errors.Is(err, account.ErrLinkNotFound):
		writeError(w, logger, http.StatusNotFound, "link_not_found", "No connected account is linked to this platform.")
	case errors.Is(err, account.ErrProviderMismatch):
		writeError(w, logger, http.StatusUnprocessableEntity, "account_provider_mismatch",
			"This connected account belongs to a different provider than the destination.")
	case errors.Is(err, account.ErrIdentityMismatch):
		writeError(w, logger, http.StatusConflict, "oauth_identity_mismatch",
			"The authorized account does not match the account being reconnected.")
	case errors.Is(err, account.ErrMissingScope):
		writeError(w, logger, http.StatusUnprocessableEntity, "oauth_scope_missing",
			"The authorization did not grant every required permission.")
	case errors.Is(err, account.ErrIntegrationNotConfigured):
		writeError(w, logger, http.StatusUnprocessableEntity, "integration_not_configured",
			"No Twitch Client ID is configured yet.")
	case errors.Is(err, account.ErrIntegrationLocked):
		writeError(w, logger, http.StatusConflict, "integration_configuration_locked",
			"The Client ID cannot be changed while connected accounts exist for this provider.")
	case errors.Is(err, account.ErrReconnectRequired):
		writeError(w, logger, http.StatusConflict, "account_reconnect_required",
			"This account must be reconnected before it can be used.")
	case errors.Is(err, account.ErrSecretStoreUnavailable):
		writeError(w, logger, http.StatusServiceUnavailable, "credential_store_unavailable",
			"Secure storage is currently unavailable.")
	case errors.Is(err, account.ErrProviderUnavailable):
		writeError(w, logger, http.StatusBadGateway, "twitch_unavailable", "Twitch could not be reached.")
	case errors.Is(err, account.ErrRateLimited):
		writeError(w, logger, http.StatusTooManyRequests, "twitch_rate_limited", "Twitch rate limit reached; try again shortly.")
	case errors.Is(err, account.ErrConflict):
		writeError(w, logger, http.StatusConflict, "conflict", "The request conflicts with the current state of the resource.")
	default:
		logger.Error("unhandled account error", slog.String("path", r.URL.Path), slog.String("method", r.Method), slog.Any("error", err))
		writeError(w, logger, http.StatusInternalServerError, "internal_error", "The server encountered an unexpected error.")
	}
}

func writeDeviceFlowError(w http.ResponseWriter, logger *slog.Logger, r *http.Request, err error) {
	switch {
	case errors.Is(err, deviceflow.ErrConflict):
		writeError(w, logger, http.StatusConflict, "oauth_attempt_conflict",
			"An authorization attempt is already in progress.")
	case errors.Is(err, deviceflow.ErrNotFound):
		writeError(w, logger, http.StatusNotFound, "oauth_attempt_not_found",
			"The requested authorization attempt does not exist.")
	case errors.Is(err, account.ErrIntegrationNotConfigured):
		writeError(w, logger, http.StatusUnprocessableEntity, "integration_not_configured",
			"No Twitch Client ID is configured yet.")
	default:
		writeAccountError(w, logger, r, err)
	}
}

// writeTwitchError maps a genuine infrastructure/provider failure - not the
// normal "blocked" outcome, which Preview/Publish already return as a plain
// value and their handlers render as a 200 body directly.
func writeTwitchError(w http.ResponseWriter, logger *slog.Logger, r *http.Request, err error) {
	if blocker, ok := twitch.AsBlocker(err); ok {
		switch blocker {
		case twitch.BlockerAccountReconnectRequired:
			writeError(w, logger, http.StatusConflict, "account_reconnect_required", "This account must be reconnected before it can be used.")
		case twitch.BlockerRateLimited:
			writeError(w, logger, http.StatusTooManyRequests, "twitch_rate_limited", "Twitch rate limit reached; try again shortly.")
		case twitch.BlockerMissingScope:
			writeError(w, logger, http.StatusUnprocessableEntity, "missing_required_scope", "This action requires a permission that was not granted.")
		default:
			writeError(w, logger, http.StatusBadGateway, "twitch_unavailable", "Twitch could not be reached.")
		}
		return
	}
	switch {
	case errors.Is(err, twitch.ErrRateLimited):
		writeError(w, logger, http.StatusTooManyRequests, "twitch_rate_limited", "Twitch rate limit reached; try again shortly.")
	case errors.Is(err, twitch.ErrUnavailable):
		writeError(w, logger, http.StatusBadGateway, "twitch_unavailable", "Twitch could not be reached.")
	case errors.Is(err, twitch.ErrInvalidResponse):
		writeError(w, logger, http.StatusBadGateway, "twitch_invalid_response", "Twitch returned an unexpected response.")
	case errors.Is(err, twitch.ErrForbidden):
		writeError(w, logger, http.StatusUnprocessableEntity, "missing_required_scope", "This action requires a permission that was not granted.")
	default:
		writeAccountError(w, logger, r, err)
	}
}
