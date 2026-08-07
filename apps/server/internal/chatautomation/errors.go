// Package chatautomation is the Stage 11B in-memory automation runtime:
// a scheduler for scheduled-message rules and a command engine for
// chat-command rules, both reading their definitions from
// internal/domain/chatautomation and driving Stage 11A's existing
// outbound dispatcher (internal/outboundchat) - never a second outbound
// pipeline, and never the Twitch HTTP client directly.
//
// Everything in this package is runtime state: next-run times, rolling
// send-per-hour counters, per-user cooldown timestamps, and activity-
// since-last-send counters are all held in memory only and reset on
// every backend restart - see docs/progress.md's Stage 11B entry and
// the Stage 11B task's own Part 6.
package chatautomation

import (
	"errors"
	"time"
)

var (
	// ErrPlaceholderInvalid means a template is syntactically malformed
	// (an unmatched brace, an empty placeholder) or names a placeholder
	// outside the closed, known set.
	ErrPlaceholderInvalid = errors.New("invalid placeholder")

	// ErrScheduleNotFound / ErrCommandNotFound mirror the domain
	// package's own sentinels for the runtime layer's own lookups.
	ErrScheduleNotFound = errors.New("chat schedule not found")
	ErrCommandNotFound  = errors.New("chat command not found")

	// ErrProviderUnsupported means a target account's provider has no
	// registered outbound-chat Provider at all.
	ErrProviderUnsupported = errors.New("outbound chat is not supported for this account's provider")
	// ErrPermissionRequired means a target account has not granted
	// outbound-chat permission (mirrors outboundchat.ErrPermissionRequired,
	// re-exported here so callers never need to import both packages
	// just to check this one case).
	ErrPermissionRequired = errors.New("outbound chat permission has not been granted")
	// ErrQueueFull means the target account's dispatcher queue - or this
	// package's own smaller automation quota within it, see dispatch.go -
	// was full at submission time.
	ErrQueueFull = errors.New("outbound send queue is full")
	// ErrRenderedMessageTooLong means an expanded template exceeded the
	// outbound provider's own 500-code-point limit.
	ErrRenderedMessageTooLong = errors.New("rendered message exceeds the provider's message length limit")
	// ErrPlaceholderUnresolved means a known placeholder could not be
	// resolved from the target's own available context (Part 19) -
	// distinct from ErrPlaceholderInvalid, which is a save-time syntax/
	// name problem, not a per-execution data-availability problem.
	ErrPlaceholderUnresolved = errors.New("a placeholder could not be resolved for this target")
)

// SkipReason is a stable, non-content runtime code explaining why an
// automated execution did not send - never a provider error string,
// never rendered text. See the Stage 11B task's own Part 3
// ("a skipped execution must produce an explicit runtime reason rather
// than indefinite queue growth") and Part 17 (commands).
type SkipReason string

const (
	SkipNone                   SkipReason = ""
	SkipStreamNotReceiving     SkipReason = "waiting_for_stream"
	SkipActivityInsufficient   SkipReason = "waiting_for_activity"
	SkipRateLimited            SkipReason = "rate_limited"
	SkipPermissionRequired     SkipReason = "permission_required"
	SkipProviderUnsupported    SkipReason = "provider_unsupported"
	SkipQueueFull              SkipReason = "queue_full"
	SkipPlaceholderUnresolved  SkipReason = "placeholder_unresolved"
	SkipRenderedMessageTooLong SkipReason = "rendered_message_too_long"
	SkipSendFailed             SkipReason = "send_failed"
	SkipResponseExpired        SkipReason = "response_expired"
	SkipRoleRequired           SkipReason = "role_required"
	SkipGlobalCooldown         SkipReason = "global_cooldown"
	SkipUserCooldown           SkipReason = "user_cooldown"
)

// clock returns the current time; a package-level type (not a struct
// field alone) so both the scheduler and the command engine share one
// fake-clock convention in tests.
type clock func() time.Time
