// Package outboundchat is the Stage 11A provider-independent outbound-chat
// foundation: a narrow sending abstraction (Provider), a bounded in-memory
// per-account dispatcher, and the domain types the HTTP layer and a future
// Stage 11B command/scheduling engine both build on.
//
// Nothing in this package imports internal/provider/twitch directly - a
// Twitch adapter implements Provider from the other side, exactly like
// internal/domain/account.Provider already keeps the connected-account
// foundation provider-independent. This package also never imports
// internal/operatorchat or internal/chatoverlay: an outbound send is a
// one-shot provider call, not a projection, and the message a sent chat
// later becomes (once it echoes back through EventSub) is operator chat's
// own concern, not this package's.
package outboundchat

import (
	"context"
	"time"

	"github.com/streaming-tree/server/internal/domain/account"
)

// Source identifies what triggered an outbound send. Only SourceManual is
// implemented in Stage 11A; SourceCommand and SourceScheduled are reserved
// names for Stage 11B so the dispatcher's own queue/priority types never
// need to change shape when those sources are added - see
// docs/progress.md's Stage 11A entry for the exact product boundary. No
// command-recognition or scheduling logic exists anywhere in this package.
type Source string

const (
	// SourceManual is an operator-typed message sent from the Chat page -
	// the only source Stage 11A implements.
	SourceManual Source = "manual"
	// SourceCommand is reserved for Stage 11B's chat-command engine. Not
	// implemented; nothing in this package ever sets or reads it yet.
	SourceCommand Source = "command"
	// SourceScheduled is reserved for Stage 11B's scheduled messages. Not
	// implemented; nothing in this package ever sets or reads it yet.
	SourceScheduled Source = "scheduled"
)

// SendMessageRequest is the provider-independent request to send one chat
// message - never a raw provider request body. See ValidateMessage and
// ValidateReplyParentMessageID for the rules Message and
// ReplyParentMessageID must satisfy before this is ever handed to a
// Provider.
type SendMessageRequest struct {
	AccountID            string
	Message              string
	ReplyParentMessageID string
	Source               Source
}

// SendMessageResult is the provider-independent outcome of a send attempt
// that genuinely completed - as opposed to a returned error (see errors.go),
// which means no trustworthy result exists at all, and must never be
// treated as if Sent were false.
//
// Deliberately excludes, and must never carry: an OAuth token, a raw
// provider response, raw HTTP headers, a provider's own error prose, a
// complete request URL, or the message text itself.
type SendMessageResult struct {
	// ProviderMessageID is the provider's own identifier for the sent
	// message, when Sent is true.
	ProviderMessageID string
	Sent              bool
	// Code is a stable, provider-independent outcome identifier, set only
	// when Sent is false (for example "dropped") - never a provider's own
	// raw drop-reason string or free-text explanation.
	Code        string
	CompletedAt time.Time
}

// Capability compares an account's currently-granted scopes against a
// provider's outbound-chat requirement, independently of that account's
// metadata and inbound-engagement health. Mirrors
// twitch.CapabilityAssessment's shape without this package depending on the
// twitch package - see Provider's own doc comment on why the dependency
// only ever points the other way.
type Capability struct {
	Required                  []string
	Granted                   []string
	Missing                   []string
	Available                 bool
	PermissionUpgradeRequired bool
}

// Provider is the narrow, provider-specific contract this package's
// dispatcher depends on. A provider adapter implements this from the other
// side (see internal/provider/twitch's own adapter) - this package never
// imports a concrete provider package, so adding a second outbound-capable
// provider later never requires touching this one, and a provider with no
// outbound-chat support at all is simply never registered with the
// dispatcher's Manager - it is never forced to implement a method it has no
// real answer for.
type Provider interface {
	ProviderID() account.ProviderID

	// AssessCapability reports whether acc's currently-granted scopes
	// satisfy this provider's outbound-chat requirement.
	AssessCapability(acc account.Account) Capability

	// SendChatMessage sends one message. clientID is the provider's
	// currently-effective integration Client ID (resolved by the caller,
	// exactly like account.Provider's own methods take it as an explicit
	// argument rather than resolving it themselves - see
	// account.Service.EffectiveClientID). A returned SendMessageResult with
	// Sent == false is a normal, structured "dropped" outcome, not a Go
	// error - see errors.go for the genuinely exceptional cases (rate
	// limited, forbidden, provider failure, delivery unknown, a token
	// rejected as unauthorized).
	SendChatMessage(ctx context.Context, acc account.Account, token account.TokenBundle, clientID string, req SendMessageRequest) (SendMessageResult, error)
}
