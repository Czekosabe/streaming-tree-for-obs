//go:build !windows

package procutil

import "os/exec"

// assignToChildJob is a no-op on every platform other than Windows.
//
// This fix is scoped to the real defect a physical/manual Windows
// retest actually found (a real physical Windows machine, not a
// hypothesis about other platforms): an orphaned MediaMTX process
// still bound to its RTMP port after an ungraceful parent termination,
// with no Windows Job Object protecting against it - see
// childjob_windows.go's own doc comment for the full mechanism.
// Whether the same class of orphaning is possible on Linux/macOS
// (Setpgid: true, used in internal/runtime/branch/process_unix.go,
// affects signal delivery, not parent-death cleanup - it does not by
// itself guarantee a child dies with its parent) has not been audited
// here; this is deliberately not claimed as already-safe, only as
// out of scope for this specific remediation.
func assignToChildJob(cmd *exec.Cmd) error { return nil }
