//go:build darwin

package cli

import (
	"context"
	"errors"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	pipelineOMPDarwinWaitIDProcess   = 1 // P_PID
	pipelineOMPDarwinChildExited     = 1 // CLD_EXITED
	pipelineOMPDarwinChildDumped     = 3 // CLD_DUMPED
	pipelineOMPDarwinReleaseMaxStops = 16
)

// pipelineOMPDarwinSiginfo mirrors the Darwin siginfo_t prefix that waitid fills.
type pipelineOMPDarwinSiginfo struct {
	Signo, Errno, Code, PID int32
	UID                     uint32
	Status                  int32
	_                       [80]byte
}

// releasePipelineOMPDarwinTrace hands the verified child back to run untraced.
//
// macOS 15.8.2 and 26.6.2 redeliver the exec SIGTRAP when PT_DETACH is issued
// from the exec stop, so the untraced child died of trace/BPT trap (v0.50.129
// release run 41035414211, both attempts). PT_CONTINUE consumes that trap and a
// queued SIGCONT then forces a signal-delivery stop that carries nothing to
// redeliver; the detach is issued from there. An exit that wins the race is
// only peeked with WNOWAIT so exec.Cmd.Wait still reaps it.
func releasePipelineOMPDarwinTrace(ctx context.Context, pid int) error {
	stopKill := context.AfterFunc(ctx, func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
	defer stopKill()
	if err := continuePipelineOMPDarwinTraceWithSignal(pid, 0); err != nil {
		return errors.Join(errors.New("consume Darwin exec trap"), err)
	}
	if err := syscall.Kill(pid, syscall.SIGCONT); err != nil {
		return errors.Join(errors.New("queue Darwin release stop"), err)
	}
	for range pipelineOMPDarwinReleaseMaxStops {
		exited, err := peekPipelineOMPDarwinChild(pid)
		if err != nil || exited {
			return errors.Join(err, ctx.Err())
		}
		var status syscall.WaitStatus
		observed, err := syscall.Wait4(pid, &status, syscall.WUNTRACED, nil)
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		if err != nil || observed != pid || !status.Stopped() || ctx.Err() != nil {
			return errors.Join(errors.New("unexpected Darwin release stop"), err, ctx.Err())
		}
		if status.StopSignal() == syscall.SIGCONT {
			_, _, errno := syscall.Syscall6(syscall.SYS_PTRACE, syscall.PT_DETACH, uintptr(pid), 1, 0, 0, 0)
			if errno != 0 {
				return errno
			}
			return nil
		}
		// A signal that raced ahead of SIGCONT belongs to the child; pass it on.
		if err := continuePipelineOMPDarwinTraceWithSignal(pid, status.StopSignal()); err != nil {
			return errors.Join(errors.New("forward Darwin child signal"), err)
		}
	}
	return errors.New("Darwin release stop did not arrive")
}

// peekPipelineOMPDarwinChild waits for the next child state change without
// consuming it, and reports whether that change is an exit.
func peekPipelineOMPDarwinChild(pid int) (bool, error) {
	for {
		var info pipelineOMPDarwinSiginfo
		_, _, errno := unix.Syscall6(
			unix.SYS_WAITID, pipelineOMPDarwinWaitIDProcess, uintptr(pid), uintptr(unsafe.Pointer(&info)),
			unix.WEXITED|unix.WSTOPPED|unix.WNOWAIT, 0, 0,
		)
		if errno == syscall.EINTR {
			continue
		}
		if errno != 0 {
			return false, errno
		}
		if info.PID != int32(pid) {
			return false, errors.New("Darwin child peek returned another process")
		}
		return info.Code >= pipelineOMPDarwinChildExited && info.Code <= pipelineOMPDarwinChildDumped, nil
	}
}

func continuePipelineOMPDarwinTraceWithSignal(pid int, signal syscall.Signal) error {
	_, _, errno := syscall.Syscall6(syscall.SYS_PTRACE, syscall.PT_CONTINUE, uintptr(pid), 1, uintptr(signal), 0, 0)
	if errno != 0 {
		return errno
	}
	return nil
}
