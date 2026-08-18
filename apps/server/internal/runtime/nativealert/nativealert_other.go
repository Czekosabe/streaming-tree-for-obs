//go:build !windows && !darwin && !linux

// Fallback for platforms with no real packaged fatal-alert mechanism (e.g.
// the BSDs). Windows, macOS (docs/macos-packaging.md §12,
// nativealert_darwin.go), and Linux (docs/linux-desktop-packaging.md §12,
// nativealert_linux.go) all have real implementations. Non-packaged
// developer builds never build with the no-console release flag in the
// first place, so stderr already reaches them - this exists only so the
// package keeps compiling for other platforms.
package nativealert

import (
	"fmt"
	"os"
)

// ShowFatalError prints title/message to stderr.
func ShowFatalError(title, message string) {
	fmt.Fprintf(os.Stderr, "%s: %s\n", title, message)
}
