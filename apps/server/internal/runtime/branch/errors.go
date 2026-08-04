package branch

import "errors"

// Stable, machine-readable error codes carried in RuntimeError.Code.
const (
	CodeStartFailed        = "branch_start_failed"
	CodeExitedUnexpectedly = "branch_exited_unexpectedly"
	CodeRestartLimit       = "branch_restart_limit_reached"
	CodeUnsupportedCodec   = "branch_unsupported_codec"
)

// Sentinel errors for control flow inside the package and for the HTTP layer
// to match on.
var (
	// ErrNotFound means the platform is not known to the manager at all
	// (never configured, or deleted).
	ErrNotFound = errors.New("branch not found")
	// ErrConflict means the branch already has a process running or starting.
	ErrConflict = errors.New("branch already running")
	// ErrNotRunning means Stop or Restart was requested for a branch that is
	// idle, blocked or already stopping.
	ErrNotRunning = errors.New("branch is not running")
	// ErrBlocked means Start was requested but at least one blocker prevents
	// it - see the returned blocker list.
	ErrBlocked = errors.New("branch is not eligible to start")
)

// NewRuntimeError builds a reportable branch error.
//
// message must be a static, English sentence this package wrote itself -
// never text interpolated from a stream key, a destination URL, or captured
// FFmpeg output. Those are redacted separately, at the point they are
// captured - see Redactor in redact.go - specifically because a message
// built here has no reliable way to know what today's secrets even are.
func NewRuntimeError(code, message string) *RuntimeError {
	return &RuntimeError{Code: code, Message: message}
}

func (e *RuntimeError) Error() string {
	return e.Code + ": " + e.Message
}
