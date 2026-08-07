package twitch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// chatBackendRateLimitedStatus is Twitch's own chat-backend "Enhance Your
// Calm" response for sending too quickly - distinct from the standard Helix
// 429, and not a constant net/http defines. See
// docs/provider-integrations/twitch-outbound-chat.md's "Twitch chat-backend
// rate limits" section.
const chatBackendRateLimitedStatus = 420

// sendChatMessageRequest is the wire body for POST /helix/chat/messages -
// see docs/provider-integrations/twitch-outbound-chat.md. Deliberately
// never carries for_source_only, pin, or any field this application does
// not send.
type sendChatMessageRequest struct {
	BroadcasterID        string `json:"broadcaster_id"`
	SenderID             string `json:"sender_id"`
	Message              string `json:"message"`
	ReplyParentMessageID string `json:"reply_parent_message_id,omitempty"`
}

type sendChatMessageDropReason struct {
	Code string `json:"code"`
	// Message is Twitch's own free-text drop explanation - decoded only so
	// this struct matches the real response shape; never read by any
	// caller in this package. See SendChatMessageResult's own doc comment.
	Message string `json:"message"`
}

type sendChatMessageDataItem struct {
	MessageID  string                     `json:"message_id"`
	IsSent     bool                       `json:"is_sent"`
	DropReason *sendChatMessageDropReason `json:"drop_reason"`
}

type sendChatMessageResponse struct {
	Data []sendChatMessageDataItem `json:"data"`
}

// SendChatMessageResult is this client's own return shape for one Send Chat
// Message attempt - never leaves this package unconverted; see
// Adapter.SendChatMessage for the outboundchat.SendMessageResult mapping.
type SendChatMessageResult struct {
	MessageID string
	IsSent    bool
	// DropReasonCode is Twitch's own stable drop_reason.code (for example
	// "automod_held"), present only when IsSent is false. The free-text
	// drop_reason.message is deliberately never exposed here - see
	// sendChatMessageDropReason's own doc comment.
	DropReasonCode string
}

// SendChatMessage sends one chat message: POST /helix/chat/messages.
// Requires a user access token with user:write:chat. broadcasterID and
// senderID are always the connected account's own provider user ID in this
// application - the caller never accepts either from the browser (see
// internal/httpapi's outbound-chat handlers).
//
// Error mapping:
//   - 401 -> ErrUnauthorized. The caller (via account.Service.WithFreshToken)
//     is expected to refresh and retry exactly once.
//   - 403 -> ErrForbidden.
//   - 429 or 420 (Twitch's own chat-backend "too fast" response) ->
//     ErrRateLimited; the parsed rate-limit hint is returned alongside so
//     the caller can surface a retry-at time.
//   - 5xx, or any other unexpected status -> ErrUnavailable: a definite
//     response was received, but it was not success.
//   - A transport-level failure (the request may never have reached
//     Twitch, or a response may have been lost mid-flight) -> transport
//     uncertain. Deliberately distinct from ErrUnavailable - see
//     ErrTransportUncertain's own doc comment.
//   - A 200 response whose body cannot be parsed into exactly one usable
//     data item -> ErrInvalidResponse (also never automatically retried by
//     any caller in this application - see the outbound-chat contract's
//     retry policy).
func (c *Client) SendChatMessage(ctx context.Context, broadcasterID, senderID, message, replyParentMessageID, accessToken, clientID string) (SendChatMessageResult, rateLimit, error) {
	body := sendChatMessageRequest{
		BroadcasterID: broadcasterID, SenderID: senderID, Message: message,
		ReplyParentMessageID: replyParentMessageID,
	}
	status, respBody, limit, err := c.doHelix(ctx, http.MethodPost, "/chat/messages", nil, body, accessToken, clientID)
	if err != nil {
		// doHelix's only error path is a transport-level failure (the
		// request never got a status code at all) - never a non-2xx HTTP
		// response, which comes back as (status, body, limit, nil) below.
		return SendChatMessageResult{}, rateLimit{}, fmt.Errorf("%w: %s", ErrTransportUncertain, err)
	}

	switch {
	case status == http.StatusOK:
		result, parseErr := parseSendChatMessageResponse(respBody)
		return result, limit, parseErr
	case status == http.StatusUnauthorized:
		return SendChatMessageResult{}, limit, wireErr(ErrUnauthorized, status, "/helix/chat/messages")
	case status == http.StatusForbidden:
		return SendChatMessageResult{}, limit, wireErr(ErrForbidden, status, "/helix/chat/messages")
	case status == http.StatusTooManyRequests || status == chatBackendRateLimitedStatus:
		return SendChatMessageResult{}, limit, wireErr(ErrRateLimited, status, "/helix/chat/messages")
	case status >= 500:
		return SendChatMessageResult{}, limit, wireErr(ErrUnavailable, status, "/helix/chat/messages")
	default:
		// 400/404 and anything else undocumented: this application always
		// validates the message length and always sends the connected
		// account's own broadcaster/sender ID, so a well-formed request
		// reaching this branch is unexpected, not a normal outcome this
		// application's own input caused - treated the same as an
		// unavailable provider rather than invented a bespoke path for a
		// case that should not occur in practice.
		return SendChatMessageResult{}, limit, wireErr(ErrUnavailable, status, "/helix/chat/messages")
	}
}

func parseSendChatMessageResponse(body []byte) (SendChatMessageResult, error) {
	var parsed sendChatMessageResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return SendChatMessageResult{}, fmt.Errorf("%w: /helix/chat/messages: %s", ErrInvalidResponse, err)
	}
	if len(parsed.Data) != 1 {
		return SendChatMessageResult{}, fmt.Errorf("%w: /helix/chat/messages: expected exactly one data item, got %d", ErrInvalidResponse, len(parsed.Data))
	}
	item := parsed.Data[0]
	if item.IsSent && item.MessageID == "" {
		return SendChatMessageResult{}, fmt.Errorf("%w: /helix/chat/messages: is_sent true with no message_id", ErrInvalidResponse)
	}

	result := SendChatMessageResult{MessageID: item.MessageID, IsSent: item.IsSent}
	if !item.IsSent && item.DropReason != nil {
		result.DropReasonCode = item.DropReason.Code
	}
	return result, nil
}
