package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/streaming-tree/server/internal/domain/account"
	engagement "github.com/streaming-tree/server/internal/domain/engagement"
	"github.com/streaming-tree/server/internal/domain/engagementsettings"
	bus "github.com/streaming-tree/server/internal/engagement"
	"github.com/streaming-tree/server/internal/provider/twitch"
	"github.com/streaming-tree/server/internal/runtime/deviceflow"
	"github.com/streaming-tree/server/internal/runtime/twitchengagement"
)

// EngagementBusService is the subset of bus.Bus the HTTP layer needs.
type EngagementBusService interface {
	Snapshot() bus.Snapshot
	EventsAfter(after uint64, limit int) ([]engagement.Event, bool)
	Subscribe(after uint64) (*bus.Subscription, bool, error)
}

// EngagementConnectorService is the subset of twitchengagement.Manager the
// HTTP layer needs.
type EngagementConnectorService interface {
	Enable(ctx context.Context, accountID string) (twitchengagement.Snapshot, error)
	Disable(ctx context.Context, accountID string) (twitchengagement.Snapshot, error)
	Restart(ctx context.Context, accountID string) (twitchengagement.Snapshot, error)
	Snapshot(accountID string) (twitchengagement.Snapshot, bool)
	Snapshots() []twitchengagement.Snapshot
}

// EngagementSettingsService is the subset of engagementsettings.Service the
// HTTP layer needs.
type EngagementSettingsService interface {
	Get(ctx context.Context, accountID string) (engagementsettings.Settings, bool, error)
}

const (
	defaultEngagementEventsLimit = 100
	maxEngagementEventsLimit     = 500
	maxSSEClients                = 32
	sseKeepaliveInterval         = 15 * time.Second
)

// registerEngagementRoutes wires the Event Bus snapshot/SSE API and the
// per-account Twitch engagement connector management API.
func registerEngagementRoutes(
	mux *http.ServeMux, logger *slog.Logger,
	accounts AccountService, deviceFlow DeviceFlowService,
	evBus EngagementBusService, settings EngagementSettingsService, connectors EngagementConnectorService,
) {
	mux.HandleFunc("GET /api/engagement/status", handleGetEngagementStatus(logger, evBus, connectors))
	mux.HandleFunc("/api/engagement/status", methodNotAllowed(logger, http.MethodGet))

	mux.HandleFunc("GET /api/engagement/events", handleGetEngagementEvents(logger, evBus))
	mux.HandleFunc("/api/engagement/events", methodNotAllowed(logger, http.MethodGet))

	var sseClients atomic.Int32
	mux.HandleFunc("GET /api/engagement/stream", handleEngagementStream(logger, evBus, &sseClients))
	mux.HandleFunc("/api/engagement/stream", methodNotAllowed(logger, http.MethodGet))

	mux.HandleFunc("GET /api/connected-accounts/{id}/engagement", handleGetAccountEngagement(logger, accounts, settings, connectors))
	mux.HandleFunc("PUT /api/connected-accounts/{id}/engagement", handlePutAccountEngagement(logger, accounts, connectors))
	mux.HandleFunc("/api/connected-accounts/{id}/engagement", methodNotAllowed(logger, http.MethodGet, http.MethodPut))

	mux.HandleFunc("POST /api/connected-accounts/{id}/engagement/authorize", handleAuthorizeEngagement(logger, accounts, deviceFlow))
	mux.HandleFunc("/api/connected-accounts/{id}/engagement/authorize", methodNotAllowed(logger, http.MethodPost))

	mux.HandleFunc("POST /api/connected-accounts/{id}/engagement/restart", handleRestartEngagement(logger, connectors))
	mux.HandleFunc("/api/connected-accounts/{id}/engagement/restart", methodNotAllowed(logger, http.MethodPost))
}

// --- response DTOs -------------------------------------------------------

type connectorResponse struct {
	AccountID string `json:"accountId"`
	Enabled   bool   `json:"enabled"`
	State     string `json:"state"`

	BlockerCodes  []string `json:"blockerCodes,omitempty"`
	MissingScopes []string `json:"missingScopes,omitempty"`

	ConnectedAt     string `json:"connectedAt,omitempty"`
	LastEventAt     string `json:"lastEventAt,omitempty"`
	LastKeepaliveAt string `json:"lastKeepaliveAt,omitempty"`
	LastDataGapAt   string `json:"lastDataGapAt,omitempty"`

	ReconnectCount            int `json:"reconnectCount"`
	ActiveSubscriptionCount   int `json:"activeSubscriptionCount"`
	ExpectedSubscriptionCount int `json:"expectedSubscriptionCount"`

	LastError string `json:"lastError,omitempty"`
}

func formatOptionalTime(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func toConnectorResponse(s twitchengagement.Snapshot) connectorResponse {
	return connectorResponse{
		AccountID: s.AccountID, Enabled: s.Enabled, State: string(s.State),
		BlockerCodes: s.BlockerCodes, MissingScopes: s.MissingScopes,
		ConnectedAt: formatOptionalTime(s.ConnectedAt), LastEventAt: formatOptionalTime(s.LastEventAt),
		LastKeepaliveAt: formatOptionalTime(s.LastKeepaliveAt), LastDataGapAt: formatOptionalTime(s.LastDataGapAt),
		ReconnectCount: s.ReconnectCount, ActiveSubscriptionCount: s.ActiveSubscriptionCount,
		ExpectedSubscriptionCount: s.ExpectedSubscriptionCount, LastError: s.LastError,
	}
}

type accountEngagementResponse struct {
	connectorResponse
	RequiredScopes            []string `json:"requiredScopes"`
	GrantedScopes             []string `json:"grantedScopes"`
	PermissionUpgradeRequired bool     `json:"permissionUpgradeRequired"`
}

func toAccountEngagementResponse(s twitchengagement.Snapshot, assessment twitch.CapabilityAssessment) accountEngagementResponse {
	return accountEngagementResponse{
		connectorResponse: toConnectorResponse(s),
		RequiredScopes:    assessment.Required, GrantedScopes: assessment.Granted,
		PermissionUpgradeRequired: assessment.PermissionUpgradeRequired,
	}
}

type engagementStatusResponse struct {
	SchemaVersion     int                 `json:"schemaVersion"`
	BufferCapacity    int                 `json:"bufferCapacity"`
	RetainedCount     int                 `json:"retainedCount"`
	OldestSequence    uint64              `json:"oldestSequence"`
	NewestSequence    uint64              `json:"newestSequence"`
	ActiveSubscribers int                 `json:"activeSubscribers"`
	Connectors        []connectorResponse `json:"connectors"`
}

type eventBadgeResponse struct {
	SetID string `json:"setId"`
	ID    string `json:"id"`
	Info  string `json:"info,omitempty"`
}

type eventUserResponse struct {
	ProviderUserID string               `json:"providerUserId,omitempty"`
	Login          string               `json:"login,omitempty"`
	DisplayName    string               `json:"displayName,omitempty"`
	AvatarURL      string               `json:"avatarUrl,omitempty"`
	Color          string               `json:"color,omitempty"`
	Badges         []eventBadgeResponse `json:"badges,omitempty"`
	Roles          []string             `json:"roles,omitempty"`
	Anonymous      bool                 `json:"anonymous"`
}

type eventFragmentResponse struct {
	Type               string `json:"type"`
	Text               string `json:"text"`
	EmoteID            string `json:"emoteId,omitempty"`
	CheermotePrefix    string `json:"cheermotePrefix,omitempty"`
	CheermoteBits      int    `json:"cheermoteBits,omitempty"`
	MentionUserID      string `json:"mentionUserId,omitempty"`
	MentionLogin       string `json:"mentionLogin,omitempty"`
	MentionDisplayName string `json:"mentionDisplayName,omitempty"`
}

type eventMessageResponse struct {
	Text      string                  `json:"text"`
	Fragments []eventFragmentResponse `json:"fragments"`
}

// eventResponse is the public, versioned shape of one normalized engagement
// event - identical whether reached via the bounded snapshot endpoint or
// the SSE stream. Never carries a raw provider payload, a token, or a
// WebSocket/session identifier - see engagement.Event's own doc comment,
// which this mirrors field-for-field.
type eventResponse struct {
	SchemaVersion int `json:"schemaVersion"`

	Sequence           uint64 `json:"sequence"`
	ID                 string `json:"id"`
	ProviderEventID    string `json:"providerEventId,omitempty"`
	ProviderID         string `json:"providerId"`
	ConnectedAccountID string `json:"connectedAccountId"`
	DestinationID      string `json:"destinationId,omitempty"`

	Type              string `json:"type"`
	ProviderEventType string `json:"providerEventType"`
	PlatformTimestamp string `json:"platformTimestamp"`
	ReceivedAt        string `json:"receivedAt"`
	Synthetic         bool   `json:"synthetic"`

	User    *eventUserResponse    `json:"user,omitempty"`
	Message *eventMessageResponse `json:"message,omitempty"`

	AmountMicros  *int64 `json:"amountMicros,omitempty"`
	Currency      string `json:"currency,omitempty"`
	DisplayAmount string `json:"displayAmount,omitempty"`
	Quantity      *int64 `json:"quantity,omitempty"`

	ModerationRef    string `json:"moderationRef,omitempty"`
	ModerationAction string `json:"moderationAction,omitempty"`

	ProviderExtra map[string]string `json:"providerExtra,omitempty"`
}

func toEventResponse(e engagement.Event) eventResponse {
	resp := eventResponse{
		SchemaVersion: int(e.SchemaVersion), Sequence: e.Sequence, ID: e.ID,
		ProviderEventID: e.ProviderEventID, ProviderID: string(e.ProviderID),
		ConnectedAccountID: e.ConnectedAccountID, DestinationID: e.DestinationID,
		Type: string(e.Type), ProviderEventType: e.ProviderEventType,
		PlatformTimestamp: e.PlatformTimestamp.UTC().Format(time.RFC3339Nano),
		ReceivedAt:        e.ReceivedAt.UTC().Format(time.RFC3339Nano),
		Synthetic:         e.Synthetic,
		Quantity:          e.Quantity,
		ModerationRef:     e.ModerationRef, ModerationAction: e.ModerationAction,
		ProviderExtra: e.ProviderExtra,
	}
	if e.Money != nil {
		amount := e.Money.AmountMicros
		resp.AmountMicros = &amount
		resp.Currency = e.Money.Currency
		resp.DisplayAmount = e.Money.DisplayAmount
	}
	if e.User != nil {
		badges := make([]eventBadgeResponse, 0, len(e.User.Badges))
		for _, b := range e.User.Badges {
			badges = append(badges, eventBadgeResponse{SetID: b.SetID, ID: b.ID, Info: b.Info})
		}
		roles := make([]string, 0, len(e.User.Roles))
		for _, r := range e.User.Roles {
			roles = append(roles, string(r))
		}
		resp.User = &eventUserResponse{
			ProviderUserID: e.User.ProviderUserID, Login: e.User.Login, DisplayName: e.User.DisplayName,
			AvatarURL: e.User.AvatarURL, Color: e.User.Color, Badges: badges, Roles: roles, Anonymous: e.User.Anonymous,
		}
	}
	if e.Message != nil {
		fragments := make([]eventFragmentResponse, 0, len(e.Message.Fragments))
		for _, f := range e.Message.Fragments {
			fragments = append(fragments, eventFragmentResponse{
				Type: string(f.Type), Text: f.Text, EmoteID: f.EmoteID,
				CheermotePrefix: f.CheermotePrefix, CheermoteBits: f.CheermoteBits,
				MentionUserID: f.MentionUserID, MentionLogin: f.MentionLogin, MentionDisplayName: f.MentionDisplayName,
			})
		}
		resp.Message = &eventMessageResponse{Text: e.Message.Text, Fragments: fragments}
	}
	return resp
}

// --- status and snapshot handlers ----------------------------------------

func handleGetEngagementStatus(logger *slog.Logger, evBus EngagementBusService, connectors EngagementConnectorService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		snap := evBus.Snapshot()
		resp := engagementStatusResponse{
			SchemaVersion: snap.SchemaVersion, BufferCapacity: snap.Capacity, RetainedCount: snap.RetainedCount,
			OldestSequence: snap.OldestSequence, NewestSequence: snap.NewestSequence, ActiveSubscribers: snap.ActiveSubscribers,
			Connectors: []connectorResponse{},
		}
		if connectors != nil {
			for _, c := range connectors.Snapshots() {
				resp.Connectors = append(resp.Connectors, toConnectorResponse(c))
			}
		}
		writeJSON(w, logger, http.StatusOK, resp)
	}
}

func parseUintQuery(r *http.Request, key string, fallback uint64) uint64 {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func handleGetEngagementEvents(logger *slog.Logger, evBus EngagementBusService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		after := parseUintQuery(r, "after", 0)
		limit := parseUintQuery(r, "limit", defaultEngagementEventsLimit)
		if limit > maxEngagementEventsLimit {
			limit = maxEngagementEventsLimit
		}
		events, gap := evBus.EventsAfter(after, int(limit))
		items := make([]eventResponse, 0, len(events))
		for _, e := range events {
			items = append(items, toEventResponse(e))
		}
		writeJSON(w, logger, http.StatusOK, map[string]any{"items": items, "gap": gap})
	}
}

// --- SSE stream ------------------------------------------------------------

func writeSSEComment(w http.ResponseWriter, text string) {
	fmt.Fprintf(w, ": %s\n\n", text)
}

func writeSSEEvent(w http.ResponseWriter, eventName string, id uint64, data any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if id > 0 {
		fmt.Fprintf(w, "id: %d\n", id)
	}
	fmt.Fprintf(w, "event: %s\n", eventName)
	fmt.Fprintf(w, "data: %s\n\n", payload)
	return nil
}

// handleEngagementStream serves GET /api/engagement/stream over
// Server-Sent Events - see the stage task's Part 7 for the full contract:
// replay via Last-Event-ID, an explicit engagement.gap/engagement.reset
// event when the requested sequence has already been evicted, periodic
// keepalive comments, and a bounded number of concurrent clients so one
// slow browser tab can never exhaust server resources.
func handleEngagementStream(logger *slog.Logger, evBus EngagementBusService, activeClients *atomic.Int32) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeError(w, logger, http.StatusInternalServerError, "internal_error", "Streaming is not supported by this response writer.")
			return
		}

		if activeClients.Add(1) > maxSSEClients {
			activeClients.Add(-1)
			writeError(w, logger, http.StatusServiceUnavailable, "engagement_stream_limit_reached", "Too many active event streams; try again shortly.")
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

		sub, gap, err := evBus.Subscribe(after)
		if err != nil {
			writeError(w, logger, http.StatusServiceUnavailable, "engagement_disabled", "The Event Bus is unavailable.")
			return
		}
		defer sub.Cancel()

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		// Disables response buffering on common reverse proxies (nginx); this
		// application is loopback-only today, but the header is harmless and
		// future-proofs a remote-server mode.
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		if gap {
			_ = writeSSEEvent(w, "engagement.gap", 0, map[string]string{"reason": "sequence_evicted"})
			flusher.Flush()
		}

		keepalive := time.NewTicker(sseKeepaliveInterval)
		defer keepalive.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case evt, open := <-sub.Events():
				if !open {
					select {
					case reason := <-sub.Closed():
						if reason == bus.ReasonSlowConsumer {
							_ = writeSSEEvent(w, "engagement.gap", 0, map[string]string{"reason": "slow_consumer"})
							flusher.Flush()
						}
					default:
					}
					return
				}
				if err := writeSSEEvent(w, "engagement.event", evt.Sequence, toEventResponse(evt)); err != nil {
					logger.Warn("failed to write SSE event", "error", err)
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

// --- per-account connector management ------------------------------------

func handleGetAccountEngagement(
	logger *slog.Logger, accounts AccountService, settings EngagementSettingsService, connectors EngagementConnectorService,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID := r.PathValue("id")
		acc, err := accounts.GetAccount(r.Context(), accountID)
		if err != nil {
			writeAccountError(w, logger, r, err)
			return
		}
		if acc.ProviderID != account.ProviderTwitch {
			writeError(w, logger, http.StatusUnprocessableEntity, "engagement_not_supported", "Only Twitch accounts support engagement in this stage.")
			return
		}

		snap, ok := connectors.Snapshot(accountID)
		if !ok {
			persisted, _, err := settings.Get(r.Context(), accountID)
			if err != nil {
				writeDomainError(w, logger, r, err)
				return
			}
			snap = twitchengagement.Snapshot{AccountID: accountID, Enabled: persisted.Enabled, State: twitchengagement.StateDisabled}
		}

		assessment := twitch.AssessEngagementCapability(acc.Scopes)
		writeJSON(w, logger, http.StatusOK, toAccountEngagementResponse(snap, assessment))
	}
}

type setEngagementRequest struct {
	Enabled bool `json:"enabled"`
}

func handlePutAccountEngagement(logger *slog.Logger, accounts AccountService, connectors EngagementConnectorService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID := r.PathValue("id")
		acc, err := accounts.GetAccount(r.Context(), accountID)
		if err != nil {
			writeAccountError(w, logger, r, err)
			return
		}
		if acc.ProviderID != account.ProviderTwitch {
			writeError(w, logger, http.StatusUnprocessableEntity, "engagement_not_supported", "Only Twitch accounts support engagement in this stage.")
			return
		}

		var body setEngagementRequest
		if err := decodeJSON(w, r, &body); err != nil {
			writeDecodeError(w, logger, err)
			return
		}

		var snap twitchengagement.Snapshot
		if body.Enabled {
			snap, err = connectors.Enable(r.Context(), accountID)
		} else {
			snap, err = connectors.Disable(r.Context(), accountID)
		}
		if err != nil {
			writeEngagementError(w, logger, r, err)
			return
		}

		assessment := twitch.AssessEngagementCapability(acc.Scopes)
		writeJSON(w, logger, http.StatusOK, toAccountEngagementResponse(snap, assessment))
	}
}

// handleAuthorizeEngagement starts an identity-bound Twitch Device Code Flow
// attempt requesting the union of the account's existing scopes and the
// Stage 8 inbound-engagement profile - see
// docs/provider-integrations/twitch-engagement.md's scope-profile design
// decision. Never removes a previously granted scope, never creates a
// second connected-account row.
func handleAuthorizeEngagement(logger *slog.Logger, accounts AccountService, deviceFlow DeviceFlowService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}
		accountID := r.PathValue("id")
		acc, err := accounts.GetAccount(r.Context(), accountID)
		if err != nil {
			writeAccountError(w, logger, r, err)
			return
		}
		if acc.ProviderID != account.ProviderTwitch {
			writeError(w, logger, http.StatusUnprocessableEntity, "engagement_not_supported", "Only Twitch accounts support engagement in this stage.")
			return
		}

		scopes := twitch.UnionScopes(acc.Scopes)
		snapshot, err := deviceFlow.StartAttemptWithScopes(r.Context(), account.ProviderTwitch, accountID, scopes)
		if err != nil {
			writeDeviceFlowError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusAccepted, toDeviceFlowResponse(snapshot))
	}
}

// handleRestartEngagement provides operational recovery: cancel and restart
// a connector without changing its persisted enabled setting or creating a
// duplicate session.
func handleRestartEngagement(logger *slog.Logger, connectors EngagementConnectorService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}
		accountID := r.PathValue("id")
		snap, err := connectors.Restart(r.Context(), accountID)
		if err != nil {
			writeEngagementError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toConnectorResponse(snap))
	}
}

// --- error mapping ---------------------------------------------------------

func writeEngagementError(w http.ResponseWriter, logger *slog.Logger, r *http.Request, err error) {
	switch {
	case errors.Is(err, twitchengagement.ErrUnsupportedProvider):
		writeError(w, logger, http.StatusUnprocessableEntity, "engagement_not_supported", "Only Twitch accounts support engagement in this stage.")
	case errors.Is(err, twitchengagement.ErrNotFound):
		writeError(w, logger, http.StatusNotFound, "engagement_connector_not_found", "No engagement connector is configured for this account.")
	case errors.Is(err, twitchengagement.ErrConflict):
		writeError(w, logger, http.StatusConflict, "engagement_connector_conflict", "The connector is busy; try again shortly.")
	default:
		writeAccountError(w, logger, r, err)
	}
}

// writeDeviceFlowError is defined in accounts.go; StartAttemptWithScopes
// reuses the exact same error surface as StartAttempt (ErrConflict,
// ErrIntegrationNotConfigured, ...), so no separate mapping is needed here.
var _ = deviceflow.ErrConflict // referenced only to document the above
