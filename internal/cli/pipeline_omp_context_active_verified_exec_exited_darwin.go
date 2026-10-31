//go:build darwin

package cli

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	pipelineOMPDarwinWaitIDProcess = 1 // P_PID
	pipelineOMPDarwinCodeExited    = 1 // CLD_EXITED
	pipelineOMPDarwinCodeDumped    = 3 // CLD_DUMPED
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
