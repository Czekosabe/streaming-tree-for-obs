//go:build darwin

// Package nativealert shows a fatal startup error to the user when the
// packaged application has no visible console window to print to
// (docs/macos-packaging.md §12) - a port already in use, an unusable
// application-data directory, a failed database migration, or missing/
// corrupt embedded frontend assets must not simply disappear on a
// Finder-launched app.
//
// macOS implementation: a narrow Cgo bridge to NSAlert (AppKit), Apple's
// own standard modal-alert API - see nativealert_darwin.m for the
// Objective-C half. The title and message are passed across as plain
// string data only, never interpolated into an executable command, an
// AppleScript, or a shell string of any kind, so there is no injection
// surface even though the caller's error text is free-form. The caller is
// responsible for keeping that text free of secrets, tokens, stream keys,
// or raw environment values - this package only displays what it is
// given. Requires CGO_ENABLED=1 (docs/macos-packaging.md §16), the same
// requirement the real macOS Keychain backend already has.
package nativealert

/*
#cgo LDFLAGS: -framework Cocoa
#include <stdlib.h>

void streamingTreeShowFatalAlert(const char *title, const char *message);
*/
import "C"
import "unsafe"

// ShowFatalError displays title/message in a modal AppKit alert and blocks
// until the user dismisses it.
func ShowFatalError(title, message string) {
	cTitle := C.CString(title)
	defer C.free(unsafe.Pointer(cTitle))
	cMessage := C.CString(message)
	defer C.free(unsafe.Pointer(cMessage))

	C.streamingTreeShowFatalAlert(cTitle, cMessage)
}
