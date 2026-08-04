//go:build windows

package mediamtx

import (
	"os"
	"os/exec"
	"syscall"
)

// configureProcessAttributes detaches MediaMTX from the parent's console.
//
// CREATE_NEW_PROCESS_GROUP stops a console Ctrl+C from being delivered straight
// to the child, so the supervisor decides when it stops.
func configureProcessAttributes(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

// terminate stops MediaMTX on Windows.
//
// HONEST LIMITATION: this is NOT a graceful shutdown. Windows has no SIGTERM,
// and MediaMTX is a console application without a window, so there is no
// message loop to post WM_CLOSE to. Delivering CTRL_BREAK_EVENT would require
// sharing a console with the child, which the backend deliberately avoids.
//
// The process is therefore terminated immediately. That is acceptable here:
// MediaMTX holds no unflushed persistent state - it is a relay with listeners
// and in-memory sessions, and the operating system reclaims both. Nothing this
// application owns is lost.
//
// The README documents this difference rather than claiming graceful shutdown
// on every platform.
func terminate(process *os.Process) error {
	return process.Kill()
}
