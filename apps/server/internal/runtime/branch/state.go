// Package branch supervises one independent FFmpeg process per configured
// destination platform ("branch"), pulling the shared local MediaMTX input
// and pushing it to that destination's configured RTMP/RTMPS server.
//
// Runtime state lives only in memory, exactly like the MediaMTX supervisor's
// own runtime state - see internal/runtime/mediamtx. No branch runtime field
// is ever written to SQLite.
package branch

import "time"

// State is the explicit branch lifecycle state.
//
// A single value, not a set of booleans: "starting and live" or "stopping and
// error" must be unrepresentable.
type State string

const (
	// StateIdle means the branch is not running and nothing asked it to run.
	StateIdle State = "idle"
	// StateBlocked means the branch cannot start right now - see Blockers.
	// Distinct from waiting_for_ingest: a blocked branch is not desired to
	// run at all until the blocker clears (for example the platform is
	// disabled or no stream key is configured).
	StateBlocked State = "blocked"
	// StateWaitingForIngest means the branch is desired to run, every other
	// requirement is satisfied, but the local MediaMTX input is not
	// currently receiving a publisher.
	StateWaitingForIngest State = "waiting_for_ingest"
	// StateStarting means the FFmpeg process was spawned and has not yet
	// produced advancing progress output.
	StateStarting State = "starting"
	// StateLive means FFmpeg has produced real, advancing progress output.
	StateLive State = "live"
	// StateRestarting means a controlled stop-then-start is in progress
	// after an unexpected exit.
	StateRestarting State = "restarting"
	// StateStopping means an explicit stop is in progress.
	StateStopping State = "stopping"
	// StateError means the branch failed and will not be restarted
	// automatically - see RuntimeError for why.
	StateError State = "error"
)

// Blocker identifiers. Stable and machine-readable: the frontend localizes
// them, so they must not be renamed casually.
const (
	BlockerPlatformDisabled      = "platform_disabled"
	BlockerOutputServerMissing   = "output_server_missing"
	BlockerStreamKeyMissing      = "stream_key_missing"
	BlockerCredentialUnavailable = "credential_store_unavailable"
	BlockerFFmpegMissing         = "ffmpeg_missing"
	BlockerFFmpegIncompatible    = "ffmpeg_incompatible"
	BlockerMediaMTXNotReady      = "mediamtx_not_ready"
	BlockerIngestNotReceiving    = "ingest_not_receiving"
)

// Progress is the subset of FFmpeg's -progress output this application
// trusts and shows. Every field is exactly what FFmpeg reported - nothing
// here is estimated, and a viewer count or provider confirmation is never
// invented from it.
type Progress struct {
	// FrameCount is -1 when FFmpeg did not report a frame count (an
	// audio-only branch, for instance).
	FrameCount int64   `json:"frameCount"`
	FPS        float64 `json:"fps"`
	OutTimeMs  int64   `json:"outTimeMs"`
	TotalSize  int64   `json:"totalSize"`
	Speed      float64 `json:"speed"`
	ObservedAt time.Time
}

// RuntimeError pairs a stable, sanitized code with an English message that
// has already been redacted - see redact.go. Never includes a stream key, a
// full destination URL, or an FFmpeg command line.
type RuntimeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Snapshot is the runtime view of one branch.
//
// Deliberately excludes: the stream key, the full destination URL, the
// FFmpeg command line, the process id, and the process environment. None is
// useful to the interface, and each would leak a secret or machine detail.
type Snapshot struct {
	PlatformID     string        `json:"platformId"`
	State          State         `json:"state"`
	DesiredRunning bool          `json:"desiredRunning"`
	Blockers       []string      `json:"blockers"`
	StartedAt      string        `json:"startedAt,omitempty"`
	LiveAt         string        `json:"liveAt,omitempty"`
	StoppedAt      string        `json:"stoppedAt,omitempty"`
	RestartCount   int           `json:"restartCount"`
	Progress       *Progress     `json:"progress"`
	LastError      *RuntimeError `json:"lastError"`
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}
