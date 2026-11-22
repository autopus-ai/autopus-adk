//go:build windows

package procgroup

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"
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

func (g *Group) start() error {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return fmt.Errorf("create job object: %w", err)
	}
	if err := setKillOnClose(job); err != nil {
		_ = windows.CloseHandle(job)
		return err
	}
	if err := g.cmd.Start(); err != nil {
		_ = windows.CloseHandle(job)
		return err
	}
	g.state.job = job
	if err := assign(job, g.cmd); err != nil {
		return g.abort(err, false)
	}
	if err := resumeProcess(g.cmd.Process.Pid); err != nil {
		return g.abort(err, true)
	}
	return nil
}

// abort takes down a process that started but could not be put to work in
// its job. Start reaps it once the lock is released (see abortedStart).
func (g *Group) abort(cause error, assigned bool) error {
	var killErr error
	if assigned {
		killErr = windows.TerminateJobObject(g.state.job, terminationCode)
	} else {
		killErr = g.cmd.Process.Kill()
	}
	return &abortedStart{err: errors.Join(cause, killErr)}
}

// kill terminates every process in the job, however it was started.
func (g *Group) kill() error {
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

// resumeProcess resumes the primary thread of a process created suspended.
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
		thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
		if err != nil {
			return fmt.Errorf("open thread: %w", err)
		}
		defer func() { _ = windows.CloseHandle(thread) }()
		if _, err := windows.ResumeThread(thread); err != nil {
			return fmt.Errorf("resume thread: %w", err)
		}
		return nil
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return fmt.Errorf("enumerate threads: %w", err)
	}
	return fmt.Errorf("resume process %d: primary thread not found", pid)
}
