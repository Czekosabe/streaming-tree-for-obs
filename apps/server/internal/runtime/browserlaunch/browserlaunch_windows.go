//go:build windows

// Package browserlaunch opens the packaged application's own local
// management URL in the user's default browser (docs/windows-packaging.md
// §6).
//
// Windows implementation: ShellExecuteW with the "open" verb, the official
// mechanism documented at
// https://learn.microsoft.com/windows/win32/api/shellapi/nf-shellapi-shellexecutew
// for launching a URL with its OS-registered default handler. No shell is
// invoked - shell32.dll's ShellExecuteW is called directly through
// golang.org/x/sys/windows, so there is no command-injection surface, and
// the only string this package ever passes is the application's own
// generated loopback URL.
package browserlaunch

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

const showNormal = 1 // SW_SHOWNORMAL

var (
	modShell32        = windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteW = modShell32.NewProc("ShellExecuteW")
)

// Open launches url in the user's default browser. url must be an
// application-generated loopback URL - never anything derived from
// browser/user-supplied input.
func Open(url string) error {
	verb, err := windows.UTF16PtrFromString("open")
	if err != nil {
		return fmt.Errorf("browserlaunch: encoding verb: %w", err)
	}
	target, err := windows.UTF16PtrFromString(url)
	if err != nil {
		return fmt.Errorf("browserlaunch: encoding url: %w", err)
	}

	ret, _, _ := procShellExecuteW.Call(
		0, // NULL parent window
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(target)),
		0, // no parameters
		0, // default working directory
		showNormal,
	)

	// ShellExecuteW's return value is an HINSTANCE cast for legacy 16-bit
	// compatibility; per its own documented contract, a value greater than
	// 32 means success and anything else is an error code.
	if ret <= 32 {
		return fmt.Errorf("browserlaunch: ShellExecuteW failed with code %d", ret)
	}
	return nil
}
