//go:build windows

package procgroup

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// terminationCode is the exit code TerminateJobObject gives every process.
const terminationCode = 1

// platformState holds the Job Object the command runs in.
type platformState struct {
	job windows.Handle
}

// prepare creates the process suspended, so it cannot start a child before
// it is inside its job: a child started earlier would escape the job.
func (g *Group) prepare() {
	if g.cmd.SysProcAttr == nil {
		g.cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	g.cmd.SysProcAttr.CreationFlags |= windows.CREATE_SUSPENDED
}

// start starts the process suspended, puts it in a new job and resumes it.
// When the job cannot be made or joined (a parent job that forbids nesting,
// for one) it runs without one: Kill then reaches the direct child only, as
// exec's default cancel did. That is a weaker cleanup, not a reason the
// command cannot run.
func (g *Group) start() error {
	if err := g.cmd.Start(); err != nil {
		return err
	}
	job, err := newJob()
	if err == nil {
		if err = assign(job, g.cmd); err != nil {
			_ = windows.CloseHandle(job)
		}
	}
	if err == nil {
		g.state.job = job
	}
	if err := resumeProcess(g.cmd.Process.Pid); err != nil {
		// A process that cannot be resumed never runs: take it down.
		return g.abort(err)
	}
	return nil
}

// abort takes down a process that started but could not be resumed. Start
// reaps it once the lock is released (see abortedStart).
func (g *Group) abort(cause error) error {
	var killErr error
	if g.state.job != 0 {
		killErr = windows.TerminateJobObject(g.state.job, terminationCode)
	} else {
		killErr = g.cmd.Process.Kill()
	}
	return &abortedStart{err: errors.Join(cause, killErr)}
}

// kill terminates every process in the job, however it was started. There
// is no SIGTERM to give a grace to.
func (g *Group) kill(time.Duration) error {
	if g.state.job == 0 {
		return g.cmd.Process.Kill()
	}
	if err := windows.TerminateJobObject(g.state.job, terminationCode); err != nil {
		return fmt.Errorf("terminate job object: %w", err)
	}
	return nil
}

// terminate has no graceful signal to send on Windows.
func (g *Group) terminate() {}

// close closes the job; KILL_ON_JOB_CLOSE kills whatever is still in it.
func (g *Group) close() error {
	if g.state.job == 0 {
		return nil
	}
	err := windows.CloseHandle(g.state.job)
	g.state.job = 0
	if err != nil {
		return fmt.Errorf("close job object: %w", err)
	}
	return nil
}

// newJob creates a Job Object that kills its processes when it is closed.
func newJob() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, fmt.Errorf("create job object: %w", err)
	}
	if err := setKillOnClose(job); err != nil {
		_ = windows.CloseHandle(job)
		return 0, err
	}
	return job, nil
}

func setKillOnClose(job windows.Handle) error {
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	_, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))
	if err != nil {
		return fmt.Errorf("configure job object: %w", err)
	}
	return nil
}

func assign(job windows.Handle, cmd *exec.Cmd) error {
	var assignErr error
	err := cmd.Process.WithHandle(func(handle uintptr) {
		assignErr = windows.AssignProcessToJobObject(job, windows.Handle(handle))
	})
	if err != nil {
		return fmt.Errorf("acquire process handle: %w", err)
	}
	if assignErr != nil {
		return fmt.Errorf("assign process to job object: %w", assignErr)
	}
	return nil
}

// resumeProcess resumes the primary thread of a process created suspended:
// the thread of pid whose suspend count was not zero.
func resumeProcess(pid int) error {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return fmt.Errorf("snapshot threads: %w", err)
	}
	defer func() { _ = windows.CloseHandle(snap) }()
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	for err = windows.Thread32First(snap, &entry); err == nil; err = windows.Thread32Next(snap, &entry) {
		if entry.OwnerProcessID != uint32(pid) {
			continue
		}
		resumed, err := resumeThread(entry.ThreadID)
		if err != nil {
			return err
		}
		if resumed {
			return nil
		}
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return fmt.Errorf("enumerate threads: %w", err)
	}
	return fmt.Errorf("resume process %d: no suspended thread found", pid)
}

// resumeThread resumes one thread and reports whether it had been suspended;
// a thread that was not (previous count 0) is not the one to resume.
func resumeThread(id uint32) (bool, error) {
	thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, id)
	if err != nil {
		return false, fmt.Errorf("open thread: %w", err)
	}
	defer func() { _ = windows.CloseHandle(thread) }()
	prev, err := windows.ResumeThread(thread)
	if err != nil {
		return false, fmt.Errorf("resume thread: %w", err)
	}
	return prev > 0, nil
}
