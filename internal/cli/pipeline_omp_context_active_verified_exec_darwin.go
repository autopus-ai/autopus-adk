//go:build darwin

package cli

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	pipelineOMPVerifiedDarwinProcInfoCallPIDInfo = 2
	pipelineOMPVerifiedDarwinRegionPathInfo      = 8
	pipelineOMPVerifiedDarwinMaxPath             = 1024
	pipelineOMPVerifiedDarwinMaxRegions          = 256
	pipelineOMPVerifiedDarwinGateTimeout         = 5 * time.Second
	pipelineOMPVerifiedDarwinVMProtectionExecute = 0x4
)

type pipelineOMPVerifiedDarwinRegion struct {
	Protection, MaxProtection, Inheritance, Flags uint32
	Offset                                        uint64
	Behavior, UserWiredCount, UserTag             uint32
	PagesResident, PagesSharedNowPrivate          uint32
	PagesSwappedOut, PagesDirtied, RefCount       uint32
	ShadowDepth, ShareMode, PrivatePagesResident  uint32
	SharedPagesResident, ObjectID, Depth          uint32
	Address, Size                                 uint64
}

type pipelineOMPVerifiedDarwinVnode struct {
	Dev                                                                        uint32
	Mode, Nlink                                                                uint16
	Ino                                                                        uint64
	UID, GID                                                                   uint32
	Atime, AtimeNsec, Mtime, MtimeNsec, Ctime, CtimeNsec, Birthtime, BirthNsec int64
	Size, Blocks                                                               int64
	BlockSize                                                                  int32
	Flags, Generation                                                          uint32
	Rdev                                                                       uint32
	Spare                                                                      [2]int64
}

type pipelineOMPVerifiedDarwinRegionPath struct {
	Region pipelineOMPVerifiedDarwinRegion
	Vnode  struct {
		Stat      pipelineOMPVerifiedDarwinVnode
		Type, Pad int32
		FSID      [2]int32
		Path      [pipelineOMPVerifiedDarwinMaxPath]byte
	}
}

func newPipelineOMPVerifiedExecCommandWithGate(
	ctx context.Context,
	path string,
	expected pipelineOMPExecutableIdentity,
	args ...string,
) (*pipelineOMPVerifiedExecCommand, error) {
	return newPipelineOMPVerifiedDarwinCommand(ctx, false, path, expected, args...)
}

func newPipelineOMPVerifiedExecCommandContext(
	ctx context.Context,
	path string,
	expected pipelineOMPExecutableIdentity,
	args ...string,
) (*pipelineOMPVerifiedExecCommand, error) {
	return newPipelineOMPVerifiedDarwinCommand(ctx, true, path, expected, args...)
}

func newPipelineOMPVerifiedDarwinCommand(
	ctx context.Context,
	boundContext bool,
	path string,
	expected pipelineOMPExecutableIdentity,
	args ...string,
) (*pipelineOMPVerifiedExecCommand, error) {
	file, err := openPipelineOMPVerifiedExecutable(path, expected)
	if err != nil {
		return nil, err
	}
	var cmd *exec.Cmd
	if boundContext {
		cmd = exec.CommandContext(ctx, path, args...)
	} else {
		cmd = exec.Command(path, args...)
	}
	return &pipelineOMPVerifiedExecCommand{
		cmd: cmd, parentFD: file, expected: expected, gateContext: ctx, traceUntilExit: boundContext,
	}, nil
}

// @AX:WARN [AUTO]: Darwin verified execution startup contains 15 if branches.
// @AX:REASON [AUTO]: sandbox ptrace startup coordinates executable identity, two exec stops, timeout, cleanup, continuation, and detach failures.
func (command *pipelineOMPVerifiedExecCommand) Start() error {
	if command == nil || command.cmd == nil || command.parentFD == nil || command.expected.info == nil {
		return errors.New("verified managed active OMP command is unavailable")
	}
	if command.inheritedDarwinPath {
		return command.startDarwinInheritedPath()
	}
	sandboxPath, sandboxIdentity, err := canonicalPipelineOMPExecutable(command.cmd.Path)
	if err != nil || sandboxPath != "/usr/bin/sandbox-exec" {
		return errors.Join(errors.New("managed active Darwin sandbox executable is unavailable"), command.Close())
	}
	if command.cmd.SysProcAttr == nil {
		command.cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	command.cmd.SysProcAttr.Ptrace = true
	startErr := command.cmd.Start()
	closeErr := command.Close()
	if startErr != nil {
		return errors.Join(startErr, closeErr)
	}
	if closeErr != nil {
		return command.failDarwinGate(errors.New("close verified managed active OMP parent FD"), closeErr)
	}
	gateCtx := command.gateContext
	if gateCtx == nil {
		gateCtx = context.Background()
	}
	gateCtx, cancel := context.WithTimeout(gateCtx, pipelineOMPVerifiedDarwinGateTimeout)
	defer cancel()
	if err := command.waitDarwinExecStop(gateCtx); err != nil {
		return command.failDarwinGate(errors.New("observe Darwin executable exec stop"), err)
	}
	if err := verifyPipelineOMPDarwinLiveExecutable(command.cmd.Process.Pid, sandboxPath, sandboxIdentity); err != nil {
		return command.failDarwinGate(errors.New("verify Darwin executable live image"), err)
	}
	if command.afterFirstDarwinStop != nil {
		command.afterFirstDarwinStop()
	}
	if gateCtx.Err() != nil {
		return command.failDarwinGate(errors.New("Darwin executable identity gate canceled"), gateCtx.Err())
	}
	if err := continuePipelineOMPDarwinTrace(command.cmd.Process.Pid); err != nil {
		return command.failDarwinGate(errors.New("continue Darwin sandbox trace"), err)
	}
	if err := command.waitDarwinExecStop(gateCtx); err != nil {
		return command.failDarwinGate(errors.New("observe managed active OMP exec stop"), err)
	}
	if err := verifyPipelineOMPDarwinLiveExecutable(
		command.cmd.Process.Pid, command.parentTargetPath(), command.expected,
	); err != nil {
		return command.failDarwinGate(errors.New("verify managed active OMP live image"), err)
	}
	if command.traceUntilExit {
		// A bounded probe exits on its own, so it stays traced: no detach, which
		// newer kernels can leave stuck (see releasePipelineOMPDarwinTrace).
		if err := continuePipelineOMPDarwinTrace(command.cmd.Process.Pid); err != nil {
			return command.failDarwinGate(errors.New("continue verified managed active OMP trace"), err)
		}
		return nil
	}
	if err := releasePipelineOMPDarwinTrace(gateCtx, command.cmd.Process.Pid); err != nil {
		return command.failDarwinGate(errors.New("detach verified managed active OMP trace"), err)
	}
	return nil
}

func (command *pipelineOMPVerifiedExecCommand) startDarwinInheritedPath() error {
	if len(command.cmd.ExtraFiles) != 0 || command.cmd.Path != command.parentFD.Name() ||
		(command.cmd.SysProcAttr != nil && command.cmd.SysProcAttr.Ptrace) {
		return errors.Join(errors.New("inherited parent sandbox verified OMP launch changed"), command.Close())
	}
	if err := verifyPipelineOMPExecutable(command.cmd.Path, command.expected); err != nil {
		return errors.Join(err, command.Close())
	}
	startErr := command.cmd.Start()
	if startErr != nil {
		return errors.Join(startErr, command.Close())
	}
	if command.afterInheritedDarwinStart != nil {
		command.afterInheritedDarwinStart()
	}
	postErr := verifyPipelineOMPExecutable(command.cmd.Path, command.expected)
	closeErr := command.Close()
	if postErr == nil && closeErr == nil {
		return nil
	}
	return errors.Join(errors.New("inherited parent sandbox verified OMP launch failed"), postErr, closeErr,
		terminateWorkflowContextManagedRPCProcessGroup(command.cmd), command.cmd.Wait())
}

func (command *pipelineOMPVerifiedExecCommand) parentTargetPath() string {
	for index, arg := range command.cmd.Args {
		if arg == "--" || arg == "-p" {
			continue
		}
		if index > 0 && command.cmd.Args[index-1] != "-p" && filepath.IsAbs(arg) && arg != command.cmd.Path {
			return filepath.Clean(arg)
		}
	}
	return ""
}

func (command *pipelineOMPVerifiedExecCommand) waitDarwinExecStop(ctx context.Context) error {
	pid := command.cmd.Process.Pid
	stopKill := context.AfterFunc(ctx, func() { _ = killPipelineOMPDarwinTrace(command.cmd) })
	defer stopKill()
	for {
		var status syscall.WaitStatus
		observed, err := syscall.Wait4(pid, &status, syscall.WUNTRACED, nil)
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		if err != nil || observed != pid || !status.Stopped() || status.StopSignal() != syscall.SIGTRAP {
			return errors.Join(errors.New("unexpected Darwin ptrace stop"), err, ctx.Err())
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return nil
	}
}

func (command *pipelineOMPVerifiedExecCommand) failDarwinGate(errs ...error) error {
	killErr := killPipelineOMPDarwinTrace(command.cmd)
	waitErr := command.cmd.Wait()
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) || errors.Is(waitErr, syscall.ECHILD) {
		waitErr = nil
	}
	return errors.Join(append(errs, killErr, waitErr)...)
}

func killPipelineOMPDarwinTrace(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil || cmd.Process.Pid <= 0 {
		return nil
	}
	_, _, ptraceErr := syscall.Syscall6(
		syscall.SYS_PTRACE, syscall.PT_KILL, uintptr(cmd.Process.Pid), 1, 0, 0, 0,
	)
	killErr := terminateWorkflowContextManagedRPCProcessGroup(cmd)
	var ptraceFailure error
	if ptraceErr != 0 && ptraceErr != syscall.ESRCH {
		ptraceFailure = ptraceErr
	}
	if errors.Is(killErr, syscall.ESRCH) || (ptraceErr == 0 && errors.Is(killErr, syscall.EPERM)) {
		killErr = nil
	}
	return errors.Join(ptraceFailure, killErr)
}

func continuePipelineOMPDarwinTrace(pid int) error {
	return continuePipelineOMPDarwinTraceWithSignal(pid, 0)
}

func verifyPipelineOMPDarwinLiveExecutable(
	pid int,
	expectedPath string,
	expected pipelineOMPExecutableIdentity,
) error {
	live, path, err := pipelineOMPDarwinLiveExecutableRegion(pid)
	if err != nil || filepath.Clean(path) != filepath.Clean(expectedPath) {
		return errors.New("Darwin live executable path mismatch")
	}
	stat, ok := expected.info.Sys().(*syscall.Stat_t)
	if !ok || live.Dev != uint32(stat.Dev) || live.Ino != stat.Ino || live.Size != expected.size ||
		uint32(live.Mode) != uint32(stat.Mode) || live.UID != stat.Uid || live.GID != stat.Gid {
		return errors.New("Darwin live executable vnode mismatch")
	}
	file, err := openPipelineOMPVerifiedExecutable(path, expected)
	if err != nil {
		return errors.New("Darwin live executable digest mismatch")
	}
	return file.Close()
}

func pipelineOMPDarwinLiveExecutableRegion(pid int) (
	pipelineOMPVerifiedDarwinVnode,
	string,
	error,
) {
	var address uint64
	for range pipelineOMPVerifiedDarwinMaxRegions {
		var info pipelineOMPVerifiedDarwinRegionPath
		//nolint:staticcheck // x/sys/unix has no libproc wrapper; live-image identity requires proc_pidinfo.
		written, _, errno := unix.Syscall6(
			unix.SYS_PROC_INFO, pipelineOMPVerifiedDarwinProcInfoCallPIDInfo, uintptr(pid),
			pipelineOMPVerifiedDarwinRegionPathInfo, uintptr(address), uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info),
		)
		if errno != 0 || written != unsafe.Sizeof(info) {
			break
		}
		path := unix.ByteSliceToString(info.Vnode.Path[:])
		if info.Region.Protection&pipelineOMPVerifiedDarwinVMProtectionExecute != 0 && filepath.IsAbs(path) {
			return info.Vnode.Stat, path, nil
		}
		next := info.Region.Address + info.Region.Size
		if info.Region.Size == 0 || next <= address {
			break
		}
		address = next
	}
	return pipelineOMPVerifiedDarwinVnode{}, "", errors.New("Darwin live executable region is unavailable")
}
