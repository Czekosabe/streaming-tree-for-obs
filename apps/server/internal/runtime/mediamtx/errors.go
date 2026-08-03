package mediamtx

import "errors"

// Stable, machine-readable error codes.
//
// These travel to the frontend, which maps them to localized messages, so they
// are part of the public contract and must not be renamed casually. An English
// fallback message always travels alongside, so an unmapped code still renders
// as a sentence.
const (
	// CodeNotInstalled means no managed installation and no override exist.
	CodeNotInstalled = "mediamtx_not_installed"

	// CodeIncompatibleVersion means a binary was found but reports a version
	// other than the pinned one.
	CodeIncompatibleVersion = "mediamtx_incompatible_version"

	// CodeUnsupportedPlatform means this OS/architecture has no official
	// release asset mapping.
	CodeUnsupportedPlatform = "mediamtx_unsupported_platform"

	// CodeChecksumMismatch means the downloaded archive did not match the
	// official checksum.
	CodeChecksumMismatch = "mediamtx_checksum_mismatch"

	// CodeDownloadFailed covers network failures, non-success statuses and
	// oversized responses.
	CodeDownloadFailed = "mediamtx_download_failed"

	// CodeArchiveInvalid means the archive was rejected: unsafe entry paths,
	// missing executable or missing license.
	CodeArchiveInvalid = "mediamtx_archive_invalid"

	// CodeInstallFailed covers filesystem failures while installing.
	CodeInstallFailed = "mediamtx_install_failed"

	// CodeInstallInProgress means another installation is already running.
	CodeInstallInProgress = "mediamtx_install_in_progress"

	// CodePermissionDenied means the binary could not be executed or written.
	CodePermissionDenied = "mediamtx_permission_denied"

	// CodePortInUse means an RTMP or Control API listener address is taken.
	CodePortInUse = "mediamtx_port_in_use"

	// CodeStartFailed means the process could not be spawned.
	CodeStartFailed = "mediamtx_start_failed"

	// CodeReadinessTimeout means the process started but its Control API never
	// answered within the startup budget.
	CodeReadinessTimeout = "mediamtx_readiness_timeout"

	// CodeExitedUnexpectedly means a ready process terminated on its own.
	CodeExitedUnexpectedly = "mediamtx_exited_unexpectedly"

	// CodeRestartLimit means the restart budget was exhausted.
	CodeRestartLimit = "mediamtx_restart_limit_reached"

	// CodeAPIUnreachable means the Control API stopped answering while the
	// process was still running.
	CodeAPIUnreachable = "mediamtx_api_unreachable"

	// CodeInvalidState means the requested transition is impossible right now.
	CodeInvalidState = "mediamtx_invalid_state"
)

// Sentinel errors for control flow inside the package.
var (
	ErrNotInstalled        = errors.New("mediamtx is not installed")
	ErrIncompatibleVersion = errors.New("mediamtx version is not supported")
	ErrUnsupportedPlatform = errors.New("no mediamtx release asset for this platform")
	ErrVersionUnreadable   = errors.New("cannot read the mediamtx version")
	ErrChecksumMismatch    = errors.New("mediamtx archive checksum mismatch")
	ErrArchiveInvalid      = errors.New("mediamtx archive is not usable")
	ErrInstallInProgress   = errors.New("a mediamtx installation is already running")
	ErrInvalidState        = errors.New("the requested transition is not possible in this state")
)

// RuntimeError pairs a stable code with an English fallback message.
//
// The code is what the frontend localizes; the message is what it shows when it
// has no mapping for the code, so a user always sees a sentence.
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
