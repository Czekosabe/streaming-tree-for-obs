//go:build windows

package procutil

import (
	"os/exec"
	"syscall"
)

// HideConsoleWindow marks cmd so Windows never allocates or shows a
// console window for it - see procutil.go for why this exists. Safe to
// call after a caller has already set cmd.SysProcAttr for its own
// reasons (e.g. CreationFlags): only the HideWindow field is touched.
func HideConsoleWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
}
