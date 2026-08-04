//go:build !windows

package mediamtx

import (
	"os"
	"os/exec"
	"syscall"
)

// configureProcessAttributes puts MediaMTX in its own process group.
//
// Without this, a Ctrl+C in the terminal reaches the child directly and it dies
// before the supervisor can stop it in an orderly way and record why.
func configureProcessAttributes(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// terminate asks MediaMTX to shut down gracefully.
//
// On Unix this really is graceful: SIGTERM lets MediaMTX close its listeners
// and flush its logs before exiting. The supervisor escalates to SIGKILL only
// after the grace period.
func terminate(process *os.Process) error {
	return process.Signal(syscall.SIGTERM)
}
