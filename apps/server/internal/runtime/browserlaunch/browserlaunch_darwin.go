//go:build darwin

// Package browserlaunch opens the packaged application's own local
// management URL in the user's default browser (docs/macos-packaging.md
// §10, docs/windows-packaging.md §6).
//
// macOS implementation: the "open" command, Apple's own documented
// mechanism for launching a URL with its OS-registered default handler
// (https://ss64.com/mac/open.html; equivalent to LSOpenCFURLRef). A fixed
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
	if err := exec.Command("open", url).Start(); err != nil {
		return fmt.Errorf("browserlaunch: %w", err)
	}
	return nil
}
