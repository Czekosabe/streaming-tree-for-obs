//go:build !windows

package procutil

import "os/exec"

// HideConsoleWindow is a no-op on every platform other than Windows -
// there is no console-window allocation to suppress (see procutil.go).
func HideConsoleWindow(cmd *exec.Cmd) {}
