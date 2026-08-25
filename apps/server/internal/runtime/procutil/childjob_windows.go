//go:build windows

package procutil

import (
	"fmt"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"
)

var (
	modKernel32                   = syscall.NewLazyDLL("kernel32.dll")
	procCreateJobObjectW          = modKernel32.NewProc("CreateJobObjectW")
	procSetInformationJobObject   = modKernel32.NewProc("SetInformationJobObject")
	procQueryInformationJobObject = modKernel32.NewProc("QueryInformationJobObject")
	procAssignProcessToJobObject  = modKernel32.NewProc("AssignProcessToJobObject")
	procOpenProcess               = modKernel32.NewProc("OpenProcess")
	procCloseHandle               = modKernel32.NewProc("CloseHandle")
)

const (
	jobObjectExtendedLimitInformation = 9
	jobObjectLimitKillOnJobClose      = 0x00002000

	processTerminate = 0x0001
	processSetQuota  = 0x0100
)

// jobobjectBasicLimitInformation mirrors JOBOBJECT_BASIC_LIMIT_INFORMATION
// exactly (field order/types, including the compiler-inserted padding
// an amd64 C compiler places before the first 8-byte-aligned SIZE_T
// field that follows the 4-byte LimitFlags - Go's own struct layout
// rules insert the identical padding automatically for the same
// sequence of types, so no explicit padding field is written here) -
// verified against learn.microsoft.com/windows/win32/api/winnt/
// ns-winnt-jobobject_basic_limit_information before being written.
type jobobjectBasicLimitInformation struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}

// ioCounters mirrors IO_COUNTERS (six ULONGLONG fields) - the
// JOBOBJECT_EXTENDED_LIMIT_INFORMATION.IoInfo field is documented as
// "Reserved", so this is never read, only sized and zeroed correctly.
type ioCounters struct {
	ReadOperationCount  uint64
	WriteOperationCount uint64
	OtherOperationCount uint64
	ReadTransferCount   uint64
	WriteTransferCount  uint64
	OtherTransferCount  uint64
}

// jobobjectExtendedLimitInformation mirrors
// JOBOBJECT_EXTENDED_LIMIT_INFORMATION exactly - verified against
// learn.microsoft.com/windows/win32/api/winnt/
// ns-winnt-jobobject_extended_limit_information before being written.
type jobobjectExtendedLimitInformation struct {
	BasicLimitInformation jobobjectBasicLimitInformation
	IoInfo                ioCounters
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

// childJob is created once, lazily, and deliberately never explicitly
// closed by this package's own code: the whole point is that Windows
// itself closes this handle (along with every other handle the server
// process holds) the moment that process exits, for any reason,
// cooperative or not - and JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE means
// that handle closing is exactly what terminates every process still
// assigned to the job at that moment. A graceful shutdown that has
// already stopped every child itself simply leaves an empty job to be
// closed; this is a safety net for the ungraceful case, not a
// replacement for graceful shutdown.
var (
	childJobOnce   sync.Once
	childJobHandle uintptr
	childJobErr    error
)

func getOrCreateChildJob() (uintptr, error) {
	childJobOnce.Do(func() {
		h, _, callErr := procCreateJobObjectW.Call(0, 0)
		if h == 0 {
			childJobErr = fmt.Errorf("CreateJobObjectW: %w", callErr)
			return
		}

		info := jobobjectExtendedLimitInformation{
			BasicLimitInformation: jobobjectBasicLimitInformation{
				LimitFlags: jobObjectLimitKillOnJobClose,
			},
		}
		ret, _, callErr := procSetInformationJobObject.Call(
			h, jobObjectExtendedLimitInformation,
			uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info),
		)
		if ret == 0 {
			procCloseHandle.Call(h)
			childJobErr = fmt.Errorf("SetInformationJobObject: %w", callErr)
			return
		}

		childJobHandle = h
	})
	return childJobHandle, childJobErr
}

func assignToChildJob(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return fmt.Errorf("procutil: AssignToChildJob called before the process started")
	}

	job, err := getOrCreateChildJob()
	if err != nil {
		return err
	}

	// AssignProcessToJobObject documents PROCESS_SET_QUOTA and
	// PROCESS_TERMINATE as the required access rights - os/exec does
	// not expose the handle it already holds internally, so a fresh
	// one is opened by PID specifically for this call and closed
	// immediately after.
	hProcess, _, callErr := procOpenProcess.Call(
		uintptr(processSetQuota|processTerminate), 0, uintptr(cmd.Process.Pid),
	)
	if hProcess == 0 {
		return fmt.Errorf("procutil: OpenProcess for job assignment: %w", callErr)
	}
	defer procCloseHandle.Call(hProcess)

	ret, _, callErr := procAssignProcessToJobObject.Call(job, hProcess)
	if ret == 0 {
		return fmt.Errorf("procutil: AssignProcessToJobObject: %w", callErr)
	}
	return nil
}
