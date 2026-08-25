//go:build !windows

package procutil

import (
	"os/exec"
	"testing"
)

// On every platform other than Windows this is a no-op - there is no
// console-window allocation to suppress, and no SysProcAttr field of
// this name exists in the standard library outside Windows.
func TestHideConsoleWindowIsNoopOffWindows(t *testing.T) {
	cmd := exec.Command("true")
	HideConsoleWindow(cmd)

	if cmd.SysProcAttr != nil {
		t.Fatalf("SysProcAttr = %+v, want nil (no-op)", cmd.SysProcAttr)
	}
}
