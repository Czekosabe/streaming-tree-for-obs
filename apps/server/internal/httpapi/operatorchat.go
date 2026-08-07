package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/streaming-tree/server/internal/domain/operatorchatprefs"
	oc "github.com/streaming-tree/server/internal/operatorchat"
	"github.com/streaming-tree/server/internal/provider/twitch/chatassets"
)

// OperatorChatProjectionService is the subset of operatorchat.Projection the
// HTTP layer needs.
type OperatorChatProjectionService interface {
	Subscribe(after uint64) (*oc.Subscription, bool, error)
	ItemsAfter(after uint64, limit int) ([]oc.Item, bool)
	Snapshot() oc.Status
}

// OperatorChatPrefsService is the subset of operatorchatprefs.Service the
// HTTP layer needs.
type OperatorChatPrefsService interface {
	Preferences(ctx context.Context) (operatorchatprefs.Preferences, error)
	ReplacePreferences(ctx context.Context, p operatorchatprefs.Preferences) (operatorchatprefs.Preferences, error)
	AccountVisibility(ctx context.Context) ([]operatorchatprefs.AccountVisibility, error)
	SetAccountVisibility(ctx context.Context, accountID string, visible bool) (operatorchatprefs.AccountVisibility, error)
	HiddenUsers(ctx context.Context) ([]operatorchatprefs.UserRef, error)
	HideUser(ctx context.Context, providerID operatorchatprefs.ProviderID, connectedAccountID, providerUserID, label string) (operatorchatprefs.UserRef, error)
	UnhideUser(ctx context.Context, id string) error
	BotUsers(ctx context.Context) ([]operatorchatprefs.UserRef, error)
	MarkBotUser(ctx context.Context, providerID operatorchatprefs.ProviderID, connectedAccountID, providerUserID, label string) (operatorchatprefs.UserRef, error)
	UnmarkBotUser(ctx context.Context, id string) error
}

// OperatorChatAssetResolver is the subset of chatassets.Resolver the HTTP
// layer needs to attach badge image URLs at serialization time - a
// presentation-layer concern kept out of internal/operatorchat itself. Nil
// is a valid Options value: items still serialize, just without resolved
// badge image URLs (text over decoration - see chatassets' own doc
// comment).
type OperatorChatAssetResolver interface {
	ResolveBadge(ctx context.Context, accountID, setID, version string) (chatassets.Image, bool)
}

const (
	defaultOperatorChatItemsLimit = 200
	maxOperatorChatItemsLimit     = 1000
	maxOperatorChatSSEClients     = 32
	operatorChatSSEKeepalive      = 15 * time.Second
)

// registerOperatorChatRoutes wires the Stage 9 unified-operator-chat API:
// the projection's status/snapshot/SSE endpoints, persisted preferences,
// and the hidden-user/bot-user lists.
//
// onBotUsersChanged is called after a successful bot-user add/remove -
// Stage 10's chat overlay filtering shares this exact list (see
// internal/chatoverlay/filtering.go's own doc comment on why it, and
// only it, is shared rather than duplicated per overlay), so every
// running overlay's projection needs a rebuild whenever it changes. May
// be nil when no chat-overlay runtime is configured.
func registerOperatorChatRoutes(
	mux *http.ServeMux, logger *slog.Logger,
	accounts AccountService, projection OperatorChatProjectionService, prefs OperatorChatPrefsService, assets OperatorChatAssetResolver,
	onBotUsersChanged func(ctx context.Context),
) {
	mux.HandleFunc("GET /api/operator-chat/status", handleGetOperatorChatStatus(logger, projection))
	mux.HandleFunc("/api/operator-chat/status", methodNotAllowed(logger, http.MethodGet))

	mux.HandleFunc("GET /api/operator-chat/items", handleGetOperatorChatItems(logger, projection, assets))
	mux.HandleFunc("/api/operator-chat/items", methodNotAllowed(logger, http.MethodGet))

	var sseClients atomic.Int32
	mux.HandleFunc("GET /api/operator-chat/stream", handleOperatorChatStream(logger, projection, assets, &sseClients))
	mux.HandleFunc("/api/operator-chat/stream", methodNotAllowed(logger, http.MethodGet))

	mux.HandleFunc("GET /api/operator-chat/preferences", handleGetOperatorChatPreferences(logger, prefs))
	mux.HandleFunc("PUT /api/operator-chat/preferences", handlePutOperatorChatPreferences(logger, prefs))
	mux.HandleFunc("/api/operator-chat/preferences", methodNotAllowed(logger, http.MethodGet, http.MethodPut))

	mux.HandleFunc("GET /api/operator-chat/account-visibility", handleGetOperatorChatAccountVisibility(logger, prefs))
	mux.HandleFunc("/api/operator-chat/account-visibility", methodNotAllowed(logger, http.MethodGet))

	mux.HandleFunc("PUT /api/operator-chat/account-visibility/{id}", handlePutOperatorChatAccountVisibility(logger, accounts, prefs))
	mux.HandleFunc("/api/operator-chat/account-visibility/{id}", methodNotAllowed(logger, http.MethodPut))

	mux.HandleFunc("GET /api/operator-chat/hidden-users", handleListOperatorChatUserRefs(logger, prefs.HiddenUsers))
	mux.HandleFunc("POST /api/operator-chat/hidden-users", handleAddOperatorChatUserRef(logger, accounts, prefs.HideUser))
	mux.HandleFunc("/api/operator-chat/hidden-users", methodNotAllowed(logger, http.MethodGet, http.MethodPost))
	mux.HandleFunc("DELETE /api/operator-chat/hidden-users/{id}", handleRemoveOperatorChatUserRef(logger, prefs.UnhideUser))
	mux.HandleFunc("/api/operator-chat/hidden-users/{id}", methodNotAllowed(logger, http.MethodDelete))

	mux.HandleFunc("GET /api/operator-chat/bot-users", handleListOperatorChatUserRefs(logger, prefs.BotUsers))
	mux.HandleFunc("POST /api/operator-chat/bot-users", handleAddOperatorChatUserRef(logger, accounts, afterUserRefAdd(prefs.MarkBotUser, onBotUsersChanged)))
	mux.HandleFunc("/api/operator-chat/bot-users", methodNotAllowed(logger, http.MethodGet, http.MethodPost))
	mux.HandleFunc("DELETE /api/operator-chat/bot-users/{id}", handleRemoveOperatorChatUserRef(logger, afterUserRefRemove(prefs.UnmarkBotUser, onBotUsersChanged)))
	mux.HandleFunc("/api/operator-chat/bot-users/{id}", methodNotAllowed(logger, http.MethodDelete))
}

// afterUserRefAdd/afterUserRefRemove call onChanged only after the
// wrapped operation actually succeeds - a failed add/remove never
// triggers a chat-overlay rebuild. onChanged may be nil.
func afterUserRefAdd(
	add func(ctx context.Context, providerID operatorchatprefs.ProviderID, connectedAccountID, providerUserID, label string) (operatorchatprefs.UserRef, error),
	onChanged func(ctx context.Context),
) func(ctx context.Context, providerID operatorchatprefs.ProviderID, connectedAccountID, providerUserID, label string) (operatorchatprefs.UserRef, error) {
	return func(ctx context.Context, providerID operatorchatprefs.ProviderID, connectedAccountID, providerUserID, label string) (operatorchatprefs.UserRef, error) {
		ref, err := add(ctx, providerID, connectedAccountID, providerUserID, label)
		if err == nil && onChanged != nil {
			onChanged(ctx)
		}
		return ref, err
	}
}

func afterUserRefRemove(remove func(ctx context.Context, id string) error, onChanged func(ctx context.Context)) func(ctx context.Context, id string) error {
	return func(ctx context.Context, id string) error {
		err := remove(ctx, id)
		if err == nil && onChanged != nil {
			onChanged(ctx)
		}
		return err
	}
}

// --- response DTOs -----------------------------------------------------

type operatorChatBadgeResponse struct {
	SetID      string `json:"setId"`
	ID         string `json:"id"`
	Info       string `json:"info,omitempty"`
	ImageURL1x string `json:"imageUrl1x,omitempty"`
	ImageURL2x string `json:"imageUrl2x,omitempty"`
	ImageURL4x string `json:"imageUrl4x,omitempty"`
}

type operatorChatUserResponse struct {
	ProviderUserID string                      `json:"providerUserId,omitempty"`
	Login          string                      `json:"login,omitempty"`
	DisplayName    string                      `json:"displayName,omitempty"`
	AvatarURL      string                      `json:"avatarUrl,omitempty"`
	Color          string                      `json:"color,omitempty"`
	Badges         []operatorChatBadgeResponse `json:"badges,omitempty"`
	Anonymous      bool                        `json:"anonymous"`
}

type operatorChatFragmentResponse struct {
	Type               string `json:"type"`
	Text               string `json:"text"`
	EmoteID            string `json:"emoteId,omitempty"`
	EmoteImageURL      string `json:"emoteImageUrl,omitempty"`
	CheermotePrefix    string `json:"cheermotePrefix,omitempty"`
	CheermoteBits      int    `json:"cheermoteBits,omitempty"`
	MentionUserID      string `json:"mentionUserId,omitempty"`
	MentionLogin       string `json:"mentionLogin,omitempty"`
	MentionDisplayName string `json:"mentionDisplayName,omitempty"`
}

type operatorChatMessageResponse struct {
	PlainText string                         `json:"plainText"`
	Fragments []operatorChatFragmentResponse `json:"fragments"`
}

type operatorChatActivityResponse struct {
	ActivityType string   `json:"activityType"`
	Amount       *float64 `json:"amount,omitempty"`
	Currency     string   `json:"currency,omitempty"`
	Quantity     *int64   `json:"quantity,omitempty"`
}

type operatorChatModerationResponse struct {
	Action           string `json:"action"`
	TargetUserID     string `json:"targetUserId,omitempty"`
	TargetMessageRef string `json:"targetMessageRef,omitempty"`
}

type operatorChatLifecycleResponse struct {
	Deleted        bool   `json:"deleted"`
	DeletedAt      string `json:"deletedAt,omitempty"`
	DeletionReason string `json:"deletionReason,omitempty"`
}

// operatorChatItemResponse is the public, versioned shape of one
// operator-chat item - identical whether reached via the bounded snapshot
// endpoint or the SSE stream. Never carries a raw provider payload, a
// token, or a session identifier - see operatorchat.Item's own doc
// comment, which this mirrors field-for-field, plus resolved (not raw)
// badge/emote image URLs attached at serialization time.
type operatorChatItemResponse struct {
	Version       int    `json:"version"`
	Sequence      uint64 `json:"sequence"`
	ID            string `json:"id"`
	SourceEventID string `json:"sourceEventId,omitempty"`
	// ProviderMessageID is present only for a message-kind item - see
	// operatorchat.Item.ProviderMessageID's own doc comment. Used by the
	// Chat page's Stage 11A Reply action; never added to the public OBS
	// overlay DTO.
	ProviderMessageID string `json:"providerMessageId,omitempty"`

	ProviderID         string `json:"providerId"`
	ConnectedAccountID string `json:"connectedAccountId"`
	DestinationID      string `json:"destinationId,omitempty"`

	Kind string `json:"kind"`

	OccurredAt string `json:"occurredAt"`
	ReceivedAt string `json:"receivedAt"`

	User       *operatorChatUserResponse       `json:"user,omitempty"`
	Message    *operatorChatMessageResponse    `json:"message,omitempty"`
	Activity   *operatorChatActivityResponse   `json:"activity,omitempty"`
	Moderation *operatorChatModerationResponse `json:"moderation,omitempty"`

	Lifecycle operatorChatLifecycleResponse `json:"lifecycle"`
	Synthetic bool                          `json:"synthetic"`
}

func toOperatorChatUserResponse(ctx context.Context, accountID string, u *oc.User, assets OperatorChatAssetResolver) *operatorChatUserResponse {
	if u == nil {
		return nil
	}
	badges := make([]operatorChatBadgeResponse, 0, len(u.Badges))
	for _, b := range u.Badges {
		resp := operatorChatBadgeResponse{SetID: b.SetID, ID: b.ID, Info: b.Info}
		if assets != nil {
			if img, ok := assets.ResolveBadge(ctx, accountID, b.SetID, b.ID); ok {
				resp.ImageURL1x, resp.ImageURL2x, resp.ImageURL4x = img.URL1x, img.URL2x, img.URL4x
			}
		}
		badges = append(badges, resp)
	}
	return &operatorChatUserResponse{
		ProviderUserID: u.ProviderUserID, Login: u.Login, DisplayName: u.DisplayName,
		AvatarURL: u.AvatarURL, Color: u.Color, Badges: badges, Anonymous: u.Anonymous,
	}
}

func toOperatorChatMessageResponse(m *oc.Message) *operatorChatMessageResponse {
	if m == nil {
		return nil
	}
	fragments := make([]operatorChatFragmentResponse, 0, len(m.Fragments))
	for _, f := range m.Fragments {
		resp := operatorChatFragmentResponse{
			Type: string(f.Type), Text: f.Text, EmoteID: f.EmoteID,
			CheermotePrefix: f.CheermotePrefix, CheermoteBits: f.CheermoteBits,
			MentionUserID: f.MentionUserID, MentionLogin: f.MentionLogin, MentionDisplayName: f.MentionDisplayName,
		}
		if f.Type == oc.FragmentEmote {
			resp.EmoteImageURL = chatassets.EmoteImageURL(f.EmoteID)
		}
		fragments = append(fragments, resp)
	}
	return &operatorChatMessageResponse{PlainText: m.PlainText, Fragments: fragments}
}

func toOperatorChatItemResponse(ctx context.Context, item oc.Item, assets OperatorChatAssetResolver) operatorChatItemResponse {
	resp := operatorChatItemResponse{
		Version: item.Version, Sequence: item.Sequence, ID: item.ID, SourceEventID: item.SourceEventID,
		ProviderMessageID: item.ProviderMessageID,
		ProviderID:        item.ProviderID, ConnectedAccountID: item.ConnectedAccountID, DestinationID: item.DestinationID,
		Kind: string(item.Kind), OccurredAt: item.OccurredAt.UTC().Format(time.RFC3339Nano), ReceivedAt: item.ReceivedAt.UTC().Format(time.RFC3339Nano),
		User: toOperatorChatUserResponse(ctx, item.ConnectedAccountID, item.User, assets), Message: toOperatorChatMessageResponse(item.Message),
		Lifecycle: operatorChatLifecycleResponse{Deleted: item.Lifecycle.Deleted, DeletionReason: string(item.Lifecycle.DeletionReason)},
		Synthetic: item.Synthetic,
	}
	if item.Lifecycle.DeletedAt != nil {
		resp.Lifecycle.DeletedAt = item.Lifecycle.DeletedAt.UTC().Format(time.RFC3339Nano)
	}
	if item.Activity != nil {
		resp.Activity = &operatorChatActivityResponse{
			ActivityType: item.Activity.ActivityType, Amount: item.Activity.Amount,
			Currency: item.Activity.Currency, Quantity: item.Activity.Quantity,
		}
	}
	if item.Moderation != nil {
		resp.Moderation = &operatorChatModerationResponse{
			Action: item.Moderation.Action, TargetUserID: item.Moderation.TargetUserID, TargetMessageRef: item.Moderation.TargetMessageRef,
		}
	}
	return resp
}

// --- item filtering ------------------------------------------------------

// operatorChatItemFilter narrows the items endpoint/stream to a set of
// connected accounts and/or item kinds, and can exclude deleted messages -
// see the stage task's Part 15/19. An empty accountIDs/kinds set means "no
// filter" (everything matches).
type operatorChatItemFilter struct {
	accountIDs     map[string]struct{}
	kinds          map[oc.Kind]struct{}
	includeDeleted bool
}

var validOperatorChatKinds = map[string]oc.Kind{
	"message": oc.KindMessage, "activity": oc.KindActivity, "moderation": oc.KindModeration, "system": oc.KindSystem,
}

func parseOperatorChatItemFilter(r *http.Request) (operatorChatItemFilter, error) {
	filter := operatorChatItemFilter{includeDeleted: true}

	if raw := r.URL.Query()["accountId"]; len(raw) > 0 {
		filter.accountIDs = make(map[string]struct{}, len(raw))
		for _, id := range raw {
			if id == "" {
				continue
			}
			filter.accountIDs[id] = struct{}{}
		}
	}

	if raw := r.URL.Query().Get("kinds"); raw != "" {
		filter.kinds = make(map[oc.Kind]struct{})
		for _, part := range strings.Split(raw, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			kind, ok := validOperatorChatKinds[part]
			if !ok {
				return operatorChatItemFilter{}, fmt.Errorf("%w: unknown kind %q", errOperatorChatInvalidFilter, part)
			}
			filter.kinds[kind] = struct{}{}
		}
	}

	if raw := r.URL.Query().Get("includeDeleted"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return operatorChatItemFilter{}, fmt.Errorf("%w: includeDeleted must be a boolean", errOperatorChatInvalidFilter)
		}
		filter.includeDeleted = parsed
	}

	return filter, nil
}

func (f operatorChatItemFilter) matches(item oc.Item) bool {
	if len(f.accountIDs) > 0 {
		if _, ok := f.accountIDs[item.ConnectedAccountID]; !ok {
			return false
		}
	}
	if len(f.kinds) > 0 {
		if _, ok := f.kinds[item.Kind]; !ok {
			return false
		}
	}
	if !f.includeDeleted && item.Kind == oc.KindMessage && item.Lifecycle.Deleted {
		return false
	}
	return true
}

var errOperatorChatInvalidFilter = errors.New("invalid operator chat filter")

// --- status and snapshot handlers ----------------------------------------

type operatorChatStatusResponse struct {
	SchemaVersion     int    `json:"schemaVersion"`
	BufferCapacity    int    `json:"bufferCapacity"`
	RetainedCount     int    `json:"retainedCount"`
	OldestSequence    uint64 `json:"oldestSequence"`
	NewestSequence    uint64 `json:"newestSequence"`
	ActiveSubscribers int    `json:"activeSubscribers"`
	BusGap            bool   `json:"busGap"`
}

func handleGetOperatorChatStatus(logger *slog.Logger, projection OperatorChatProjectionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		snap := projection.Snapshot()
		writeJSON(w, logger, http.StatusOK, operatorChatStatusResponse{
			SchemaVersion: snap.SchemaVersion, BufferCapacity: snap.Capacity, RetainedCount: snap.RetainedCount,
			OldestSequence: snap.OldestSequence, NewestSequence: snap.NewestSequence,
			ActiveSubscribers: snap.ActiveSubscribers, BusGap: snap.BusGap,
		})
	}
}

func handleGetOperatorChatItems(logger *slog.Logger, projection OperatorChatProjectionService, assets OperatorChatAssetResolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter, err := parseOperatorChatItemFilter(r)
		if err != nil {
			writeError(w, logger, http.StatusUnprocessableEntity, "operator_chat_invalid_filter", err.Error())
			return
		}

		after := parseUintQuery(r, "after", 0)
		limit := parseUintQuery(r, "limit", defaultOperatorChatItemsLimit)
		if limit > maxOperatorChatItemsLimit {
			limit = maxOperatorChatItemsLimit
		}

		items, gap := projection.ItemsAfter(after, int(limit))
		resp := make([]operatorChatItemResponse, 0, len(items))
		for _, item := range items {
			if !filter.matches(item) {
				continue
			}
			resp = append(resp, toOperatorChatItemResponse(r.Context(), item, assets))
		}
		writeJSON(w, logger, http.StatusOK, map[string]any{"items": resp, "gap": gap})
	}
}

// --- SSE stream ------------------------------------------------------------

// handleOperatorChatStream serves GET /api/operator-chat/stream over
// Server-Sent Events - mirrors handleEngagementStream's own contract
// (Last-Event-ID replay, an explicit gap event, periodic keepalive, a
// bounded client count) over the operator-chat projection's own revision
// stream instead of the Event Bus's. Every revision (a brand-new item or a
// lifecycle update to an existing one) is sent as one
// "operator-chat.item" event carrying the item's complete current state -
// see operatorchat's own doc comment on why this stage uses a single
// complete-upsert stream rather than separate item/update event names.
func handleOperatorChatStream(logger *slog.Logger, projection OperatorChatProjectionService, assets OperatorChatAssetResolver, activeClients *atomic.Int32) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeError(w, logger, http.StatusInternalServerError, "internal_error", "Streaming is not supported by this response writer.")
			return
		}

		filter, err := parseOperatorChatItemFilter(r)
		if err != nil {
			writeError(w, logger, http.StatusUnprocessableEntity, "operator_chat_invalid_filter", err.Error())
			return
		}

		if activeClients.Add(1) > maxOperatorChatSSEClients {
			activeClients.Add(-1)
			writeError(w, logger, http.StatusServiceUnavailable, "operator_chat_stream_limit_reached", "Too many active chat streams; try again shortly.")
			return
		}
		defer activeClients.Add(-1)

		var after uint64
		if raw := r.Header.Get("Last-Event-ID"); raw != "" {
			if parsed, err := strconv.ParseUint(raw, 10, 64); err == nil {
				after = parsed
			}
		} else {
			after = parseUintQuery(r, "after", 0)
		}

		sub, gap, err := projection.Subscribe(after)
		if err != nil {
			writeError(w, logger, http.StatusServiceUnavailable, "operator_chat_unavailable", "The operator chat projection is unavailable.")
			return
		}
		defer sub.Cancel()

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		if gap {
			_ = writeSSEEvent(w, "operator-chat.gap", 0, map[string]string{"reason": "sequence_evicted"})
			flusher.Flush()
		}

		keepalive := time.NewTicker(operatorChatSSEKeepalive)
		defer keepalive.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case item, open := <-sub.Items():
				if !open {
					select {
					case reason := <-sub.Closed():
						if reason == oc.ReasonSlowConsumer {
							_ = writeSSEEvent(w, "operator-chat.gap", 0, map[string]string{"reason": "slow_consumer"})
							flusher.Flush()
						}
					default:
					}
					return
				}
				if !filter.matches(item) {
					continue
				}
				if err := writeSSEEvent(w, "operator-chat.item", item.Sequence, toOperatorChatItemResponse(r.Context(), item, assets)); err != nil {
					logger.Warn("failed to write operator chat SSE event", "error", err)
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

// --- preferences -----------------------------------------------------------

type operatorChatPreferencesResponse struct {
	ShowPlatformIcon    bool `json:"showPlatformIcon"`
	ShowPlatformName    bool `json:"showPlatformName"`
	ShowAccountLabel    bool `json:"showAccountLabel"`
	ShowBadges          bool `json:"showBadges"`
	ShowTimestamps      bool `json:"showTimestamps"`
	ShowActivityEvents  bool `json:"showActivityEvents"`
	ShowDeletedMessages bool `json:"showDeletedMessages"`
	HideCommandMessages bool `json:"hideCommandMessages"`
	CompactMode         bool `json:"compactMode"`
}

func toOperatorChatPreferencesResponse(p operatorchatprefs.Preferences) operatorChatPreferencesResponse {
	return operatorChatPreferencesResponse{
		ShowPlatformIcon: p.ShowPlatformIcon, ShowPlatformName: p.ShowPlatformName, ShowAccountLabel: p.ShowAccountLabel,
		ShowBadges: p.ShowBadges, ShowTimestamps: p.ShowTimestamps, ShowActivityEvents: p.ShowActivityEvents,
		ShowDeletedMessages: p.ShowDeletedMessages, HideCommandMessages: p.HideCommandMessages, CompactMode: p.CompactMode,
	}
}

func handleGetOperatorChatPreferences(logger *slog.Logger, prefs OperatorChatPrefsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := prefs.Preferences(r.Context())
		if err != nil {
			writeOperatorChatError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toOperatorChatPreferencesResponse(p))
	}
}

func handlePutOperatorChatPreferences(logger *slog.Logger, prefs OperatorChatPrefsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body operatorChatPreferencesResponse
		if err := decodeJSON(w, r, &body); err != nil {
			writeDecodeError(w, logger, err)
			return
		}

		saved, err := prefs.ReplacePreferences(r.Context(), operatorchatprefs.Preferences{
			ShowPlatformIcon: body.ShowPlatformIcon, ShowPlatformName: body.ShowPlatformName, ShowAccountLabel: body.ShowAccountLabel,
			ShowBadges: body.ShowBadges, ShowTimestamps: body.ShowTimestamps, ShowActivityEvents: body.ShowActivityEvents,
			ShowDeletedMessages: body.ShowDeletedMessages, HideCommandMessages: body.HideCommandMessages, CompactMode: body.CompactMode,
		})
		if err != nil {
			writeOperatorChatError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toOperatorChatPreferencesResponse(saved))
	}
}

// --- account visibility -----------------------------------------------------

type operatorChatAccountVisibilityResponse struct {
	AccountID string `json:"accountId"`
	Visible   bool   `json:"visible"`
}

func handleGetOperatorChatAccountVisibility(logger *slog.Logger, prefs OperatorChatPrefsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := prefs.AccountVisibility(r.Context())
		if err != nil {
			writeOperatorChatError(w, logger, r, err)
			return
		}
		resp := make([]operatorChatAccountVisibilityResponse, 0, len(list))
		for _, v := range list {
			resp = append(resp, operatorChatAccountVisibilityResponse{AccountID: v.AccountID, Visible: v.Visible})
		}
		writeJSON(w, logger, http.StatusOK, map[string]any{"items": resp})
	}
}

type setOperatorChatAccountVisibilityRequest struct {
	Visible bool `json:"visible"`
}

func handlePutOperatorChatAccountVisibility(logger *slog.Logger, accounts AccountService, prefs OperatorChatPrefsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID := r.PathValue("id")
		if _, err := accounts.GetAccount(r.Context(), accountID); err != nil {
			writeAccountError(w, logger, r, err)
			return
		}

		var body setOperatorChatAccountVisibilityRequest
		if err := decodeJSON(w, r, &body); err != nil {
			writeDecodeError(w, logger, err)
			return
		}

		saved, err := prefs.SetAccountVisibility(r.Context(), accountID, body.Visible)
		if err != nil {
			writeOperatorChatError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, operatorChatAccountVisibilityResponse{AccountID: saved.AccountID, Visible: saved.Visible})
	}
}

// --- hidden users / bot users -----------------------------------------------

type operatorChatUserRefResponse struct {
	ID                 string `json:"id"`
	ProviderID         string `json:"providerId"`
	ConnectedAccountID string `json:"connectedAccountId"`
	ProviderUserID     string `json:"providerUserId"`
	Label              string `json:"label,omitempty"`
	CreatedAt          string `json:"createdAt"`
}

func toOperatorChatUserRefResponse(ref operatorchatprefs.UserRef) operatorChatUserRefResponse {
	return operatorChatUserRefResponse{
		ID: ref.ID, ProviderID: string(ref.ProviderID), ConnectedAccountID: ref.ConnectedAccountID,
		ProviderUserID: ref.ProviderUserID, Label: ref.Label, CreatedAt: ref.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func handleListOperatorChatUserRefs(logger *slog.Logger, list func(ctx context.Context) ([]operatorchatprefs.UserRef, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		refs, err := list(r.Context())
		if err != nil {
			writeOperatorChatError(w, logger, r, err)
			return
		}
		resp := make([]operatorChatUserRefResponse, 0, len(refs))
		for _, ref := range refs {
			resp = append(resp, toOperatorChatUserRefResponse(ref))
		}
		writeJSON(w, logger, http.StatusOK, map[string]any{"items": resp})
	}
}

type addOperatorChatUserRefRequest struct {
	ProviderID         string `json:"providerId"`
	ConnectedAccountID string `json:"connectedAccountId"`
	ProviderUserID     string `json:"providerUserId"`
	Label              string `json:"label"`
}

func handleAddOperatorChatUserRef(
	logger *slog.Logger, accounts AccountService,
	add func(ctx context.Context, providerID operatorchatprefs.ProviderID, connectedAccountID, providerUserID, label string) (operatorchatprefs.UserRef, error),
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body addOperatorChatUserRefRequest
		if err := decodeJSON(w, r, &body); err != nil {
			writeDecodeError(w, logger, err)
			return
		}
		if body.ProviderID == "" || body.ConnectedAccountID == "" || body.ProviderUserID == "" {
			writeError(w, logger, http.StatusUnprocessableEntity, "operator_chat_user_invalid",
				"providerId, connectedAccountId, and providerUserId are all required.")
			return
		}
		if _, err := accounts.GetAccount(r.Context(), body.ConnectedAccountID); err != nil {
			writeAccountError(w, logger, r, err)
			return
		}

		saved, err := add(r.Context(), operatorchatprefs.ProviderID(body.ProviderID), body.ConnectedAccountID, body.ProviderUserID, body.Label)
		if err != nil {
			writeOperatorChatError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toOperatorChatUserRefResponse(saved))
	}
}

func handleRemoveOperatorChatUserRef(logger *slog.Logger, remove func(ctx context.Context, id string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}
		id := r.PathValue("id")
		if err := remove(r.Context(), id); err != nil {
			writeOperatorChatError(w, logger, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// --- error mapping ---------------------------------------------------------

func writeOperatorChatError(w http.ResponseWriter, logger *slog.Logger, r *http.Request, err error) {
	switch {
	case errors.Is(err, oc.ErrClosed):
		writeError(w, logger, http.StatusServiceUnavailable, "operator_chat_unavailable", "The operator chat projection is unavailable.")
	case errors.Is(err, operatorchatprefs.ErrAccountNotFound):
		writeError(w, logger, http.StatusNotFound, "operator_chat_account_not_found", "The referenced connected account does not exist.")
	case errors.Is(err, operatorchatprefs.ErrUserNotFound):
		writeError(w, logger, http.StatusNotFound, "operator_chat_user_invalid", "No matching entry exists.")
	default:
		writeDomainError(w, logger, r, err)
	}
}
