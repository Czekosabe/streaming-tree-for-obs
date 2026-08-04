//go:build windows

package branch

import (
	"os"
	"os/exec"
	"syscall"
)

// configureProcessAttributes detaches FFmpeg from the parent's console, so a
// console Ctrl+C is not delivered straight to the child.
func configureProcessAttributes(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

// terminate stops FFmpeg on Windows.
//
// HONEST LIMITATION: same as the MediaMTX process wrapper's terminate() -
// Windows has no SIGTERM, and FFmpeg here runs with no console to post
// WM_CLOSE to, so this is an immediate kill, not a graceful shutdown. FFmpeg
// holds no unflushed persistent state of its own beyond the in-flight
// network write, which the operating system reclaims; the destination
// platform sees the connection simply drop, the same as any other network
// interruption it must already tolerate.
func terminate(process *os.Process) error {
	return process.Kill()
}
