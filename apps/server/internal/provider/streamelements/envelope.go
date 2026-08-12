// Package streamelements is this application's typed adapter over the
// real StreamElements Astro WebSocket API - the wire shapes, the exact
// decimal-to-integer-micros money conversion, and the normalizer that
// turns one raw tip into a provider-independent engagement.Event.
// Researched and recorded in docs/provider-integrations/
// external-donations.md.
//
// Every wire-format struct in this file exists only here: HTTP/WebSocket
// handlers and internal/runtime/streamelementsengagement never see a raw
// Astro JSON shape directly - see normalize.go's own NormalizeTip.
package streamelements

import "encoding/json"

// MessageType is an Astro envelope's own `type` field - see
// docs/provider-integrations/external-donations.md §5 for the complete,
// verified envelope shape this mirrors exactly.
type MessageType string

const (
	// Server -> client.
	MessageTypeWelcome   MessageType = "welcome"
	MessageTypeResponse  MessageType = "response"
	MessageTypeMessage   MessageType = "message"
	MessageTypeReconnect MessageType = "reconnect"

	// Client -> server.
	MessageTypeSubscribe   MessageType = "subscribe"
	MessageTypeUnsubscribe MessageType = "unsubscribe"
)

// Topics this application subscribes to. Deliberately never
// channel.activities - see docs/provider-integrations/
// external-donations.md §6/§16: a second, duplicate tip-shaped path is
// never opened.
const (
	TopicChannelTips           = "channel.tips"
	TopicChannelTipsModeration = "channel.tips.moderation"
)

// ScopeTipsRead and ScopeTipsModeration are the two scopes these topics
// require - recorded here for documentation; this application does not
// request scopes explicitly (a personal JWT's own grant already covers
// whatever it covers - see docs/provider-integrations/
// external-donations.md §3).
const (
	ScopeTipsRead       = "tips:read"
	ScopeTipsModeration = "tips:moderation"
)

// TokenTypeJWT is the only token_type Stage 16A ever sends - see
// docs/provider-integrations/external-donations.md §3 for why a personal
// JWT was chosen over apikey/oauth2.
const TokenTypeJWT = "jwt"

// Envelope is the outer message shape every Astro message uses, both
// directions - verified byte-for-byte against the official examples page
// (docs/provider-integrations/external-donations.md §5).
type Envelope struct {
	ID    string          `json:"id,omitempty"`
	TS    string          `json:"ts,omitempty"`
	Type  MessageType     `json:"type"`
	Topic string          `json:"topic,omitempty"`
	Room  string          `json:"room,omitempty"`
	Nonce string          `json:"nonce,omitempty"`
	Error string          `json:"error,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
}

// SubscribeRequest is the client -> server `subscribe` envelope's own
// `data` object.
type SubscribeRequest struct {
	Topic     string `json:"topic"`
	Room      string `json:"room,omitempty"`
	Token     string `json:"token"`
	TokenType string `json:"token_type"`
}

// UnsubscribeRequest is the client -> server `unsubscribe` envelope's own
// `data` object. Room is omitted to unsubscribe every room for Topic -
// this application never needs that (one connector always has exactly
// one room), included only for completeness/testability.
type UnsubscribeRequest struct {
	Topic string `json:"topic"`
	Room  string `json:"room,omitempty"`
}

// WelcomeData is the `welcome` envelope's own `data` object.
type WelcomeData struct {
	ClientID string `json:"client_id"`
}

// ReconnectData is the `reconnect` envelope's own `data` object - kept in
// runtime memory only, never persisted/logged (docs/provider-
// integrations/external-donations.md §5/§29).
type ReconnectData struct {
	ReconnectToken string `json:"reconnect_token"`
}

// ResponseData is the `response` envelope's own `data` object - present
// on both a subscribe success and a subscribe error (the error code
// itself lives on the envelope's own top-level `error` field, not here).
type ResponseData struct {
	Message string `json:"message,omitempty"`
}
