//go:build !windows && !darwin && !linux

// Fallback for any other Unix-like target (e.g. the BSDs). Not a packaging
// target for this project; this exists only so an unusual developer build
// keeps compiling, using the same xdg-open convention Linux uses.
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
