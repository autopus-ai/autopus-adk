//go:build darwin

package cli

import (
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	pipelineOMPDarwinWaitIDProcess = 1 // P_PID
	pipelineOMPDarwinCodeExited    = 1 // CLD_EXITED
	pipelineOMPDarwinCodeDumped    = 3 // CLD_DUMPED

	// pipelineOMPDarwinExitGrace bounds how long a child whose image is gone
	// may take to become waitable. The kernel unmaps the address space before
	// the child turns into a zombie, and under load that gap reached tens of
	// milliseconds.
	pipelineOMPDarwinExitGrace = 500 * time.Millisecond
)

// pipelineOMPDarwinSiginfo mirrors the Darwin siginfo_t prefix that waitid fills.
type pipelineOMPDarwinSiginfo struct {
	Signo, Errno, Code, PID int32
	UID                     uint32
	Status                  int32
	_                       [80]byte
}

// pipelineOMPDarwinChildExited reports, without reaping it, whether the child
// has already exited, so exec.Cmd.Wait still collects its status.
func pipelineOMPDarwinChildExited(pid int) bool {
	for {
		var info pipelineOMPDarwinSiginfo
		//nolint:staticcheck // x/sys/unix has no waitid wrapper; WNOWAIT keeps the status for exec.Cmd.Wait.
		_, _, errno := unix.Syscall6(
			unix.SYS_WAITID, pipelineOMPDarwinWaitIDProcess, uintptr(pid), uintptr(unsafe.Pointer(&info)),
			unix.WEXITED|unix.WNOHANG|unix.WNOWAIT, 0, 0,
		)
		if errno == syscall.EINTR {
			continue
		}
		return errno == 0 && info.PID == int32(pid) &&
			info.Code >= pipelineOMPDarwinCodeExited && info.Code <= pipelineOMPDarwinCodeDumped
	}
}

// pipelineOMPDarwinChildExitedWithin reports whether the child exits within
// grace. It lets a check that found the image already unmapped tell an
// exiting child from a live one before it fails closed; a live child still
// fails, only later.
func pipelineOMPDarwinChildExitedWithin(pid int, grace time.Duration) bool {
	deadline := time.Now().Add(grace)
	for {
		if pipelineOMPDarwinChildExited(pid) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(pipelineOMPVerifiedDarwinLivePoll)
	}
}
