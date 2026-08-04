//go:build !windows

package branch

import (
	"os"
	"os/exec"
	"syscall"
)

// configureProcessAttributes puts FFmpeg in its own process group, so a
// terminal Ctrl+C does not reach it directly before the manager can stop it
// in an orderly way.
func configureProcessAttributes(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// terminate asks FFmpeg to shut down gracefully. FFmpeg handles SIGTERM by
// finishing the current write and exiting cleanly.
func terminate(process *os.Process) error {
	return process.Signal(syscall.SIGTERM)
}
