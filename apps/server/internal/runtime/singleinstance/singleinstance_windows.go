//go:build windows

// Package singleinstance detects whether another packaged instance of
// Streaming Tree for OBS is already running (docs/windows-packaging.md
// §9).
//
// Windows implementation: a named mutex via CreateMutexW, the official,
// documented single-instance pattern
// (https://learn.microsoft.com/windows/win32/api/synchapi/nf-synchapi-createmutexw):
// the first process to call CreateMutexW with a given name creates and owns
// it; a second process's call with the same name succeeds but reports
// ERROR_ALREADY_EXISTS - the reliable signal used here, never "something
// answers on the HTTP port." The mutex is process-lifetime-scoped: Windows
// closes it automatically (including on a crash), so no stale-lock
// recovery code is needed.
package singleinstance

import (
	"errors"

	"golang.org/x/sys/windows"
)

// mutexName is fixed and application-specific, never derived from user
// input. The "Local\" prefix scopes it to the current login session,
// matching this application's own per-user (not per-machine) install and
// data model.
const mutexName = `Local\StreamingTreeForOBS.SingleInstance`

// Acquire attempts to become the one running packaged instance. ok is true
// when this process is the first (and therefore owns the application); ok
// is false when another instance already holds it - the caller must not
// start a second backend in that case. The returned release func should be
// deferred for the lifetime of the process when ok is true; it is a no-op
// otherwise.
func Acquire() (ok bool, release func(), err error) {
	name, encErr := windows.UTF16PtrFromString(mutexName)
	if encErr != nil {
		return false, func() {}, encErr
	}

	handle, createErr := windows.CreateMutex(nil, false, name)
	if createErr != nil {
		if errors.Is(createErr, windows.ERROR_ALREADY_EXISTS) {
			// CreateMutexW still returns a valid, usable handle in this
			// case (per its own documented contract) - it is simply not
			// this process's to own.
			_ = windows.CloseHandle(handle)
			return false, func() {}, nil
		}
		return false, func() {}, createErr
	}

	return true, func() { _ = windows.CloseHandle(handle) }, nil
}
