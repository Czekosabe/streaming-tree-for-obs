//go:build linux

// Package nativealert shows a fatal startup error to the user when the
// packaged application has no visible console window to print to
// (docs/linux-desktop-packaging.md §12).
//
// There is no single universal Linux modal-dialog API the way Win32
// (MessageBoxW) or AppKit (NSAlert) provide one. This is an honestly
// best-effort implementation: if zenity (GTK) is found on PATH, it is
// used; otherwise if kdialog (KDE) is found, that is used; otherwise this
// falls back to the existing stderr behavior. Neither zenity nor kdialog
// is guaranteed present on every Linux installation, so this is
// deliberately never described as a guaranteed cross-desktop mechanism the
// way the Windows/macOS implementations are. Both tools are invoked with a
// fixed executable name and fixed flags via argv - never a shell string -
// and the title/message are passed as literal argv elements, never
// interpolated into any command.
package nativealert

import (
	"fmt"
	"os"
	"os/exec"
)

// ShowFatalError displays title/message via zenity or kdialog if either is
// available, blocking until the user dismisses it; otherwise it prints to
// stderr, exactly like the generic fallback.
func ShowFatalError(title, message string) {
	if path, err := exec.LookPath("zenity"); err == nil {
		if runErr := exec.Command(path, "--error", "--title="+title, "--text="+message).Run(); runErr == nil {
			return
		}
	}
	if path, err := exec.LookPath("kdialog"); err == nil {
		if runErr := exec.Command(path, "--error", message, "--title", title).Run(); runErr == nil {
			return
		}
	}
	fmt.Fprintf(os.Stderr, "%s: %s\n", title, message)
}
