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
	pipelineOMPVerifiedDarwinLivePoll            = time.Millisecond
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
		cmd: cmd, parentFD: file, expected: expected, gateContext: ctx,
	}, nil
}

// @AX:WARN [AUTO]: Darwin verified execution startup contains many fail-closed branches.
// @AX:REASON [AUTO]: sandboxed startup coordinates executable identity before launch, live-image identity after launch, timeout, and cleanup.
//
// Start launches the sandboxed child untraced and verifies the image the kernel
// actually mapped. The gate used to stop the child at each exec with ptrace,
// but macOS 15.8.2 and 26.6.2 broke every way of letting a traced child run:
// PT_DETACH from the exec stop redelivered SIGTRAP and killed it (v0.50.129
// release run 41035414211), detaching from a later SIGCONT stop left about one
// child in two hundred asleep for good (v0.50.130 release run 41148530560),
// and staying the tracer froze multi-threaded children that take signals.
//
// The target file is verified before launch; once sandbox-exec has exec'd it,
// the live executable region's vnode and digest are verified, then the file
// again. Any mismatch kills the process group. A target swapped between the
// checks can run inside the sandbox until the live check kills it; writing it
// needs the same uid and the private 0700 runtime root.
func (command *pipelineOMPVerifiedExecCommand) Start() error {
	if command == nil || command.cmd == nil || command.parentFD == nil || command.expected.info == nil {
		return errors.New("verified managed active OMP command is unavailable")
	}
	if command.inheritedDarwinPath {
		return command.startDarwinInheritedPath()
	}
	sandboxPath, _, err := canonicalPipelineOMPExecutable(command.cmd.Path)
	target := command.parentTargetPath()
	if err != nil || sandboxPath != "/usr/bin/sandbox-exec" || target == "" ||
		(command.cmd.SysProcAttr != nil && command.cmd.SysProcAttr.Ptrace) {
		return errors.Join(errors.New("managed active Darwin sandbox executable is unavailable"), command.Close())
	}
	if err := verifyPipelineOMPExecutable(target, command.expected); err != nil {
		return errors.Join(err, command.Close())
	}
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
	if command.afterFirstDarwinStop != nil {
		command.afterFirstDarwinStop()
	}
	if gateCtx.Err() != nil {
		return command.failDarwinGate(errors.New("Darwin executable identity gate canceled"), gateCtx.Err())
	}
	if err := command.awaitDarwinLiveTarget(gateCtx, target); err != nil {
		return command.failDarwinGate(errors.New("verify managed active OMP live image"), err)
	}
	if err := verifyPipelineOMPExecutable(target, command.expected); err != nil {
		return command.failDarwinGate(errors.New("managed active OMP identity changed after launch"), err)
	}
	return nil
}

// awaitDarwinLiveTarget polls until sandbox-exec has exec'd the target and then
// verifies the mapped image. A child that already exited cannot be inspected;
// its file identity is still checked after launch by the caller.
func (command *pipelineOMPVerifiedExecCommand) awaitDarwinLiveTarget(ctx context.Context, target string) error {
	pid := command.cmd.Process.Pid
	for {
		_, path, err := pipelineOMPDarwinLiveExecutableRegion(pid)
		if err == nil && filepath.Clean(path) == filepath.Clean(target) {
			return verifyPipelineOMPDarwinLiveExecutable(pid, target, command.expected)
		}
		if err == nil && filepath.Clean(path) != "/usr/bin/sandbox-exec" {
			return errors.New("Darwin live executable is neither the sandbox nor the target")
		}
		if err != nil && pipelineOMPDarwinChildExited(pid) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pipelineOMPVerifiedDarwinLivePoll):
		}
	}
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

func (command *pipelineOMPVerifiedExecCommand) failDarwinGate(errs ...error) error {
	killErr := terminateWorkflowContextManagedRPCProcessGroup(command.cmd)
	waitErr := command.cmd.Wait()
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) || errors.Is(waitErr, syscall.ECHILD) {
		waitErr = nil
	}
	return errors.Join(append(errs, killErr, waitErr)...)
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
