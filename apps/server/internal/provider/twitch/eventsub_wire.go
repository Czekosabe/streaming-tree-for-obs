package twitch

import "encoding/json"

// EventSub WebSocket message types this connector understands. See
// docs/provider-integrations/twitch-engagement.md's "WebSocket lifecycle"
// section - these wire values come directly from Twitch's own
// metadata.message_type field, never guessed.
const (
	MessageTypeWelcome      = "session_welcome"
	MessageTypeKeepalive    = "session_keepalive"
	MessageTypeNotification = "notification"
	MessageTypeReconnect    = "session_reconnect"
	MessageTypeRevocation   = "revocation"
)

// EventSubEnvelope is every EventSub WebSocket message's common shape.
// Deliberately unexported field types stay internal to this package - see
// this package's own doc comment on why Twitch wire models never leave it.
type EventSubEnvelope struct {
	Metadata EventSubMetadata `json:"metadata"`
	Payload  json.RawMessage  `json:"payload"`
}

// EventSubMetadata is the envelope's metadata block.
type EventSubMetadata struct {
	MessageID           string `json:"message_id"`
	MessageType         string `json:"message_type"`
	MessageTimestamp    string `json:"message_timestamp"`
	SubscriptionType    string `json:"subscription_type,omitempty"`
	SubscriptionVersion string `json:"subscription_version,omitempty"`
}

// EventSubSession is the "session" object carried by session_welcome and
// session_reconnect payloads.
type EventSubSession struct {
	ID                      string `json:"id"`
	Status                  string `json:"status"`
	KeepaliveTimeoutSeconds *int   `json:"keepalive_timeout_seconds"`
	ReconnectURL            string `json:"reconnect_url"`
}

type eventSubWelcomePayload struct {
	Session EventSubSession `json:"session"`
}

type eventSubReconnectPayload struct {
	Session EventSubSession `json:"session"`
}

// EventSubSubscriptionRef is the "subscription" object carried by
// notification and revocation payloads.
type EventSubSubscriptionRef struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Type    string `json:"type"`
	Version string `json:"version"`
}

type eventSubNotificationPayload struct {
	Subscription EventSubSubscriptionRef `json:"subscription"`
	Event        json.RawMessage         `json:"event"`
}

type eventSubRevocationPayload struct {
	Subscription EventSubSubscriptionRef `json:"subscription"`
}

// ParseWelcome decodes a session_welcome envelope's payload.
func ParseWelcome(payload json.RawMessage) (EventSubSession, error) {
	var p eventSubWelcomePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return EventSubSession{}, err
	}
	return p.Session, nil
}

// ParseReconnect decodes a session_reconnect envelope's payload.
func ParseReconnect(payload json.RawMessage) (EventSubSession, error) {
	var p eventSubReconnectPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return EventSubSession{}, err
	}
	return p.Session, nil
}

// ParseNotification decodes a notification envelope's payload into the
// subscription reference and the still-raw, type-specific event body.
func ParseNotification(payload json.RawMessage) (EventSubSubscriptionRef, json.RawMessage, error) {
	var p eventSubNotificationPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return EventSubSubscriptionRef{}, nil, err
	}
	return p.Subscription, p.Event, nil
}

// ParseRevocation decodes a revocation envelope's payload.
func ParseRevocation(payload json.RawMessage) (EventSubSubscriptionRef, error) {
	var p eventSubRevocationPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return EventSubSubscriptionRef{}, err
	}
	return p.Subscription, nil
}
