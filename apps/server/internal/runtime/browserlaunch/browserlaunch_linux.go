//go:build linux

// Package browserlaunch opens the packaged application's own local
// management URL in the user's default browser. Stage 20A/20C1's actual
// packaging targets are Windows and macOS (docs/windows-packaging.md,
// docs/macos-packaging.md); this exists so a Linux developer build (and,
// ahead of Stage 20D, a future Linux package) keeps compiling and behaves
// reasonably.
//
// Linux implementation: xdg-open, the freedesktop.org-documented mechanism
// for launching a URL with its desktop-registered default handler. A fixed
// executable name is invoked directly with argv - never a shell string -
// so there is no command-injection surface even though url is always an
// application-generated loopback URL.
package browserlaunch

import (
	"fmt"
	"os/exec"
)

// Open launches url in the user's default browser.
func Open(url string) error {
	if err := exec.Command("xdg-open", url).Start(); err != nil {
		return fmt.Errorf("browserlaunch: %w", err)
	}
	return nil
}
