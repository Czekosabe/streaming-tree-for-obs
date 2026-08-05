package twitch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// EventSubSubscriptionDef describes one selected EventSub subscription type
// - see docs/provider-integrations/twitch-engagement.md for the researched
// type/version/condition/scope table this list implements exactly.
type EventSubSubscriptionDef struct {
	Type          string
	Version       string
	RequiredScope string // empty means no scope is required (channel.raid, stream.online/offline)
	Condition     func(accountUserID string) map[string]string
}

func broadcasterCondition(userID string) map[string]string {
	return map[string]string{"broadcaster_user_id": userID}
}

func chatCondition(userID string) map[string]string {
	return map[string]string{"broadcaster_user_id": userID, "user_id": userID}
}

func followCondition(userID string) map[string]string {
	return map[string]string{"broadcaster_user_id": userID, "moderator_user_id": userID}
}

func incomingRaidCondition(userID string) map[string]string {
	// to_broadcaster_user_id only - see docs/provider-integrations/
	// twitch-engagement.md: outgoing raids (from_broadcaster_user_id) are
	// deliberately not subscribed to in Stage 8A.
	return map[string]string{"to_broadcaster_user_id": userID}
}

// EventSubSubscriptionDefs is the complete Stage 8A selected subscription
// set, in the fixed order subscriptions are created.
var EventSubSubscriptionDefs = []EventSubSubscriptionDef{
	{Type: "channel.chat.message", Version: "1", RequiredScope: "user:read:chat", Condition: chatCondition},
	{Type: "channel.chat.message_delete", Version: "1", RequiredScope: "user:read:chat", Condition: chatCondition},
	{Type: "channel.chat.clear", Version: "1", RequiredScope: "user:read:chat", Condition: chatCondition},
	{Type: "channel.chat.clear_user_messages", Version: "1", RequiredScope: "user:read:chat", Condition: chatCondition},
	{Type: "channel.follow", Version: "2", RequiredScope: "moderator:read:followers", Condition: followCondition},
	{Type: "channel.subscribe", Version: "1", RequiredScope: "channel:read:subscriptions", Condition: broadcasterCondition},
	{Type: "channel.subscription.gift", Version: "1", RequiredScope: "channel:read:subscriptions", Condition: broadcasterCondition},
	{Type: "channel.subscription.message", Version: "1", RequiredScope: "channel:read:subscriptions", Condition: broadcasterCondition},
	{Type: "channel.cheer", Version: "1", RequiredScope: "bits:read", Condition: broadcasterCondition},
	{Type: "channel.raid", Version: "1", RequiredScope: "", Condition: incomingRaidCondition},
	{Type: "channel.channel_points_custom_reward_redemption.add", Version: "1", RequiredScope: "channel:read:redemptions", Condition: broadcasterCondition},
	{Type: "stream.online", Version: "1", RequiredScope: "", Condition: broadcasterCondition},
	{Type: "stream.offline", Version: "1", RequiredScope: "", Condition: broadcasterCondition},
}

type eventSubTransport struct {
	Method    string `json:"method"`
	SessionID string `json:"session_id"`
}

type createEventSubSubscriptionRequest struct {
	Type      string            `json:"type"`
	Version   string            `json:"version"`
	Condition map[string]string `json:"condition"`
	Transport eventSubTransport `json:"transport"`
}

type createEventSubSubscriptionResponse struct {
	Data []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	} `json:"data"`
}

// CreateEventSubSubscription creates one WebSocket-transport EventSub
// subscription and returns Twitch's own subscription id.
//
// A user access token is required for WebSocket transport - see
// docs/provider-integrations/twitch-engagement.md; an app access token is
// rejected by Twitch itself, not specially handled here.
func (c *Client) CreateEventSubSubscription(
	ctx context.Context, accessToken, clientID string, def EventSubSubscriptionDef, accountUserID, sessionID string,
) (string, error) {
	body := createEventSubSubscriptionRequest{
		Type: def.Type, Version: def.Version, Condition: def.Condition(accountUserID),
		Transport: eventSubTransport{Method: "websocket", SessionID: sessionID},
	}

	status, respBody, _, err := c.doHelix(ctx, http.MethodPost, "/eventsub/subscriptions", nil, body, accessToken, clientID)
	if err != nil {
		return "", err
	}

	switch status {
	case http.StatusAccepted, http.StatusOK, http.StatusCreated:
		var parsed createEventSubSubscriptionResponse
		if err := json.Unmarshal(respBody, &parsed); err != nil || len(parsed.Data) == 0 {
			return "", fmt.Errorf("%w: create eventsub subscription %s: malformed success response", ErrInvalidResponse, def.Type)
		}
		return parsed.Data[0].ID, nil
	case http.StatusUnauthorized:
		return "", ErrUnauthorized
	case http.StatusForbidden:
		return "", fmt.Errorf("%w: missing scope for %s", ErrForbidden, def.Type)
	case http.StatusTooManyRequests:
		return "", ErrRateLimited
	default:
		if status >= 500 {
			return "", wireErr(ErrUnavailable, status, "/eventsub/subscriptions")
		}
		return "", wireErr(ErrInvalidResponse, status, "/eventsub/subscriptions")
	}
}
