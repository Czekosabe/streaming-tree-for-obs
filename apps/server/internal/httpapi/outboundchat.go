package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/streaming-tree/server/internal/domain/account"
	"github.com/streaming-tree/server/internal/outboundchat"
	"github.com/streaming-tree/server/internal/provider/twitch"
)

// sharedChatWarningID is a stable, localized-on-the-frontend identifier
// disclosing that a User Access Token send may be distributed across a
// Twitch Shared Chat session - always present, never a claim that Shared
// Chat is currently active (this stage has no reliable way to detect
// that). See docs/provider-integrations/twitch-outbound-chat.md.
const sharedChatWarningID = "twitch_shared_chat_distribution_possible"

// maxOutboundChatMessageBodyBytes caps the send-message request body well
// below the general maxRequestBodyBytes ceiling: a real body never needs
// more than a few hundred bytes for a 500-code-point message plus a
// reply-parent id, so 8 KiB is already generous, not a tight fit.
const maxOutboundChatMessageBodyBytes = 8 * 1024

// OutboundChatService is the subset of outboundchat.Manager the HTTP layer
// needs.
type OutboundChatService interface {
	Send(ctx context.Context, req outboundchat.SendMessageRequest) (outboundchat.SendMessageResult, error)
	Status(ctx context.Context, accountID string) (outboundchat.Snapshot, error)
}

// registerOutboundChatRoutes wires the Stage 11A manual outbound-chat API:
// per-account status, permission-upgrade authorization (reusing the
// existing identity-bound Device Code Flow), and sending one message.
func registerOutboundChatRoutes(
	mux *http.ServeMux, logger *slog.Logger,
	accounts AccountService, deviceFlow DeviceFlowService, outboundChat OutboundChatService,
) {
	mux.HandleFunc("GET /api/connected-accounts/{id}/outbound-chat", handleGetOutboundChatStatus(logger, accounts, outboundChat))
	mux.HandleFunc("/api/connected-accounts/{id}/outbound-chat", methodNotAllowed(logger, http.MethodGet))

	mux.HandleFunc("POST /api/connected-accounts/{id}/outbound-chat/authorize", handleAuthorizeOutboundChat(logger, accounts, deviceFlow))
	mux.HandleFunc("/api/connected-accounts/{id}/outbound-chat/authorize", methodNotAllowed(logger, http.MethodPost))

	mux.HandleFunc("POST /api/connected-accounts/{id}/outbound-chat/messages", handleSendOutboundChatMessage(logger, accounts, outboundChat))
	mux.HandleFunc("/api/connected-accounts/{id}/outbound-chat/messages", methodNotAllowed(logger, http.MethodPost))
}

// --- response DTOs -------------------------------------------------------

// outboundChatStatusResponse never carries a credential or message content
// - see outboundchat.Snapshot's own doc comment, which this is built from
// exclusively.
type outboundChatStatusResponse struct {
	ProviderID string `json:"providerId"`
	// Capability is one of "unsupported" / "permission_required" / "ready"
	// / "error" - see toOutboundChatCapabilityLabel.
	Capability     string   `json:"capability"`
	RequiredScopes []string `json:"requiredScopes,omitempty"`
	GrantedScopes  []string `json:"grantedScopes,omitempty"`
	MissingScopes  []string `json:"missingScopes,omitempty"`

	DispatcherState string `json:"dispatcherState"`
	QueueDepth      int    `json:"queueDepth"`
	QueueCapacity   int    `json:"queueCapacity"`
	LastAttemptAt   string `json:"lastAttemptAt,omitempty"`
	LastSuccessAt   string `json:"lastSuccessAt,omitempty"`
	LastErrorCode   string `json:"lastErrorCode,omitempty"`
	RetryAt         string `json:"retryAt,omitempty"`

	CanSendNow        bool   `json:"canSendNow"`
	SharedChatWarning string `json:"sharedChatWarning"`
}

func toOutboundChatCapabilityLabel(acc account.Account, snap outboundchat.Snapshot) string {
	if !snap.ProviderSupported {
		return "unsupported"
	}
	if acc.Status == account.StatusReconnectRequired {
		return "error"
	}
	if snap.Capability.PermissionUpgradeRequired {
		return "permission_required"
	}
	return "ready"
}

func toOutboundChatStatusResponse(acc account.Account, snap outboundchat.Snapshot) outboundChatStatusResponse {
	resp := outboundChatStatusResponse{
		ProviderID: string(acc.ProviderID), Capability: toOutboundChatCapabilityLabel(acc, snap),
		RequiredScopes: snap.Capability.Required, GrantedScopes: snap.Capability.Granted, MissingScopes: snap.Capability.Missing,
		DispatcherState: string(snap.State), QueueDepth: snap.QueueDepth, QueueCapacity: snap.QueueCapacity,
		LastErrorCode: snap.LastErrorCode, SharedChatWarning: sharedChatWarningID,
	}
	resp.CanSendNow = resp.Capability == "ready"
	if snap.LastAttemptAt != nil {
		resp.LastAttemptAt = snap.LastAttemptAt.UTC().Format(time.RFC3339Nano)
	}
	if snap.LastSuccessAt != nil {
		resp.LastSuccessAt = snap.LastSuccessAt.UTC().Format(time.RFC3339Nano)
	}
	if snap.RetryAt != nil {
		resp.RetryAt = snap.RetryAt.UTC().Format(time.RFC3339Nano)
	}
	return resp
}

type sendOutboundChatMessageRequest struct {
	Message              string `json:"message"`
	ReplyParentMessageID string `json:"replyParentMessageId,omitempty"`
}

// sendOutboundChatMessageResponse never echoes the sent text - see the
// stage task's own "a successful response should not echo the sent text"
// requirement.
type sendOutboundChatMessageResponse struct {
	Sent              bool   `json:"sent"`
	ProviderMessageID string `json:"providerMessageId,omitempty"`
	SentAt            string `json:"sentAt,omitempty"`
}

// --- handlers --------------------------------------------------------------

func handleGetOutboundChatStatus(logger *slog.Logger, accounts AccountService, outboundChat OutboundChatService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID := r.PathValue("id")
		acc, err := accounts.GetAccount(r.Context(), accountID)
		if err != nil {
			writeAccountError(w, logger, r, err)
			return
		}
		snap, err := outboundChat.Status(r.Context(), accountID)
		if err != nil {
			writeOutboundChatError(w, logger, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toOutboundChatStatusResponse(acc, snap))
	}
}

// handleAuthorizeOutboundChat starts an identity-bound Twitch Device Code
// Flow attempt requesting the union of the account's existing scopes and
// the Stage 11A outbound-chat profile - see
// docs/provider-integrations/twitch-outbound-chat.md. Never removes a
// previously granted scope, never creates a second connected-account row -
// mirrors handleAuthorizeEngagement exactly, requesting a different scope
// profile.
func handleAuthorizeOutboundChat(logger *slog.Logger, accounts AccountService, deviceFlow DeviceFlowService) http.HandlerFunc {
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
			writeError(w, logger, http.StatusServiceUnavailable, "outbound_chat_unsupported", "Only Twitch accounts support outbound chat in this stage.")
			return
		}

		scopes := twitch.UnionScopesWithOutboundChat(acc.Scopes)
		snapshot, err := deviceFlow.StartAttemptWithScopes(r.Context(), account.ProviderTwitch, accountID, scopes)
		if err != nil {
			writeDeviceFlowError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusAccepted, toDeviceFlowResponse(snapshot))
	}
}

func handleSendOutboundChatMessage(logger *slog.Logger, accounts AccountService, outboundChat OutboundChatService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID := r.PathValue("id")
		if _, err := accounts.GetAccount(r.Context(), accountID); err != nil {
			writeAccountError(w, logger, r, err)
			return
		}

		var body sendOutboundChatMessageRequest
		if err := decodeJSONWithLimit(w, r, &body, maxOutboundChatMessageBodyBytes); err != nil {
			writeDecodeError(w, logger, err)
			return
		}

		result, err := outboundChat.Send(r.Context(), outboundchat.SendMessageRequest{
			AccountID: accountID, Message: body.Message, ReplyParentMessageID: body.ReplyParentMessageID,
			Source: outboundchat.SourceManual,
		})
		if err != nil {
			writeOutboundChatError(w, logger, err)
			return
		}
		if !result.Sent {
			writeError(w, logger, http.StatusUnprocessableEntity, "outbound_chat_message_dropped",
				"Twitch did not deliver this message.")
			return
		}
		writeJSON(w, logger, http.StatusOK, sendOutboundChatMessageResponse{
			Sent: true, ProviderMessageID: result.ProviderMessageID, SentAt: result.CompletedAt.UTC().Format(time.RFC3339Nano),
		})
	}
}

// writeOutboundChatError maps an outboundchat/account error to one of the
// stable outbound_chat_* codes and a status per the stage task's own
// mapping table. Never forwards a provider's raw message/prose.
func writeOutboundChatError(w http.ResponseWriter, logger *slog.Logger, err error) {
	var rateLimitErr *outboundchat.RateLimitedError
	switch {
	case errors.Is(err, account.ErrNotFound):
		// Reuses the same stable code every other per-account endpoint in
		// this application already returns via writeAccountError, rather
		// than a second synonym - see writeAccountError's own case. In
		// practice every handler above already checks account existence
		// before reaching outboundChat.Send/Status, so this branch is
		// defensive coverage for any future caller that does not.
		writeError(w, logger, http.StatusNotFound, "account_not_found", "The requested connected account does not exist.")
	case errors.Is(err, outboundchat.ErrUnsupportedProvider):
		writeError(w, logger, http.StatusServiceUnavailable, "outbound_chat_unsupported", "This provider does not support outbound chat.")
	case errors.Is(err, outboundchat.ErrPermissionRequired), errors.Is(err, account.ErrMissingScope):
		writeError(w, logger, http.StatusUnprocessableEntity, "outbound_chat_permission_required", "Outbound chat permission has not been granted for this account.")
	case errors.Is(err, account.ErrReconnectRequired):
		writeError(w, logger, http.StatusConflict, "account_reconnect_required", "This account must be reconnected before it can send chat messages.")
	case errors.Is(err, outboundchat.ErrQueueFull):
		writeError(w, logger, http.StatusTooManyRequests, "outbound_chat_queue_full", "Too many messages are already queued for this account.")
	case errors.As(err, &rateLimitErr):
		writeError(w, logger, http.StatusTooManyRequests, "outbound_chat_rate_limited", "Sending is temporarily rate limited.")
	case errors.Is(err, outboundchat.ErrForbidden):
		writeError(w, logger, http.StatusForbidden, "outbound_chat_forbidden", "Twitch rejected this send - the account may be banned or timed out in this chat room.")
	case errors.Is(err, outboundchat.ErrDeliveryUnknown):
		writeError(w, logger, http.StatusBadGateway, "outbound_chat_delivery_unknown", "The message may or may not have been delivered - no trustworthy result was received.")
	case errors.Is(err, outboundchat.ErrProviderFailure):
		writeError(w, logger, http.StatusBadGateway, "outbound_chat_provider_failure", "The chat provider returned an error.")
	case errors.Is(err, outboundchat.ErrCancelled):
		writeError(w, logger, http.StatusServiceUnavailable, "outbound_chat_cancelled", "The send was cancelled.")
	case errors.Is(err, outboundchat.ErrMessageEmpty), errors.Is(err, outboundchat.ErrMessageTooLong),
		errors.Is(err, outboundchat.ErrMessageInvalidUTF8), errors.Is(err, outboundchat.ErrMessageControlCharacter):
		writeError(w, logger, http.StatusUnprocessableEntity, "validation_failed", "The message is invalid.")
	case errors.Is(err, outboundchat.ErrReplyParentMessageIDInvalid):
		writeError(w, logger, http.StatusUnprocessableEntity, "validation_failed", "The reply reference is invalid.")
	default:
		writeError(w, logger, http.StatusInternalServerError, "internal_error", "An unexpected error occurred.")
	}
}
