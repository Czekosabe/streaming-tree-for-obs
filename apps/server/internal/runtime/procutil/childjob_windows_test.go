//go:build windows

package procutil

import (
	"os/exec"
	"testing"
	"unsafe"
)

// TestChildJobHasKillOnCloseFlag proves, by reading the flag back out
// through the real QueryInformationJobObject Win32 API (not by
// trusting the SetInformationJobObject call succeeded, or by trusting
// the struct literal was written correctly), that the job object this
// package creates genuinely carries JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
// - the one flag the whole child-process safety net depends on. A
// wrong struct layout or flag value here would silently create a job
// that assigns processes successfully but never actually protects
// against an ungraceful parent death, which no failing assignment call
// would ever reveal on its own.
func TestChildJobHasKillOnCloseFlag(t *testing.T) {
	job, err := getOrCreateChildJob()
	if err != nil {
		t.Fatalf("getOrCreateChildJob() error = %v", err)
	}

	var info jobobjectExtendedLimitInformation
	var returned uint32
	ret, _, callErr := procQueryInformationJobObject.Call(
		job, jobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info),
		uintptr(unsafe.Pointer(&returned)),
	)
	if ret == 0 {
		t.Fatalf("QueryInformationJobObject failed: %v", callErr)
	}

	if info.BasicLimitInformation.LimitFlags&jobObjectLimitKillOnJobClose == 0 {
		t.Fatalf("LimitFlags = %#x, missing JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE (%#x)",
			info.BasicLimitInformation.LimitFlags, uint32(jobObjectLimitKillOnJobClose))
	}
}

// TestAssignToChildJobSucceeds spawns a real, short-lived child
// process and confirms AssignProcessToJobObject genuinely succeeds
// against it - not mocked, a real Win32 call against a real process.
// The end-to-end claim ("the child actually dies when the parent
// process itself is killed") cannot be exercised from inside this same
// process (that would require killing the test binary itself) - it
// was verified manually against the real packaged application
// instead; see docs/progress.md's own entry for that evidence.
func TestAssignToChildJobSucceeds(t *testing.T) {
	cmd := exec.Command("cmd.exe", "/c", "timeout", "/t", "5", "/nobreak")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start test child process: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})

	if err := AssignToChildJob(cmd); err != nil {
		t.Fatalf("AssignToChildJob() error = %v", err)
	}
}
