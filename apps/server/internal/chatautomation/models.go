package chatautomation

import "time"

// ScheduleState is one schedule's own runtime activity state - distinct
// from whether it is enabled, and distinct from any single target
// account's own capability. See the Stage 11B task's own Part 25.
type ScheduleState string

const (
	ScheduleDisabled           ScheduleState = "disabled"
	ScheduleScheduled          ScheduleState = "scheduled"
	ScheduleWaitingForStream   ScheduleState = "waiting_for_stream"
	ScheduleWaitingForActivity ScheduleState = "waiting_for_activity"
	ScheduleRateLimited        ScheduleState = "rate_limited"
	SchedulePermissionRequired ScheduleState = "permission_required"
	ScheduleSending            ScheduleState = "sending"
	ScheduleError              ScheduleState = "error"
)

// TargetSnapshot is one schedule or command's per-account runtime state
// - never message text, never a triggering username.
type TargetSnapshot struct {
	AccountID      string
	LastAttemptAt  *time.Time
	LastSuccessAt  *time.Time
	LastSkipReason SkipReason
	SendsThisHour  int
}

// ScheduleSnapshot is one schedule's full runtime state - see the Stage
// 11B task's own Part 25. Never message text, never a rendered
// template, never a token.
type ScheduleSnapshot struct {
	ScheduleID     string
	Enabled        bool
	State          ScheduleState
	NextRunAt      *time.Time
	TargetCount    int
	LastAttemptAt  *time.Time
	LastSuccessAt  *time.Time
	LastSkipReason SkipReason
	Targets        []TargetSnapshot
}

// CommandSnapshot is one command's runtime status - see Part 26. Never
// a triggering username, never message/response text.
type CommandSnapshot struct {
	CommandID      string
	Enabled        bool
	TargetCount    int
	MatchCount     int64
	ResponseCount  int64
	LastResponseAt *time.Time
}

// EngineStatus is the command engine's own non-content status - Part 26.
type EngineStatus struct {
	Running             bool
	SubscribedToBus     bool
	CommandCount        int
	EnabledCommandCount int
	TotalMatched        int64
	TotalResponses      int64
	TotalCooldownSkips  int64
	TotalRoleSkips      int64
	TotalSelfSkips      int64
	LastErrorCode       string
}

// Status is the automation runtime's full status snapshot - the HTTP
// status endpoint's own source of truth.
type Status struct {
	Engine    EngineStatus
	Schedules []ScheduleSnapshot
	Commands  []CommandSnapshot
}

// SendResult is one target's outcome from a manual "Send now" - Part 11
// requires one result per target, and one failed target must never
// prevent another from succeeding.
type SendResult struct {
	AccountID         string
	Sent              bool
	ProviderMessageID string
	SkipReason        SkipReason
}
