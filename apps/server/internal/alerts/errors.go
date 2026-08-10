// Package alerts is the Stage 12A runtime: the alert matching engine, the
// bounded per-profile alert queue, playback, the public alert
// projection/SSE protocol, and the local synthetic test-alert path.
//
// Mirrors internal/chatautomation's own split from its sibling domain
// package: internal/domain/alerts holds persisted profile/rule
// definitions ("what is configured"); this package holds every piece of
// in-memory runtime state ("what is happening right now") - a queued
// alert, a current-alert snapshot, a replay slot, or a counter is never
// persisted, and resets on every backend restart.
//
// Hard rule: this package never imports internal/provider/twitch. It
// consumes only the normalized Engagement Event Bus
// (internal/engagement, internal/domain/engagement) - a provider
// connector publishes to that bus; this package never talks to a
// provider directly. See docs/progress.md's Stage 12A entries for the
// full architecture rationale.
package alerts

import "errors"

var (
	// ErrProfileNotFound/ErrRuleNotFound mirror the domain package's own
	// sentinels for the runtime layer's own not-found cases (e.g. a
	// queue command for an unknown profile).
	ErrProfileNotFound = errors.New("alert profile not found")
	ErrRuleNotFound    = errors.New("alert rule not found")

	// ErrPlaceholderInvalid means an alert template is malformed (an
	// unmatched brace, an empty placeholder) - see templates.go.
	ErrPlaceholderInvalid = errors.New("alert placeholder is invalid")
	// ErrRenderedTextTooLong means a rule's rendered alert text exceeds
	// MaxRenderedCodePoints after placeholder expansion.
	ErrRenderedTextTooLong = errors.New("rendered alert text is too long")

	// ErrQueuePaused means a queue command that requires the queue to be
	// running was rejected because it is currently paused.
	ErrQueuePaused = errors.New("alert queue is paused")
	// ErrQueueEmpty means an operation (skip current, replay previous)
	// had nothing to act on.
	ErrQueueEmpty = errors.New("alert queue is empty")
	// ErrQueueFull means a synthetic/test alert could not be enqueued
	// because the profile's bounded queue is already at capacity and the
	// new item could not displace a lower-priority one - see queue.go's
	// own capacity-eviction algorithm.
	ErrQueueFull = errors.New("alert queue is full")
	// ErrProfileDisabled means an operation was attempted against a
	// disabled alert profile.
	ErrProfileDisabled = errors.New("alert profile is disabled")

	// ErrNoReplaySnapshot means Replay Previous was called with no
	// completed/skipped alert yet recorded for this profile.
	ErrNoReplaySnapshot = errors.New("no previous alert available to replay")

	// ErrProjectionClosed means Subscribe was called after the owning
	// profile's runtime was shut down (profile deleted, or backend
	// shutting down).
	ErrProjectionClosed = errors.New("alert profile playback stream is closed")

	// ErrVisualDesignUnavailable means the Manager was constructed with
	// no VisualDesignService (never expected in production - only a
	// test double that predates Stage 13A and does not exercise it).
	ErrVisualDesignUnavailable = errors.New("visual design service is not available")
)

// SkipReason is a stable, loggable reason a queued item never played -
// never the alert's own rendered text or username (see the Stage 12A
// task's own Part 43).
type SkipReason string

const (
	SkipNone       SkipReason = ""
	SkipExpired    SkipReason = "expired"
	SkipCapacity   SkipReason = "capacity_dropped"
	SkipManual     SkipReason = "manually_skipped"
	SkipProfileOff SkipReason = "profile_disabled"
	// SkipPreempted means a currently-playing alert was replaced by a
	// strictly-higher-priority, eligible incoming alert before its own
	// duration finished (Stage 12B task Part 16-18) - tracked separately
	// from SkipManual (an operator's own Skip Current action) and its
	// own dedicated counter, TotalPreempted.
	SkipPreempted SkipReason = "preempted"
)
