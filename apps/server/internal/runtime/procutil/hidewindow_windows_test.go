//go:build windows

package procutil

import (
	"os/exec"
	"syscall"
	"testing"
)

func TestHideConsoleWindowSetsHideWindow(t *testing.T) {
	cmd := exec.Command("cmd.exe")
	HideConsoleWindow(cmd)

	if cmd.SysProcAttr == nil {
		t.Fatal("SysProcAttr is nil, want it set")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Fatal("HideWindow = false, want true")
	}
}

// A caller (branch/process_windows.go, mediamtx/process_windows.go) may
// have already set SysProcAttr for its own reasons (CreationFlags) -
// HideConsoleWindow must add HideWindow without discarding that.
func TestHideConsoleWindowPreservesExistingSysProcAttr(t *testing.T) {
	cmd := exec.Command("cmd.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}

	HideConsoleWindow(cmd)

	if cmd.SysProcAttr.CreationFlags != syscall.CREATE_NEW_PROCESS_GROUP {
		t.Fatalf("CreationFlags = %v, want it preserved", cmd.SysProcAttr.CreationFlags)
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Fatal("HideWindow = false, want true")
	}
}
