//go:build !windows && !darwin

// Fallback for platforms with no real packaged fatal-alert mechanism yet
// (e.g. Linux, ahead of Stage 20D). Windows has a real implementation
// (docs/windows-packaging.md §7) and so does macOS
// (docs/macos-packaging.md §12, nativealert_darwin.go). Non-packaged
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
