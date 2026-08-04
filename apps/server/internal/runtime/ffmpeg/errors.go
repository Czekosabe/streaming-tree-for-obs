package ffmpeg

import "errors"

// Stable, machine-readable error codes for the FFmpeg dependency.
//
// These travel to the frontend, which maps them to localized messages. An
// English fallback message always travels alongside, so an unmapped code
// still renders as a sentence.
const (
	// CodeNotFound means no executable was found at any resolution step.
	CodeNotFound = "ffmpeg_not_found"

	// CodeNotExecutable means a path was found but is a directory, or is not
	// a regular executable file.
	CodeNotExecutable = "ffmpeg_not_executable"

	// CodeExecutionFailed means the binary could not be run at all - it
	// exists and looks executable, but `ffmpeg -version` did not complete.
	CodeExecutionFailed = "ffmpeg_execution_failed"

	// CodeBelowMinimumVersion means a version was parsed and is older than
	// MinimumVersion.
	CodeBelowMinimumVersion = "ffmpeg_below_minimum_version"

	// CodeMissingCapability means the binary runs and reports a usable
	// version (or none at all), but lacks a capability this application
	// requires - see Capabilities.
	CodeMissingCapability = "ffmpeg_missing_capability"
)

// Sentinel errors for control flow inside the package.
var (
	ErrNotFound     = errors.New("ffmpeg executable not found")
	ErrIncompatible = errors.New("ffmpeg executable is not compatible")
)

// RuntimeError pairs a stable code with an English fallback message.
//
// The code is what the frontend localizes; the message is what it shows when
// it has no mapping for the code, so a user always sees a sentence.
type RuntimeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// NewRuntimeError builds a reportable error.
func NewRuntimeError(code, message string) *RuntimeError {
	return &RuntimeError{Code: code, Message: message}
}

func (e *RuntimeError) Error() string {
	return e.Code + ": " + e.Message
}
