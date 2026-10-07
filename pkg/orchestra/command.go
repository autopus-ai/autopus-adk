package orchestra

import (
	"context"
	"io"
	"os/exec"
	"time"
)

// command는 실행 커맨드 인터페이스이다 (테스트 목 지원).
type command interface {
	StdinPipe() (io.WriteCloser, error)
	SetStdin(r io.Reader)
	SetStdout(w io.Writer)
	SetStderr(w io.Writer)
	SetDir(dir string)
	Start() error
	Wait() error
	ExitCode() int
	PID() int
	Terminate(reason string) error
}

// execCommand는 실제 exec.Cmd 래퍼이다.
type execCommand struct {
	cmd *exec.Cmd
}

// newCommand는 컨텍스트 기반 커맨드를 생성한다.
var newCommand = func(ctx context.Context, name string, args ...string) command {
	cmd := exec.CommandContext(ctx, name, args...)
	configureCommand(cmd)
	return &execCommand{cmd: cmd}
}

var providerWaitGracePeriod = 2 * time.Second

func (e *execCommand) StdinPipe() (io.WriteCloser, error) {
	return e.cmd.StdinPipe()
}

func (e *execCommand) SetStdin(r io.Reader) {
	e.cmd.Stdin = r
}

func (e *execCommand) SetStdout(w io.Writer) {
	e.cmd.Stdout = w
}

func (e *execCommand) SetStderr(w io.Writer) {
	e.cmd.Stderr = w
}

// SetEnv replaces the inherited environment of the process.
func (e *execCommand) SetEnv(env []string) {
	e.cmd.Env = env
}

// SetDir pins the process working directory; an empty dir inherits the caller's.
func (e *execCommand) SetDir(dir string) {
	e.cmd.Dir = dir
}

func (e *execCommand) Start() error {
	return e.cmd.Start()
}

func (e *execCommand) Wait() error {
	return e.cmd.Wait()
}

func (e *execCommand) ExitCode() int {
	if e.cmd.ProcessState == nil {
		return -1
	}
	return e.cmd.ProcessState.ExitCode()
}

// PID reports the started process ID, or 0 before Start succeeds.
func (e *execCommand) PID() int {
	if e.cmd.Process == nil {
		return 0
	}
	return e.cmd.Process.Pid
}

func (e *execCommand) Terminate(reason string) error {
	return terminateCommand(e.cmd, reason)
}
