package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	co "github.com/streaming-tree/server/internal/chatoverlay"
	chatoverlaydomain "github.com/streaming-tree/server/internal/domain/chatoverlay"
	"github.com/streaming-tree/server/internal/provider/twitch/chatassets"
)

// ChatOverlayProfileService is the subset of chatoverlaydomain.Service the
// HTTP layer needs: profile CRUD, slug rotation, and the four per-overlay
// child lists (accounts, hidden users, blocked terms, activity types).
type ChatOverlayProfileService interface {
	CreateProfile(ctx context.Context, name string) (chatoverlaydomain.Profile, error)
	GetProfile(ctx context.Context, id string) (chatoverlaydomain.Profile, error)
	GetProfileByPublicSlug(ctx context.Context, slug string) (chatoverlaydomain.Profile, error)
	ListProfiles(ctx context.Context) ([]chatoverlaydomain.Profile, error)
	ReplaceProfile(ctx context.Context, p chatoverlaydomain.Profile) (chatoverlaydomain.Profile, error)
	DeleteProfile(ctx context.Context, id string) error
	RotatePublicSlug(ctx context.Context, id string) (chatoverlaydomain.Profile, error)

	Accounts(ctx context.Context, overlayID string) ([]string, error)
	SetAccounts(ctx context.Context, overlayID string, accountIDs []string) error

	HiddenUsers(ctx context.Context, overlayID string) ([]chatoverlaydomain.HiddenUser, error)
	HideUser(ctx context.Context, overlayID string, providerID chatoverlaydomain.ProviderID, connectedAccountID, providerUserID, label string) (chatoverlaydomain.HiddenUser, error)
	UnhideUser(ctx context.Context, overlayID string, providerID chatoverlaydomain.ProviderID, connectedAccountID, providerUserID string) error

	BlockedTerms(ctx context.Context, overlayID string) ([]chatoverlaydomain.BlockedTerm, error)
	AddBlockedTerm(ctx context.Context, overlayID, value string, mode chatoverlaydomain.MatchMode) (chatoverlaydomain.BlockedTerm, error)
	RemoveBlockedTerm(ctx context.Context, overlayID, id string) error

	ActivityTypes(ctx context.Context, overlayID string) ([]string, error)
	SetActivityTypes(ctx context.Context, overlayID string, activityTypes []string) error
}

// ChatOverlayRuntime is the subset of chatoverlay.Manager the HTTP layer
// needs: looking up (and lazily creating) a profile's running Projection,
// and telling the runtime a profile's settings changed or the profile was
// deleted.
type ChatOverlayRuntime interface {
	EnsureOverlay(ctx context.Context, overlayID string) (*co.Projection, error)
	Get(overlayID string) (*co.Projection, bool)
	Rebuild(ctx context.Context, overlayID string) error
	Remove(ctx context.Context, overlayID string)
}

const (
	maxChatOverlaySSEClientsPerOverlay = 8
	chatOverlaySSEKeepalive            = 15 * time.Second
)

// registerChatOverlayRoutes wires the Stage 10 chat-overlay management API
// (/api/chat-overlays/...) and the public, unauthenticated overlay API
// (/api/public/chat-overlays/...) an OBS Browser Source actually loads.
func registerChatOverlayRoutes(
	mux *http.ServeMux, logger *slog.Logger,
	accounts AccountService, profiles ChatOverlayProfileService, runtime ChatOverlayRuntime, assets OperatorChatAssetResolver,
) {
	mux.HandleFunc("GET /api/chat-overlays", handleListChatOverlays(logger, profiles))
	mux.HandleFunc("POST /api/chat-overlays", handleCreateChatOverlay(logger, profiles, runtime))
	mux.HandleFunc("/api/chat-overlays", methodNotAllowed(logger, http.MethodGet, http.MethodPost))

	mux.HandleFunc("GET /api/chat-overlays/{id}", handleGetChatOverlay(logger, profiles))
	mux.HandleFunc("PUT /api/chat-overlays/{id}", handlePutChatOverlay(logger, profiles, runtime))
	mux.HandleFunc("DELETE /api/chat-overlays/{id}", handleDeleteChatOverlay(logger, profiles, runtime))
	mux.HandleFunc("/api/chat-overlays/{id}", methodNotAllowed(logger, http.MethodGet, http.MethodPut, http.MethodDelete))

	mux.HandleFunc("POST /api/chat-overlays/{id}/rotate-public-slug", handleRotateChatOverlayPublicSlug(logger, profiles))
	mux.HandleFunc("/api/chat-overlays/{id}/rotate-public-slug", methodNotAllowed(logger, http.MethodPost))

	mux.HandleFunc("GET /api/chat-overlays/{id}/accounts", handleGetChatOverlayAccounts(logger, profiles))
	mux.HandleFunc("PUT /api/chat-overlays/{id}/accounts", handlePutChatOverlayAccounts(logger, accounts, profiles, runtime))
	mux.HandleFunc("/api/chat-overlays/{id}/accounts", methodNotAllowed(logger, http.MethodGet, http.MethodPut))

	mux.HandleFunc("GET /api/chat-overlays/{id}/hidden-users", handleListChatOverlayHiddenUsers(logger, profiles))
	mux.HandleFunc("POST /api/chat-overlays/{id}/hidden-users", handleAddChatOverlayHiddenUser(logger, accounts, profiles, runtime))
	mux.HandleFunc("DELETE /api/chat-overlays/{id}/hidden-users", handleRemoveChatOverlayHiddenUser(logger, profiles, runtime))
	mux.HandleFunc("/api/chat-overlays/{id}/hidden-users", methodNotAllowed(logger, http.MethodGet, http.MethodPost, http.MethodDelete))

	mux.HandleFunc("GET /api/chat-overlays/{id}/blocked-terms", handleListChatOverlayBlockedTerms(logger, profiles))
	mux.HandleFunc("POST /api/chat-overlays/{id}/blocked-terms", handleAddChatOverlayBlockedTerm(logger, profiles, runtime))
	mux.HandleFunc("/api/chat-overlays/{id}/blocked-terms", methodNotAllowed(logger, http.MethodGet, http.MethodPost))
	mux.HandleFunc("DELETE /api/chat-overlays/{id}/blocked-terms/{termId}", handleRemoveChatOverlayBlockedTerm(logger, profiles, runtime))
	mux.HandleFunc("/api/chat-overlays/{id}/blocked-terms/{termId}", methodNotAllowed(logger, http.MethodDelete))

	mux.HandleFunc("GET /api/chat-overlays/{id}/activity-types", handleGetChatOverlayActivityTypes(logger, profiles))
	mux.HandleFunc("PUT /api/chat-overlays/{id}/activity-types", handlePutChatOverlayActivityTypes(logger, profiles, runtime))
	mux.HandleFunc("/api/chat-overlays/{id}/activity-types", methodNotAllowed(logger, http.MethodGet, http.MethodPut))

	streamLimiter := newChatOverlayStreamLimiter()
	mux.HandleFunc("GET /api/public/chat-overlays/{slug}/config", handleGetPublicChatOverlayConfig(logger, profiles))
	mux.HandleFunc("/api/public/chat-overlays/{slug}/config", methodNotAllowed(logger, http.MethodGet))

	mux.HandleFunc("GET /api/public/chat-overlays/{slug}/items", handleGetPublicChatOverlayItems(logger, profiles, runtime, assets))
	mux.HandleFunc("/api/public/chat-overlays/{slug}/items", methodNotAllowed(logger, http.MethodGet))

	mux.HandleFunc("GET /api/public/chat-overlays/{slug}/stream", handlePublicChatOverlayStream(logger, profiles, runtime, assets, streamLimiter))
	mux.HandleFunc("/api/public/chat-overlays/{slug}/stream", methodNotAllowed(logger, http.MethodGet))
}

// --- profile response/request DTOs -----------------------------------------

type chatOverlayProfileResponse struct {
	ID         string `json:"id"`
	PublicSlug string `json:"publicSlug"`
	Name       string `json:"name"`
	Enabled    bool   `json:"enabled"`

	LayoutMode          string `json:"layoutMode"`
	StackDirection      string `json:"stackDirection"`
	HorizontalAlignment string `json:"horizontalAlignment"`

	ShowPlatformIcon       bool `json:"showPlatformIcon"`
	ShowPlatformName       bool `json:"showPlatformName"`
	ShowAccountLabel       bool `json:"showAccountLabel"`
	ShowAvatar             bool `json:"showAvatar"`
	ShowBadges             bool `json:"showBadges"`
	ShowTimestamp          bool `json:"showTimestamp"`
	ShowActivityEvents     bool `json:"showActivityEvents"`
	ShowDeletedPlaceholder bool `json:"showDeletedPlaceholder"`
	HideCommands           bool `json:"hideCommands"`
	HideBots               bool `json:"hideBots"`

	MaxVisibleItems        int `json:"maxVisibleItems"`
	MessageLifetimeSeconds int `json:"messageLifetimeSeconds"`

	FontFamily        string  `json:"fontFamily"`
	FontSize          int     `json:"fontSize"`
	FontWeight        int     `json:"fontWeight"`
	LineHeight        float64 `json:"lineHeight"`
	TextColor         string  `json:"textColor"`
	UsernameColorMode string  `json:"usernameColorMode"`
	BubbleColor       string  `json:"bubbleColor"`
	BubbleOpacity     float64 `json:"bubbleOpacity"`
	BorderRadius      int     `json:"borderRadius"`
	ItemSpacing       int     `json:"itemSpacing"`
	TextOutline       bool    `json:"textOutline"`
	TextShadow        bool    `json:"textShadow"`

	EntryAnimation      string `json:"entryAnimation"`
	ExitAnimation       string `json:"exitAnimation"`
	AnimationDurationMS int    `json:"animationDurationMs"`

	HighlightBroadcaster bool `json:"highlightBroadcaster"`
	HighlightModerators  bool `json:"highlightModerators"`
	HighlightSubscribers bool `json:"highlightSubscribers"`
	HighlightVIPs        bool `json:"highlightVips"`

	Language string `json:"language"`

	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

func toChatOverlayProfileResponse(p chatoverlaydomain.Profile) chatOverlayProfileResponse {
	return chatOverlayProfileResponse{
		ID: p.ID, PublicSlug: p.PublicSlug, Name: p.Name, Enabled: p.Enabled,
		LayoutMode: string(p.LayoutMode), StackDirection: string(p.StackDirection), HorizontalAlignment: string(p.HorizontalAlignment),

		ShowPlatformIcon: p.ShowPlatformIcon, ShowPlatformName: p.ShowPlatformName, ShowAccountLabel: p.ShowAccountLabel,
		ShowAvatar: p.ShowAvatar, ShowBadges: p.ShowBadges, ShowTimestamp: p.ShowTimestamp,
		ShowActivityEvents: p.ShowActivityEvents, ShowDeletedPlaceholder: p.ShowDeletedPlaceholder,
		HideCommands: p.HideCommands, HideBots: p.HideBots,

		MaxVisibleItems: p.MaxVisibleItems, MessageLifetimeSeconds: p.MessageLifetimeSeconds,

		FontFamily: string(p.FontFamily), FontSize: p.FontSize, FontWeight: p.FontWeight, LineHeight: p.LineHeight,
		TextColor: p.TextColor, UsernameColorMode: string(p.UsernameColorMode), BubbleColor: p.BubbleColor,
		BubbleOpacity: p.BubbleOpacity, BorderRadius: p.BorderRadius, ItemSpacing: p.ItemSpacing,
		TextOutline: p.TextOutline, TextShadow: p.TextShadow,

		EntryAnimation: string(p.EntryAnimation), ExitAnimation: string(p.ExitAnimation), AnimationDurationMS: p.AnimationDurationMS,

		HighlightBroadcaster: p.HighlightBroadcaster, HighlightModerators: p.HighlightModerators,
		HighlightSubscribers: p.HighlightSubscribers, HighlightVIPs: p.HighlightVIPs,

		Language: string(p.Language),

		CreatedAt: p.CreatedAt.UTC().Format(time.RFC3339Nano), UpdatedAt: p.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

// putChatOverlayProfileRequest carries every editable profile field -
// identity fields (id, publicSlug, createdAt) are never part of this body,
// mirroring ReplaceProfile's own contract.
type putChatOverlayProfileRequest struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`

	LayoutMode          string `json:"layoutMode"`
	StackDirection      string `json:"stackDirection"`
	HorizontalAlignment string `json:"horizontalAlignment"`

	ShowPlatformIcon       bool `json:"showPlatformIcon"`
	ShowPlatformName       bool `json:"showPlatformName"`
	ShowAccountLabel       bool `json:"showAccountLabel"`
	ShowAvatar             bool `json:"showAvatar"`
	ShowBadges             bool `json:"showBadges"`
	ShowTimestamp          bool `json:"showTimestamp"`
	ShowActivityEvents     bool `json:"showActivityEvents"`
	ShowDeletedPlaceholder bool `json:"showDeletedPlaceholder"`
	HideCommands           bool `json:"hideCommands"`
	HideBots               bool `json:"hideBots"`

	MaxVisibleItems        int `json:"maxVisibleItems"`
	MessageLifetimeSeconds int `json:"messageLifetimeSeconds"`

	FontFamily        string  `json:"fontFamily"`
	FontSize          int     `json:"fontSize"`
	FontWeight        int     `json:"fontWeight"`
	LineHeight        float64 `json:"lineHeight"`
	TextColor         string  `json:"textColor"`
	UsernameColorMode string  `json:"usernameColorMode"`
	BubbleColor       string  `json:"bubbleColor"`
	BubbleOpacity     float64 `json:"bubbleOpacity"`
	BorderRadius      int     `json:"borderRadius"`
	ItemSpacing       int     `json:"itemSpacing"`
	TextOutline       bool    `json:"textOutline"`
	TextShadow        bool    `json:"textShadow"`

	EntryAnimation      string `json:"entryAnimation"`
	ExitAnimation       string `json:"exitAnimation"`
	AnimationDurationMS int    `json:"animationDurationMs"`

	HighlightBroadcaster bool `json:"highlightBroadcaster"`
	HighlightModerators  bool `json:"highlightModerators"`
	HighlightSubscribers bool `json:"highlightSubscribers"`
	HighlightVIPs        bool `json:"highlightVips"`

	Language string `json:"language"`
}

// applyTo returns existing with every editable field replaced by body's -
// identity fields (ID, PublicSlug, CreatedAt) come from existing and are
// never touched, matching ReplaceProfile's own contract.
func (body putChatOverlayProfileRequest) applyTo(existing chatoverlaydomain.Profile) chatoverlaydomain.Profile {
	existing.Name = body.Name
	existing.Enabled = body.Enabled
	existing.LayoutMode = chatoverlaydomain.LayoutMode(body.LayoutMode)
	existing.StackDirection = chatoverlaydomain.StackDirection(body.StackDirection)
	existing.HorizontalAlignment = chatoverlaydomain.HorizontalAlignment(body.HorizontalAlignment)

	existing.ShowPlatformIcon = body.ShowPlatformIcon
	existing.ShowPlatformName = body.ShowPlatformName
	existing.ShowAccountLabel = body.ShowAccountLabel
	existing.ShowAvatar = body.ShowAvatar
	existing.ShowBadges = body.ShowBadges
	existing.ShowTimestamp = body.ShowTimestamp
	existing.ShowActivityEvents = body.ShowActivityEvents
	existing.ShowDeletedPlaceholder = body.ShowDeletedPlaceholder
	existing.HideCommands = body.HideCommands
	existing.HideBots = body.HideBots

	existing.MaxVisibleItems = body.MaxVisibleItems
	existing.MessageLifetimeSeconds = body.MessageLifetimeSeconds

	existing.FontFamily = chatoverlaydomain.FontFamily(body.FontFamily)
	existing.FontSize = body.FontSize
	existing.FontWeight = body.FontWeight
	existing.LineHeight = body.LineHeight
	existing.TextColor = body.TextColor
	existing.UsernameColorMode = chatoverlaydomain.UsernameColorMode(body.UsernameColorMode)
	existing.BubbleColor = body.BubbleColor
	existing.BubbleOpacity = body.BubbleOpacity
	existing.BorderRadius = body.BorderRadius
	existing.ItemSpacing = body.ItemSpacing
	existing.TextOutline = body.TextOutline
	existing.TextShadow = body.TextShadow

	existing.EntryAnimation = chatoverlaydomain.Animation(body.EntryAnimation)
	existing.ExitAnimation = chatoverlaydomain.Animation(body.ExitAnimation)
	existing.AnimationDurationMS = body.AnimationDurationMS

	existing.HighlightBroadcaster = body.HighlightBroadcaster
	existing.HighlightModerators = body.HighlightModerators
	existing.HighlightSubscribers = body.HighlightSubscribers
	existing.HighlightVIPs = body.HighlightVIPs

	existing.Language = chatoverlaydomain.Language(body.Language)
	return existing
}

// --- profile CRUD handlers --------------------------------------------------

func handleListChatOverlays(logger *slog.Logger, profiles ChatOverlayProfileService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := profiles.ListProfiles(r.Context())
		if err != nil {
			writeChatOverlayError(w, logger, r, err)
			return
		}
		resp := make([]chatOverlayProfileResponse, 0, len(list))
		for _, p := range list {
			resp = append(resp, toChatOverlayProfileResponse(p))
		}
		writeJSON(w, logger, http.StatusOK, map[string]any{"items": resp})
	}
}

type createChatOverlayProfileRequest struct {
	Name string `json:"name"`
}

func handleCreateChatOverlay(logger *slog.Logger, profiles ChatOverlayProfileService, runtime ChatOverlayRuntime) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body createChatOverlayProfileRequest
		if err := decodeJSON(w, r, &body); err != nil {
			writeDecodeError(w, logger, err)
			return
		}

		saved, err := profiles.CreateProfile(r.Context(), body.Name)
		if err != nil {
			writeChatOverlayError(w, logger, r, err)
			return
		}
		rebuildChatOverlayRuntime(r.Context(), logger, runtime, saved.ID)
		writeJSON(w, logger, http.StatusOK, toChatOverlayProfileResponse(saved))
	}
}

func handleGetChatOverlay(logger *slog.Logger, profiles ChatOverlayProfileService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := profiles.GetProfile(r.Context(), r.PathValue("id"))
		if err != nil {
			writeChatOverlayError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toChatOverlayProfileResponse(p))
	}
}

func handlePutChatOverlay(logger *slog.Logger, profiles ChatOverlayProfileService, runtime ChatOverlayRuntime) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		existing, err := profiles.GetProfile(r.Context(), id)
		if err != nil {
			writeChatOverlayError(w, logger, r, err)
			return
		}

		var body putChatOverlayProfileRequest
		if err := decodeJSON(w, r, &body); err != nil {
			writeDecodeError(w, logger, err)
			return
		}

		saved, err := profiles.ReplaceProfile(r.Context(), body.applyTo(existing))
		if err != nil {
			writeChatOverlayError(w, logger, r, err)
			return
		}
		rebuildChatOverlayRuntime(r.Context(), logger, runtime, saved.ID)
		writeJSON(w, logger, http.StatusOK, toChatOverlayProfileResponse(saved))
	}
}

func handleDeleteChatOverlay(logger *slog.Logger, profiles ChatOverlayProfileService, runtime ChatOverlayRuntime) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}
		id := r.PathValue("id")
		if err := profiles.DeleteProfile(r.Context(), id); err != nil {
			writeChatOverlayError(w, logger, r, err)
			return
		}
		runtime.Remove(r.Context(), id)
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleRotateChatOverlayPublicSlug(logger *slog.Logger, profiles ChatOverlayProfileService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}
		saved, err := profiles.RotatePublicSlug(r.Context(), r.PathValue("id"))
		if err != nil {
			writeChatOverlayError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toChatOverlayProfileResponse(saved))
	}
}

// rebuildChatOverlayRuntime applies a just-saved settings change to the
// live runtime projection so its public output (and any connected
// Browser Source) reflects it immediately - see Part 19's "successful
// Save triggers a public reset/rebuild." A failure here is logged, not
// surfaced to the client: the settings write itself already succeeded,
// and the runtime will catch up on its own next rebuild.
func rebuildChatOverlayRuntime(ctx context.Context, logger *slog.Logger, runtime ChatOverlayRuntime, overlayID string) {
	if err := runtime.Rebuild(ctx, overlayID); err != nil {
		logger.Warn("failed to rebuild the live chat overlay projection after a settings change",
			slog.String("overlay_id", overlayID), slog.Any("error", err))
	}
}

// --- accounts ----------------------------------------------------------------

func handleGetChatOverlayAccounts(logger *slog.Logger, profiles ChatOverlayProfileService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := profiles.Accounts(r.Context(), r.PathValue("id"))
		if err != nil {
			writeChatOverlayError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, map[string]any{"accountIds": list})
	}
}

type putChatOverlayAccountsRequest struct {
	AccountIDs []string `json:"accountIds"`
}

func handlePutChatOverlayAccounts(logger *slog.Logger, accounts AccountService, profiles ChatOverlayProfileService, runtime ChatOverlayRuntime) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var body putChatOverlayAccountsRequest
		if err := decodeJSON(w, r, &body); err != nil {
			writeDecodeError(w, logger, err)
			return
		}

		for _, accountID := range body.AccountIDs {
			if _, err := accounts.GetAccount(r.Context(), accountID); err != nil {
				writeError(w, logger, http.StatusUnprocessableEntity, "chat_overlay_account_not_found",
					"One of the selected connected accounts does not exist.")
				return
			}
		}

		if err := profiles.SetAccounts(r.Context(), id, body.AccountIDs); err != nil {
			writeChatOverlayError(w, logger, r, err)
			return
		}
		rebuildChatOverlayRuntime(r.Context(), logger, runtime, id)
		writeJSON(w, logger, http.StatusOK, map[string]any{"accountIds": body.AccountIDs})
	}
}

// --- hidden users --------------------------------------------------------------

type chatOverlayHiddenUserResponse struct {
	ProviderID         string `json:"providerId"`
	ConnectedAccountID string `json:"connectedAccountId"`
	ProviderUserID     string `json:"providerUserId"`
	Label              string `json:"label,omitempty"`
	CreatedAt          string `json:"createdAt"`
}

func toChatOverlayHiddenUserResponse(u chatoverlaydomain.HiddenUser) chatOverlayHiddenUserResponse {
	return chatOverlayHiddenUserResponse{
		ProviderID: string(u.ProviderID), ConnectedAccountID: u.ConnectedAccountID,
		ProviderUserID: u.ProviderUserID, Label: u.Label, CreatedAt: u.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func handleListChatOverlayHiddenUsers(logger *slog.Logger, profiles ChatOverlayProfileService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := profiles.HiddenUsers(r.Context(), r.PathValue("id"))
		if err != nil {
			writeChatOverlayError(w, logger, r, err)
			return
		}
		resp := make([]chatOverlayHiddenUserResponse, 0, len(list))
		for _, u := range list {
			resp = append(resp, toChatOverlayHiddenUserResponse(u))
		}
		writeJSON(w, logger, http.StatusOK, map[string]any{"items": resp})
	}
}

type addChatOverlayHiddenUserRequest struct {
	ProviderID         string `json:"providerId"`
	ConnectedAccountID string `json:"connectedAccountId"`
	ProviderUserID     string `json:"providerUserId"`
	Label              string `json:"label"`
}

func handleAddChatOverlayHiddenUser(logger *slog.Logger, accounts AccountService, profiles ChatOverlayProfileService, runtime ChatOverlayRuntime) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var body addChatOverlayHiddenUserRequest
		if err := decodeJSON(w, r, &body); err != nil {
			writeDecodeError(w, logger, err)
			return
		}
		if body.ProviderID == "" || body.ConnectedAccountID == "" || body.ProviderUserID == "" {
			writeError(w, logger, http.StatusUnprocessableEntity, "chat_overlay_user_invalid",
				"providerId, connectedAccountId, and providerUserId are all required.")
			return
		}
		if _, err := accounts.GetAccount(r.Context(), body.ConnectedAccountID); err != nil {
			writeError(w, logger, http.StatusUnprocessableEntity, "chat_overlay_account_not_found",
				"The referenced connected account does not exist.")
			return
		}

		saved, err := profiles.HideUser(r.Context(), id, chatoverlaydomain.ProviderID(body.ProviderID), body.ConnectedAccountID, body.ProviderUserID, body.Label)
		if err != nil {
			writeChatOverlayError(w, logger, r, err)
			return
		}
		rebuildChatOverlayRuntime(r.Context(), logger, runtime, id)
		writeJSON(w, logger, http.StatusOK, toChatOverlayHiddenUserResponse(saved))
	}
}

func handleRemoveChatOverlayHiddenUser(logger *slog.Logger, profiles ChatOverlayProfileService, runtime ChatOverlayRuntime) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}
		id := r.PathValue("id")
		q := r.URL.Query()
		providerID, connectedAccountID, providerUserID := q.Get("providerId"), q.Get("connectedAccountId"), q.Get("providerUserId")
		if providerID == "" || connectedAccountID == "" || providerUserID == "" {
			writeError(w, logger, http.StatusUnprocessableEntity, "chat_overlay_user_invalid",
				"providerId, connectedAccountId, and providerUserId query parameters are all required.")
			return
		}

		if err := profiles.UnhideUser(r.Context(), id, chatoverlaydomain.ProviderID(providerID), connectedAccountID, providerUserID); err != nil {
			writeChatOverlayError(w, logger, r, err)
			return
		}
		rebuildChatOverlayRuntime(r.Context(), logger, runtime, id)
		w.WriteHeader(http.StatusNoContent)
	}
}

// --- blocked terms ---------------------------------------------------------

type chatOverlayBlockedTermResponse struct {
	ID        string `json:"id"`
	Value     string `json:"value"`
	MatchMode string `json:"matchMode"`
	CreatedAt string `json:"createdAt"`
}

func toChatOverlayBlockedTermResponse(t chatoverlaydomain.BlockedTerm) chatOverlayBlockedTermResponse {
	return chatOverlayBlockedTermResponse{
		ID: t.ID, Value: t.Value, MatchMode: string(t.MatchMode), CreatedAt: t.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func handleListChatOverlayBlockedTerms(logger *slog.Logger, profiles ChatOverlayProfileService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := profiles.BlockedTerms(r.Context(), r.PathValue("id"))
		if err != nil {
			writeChatOverlayError(w, logger, r, err)
			return
		}
		resp := make([]chatOverlayBlockedTermResponse, 0, len(list))
		for _, t := range list {
			resp = append(resp, toChatOverlayBlockedTermResponse(t))
		}
		writeJSON(w, logger, http.StatusOK, map[string]any{"items": resp})
	}
}

type addChatOverlayBlockedTermRequest struct {
	Value     string `json:"value"`
	MatchMode string `json:"matchMode"`
}

func handleAddChatOverlayBlockedTerm(logger *slog.Logger, profiles ChatOverlayProfileService, runtime ChatOverlayRuntime) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var body addChatOverlayBlockedTermRequest
		if err := decodeJSON(w, r, &body); err != nil {
			writeDecodeError(w, logger, err)
			return
		}

		saved, err := profiles.AddBlockedTerm(r.Context(), id, body.Value, chatoverlaydomain.MatchMode(body.MatchMode))
		if err != nil {
			writeChatOverlayError(w, logger, r, err)
			return
		}
		rebuildChatOverlayRuntime(r.Context(), logger, runtime, id)
		writeJSON(w, logger, http.StatusOK, toChatOverlayBlockedTermResponse(saved))
	}
}

func handleRemoveChatOverlayBlockedTerm(logger *slog.Logger, profiles ChatOverlayProfileService, runtime ChatOverlayRuntime) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}
		id := r.PathValue("id")
		if err := profiles.RemoveBlockedTerm(r.Context(), id, r.PathValue("termId")); err != nil {
			writeChatOverlayError(w, logger, r, err)
			return
		}
		rebuildChatOverlayRuntime(r.Context(), logger, runtime, id)
		w.WriteHeader(http.StatusNoContent)
	}
}

// --- activity types ----------------------------------------------------------

func handleGetChatOverlayActivityTypes(logger *slog.Logger, profiles ChatOverlayProfileService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := profiles.ActivityTypes(r.Context(), r.PathValue("id"))
		if err != nil {
			writeChatOverlayError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, map[string]any{"activityTypes": list})
	}
}

type putChatOverlayActivityTypesRequest struct {
	ActivityTypes []string `json:"activityTypes"`
}

func handlePutChatOverlayActivityTypes(logger *slog.Logger, profiles ChatOverlayProfileService, runtime ChatOverlayRuntime) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var body putChatOverlayActivityTypesRequest
		if err := decodeJSON(w, r, &body); err != nil {
			writeDecodeError(w, logger, err)
			return
		}
		if err := profiles.SetActivityTypes(r.Context(), id, body.ActivityTypes); err != nil {
			writeChatOverlayError(w, logger, r, err)
			return
		}
		rebuildChatOverlayRuntime(r.Context(), logger, runtime, id)
		writeJSON(w, logger, http.StatusOK, map[string]any{"activityTypes": body.ActivityTypes})
	}
}

// --- public config -----------------------------------------------------------

// publicChatOverlayConfigResponse carries only what the Browser Source
// renderer needs to draw the overlay - never the management id, blocked
// terms, hidden-user lists, or raw account ids. See Part 15's own
// requirement list.
type publicChatOverlayConfigResponse struct {
	SchemaVersion int `json:"schemaVersion"`

	LayoutMode          string `json:"layoutMode"`
	StackDirection      string `json:"stackDirection"`
	HorizontalAlignment string `json:"horizontalAlignment"`

	ShowPlatformIcon bool `json:"showPlatformIcon"`
	ShowPlatformName bool `json:"showPlatformName"`
	ShowTimestamp    bool `json:"showTimestamp"`

	MaxVisibleItems        int `json:"maxVisibleItems"`
	MessageLifetimeSeconds int `json:"messageLifetimeSeconds"`

	FontFamily        string  `json:"fontFamily"`
	FontSize          int     `json:"fontSize"`
	FontWeight        int     `json:"fontWeight"`
	LineHeight        float64 `json:"lineHeight"`
	TextColor         string  `json:"textColor"`
	UsernameColorMode string  `json:"usernameColorMode"`
	BubbleColor       string  `json:"bubbleColor"`
	BubbleOpacity     float64 `json:"bubbleOpacity"`
	BorderRadius      int     `json:"borderRadius"`
	ItemSpacing       int     `json:"itemSpacing"`
	TextOutline       bool    `json:"textOutline"`
	TextShadow        bool    `json:"textShadow"`

	EntryAnimation      string `json:"entryAnimation"`
	ExitAnimation       string `json:"exitAnimation"`
	AnimationDurationMS int    `json:"animationDurationMs"`

	HighlightBroadcaster bool `json:"highlightBroadcaster"`
	HighlightModerators  bool `json:"highlightModerators"`
	HighlightSubscribers bool `json:"highlightSubscribers"`
	HighlightVIPs        bool `json:"highlightVips"`

	Language string `json:"language"`
}

func toPublicChatOverlayConfigResponse(p chatoverlaydomain.Profile) publicChatOverlayConfigResponse {
	return publicChatOverlayConfigResponse{
		SchemaVersion: co.CurrentVersion,
		LayoutMode:    string(p.LayoutMode), StackDirection: string(p.StackDirection), HorizontalAlignment: string(p.HorizontalAlignment),

		ShowPlatformIcon: p.ShowPlatformIcon, ShowPlatformName: p.ShowPlatformName, ShowTimestamp: p.ShowTimestamp,

		MaxVisibleItems: p.MaxVisibleItems, MessageLifetimeSeconds: p.MessageLifetimeSeconds,

		FontFamily: string(p.FontFamily), FontSize: p.FontSize, FontWeight: p.FontWeight, LineHeight: p.LineHeight,
		TextColor: p.TextColor, UsernameColorMode: string(p.UsernameColorMode), BubbleColor: p.BubbleColor,
		BubbleOpacity: p.BubbleOpacity, BorderRadius: p.BorderRadius, ItemSpacing: p.ItemSpacing,
		TextOutline: p.TextOutline, TextShadow: p.TextShadow,

		EntryAnimation: string(p.EntryAnimation), ExitAnimation: string(p.ExitAnimation), AnimationDurationMS: p.AnimationDurationMS,

		HighlightBroadcaster: p.HighlightBroadcaster, HighlightModerators: p.HighlightModerators,
		HighlightSubscribers: p.HighlightSubscribers, HighlightVIPs: p.HighlightVIPs,

		Language: string(p.Language),
	}
}

// resolvePublicChatOverlayProfile looks a profile up by its public slug
// and reports whether it is unavailable (unknown slug or disabled) -
// config/items answer an explicit, structured error for an unavailable
// overlay (so the management preview surfaces it clearly); the stream
// handler instead renders an empty overlay rather than a hard error (see
// handlePublicChatOverlayStream's own doc comment).
func resolvePublicChatOverlayProfile(ctx context.Context, profiles ChatOverlayProfileService, slug string) (chatoverlaydomain.Profile, bool, error) {
	p, err := profiles.GetProfileByPublicSlug(ctx, slug)
	if err != nil {
		return chatoverlaydomain.Profile{}, false, err
	}
	return p, p.Enabled, nil
}

func handleGetPublicChatOverlayConfig(logger *slog.Logger, profiles ChatOverlayProfileService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, enabled, err := resolvePublicChatOverlayProfile(r.Context(), profiles, r.PathValue("slug"))
		if err != nil {
			writeChatOverlayError(w, logger, r, err)
			return
		}
		if !enabled {
			writeError(w, logger, http.StatusConflict, "chat_overlay_disabled", "This overlay is currently disabled.")
			return
		}
		writeJSON(w, logger, http.StatusOK, toPublicChatOverlayConfigResponse(p))
	}
}

// --- public items --------------------------------------------------------

type publicChatOverlayBadgeResponse struct {
	SetID      string `json:"setId"`
	ID         string `json:"id"`
	ImageURL1x string `json:"imageUrl1x,omitempty"`
	ImageURL2x string `json:"imageUrl2x,omitempty"`
	ImageURL4x string `json:"imageUrl4x,omitempty"`
}

type publicChatOverlayUserResponse struct {
	DisplayName   string                           `json:"displayName,omitempty"`
	Color         string                           `json:"color,omitempty"`
	AvatarURL     string                           `json:"avatarUrl,omitempty"`
	Badges        []publicChatOverlayBadgeResponse `json:"badges,omitempty"`
	Anonymous     bool                             `json:"anonymous"`
	IsBroadcaster bool                             `json:"isBroadcaster,omitempty"`
	IsModerator   bool                             `json:"isModerator,omitempty"`
	IsSubscriber  bool                             `json:"isSubscriber,omitempty"`
	IsVIP         bool                             `json:"isVip,omitempty"`
}

type publicChatOverlayFragmentResponse struct {
	Type          string `json:"type"`
	Text          string `json:"text"`
	EmoteImageURL string `json:"emoteImageUrl,omitempty"`
}

type publicChatOverlayMessageResponse struct {
	PlainText string                              `json:"plainText"`
	Fragments []publicChatOverlayFragmentResponse `json:"fragments"`
}

type publicChatOverlayActivityResponse struct {
	ActivityType string   `json:"activityType"`
	Amount       *float64 `json:"amount,omitempty"`
	Currency     string   `json:"currency,omitempty"`
	Quantity     *int64   `json:"quantity,omitempty"`
}

// publicChatOverlayRemoveResponse is the wire shape of a
// "chat-overlay.remove" SSE event's data payload - only a stable id and
// a stable, closed-enum reason (see co.RemoveReason's own doc comment).
// Never the removed item, its message text, or any other content -
// there is no field here a bug could accidentally populate with it.
type publicChatOverlayRemoveResponse struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// publicChatOverlayItemResponse is the wire shape of one public overlay
// item - deliberately smaller than operatorChatItemResponse. Never
// carries a raw connected-account id, a provider user id, or (for a
// deleted item) the original message text - see co.Item's own doc
// comment, which this mirrors field-for-field minus everything
// operator-only.
type publicChatOverlayItemResponse struct {
	Version      int    `json:"version"`
	Sequence     uint64 `json:"sequence"`
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	ProviderID   string `json:"providerId"`
	AccountLabel string `json:"accountLabel,omitempty"`
	OccurredAt   string `json:"occurredAt"`

	User     *publicChatOverlayUserResponse     `json:"user,omitempty"`
	Message  *publicChatOverlayMessageResponse  `json:"message,omitempty"`
	Activity *publicChatOverlayActivityResponse `json:"activity,omitempty"`

	Deleted   bool `json:"deleted"`
	Synthetic bool `json:"synthetic"`
}

func toPublicChatOverlayUserResponse(ctx context.Context, sourceAccountID string, u *co.User, assets OperatorChatAssetResolver) *publicChatOverlayUserResponse {
	if u == nil {
		return nil
	}
	if u.Anonymous {
		return &publicChatOverlayUserResponse{Anonymous: true}
	}
	badges := make([]publicChatOverlayBadgeResponse, 0, len(u.Badges))
	for _, b := range u.Badges {
		resp := publicChatOverlayBadgeResponse{SetID: b.SetID, ID: b.ID}
		if assets != nil {
			if img, ok := assets.ResolveBadge(ctx, sourceAccountID, b.SetID, b.ID); ok {
				resp.ImageURL1x, resp.ImageURL2x, resp.ImageURL4x = img.URL1x, img.URL2x, img.URL4x
			}
		}
		badges = append(badges, resp)
	}
	return &publicChatOverlayUserResponse{
		DisplayName: u.DisplayName, Color: u.Color, AvatarURL: u.AvatarURL, Badges: badges,
		IsBroadcaster: u.IsBroadcaster, IsModerator: u.IsModerator, IsSubscriber: u.IsSubscriber, IsVIP: u.IsVIP,
	}
}

func toPublicChatOverlayMessageResponse(m *co.Message) *publicChatOverlayMessageResponse {
	if m == nil {
		return nil
	}
	fragments := make([]publicChatOverlayFragmentResponse, 0, len(m.Fragments))
	for _, f := range m.Fragments {
		resp := publicChatOverlayFragmentResponse{Type: string(f.Type), Text: f.Text}
		if f.Type == co.FragmentEmote {
			resp.EmoteImageURL = chatassets.EmoteImageURL(f.EmoteID)
		}
		fragments = append(fragments, resp)
	}
	return &publicChatOverlayMessageResponse{PlainText: m.PlainText, Fragments: fragments}
}

func toPublicChatOverlayItemResponse(ctx context.Context, item co.Item, assets OperatorChatAssetResolver) publicChatOverlayItemResponse {
	resp := publicChatOverlayItemResponse{
		Version: item.Version, Sequence: item.Sequence, ID: item.ID, Kind: string(item.Kind),
		ProviderID: item.ProviderID, AccountLabel: item.AccountLabel, OccurredAt: item.OccurredAt.UTC().Format(time.RFC3339Nano),
		User: toPublicChatOverlayUserResponse(ctx, item.SourceAccountID, item.User, assets), Message: toPublicChatOverlayMessageResponse(item.Message),
		Deleted: item.Deleted, Synthetic: item.Synthetic,
	}
	if item.Activity != nil {
		resp.Activity = &publicChatOverlayActivityResponse{
			ActivityType: item.Activity.ActivityType, Amount: item.Activity.Amount,
			Currency: item.Activity.Currency, Quantity: item.Activity.Quantity,
		}
	}
	return resp
}

func handleGetPublicChatOverlayItems(logger *slog.Logger, profiles ChatOverlayProfileService, runtime ChatOverlayRuntime, assets OperatorChatAssetResolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, enabled, err := resolvePublicChatOverlayProfile(r.Context(), profiles, r.PathValue("slug"))
		if err != nil {
			writeChatOverlayError(w, logger, r, err)
			return
		}
		if !enabled {
			writeError(w, logger, http.StatusConflict, "chat_overlay_disabled", "This overlay is currently disabled.")
			return
		}

		proj, err := runtime.EnsureOverlay(r.Context(), p.ID)
		if err != nil {
			writeError(w, logger, http.StatusServiceUnavailable, "chat_overlay_unavailable", "The overlay projection is unavailable.")
			return
		}

		items := proj.CurrentItems()
		resp := make([]publicChatOverlayItemResponse, 0, len(items))
		for _, item := range items {
			resp = append(resp, toPublicChatOverlayItemResponse(r.Context(), item, assets))
		}
		writeJSON(w, logger, http.StatusOK, map[string]any{"items": resp})
	}
}

// --- public SSE stream -----------------------------------------------------

// chatOverlayStreamLimiter bounds live SSE clients per overlay (Part 15:
// "bounded clients per overlay") - independent per overlay, so one
// overlay reaching its cap never affects another's stream.
type chatOverlayStreamLimiter struct {
	mu     sync.Mutex
	counts map[string]int
}

func newChatOverlayStreamLimiter() *chatOverlayStreamLimiter {
	return &chatOverlayStreamLimiter{counts: make(map[string]int)}
}

func (l *chatOverlayStreamLimiter) acquire(overlayID string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.counts[overlayID] >= maxChatOverlaySSEClientsPerOverlay {
		return false
	}
	l.counts[overlayID]++
	return true
}

func (l *chatOverlayStreamLimiter) release(overlayID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.counts[overlayID]--
	if l.counts[overlayID] <= 0 {
		delete(l.counts, overlayID)
	}
}

// handlePublicChatOverlayStream serves GET
// /api/public/chat-overlays/{slug}/stream over Server-Sent Events -
// mirrors handleOperatorChatStream's own contract (Last-Event-ID replay,
// an explicit gap event, periodic keepalive, a bounded client count)
// over the public per-overlay revision stream instead. An unknown or
// disabled overlay never answers with a hard HTTP error here (Part 15:
// "renders transparent/empty by default, not a large backend error on
// the live broadcast") - it opens a normal 200 SSE connection, sends one
// empty reset, and then idles on keepalives only.
func handlePublicChatOverlayStream(
	logger *slog.Logger, profiles ChatOverlayProfileService, runtime ChatOverlayRuntime, assets OperatorChatAssetResolver, limiter *chatOverlayStreamLimiter,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeError(w, logger, http.StatusInternalServerError, "internal_error", "Streaming is not supported by this response writer.")
			return
		}

		p, enabled, err := resolvePublicChatOverlayProfile(r.Context(), profiles, r.PathValue("slug"))
		unavailable := err != nil || !enabled

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		keepalive := time.NewTicker(chatOverlaySSEKeepalive)
		defer keepalive.Stop()

		if unavailable {
			_ = writeSSEEvent(w, "chat-overlay.reset", 0, map[string]any{"items": []publicChatOverlayItemResponse{}})
			flusher.Flush()
			for {
				select {
				case <-r.Context().Done():
					return
				case <-keepalive.C:
					writeSSEComment(w, "keepalive")
					flusher.Flush()
				}
			}
		}

		if !limiter.acquire(p.ID) {
			_ = writeSSEEvent(w, "chat-overlay.gap", 0, map[string]string{"reason": "stream_limit_reached"})
			flusher.Flush()
			return
		}
		defer limiter.release(p.ID)

		proj, err := runtime.EnsureOverlay(r.Context(), p.ID)
		if err != nil {
			_ = writeSSEEvent(w, "chat-overlay.reset", 0, map[string]any{"items": []publicChatOverlayItemResponse{}})
			flusher.Flush()
			return
		}

		var after uint64
		if raw := r.Header.Get("Last-Event-ID"); raw != "" {
			if parsed, err := strconv.ParseUint(raw, 10, 64); err == nil {
				after = parsed
			}
		} else {
			after = parseUintQuery(r, "after", 0)
		}

		sub, gap, err := proj.Subscribe(after)
		if err != nil {
			_ = writeSSEEvent(w, "chat-overlay.reset", 0, map[string]any{"items": []publicChatOverlayItemResponse{}})
			flusher.Flush()
			return
		}
		defer sub.Cancel()

		if gap {
			_ = writeSSEEvent(w, "chat-overlay.gap", 0, map[string]string{"reason": "sequence_evicted"})
			flusher.Flush()
		}

		for {
			select {
			case <-r.Context().Done():
				return
			case rev, open := <-sub.Revisions():
				if !open {
					select {
					case reason := <-sub.Closed():
						if reason == co.ReasonSlowConsumer {
							_ = writeSSEEvent(w, "chat-overlay.gap", 0, map[string]string{"reason": "slow_consumer"})
							flusher.Flush()
						}
					default:
					}
					return
				}
				if err := writeChatOverlayRevisionEvent(r.Context(), w, rev, assets); err != nil {
					logger.Warn("failed to write chat overlay SSE event", "error", err)
					return
				}
				flusher.Flush()
			case <-keepalive.C:
				writeSSEComment(w, "keepalive")
				flusher.Flush()
			}
		}
	}
}

func writeChatOverlayRevisionEvent(ctx context.Context, w http.ResponseWriter, rev co.Revision, assets OperatorChatAssetResolver) error {
	switch rev.Operation {
	case co.OpUpsert:
		if rev.Item == nil {
			return nil
		}
		return writeSSEEvent(w, "chat-overlay.upsert", rev.Sequence, toPublicChatOverlayItemResponse(ctx, *rev.Item, assets))
	case co.OpRemove:
		reason := rev.Reason
		if reason == "" {
			reason = co.RemoveReasonUnknown
		}
		return writeSSEEvent(w, "chat-overlay.remove", rev.Sequence, publicChatOverlayRemoveResponse{ID: rev.RemovedID, Reason: string(reason)})
	case co.OpReset:
		resp := make([]publicChatOverlayItemResponse, 0, len(rev.ResetItems))
		for _, item := range rev.ResetItems {
			resp = append(resp, toPublicChatOverlayItemResponse(ctx, item, assets))
		}
		return writeSSEEvent(w, "chat-overlay.reset", rev.Sequence, map[string]any{"items": resp})
	default:
		return nil
	}
}

// --- error mapping ---------------------------------------------------------

func writeChatOverlayError(w http.ResponseWriter, logger *slog.Logger, r *http.Request, err error) {
	switch {
	case errors.Is(err, chatoverlaydomain.ErrNotFound), errors.Is(err, chatoverlaydomain.ErrPublicSlugNotFound):
		writeError(w, logger, http.StatusNotFound, "chat_overlay_not_found", "The requested overlay does not exist.")
	case errors.Is(err, chatoverlaydomain.ErrAccountNotFound):
		writeError(w, logger, http.StatusUnprocessableEntity, "chat_overlay_account_not_found", "The referenced connected account does not exist.")
	case errors.Is(err, chatoverlaydomain.ErrUserNotFound):
		writeError(w, logger, http.StatusNotFound, "chat_overlay_user_invalid", "No matching hidden-user entry exists.")
	case errors.Is(err, chatoverlaydomain.ErrTermNotFound):
		writeError(w, logger, http.StatusNotFound, "chat_overlay_term_invalid", "No matching blocked-term entry exists.")
	case errors.Is(err, co.ErrClosed), errors.Is(err, co.ErrNotFound):
		writeError(w, logger, http.StatusServiceUnavailable, "chat_overlay_unavailable", "The overlay projection is unavailable.")
	default:
		writeDomainError(w, logger, r, err)
	}
}
