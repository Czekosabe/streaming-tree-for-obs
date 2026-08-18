//go:build !windows

// Non-Windows fallback. Stage 20A's actual packaging target is
// Windows-first (docs/windows-packaging.md §7); non-Windows developer
// builds never build with the no-console release flag in the first place,
// so stderr already reaches them - this exists only so the package keeps
// compiling.
package nativealert

import (
	"fmt"
	"os"
)

// ShowFatalError prints title/message to stderr.
func ShowFatalError(title, message string) {
	fmt.Fprintf(os.Stderr, "%s: %s\n", title, message)
}
