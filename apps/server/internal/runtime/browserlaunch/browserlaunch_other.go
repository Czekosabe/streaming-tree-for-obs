//go:build !windows

// Non-Windows fallback. Stage 20A's actual packaging target is Windows-
// first (docs/windows-packaging.md §6); this exists only so a non-Windows
// developer build keeps compiling and behaves reasonably, using each
// platform's own standard "open with the default handler" command with a
// fixed argv (never a shell string, so there is no command-injection
// surface even though url is always an application-generated loopback URL).
package browserlaunch

import (
	"fmt"
	"os/exec"
	"runtime"
)

// Open launches url in the user's default browser.
func Open(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("browserlaunch: %w", err)
	}
	return nil
}
