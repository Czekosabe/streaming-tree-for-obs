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
	"github.com/streaming-tree/server/internal/domain/remotetarget"
	"github.com/streaming-tree/server/internal/provider/youtube"
	"github.com/streaming-tree/server/internal/runtime/youtubeauth"
)

// YouTubeAuthService is the subset of youtubeauth.Manager the HTTP layer
// needs.
type YouTubeAuthService interface {
	StartAttempt(ctx context.Context, reconnectAccountID string) (youtubeauth.Snapshot, error)
	GetAttempt(attemptID string) (youtubeauth.Snapshot, error)
	CancelAttempt(attemptID string) (youtubeauth.Snapshot, error)
	SelectChannel(ctx context.Context, attemptID, channelID string) (youtubeauth.Snapshot, error)
}

// YouTubeMetadataService is the subset of youtube.MetadataService the HTTP
// layer needs.
type YouTubeMetadataService interface {
	ListBroadcasts(ctx context.Context, accountID string) ([]youtube.Broadcast, error)
	ListCategories(ctx context.Context, accountID string) ([]youtube.Category, error)
	EffectiveRegion(ctx context.Context, accountID string) (string, error)
	SetRegion(ctx context.Context, accountID, region string, now time.Time) error
	Preview(ctx context.Context, platformProviderID string, local platform.Metadata, link account.Link, linked bool, target remotetarget.Target, hasTarget bool) (youtube.Preview, error)
	Publish(ctx context.Context, platformProviderID string, local platform.Metadata, link account.Link, linked bool, target remotetarget.Target, hasTarget bool, now time.Time) (youtube.PublishResult, []string, error)
}

// RemoteTargetService is the subset of remotetarget.Service the HTTP layer
// needs.
type RemoteTargetService interface {
	GetTarget(ctx context.Context, platformID string) (remotetarget.Target, bool, error)
	SetTarget(ctx context.Context, platformID, platformProviderID, resourceType, resourceID, displayName string) (remotetarget.Target, error)
	DeleteTarget(ctx context.Context, platformID string) error
}

// registerYouTubeRoutes wires the YouTube integration-config, OAuth-attempt,
// broadcast, category, region and remote-target API.
func registerYouTubeRoutes(
	mux *http.ServeMux, logger *slog.Logger,
	platforms PlatformService, accounts AccountService, youtubeAuth YouTubeAuthService,
	youtubeMetadata YouTubeMetadataService, remoteTargets RemoteTargetService,
) {
	mux.HandleFunc("GET /api/integrations/youtube/config", handleGetYouTubeIntegrationConfig(logger, accounts))
	mux.HandleFunc("PUT /api/integrations/youtube/config", handleSetYouTubeIntegrationConfig(logger, accounts))
	mux.HandleFunc("/api/integrations/youtube/config", methodNotAllowed(logger, http.MethodGet, http.MethodPut))

	mux.HandleFunc("POST /api/integrations/youtube/oauth-attempts", handleStartYouTubeAttempt(logger, youtubeAuth))
	mux.HandleFunc("/api/integrations/youtube/oauth-attempts", methodNotAllowed(logger, http.MethodPost))

	mux.HandleFunc("GET /api/integrations/youtube/oauth-attempts/{id}", handleGetYouTubeAttempt(logger, youtubeAuth))
	mux.HandleFunc("DELETE /api/integrations/youtube/oauth-attempts/{id}", handleCancelYouTubeAttempt(logger, youtubeAuth))
	mux.HandleFunc("/api/integrations/youtube/oauth-attempts/{id}", methodNotAllowed(logger, http.MethodGet, http.MethodDelete))

	mux.HandleFunc("POST /api/integrations/youtube/oauth-attempts/{id}/channel", handleSelectYouTubeChannel(logger, youtubeAuth))
	mux.HandleFunc("/api/integrations/youtube/oauth-attempts/{id}/channel", methodNotAllowed(logger, http.MethodPost))

	mux.HandleFunc("GET /api/connected-accounts/{id}/youtube/broadcasts", handleListBroadcasts(logger, accounts, youtubeMetadata))
	mux.HandleFunc("/api/connected-accounts/{id}/youtube/broadcasts", methodNotAllowed(logger, http.MethodGet))

	mux.HandleFunc("GET /api/connected-accounts/{id}/youtube/categories", handleListYouTubeCategories(logger, accounts, youtubeMetadata))
	mux.HandleFunc("/api/connected-accounts/{id}/youtube/categories", methodNotAllowed(logger, http.MethodGet))

	mux.HandleFunc("GET /api/connected-accounts/{id}/youtube/region", handleGetYouTubeRegion(logger, accounts, youtubeMetadata))
	mux.HandleFunc("PUT /api/connected-accounts/{id}/youtube/region", handleSetYouTubeRegion(logger, accounts, youtubeMetadata))
	mux.HandleFunc("/api/connected-accounts/{id}/youtube/region", methodNotAllowed(logger, http.MethodGet, http.MethodPut))

	mux.HandleFunc("GET /api/platforms/{id}/remote-target", handleGetRemoteTarget(logger, platforms, remoteTargets))
	mux.HandleFunc("PUT /api/platforms/{id}/remote-target", handleSetRemoteTarget(logger, platforms, accounts, youtubeMetadata, remoteTargets))
	mux.HandleFunc("DELETE /api/platforms/{id}/remote-target", handleDeleteRemoteTarget(logger, platforms, remoteTargets))
	mux.HandleFunc("/api/platforms/{id}/remote-target", methodNotAllowed(logger, http.MethodGet, http.MethodPut, http.MethodDelete))
}

// --- integration config ------------------------------------------------

func handleGetYouTubeIntegrationConfig(logger *slog.Logger, accounts AccountService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg, err := accounts.IntegrationConfig(r.Context(), account.ProviderYouTube)
		if err != nil {
			writeAccountError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toIntegrationConfigResponse(cfg))
	}
}

func handleSetYouTubeIntegrationConfig(logger *slog.Logger, accounts AccountService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body setIntegrationConfigRequest
		if err := decodeJSON(w, r, &body); err != nil {
			writeDecodeError(w, logger, err)
			return
		}
		cfg, err := accounts.SetIntegrationClientID(r.Context(), account.ProviderYouTube, body.ClientID)
		if err != nil {
			writeAccountError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toIntegrationConfigResponse(cfg))
	}
}

// --- OAuth attempts ------------------------------------------------------

type channelSummaryResponse struct {
	ChannelID    string `json:"channelId"`
	Title        string `json:"title"`
	ThumbnailURL string `json:"thumbnailUrl,omitempty"`
}

// oauthAttemptResponse is the public shape of a YouTube OAuth attempt.
//
// Never carries: an authorization code, a PKCE verifier, a state value, an
// access token, a refresh token, an ID token, a client secret, or a raw
// Google response - see youtubeauth.Snapshot's own doc comment, which this
// mirrors field-for-field.
type oauthAttemptResponse struct {
	AttemptID          string                   `json:"attemptId"`
	ProviderID         string                   `json:"providerId"`
	State              string                   `json:"state"`
	AuthorizationURL   string                   `json:"authorizationUrl,omitempty"`
	CreatedAt          string                   `json:"createdAt"`
	ExpiresAt          string                   `json:"expiresAt,omitempty"`
	ConnectedAccountID string                   `json:"connectedAccountId,omitempty"`
	Channels           []channelSummaryResponse `json:"channels,omitempty"`
	ErrorCode          string                   `json:"errorCode,omitempty"`
	ErrorMessage       string                   `json:"errorMessage,omitempty"`
}

func toOAuthAttemptResponse(s youtubeauth.Snapshot) oauthAttemptResponse {
	resp := oauthAttemptResponse{
		AttemptID: s.AttemptID, ProviderID: s.ProviderID, State: string(s.State),
		AuthorizationURL:   s.AuthorizationURL,
		CreatedAt:          s.CreatedAt.UTC().Format(time.RFC3339Nano),
		ConnectedAccountID: s.ConnectedAccountID, ErrorCode: s.ErrorCode, ErrorMessage: s.ErrorMessage,
	}
	if !s.ExpiresAt.IsZero() {
		resp.ExpiresAt = s.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	for _, ch := range s.Channels {
		resp.Channels = append(resp.Channels, channelSummaryResponse{ChannelID: ch.ChannelID, Title: ch.Title, ThumbnailURL: ch.ThumbnailURL})
	}
	return resp
}

func handleStartYouTubeAttempt(logger *slog.Logger, youtubeAuth YouTubeAuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}
		snapshot, err := youtubeAuth.StartAttempt(r.Context(), "")
		if err != nil {
			writeYouTubeAuthError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusAccepted, toOAuthAttemptResponse(snapshot))
	}
}

func handleGetYouTubeAttempt(logger *slog.Logger, youtubeAuth YouTubeAuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		snapshot, err := youtubeAuth.GetAttempt(r.PathValue("id"))
		if err != nil {
			writeYouTubeAuthError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toOAuthAttemptResponse(snapshot))
	}
}

func handleCancelYouTubeAttempt(logger *slog.Logger, youtubeAuth YouTubeAuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}
		snapshot, err := youtubeAuth.CancelAttempt(r.PathValue("id"))
		if err != nil {
			writeYouTubeAuthError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toOAuthAttemptResponse(snapshot))
	}
}

type selectChannelRequest struct {
	ChannelID string `json:"channelId"`
}

func handleSelectYouTubeChannel(logger *slog.Logger, youtubeAuth YouTubeAuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body selectChannelRequest
		if err := decodeJSON(w, r, &body); err != nil {
			writeDecodeError(w, logger, err)
			return
		}
		if strings.TrimSpace(body.ChannelID) == "" {
			writeError(w, logger, http.StatusUnprocessableEntity, "validation_failed", "channelId is required.")
			return
		}
		snapshot, err := youtubeAuth.SelectChannel(r.Context(), r.PathValue("id"), body.ChannelID)
		if err != nil {
			writeYouTubeAuthError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toOAuthAttemptResponse(snapshot))
	}
}

// --- broadcasts ------------------------------------------------------------

type broadcastResponse struct {
	ID                 string `json:"id"`
	Title              string `json:"title"`
	LifeCycleStatus    string `json:"lifeCycleStatus"`
	PrivacyStatus      string `json:"privacyStatus"`
	ScheduledStartTime string `json:"scheduledStartTime,omitempty"`
	ActualStartTime    string `json:"actualStartTime,omitempty"`
}

func handleListBroadcasts(logger *slog.Logger, accounts AccountService, youtubeMetadata YouTubeMetadataService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID := r.PathValue("id")
		if _, err := accounts.GetAccount(r.Context(), accountID); err != nil {
			writeAccountError(w, logger, r, err)
			return
		}
		results, err := youtubeMetadata.ListBroadcasts(r.Context(), accountID)
		if err != nil {
			writeYouTubeError(w, logger, r, err)
			return
		}
		items := make([]broadcastResponse, 0, len(results))
		for _, b := range results {
			items = append(items, broadcastResponse{
				ID: b.ID, Title: b.Title, LifeCycleStatus: b.LifeCycleStatus, PrivacyStatus: b.PrivacyStatus,
				ScheduledStartTime: b.ScheduledStartTime, ActualStartTime: b.ActualStartTime,
			})
		}
		writeJSON(w, logger, http.StatusOK, map[string]any{"items": items})
	}
}

// --- categories and region -------------------------------------------------

const (
	regionCodeMinLength = 2
	regionCodeMaxLength = 2
)

func handleListYouTubeCategories(logger *slog.Logger, accounts AccountService, youtubeMetadata YouTubeMetadataService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID := r.PathValue("id")
		if _, err := accounts.GetAccount(r.Context(), accountID); err != nil {
			writeAccountError(w, logger, r, err)
			return
		}
		results, err := youtubeMetadata.ListCategories(r.Context(), accountID)
		if err != nil {
			writeYouTubeError(w, logger, r, err)
			return
		}
		items := make([]categoryItemResponse, 0, len(results))
		for _, c := range results {
			items = append(items, categoryItemResponse{ID: c.ID, Name: c.Name})
		}
		writeJSON(w, logger, http.StatusOK, map[string]any{"items": items})
	}
}

func handleGetYouTubeRegion(logger *slog.Logger, accounts AccountService, youtubeMetadata YouTubeMetadataService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID := r.PathValue("id")
		if _, err := accounts.GetAccount(r.Context(), accountID); err != nil {
			writeAccountError(w, logger, r, err)
			return
		}
		region, err := youtubeMetadata.EffectiveRegion(r.Context(), accountID)
		if err != nil {
			writeYouTubeError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, map[string]any{"region": region})
	}
}

type setRegionRequest struct {
	Region string `json:"region"`
}

func handleSetYouTubeRegion(logger *slog.Logger, accounts AccountService, youtubeMetadata YouTubeMetadataService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID := r.PathValue("id")
		if _, err := accounts.GetAccount(r.Context(), accountID); err != nil {
			writeAccountError(w, logger, r, err)
			return
		}
		var body setRegionRequest
		if err := decodeJSON(w, r, &body); err != nil {
			writeDecodeError(w, logger, err)
			return
		}
		region := strings.ToUpper(strings.TrimSpace(body.Region))
		length := utf8.RuneCountInString(region)
		if length < regionCodeMinLength || length > regionCodeMaxLength {
			writeError(w, logger, http.StatusUnprocessableEntity, "validation_failed", "region must be a two-letter ISO 3166-1 alpha-2 code.")
			return
		}
		if err := youtubeMetadata.SetRegion(r.Context(), accountID, region, time.Now().UTC()); err != nil {
			writeYouTubeError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, map[string]any{"region": region})
	}
}

// --- remote target -----------------------------------------------------

type remoteTargetResponse struct {
	PlatformID   string `json:"platformId"`
	ProviderID   string `json:"providerId"`
	ResourceType string `json:"resourceType"`
	ResourceID   string `json:"resourceId"`
	DisplayName  string `json:"displayName"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
}

func toRemoteTargetResponse(t remotetarget.Target) remoteTargetResponse {
	return remoteTargetResponse{
		PlatformID: t.PlatformID, ProviderID: t.ProviderID, ResourceType: t.ResourceType, ResourceID: t.ResourceID,
		DisplayName: t.DisplayName, CreatedAt: t.CreatedAt.UTC().Format(time.RFC3339Nano), UpdatedAt: t.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func handleGetRemoteTarget(logger *slog.Logger, platforms PlatformService, remoteTargets RemoteTargetService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !requirePlatform(w, logger, r, platforms, id) {
			return
		}
		target, found, err := remoteTargets.GetTarget(r.Context(), id)
		if err != nil {
			writeDomainError(w, logger, r, err)
			return
		}
		if !found {
			writeJSON(w, logger, http.StatusOK, nil)
			return
		}
		writeJSON(w, logger, http.StatusOK, toRemoteTargetResponse(target))
	}
}

type setRemoteTargetRequest struct {
	ResourceID string `json:"resourceId"`
}

// handleSetRemoteTarget verifies the selected broadcast actually belongs to
// the linked channel (a GetBroadcast round-trip) before persisting it - see
// docs/provider-integrations/youtube.md's "Remote broadcast target"
// section. Only a YouTube destination with a linked, healthy account may
// set a target.
func handleSetRemoteTarget(
	logger *slog.Logger, platforms PlatformService, accounts AccountService,
	youtubeMetadata YouTubeMetadataService, remoteTargets RemoteTargetService,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		p, err := platforms.Get(r.Context(), id)
		if err != nil {
			writeDomainError(w, logger, r, err)
			return
		}
		if p.ProviderID != platform.ProviderYouTube {
			writeError(w, logger, http.StatusUnprocessableEntity, "account_provider_mismatch", "Only a YouTube destination can have a remote broadcast target.")
			return
		}

		var body setRemoteTargetRequest
		if err := decodeJSON(w, r, &body); err != nil {
			writeDecodeError(w, logger, err)
			return
		}
		if strings.TrimSpace(body.ResourceID) == "" {
			writeError(w, logger, http.StatusUnprocessableEntity, "validation_failed", "resourceId is required.")
			return
		}

		link, found, err := accounts.GetLink(r.Context(), id)
		if err != nil {
			writeAccountError(w, logger, r, err)
			return
		}
		if !found {
			writeError(w, logger, http.StatusConflict, "account_not_linked", "Link a YouTube account to this destination before selecting a broadcast.")
			return
		}

		broadcasts, err := youtubeMetadata.ListBroadcasts(r.Context(), link.AccountID)
		if err != nil {
			writeYouTubeError(w, logger, r, err)
			return
		}
		var matched *broadcastResponse
		for _, b := range broadcasts {
			if b.ID == body.ResourceID {
				matched = &broadcastResponse{ID: b.ID, Title: b.Title}
				break
			}
		}
		if matched == nil {
			writeError(w, logger, http.StatusUnprocessableEntity, "youtube_broadcast_not_found", "The selected broadcast does not belong to the linked channel.")
			return
		}

		target, err := remoteTargets.SetTarget(r.Context(), id, string(p.ProviderID), remotetarget.ResourceTypeLiveBroadcast, matched.ID, matched.Title)
		if err != nil {
			writeDomainError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toRemoteTargetResponse(target))
	}
}

func handleDeleteRemoteTarget(logger *slog.Logger, platforms PlatformService, remoteTargets RemoteTargetService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !requirePlatform(w, logger, r, platforms, id) {
			return
		}
		if err := remoteTargets.DeleteTarget(r.Context(), id); err != nil {
			writeDomainError(w, logger, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// --- error mapping ------------------------------------------------------

func writeYouTubeAuthError(w http.ResponseWriter, logger *slog.Logger, r *http.Request, err error) {
	switch {
	case errors.Is(err, youtubeauth.ErrConflict):
		writeError(w, logger, http.StatusConflict, "youtube_oauth_attempt_conflict", "An authorization attempt is already in progress.")
	case errors.Is(err, youtubeauth.ErrNotFound):
		writeError(w, logger, http.StatusNotFound, "youtube_oauth_attempt_not_found", "The requested authorization attempt does not exist.")
	case errors.Is(err, youtubeauth.ErrChannelSelectionNotPending):
		writeError(w, logger, http.StatusConflict, "youtube_channel_selection_required", "This attempt is not awaiting a channel selection.")
	case errors.Is(err, youtubeauth.ErrInvalidChannelSelection):
		writeError(w, logger, http.StatusUnprocessableEntity, "youtube_channel_not_found", "The selected channel was not offered by this attempt.")
	case errors.Is(err, account.ErrIntegrationNotConfigured):
		writeError(w, logger, http.StatusUnprocessableEntity, "youtube_integration_not_configured", "No YouTube Client ID is configured yet.")
	default:
		writeAccountError(w, logger, r, err)
	}
}

// writeYouTubeError maps a genuine infrastructure/provider failure - not the
// normal "blocked" outcome, which Preview/Publish already return as a plain
// value.
func writeYouTubeError(w http.ResponseWriter, logger *slog.Logger, r *http.Request, err error) {
	if blocker, ok := youtube.AsBlocker(err); ok {
		switch blocker {
		case youtube.BlockerAccountReconnectRequired:
			writeError(w, logger, http.StatusConflict, "account_reconnect_required", "This account must be reconnected before it can be used.")
		case youtube.BlockerQuotaExceeded:
			writeError(w, logger, http.StatusTooManyRequests, "youtube_quota_exceeded", "YouTube's API quota was exceeded; try again later.")
		case youtube.BlockerMissingScope:
			writeError(w, logger, http.StatusUnprocessableEntity, "missing_required_scope", "This action requires a permission that was not granted.")
		case youtube.BlockerLiveStreamingNotEnabled:
			writeError(w, logger, http.StatusUnprocessableEntity, "youtube_live_streaming_not_enabled", "Live streaming is not enabled for this channel.")
		case youtube.BlockerRegionRequired:
			writeError(w, logger, http.StatusUnprocessableEntity, "youtube_region_required", "A category region is required.")
		default:
			writeError(w, logger, http.StatusBadGateway, "youtube_unavailable", "YouTube could not be reached.")
		}
		return
	}
	switch {
	case errors.Is(err, youtube.ErrQuotaExceeded):
		writeError(w, logger, http.StatusTooManyRequests, "youtube_quota_exceeded", "YouTube's API quota was exceeded; try again later.")
	case errors.Is(err, youtube.ErrRateLimited):
		writeError(w, logger, http.StatusTooManyRequests, "youtube_rate_limited", "YouTube rate limit reached; try again shortly.")
	case errors.Is(err, youtube.ErrLiveStreamingNotEnabled):
		writeError(w, logger, http.StatusUnprocessableEntity, "youtube_live_streaming_not_enabled", "Live streaming is not enabled for this channel.")
	case errors.Is(err, youtube.ErrUnavailable):
		writeError(w, logger, http.StatusBadGateway, "youtube_unavailable", "YouTube could not be reached.")
	case errors.Is(err, youtube.ErrInvalidResponse):
		writeError(w, logger, http.StatusBadGateway, "youtube_invalid_response", "YouTube returned an unexpected response.")
	case errors.Is(err, youtube.ErrForbidden):
		writeError(w, logger, http.StatusUnprocessableEntity, "missing_required_scope", "This action requires a permission that was not granted.")
	default:
		writeAccountError(w, logger, r, err)
	}
}
