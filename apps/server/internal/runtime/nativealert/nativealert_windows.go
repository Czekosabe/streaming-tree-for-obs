//go:build windows

// Package nativealert shows a fatal startup error to the user when the
// packaged application has no visible console window to print to
// (docs/windows-packaging.md §7/§13) - a port already in use, an unusable
// application-data directory, a failed database migration, or missing/
// corrupt embedded frontend assets must not simply disappear.
//
// Windows implementation: MessageBoxW (user32.dll), the standard Win32
// modal message-box API. The caller is responsible for keeping the message
// text free of secrets, tokens, stream keys, or raw environment values -
// this package only displays what it is given.
package nativealert

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	mbOK          = 0x00000000
	mbIconError   = 0x00000010
	mbSystemModal = 0x00001000
)

var (
	modUser32       = windows.NewLazySystemDLL("user32.dll")
	procMessageBoxW = modUser32.NewProc("MessageBoxW")
)

// ShowFatalError displays title/message in a modal error dialog and blocks
// until the user dismisses it.
func ShowFatalError(title, message string) {
	titlePtr, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return
	}
	messagePtr, err := windows.UTF16PtrFromString(message)
	if err != nil {
		return
	}

	_, _, _ = procMessageBoxW.Call(
		0, // NULL owner window
		uintptr(unsafe.Pointer(messagePtr)),
		uintptr(unsafe.Pointer(titlePtr)),
		uintptr(mbOK|mbIconError|mbSystemModal),
	)
}
