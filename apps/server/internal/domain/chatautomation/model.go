// Package chatautomation holds the Stage 11B persisted automation
// definitions: scheduled-message rules and safe chat-command rules -
// user-authored configuration an operator explicitly creates, mirroring
// internal/domain/chatoverlay's own split between "what is configured"
// (this package) and "what is happening right now" (the sibling runtime
// package internal/chatautomation, which never persists a queued job, a
// next-run time, a cooldown timestamp, or an activity counter).
//
// Deliberately does not hold anything about live chat content, an
// outbound send's own result, or a triggering user's identity - see
// internal/chatautomation for the in-memory scheduler and command engine
// that read these definitions and drive Stage 11A's existing outbound
// dispatcher (internal/outboundchat). This package never imports
// internal/outboundchat or internal/provider/twitch.
package chatautomation

import "time"

// Role is the closed, fixed command-permission enum - never a free-text
// role. See internal/chatautomation/commands.go for the semantic
// (not-purely-hierarchical) matching rules.
type Role string

const (
	RoleEveryone    Role = "everyone"
	RoleSubscriber  Role = "subscriber"
	RoleVIP         Role = "vip"
	RoleModerator   Role = "moderator"
	RoleBroadcaster Role = "broadcaster"
)

// ValidRoles lists every accepted Role value, in the order shown by the
// frontend's role selector.
var ValidRoles = []Role{RoleEveryone, RoleSubscriber, RoleVIP, RoleModerator, RoleBroadcaster}

func (r Role) valid() bool {
	for _, v := range ValidRoles {
		if r == v {
			return true
		}
	}
	return false
}

// Target is one explicit per-account destination for a schedule or
// command. PlatformID is optional: when set it provides deterministic
// local metadata context for placeholders (see the Stage 11B task's own
// Part 4); when empty, placeholders requiring destination metadata
// simply become unresolved rather than this application silently
// guessing a linked platform.
type Target struct {
	AccountID  string
	PlatformID string
}

// ScheduleMessage is one message alternative within a schedule's message
// group. Position is a stable authoring/display order; which message is
// actually selected at execution time is runtime-only state (see
// internal/chatautomation/scheduler.go), never persisted here.
type ScheduleMessage struct {
	ID              string
	MessageTemplate string
	Position        int
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Schedule is one persisted scheduled-message definition.
type Schedule struct {
	ID      string
	Name    string
	Enabled bool

	IntervalSeconds   int
	FirstDelaySeconds int
	JitterSeconds     int

	// OnlyWhileIngestReceiving refers specifically to this application's
	// own local MediaMTX ingest path state - never Twitch stream.online,
	// never an FFmpeg output branch, never viewer presence. See the
	// Stage 11B task's own Part 8.
	OnlyWhileIngestReceiving bool
	// MinimumChatMessages is how many eligible human chat.message events
	// must arrive on a target account since that schedule/account's
	// previous successful send before the next due execution may send.
	MinimumChatMessages int
	// MaximumSendsPerHour is a rolling, schedule/account-level ceiling on
	// successful scheduled sends - an automation-behavior control, not a
	// provider-rate-limit guarantee (internal/outboundchat's dispatcher
	// remains authoritative).
	MaximumSendsPerHour int

	Targets  []Target
	Messages []ScheduleMessage

	CreatedAt time.Time
	UpdatedAt time.Time
}

// Command is one persisted safe chat-command definition. Name is the
// canonical command name without its leading "!" - the prefix itself is
// a fixed Stage 11B constant (see internal/chatautomation/commands.go),
// never stored - always lowercase, ASCII [a-z0-9_-], 1-32 characters
// (see validation.go).
type Command struct {
	ID      string
	Name    string
	Enabled bool

	ResponseTemplate string
	RequiredRole     Role

	GlobalCooldownSeconds int
	UserCooldownSeconds   int

	Aliases []string
	Targets []Target

	CreatedAt time.Time
	UpdatedAt time.Time
}
