package healthband

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// Git Execution Policy of SPEC-SIGMABAND-002 (REQ-05, REQ-10, REQ-12). Every
// git command of the local patch flow, the Patch Policy, the Cleanup Rules,
// and recovery runs through GitPolicyRunner: it admits only the allowlisted
// command forms (CheckGitCommand), starts git from a scrubbed environment
// with the policy flags in front of the arguments, and stops a command that
// outlives its step group. SPEC-SIGMABAND-001's checkBandCommand stays
// unchanged, so a run with the flag off never reaches this runner.

const (
	// BandGitName and BandGitEmail are the user, author, and committer
	// identity of every band git command (item 2), so no band commit carries
	// the user's identity even when author.* or committer.* is configured.
	BandGitName  = "autopus-band"
	BandGitEmail = "band@autopus.invalid"
	// GitStopGrace is how long before its context deadline a running command
	// gets SIGTERM on its process group; SIGKILL follows at the deadline.
	GitStopGrace = 5 * time.Second
	// GitPolicyOutputBytes bounds the stdout that Run and RunInput keep.
	GitPolicyOutputBytes = 64 << 20
	// gitPolicyWaitDelay bounds how long Wait lingers on pipes after git ends.
	gitPolicyWaitDelay = time.Second
)

var (
	// ErrGitCommandNotAllowed refuses a command outside the allowlist.
	ErrGitCommandNotAllowed = errors.New("healthband: git command outside the Git Execution Policy allowlist")
	// ErrGitStopped marks a command that the runner signalled at or near the
	// deadline of its context.
	ErrGitStopped = errors.New("healthband: git command stopped at its step-group deadline")
	// ErrGitOutputTooLarge refuses a stdout above the runner's bound.
	ErrGitOutputTooLarge = errors.New("healthband: git output exceeds its bound")
)

// gitPolicyEnvSet are the variables item 1 sets on every command, after
// every inherited GIT_* variable is removed.
var gitPolicyEnvSet = []string{
	"GIT_TERMINAL_PROMPT=0", "GIT_EDITOR=:", "GIT_PAGER=cat", "GIT_ATTR_NOSYSTEM=1",
	"GIT_LFS_SKIP_SMUDGE=1", "GIT_NO_LAZY_FETCH=1", "GIT_NO_REPLACE_OBJECTS=1",
}

// gitPolicyConfig are the -c values of item 2 in argv order. A later -c
// value wins over every configuration file, and the empty git-lfs values
// start no filter, so band never runs git-lfs (probe sb2-rev10).
var gitPolicyConfig = []string{
	"core.hooksPath=/dev/null",
	"core.attributesFile=/dev/null",
	"core.fsmonitor=false",
	"core.untrackedCache=false",
	"core.symlinks=false",
	"core.ignoreStat=false",
	"core.sparseCheckout=false",
	"core.useReplaceRefs=false",
	"commit.gpgSign=false",
	"user.name=" + BandGitName,
	"user.email=" + BandGitEmail,
	"author.name=" + BandGitName,
	"author.email=" + BandGitEmail,
	"committer.name=" + BandGitName,
	"committer.email=" + BandGitEmail,
	"gc.auto=0",
	"maintenance.auto=false",
	"filter.lfs.process=",
	"filter.lfs.clean=",
	"filter.lfs.smudge=",
	"filter.lfs.required=false",
}

// GitPolicyFlags returns the -c flags that precede every band git command.
func GitPolicyFlags() []string {
	flags := make([]string, 0, 2*len(gitPolicyConfig))
	for _, value := range gitPolicyConfig {
		flags = append(flags, "-c", value)
	}
	return flags
}

// GitPolicyArgv returns the git argv of args: the policy flags, then args.
func GitPolicyArgv(args ...string) []string {
	return append(GitPolicyFlags(), args...)
}

// GitPolicyEnv returns base without any GIT_* variable, plus the policy
// variables, plus GIT_INDEX_FILE when indexFile is set (only the temp-index
// commands of Local Patch Flow step 8 get one).
func GitPolicyEnv(base []string, indexFile string) []string {
	env := append(orchestra.EnvironWithout(base, []string{"GIT_*"}), gitPolicyEnvSet...)
	if indexFile != "" {
		env = append(env, "GIT_INDEX_FILE="+indexFile)
	}
	return env
}

// GitCommandError is a band git command that failed. Command is its
// subcommand; ExitCode is -1 when git did not exit on its own, and Stopped
// reports that the runner signalled it (errors.Is(err, ErrGitStopped)).
type GitCommandError struct {
	Command  string
	ExitCode int
	Stopped  bool
	Err      error
}

func (e *GitCommandError) Error() string {
	if e.Stopped {
		return fmt.Sprintf("git %s: stopped at its deadline: %v", e.Command, e.Err)
	}
	return fmt.Sprintf("git %s: %v", e.Command, e.Err)
}

func (e *GitCommandError) Unwrap() []error {
	if e.Stopped {
		return []error{e.Err, ErrGitStopped}
	}
	return []error{e.Err}
}

// GitExitCode returns the exit code of a failed band git command, or -1
// when err is not a GitCommandError or git did not exit on its own.
func GitExitCode(err error) int {
	var failed *GitCommandError
	if errors.As(err, &failed) {
		return failed.ExitCode
	}
	return -1
}

// GitPolicyRunner runs allowlisted git commands under the policy. Its zero
// value runs git from PATH in the current directory with os.Environ; In and
// WithIndexFile return adjusted copies.
type GitPolicyRunner struct {
	Binary    string          // git executable; "" is git from PATH
	Dir       string          // working directory of every command
	Environ   func() []string // base environment; nil is os.Environ
	IndexFile string          // GIT_INDEX_FILE of the temp-index commands
	MaxOutput int             // stdout bound of Run and RunInput; 0 is GitPolicyOutputBytes
	StopGrace time.Duration   // SIGTERM lead before the deadline; 0 is GitStopGrace
}

// In returns a copy of r that runs in dir.
func (r GitPolicyRunner) In(dir string) GitPolicyRunner { r.Dir = dir; return r }

// WithIndexFile returns a copy of r whose commands get GIT_INDEX_FILE=path.
func (r GitPolicyRunner) WithIndexFile(path string) GitPolicyRunner { r.IndexFile = path; return r }

// Run executes one allowlisted command and returns its stdout.
func (r GitPolicyRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	return r.RunInput(ctx, nil, args...)
}

// RunInput is Run with stdin as the command's standard input, such as the
// diff of git apply, which stays in memory until the Patch Policy accepts it.
func (r GitPolicyRunner) RunInput(ctx context.Context, stdin []byte, args ...string) ([]byte, error) {
	limit := r.MaxOutput
	if limit <= 0 {
		limit = GitPolicyOutputBytes
	}
	out := &gitPolicyBuffer{limit: limit}
	var input io.Reader
	if stdin != nil {
		input = bytes.NewReader(stdin)
	}
	err := r.run(ctx, input, out, args)
	if out.over {
		return nil, fmt.Errorf("%w: git %s above %d bytes", ErrGitOutputTooLarge, args[0], limit)
	}
	if err != nil {
		return nil, err
	}
	return out.buf.Bytes(), nil
}

// RunTo executes one allowlisted command with stdout streamed to w, such as
// a hash of git status and git diff; the writer sets its own bound.
func (r GitPolicyRunner) RunTo(ctx context.Context, w io.Writer, args ...string) error {
	return r.run(ctx, nil, w, args)
}

// run starts git directly, never through a shell, with stderr on the null
// device (its text is untrusted and never read). The command runs in its own
// process group: SIGTERM reaches the group StopGrace before the context
// deadline and SIGKILL when the context ends, so no git child outlives it.
func (r GitPolicyRunner) run(ctx context.Context, stdin io.Reader, stdout io.Writer, args []string) error {
	if err := CheckGitCommand(args, r.IndexFile); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return &GitCommandError{Command: args[0], ExitCode: -1, Stopped: true, Err: err}
	}
	binary, environ, grace := r.Binary, r.Environ, r.StopGrace
	if binary == "" {
		binary = "git"
	}
	if environ == nil {
		environ = os.Environ
	}
	if grace <= 0 {
		grace = GitStopGrace
	}
	cmd := exec.CommandContext(ctx, binary, GitPolicyArgv(args...)...) //nolint:gosec // allowlisted argv, no shell
	cmd.Dir, cmd.Env = r.Dir, GitPolicyEnv(environ(), r.IndexFile)
	cmd.Stdin, cmd.Stdout = stdin, stdout
	cmd.WaitDelay = gitPolicyWaitDelay
	stop := &gitPolicyStop{cmd: cmd}
	gitPolicyNewGroup(cmd)
	cmd.Cancel = func() error { return stop.signal(true) }
	if err := cmd.Start(); err != nil {
		return &GitCommandError{Command: args[0], ExitCode: -1, Err: err}
	}
	stop.arm(ctx, grace)
	err := cmd.Wait()
	signalled := stop.disarm()
	if err == nil {
		return nil
	}
	exitCode := -1
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		exitCode = exitErr.ExitCode()
	}
	stopped := signalled || ctx.Err() != nil
	return &GitCommandError{Command: args[0], ExitCode: exitCode, Stopped: stopped, Err: err}
}

// gitPolicyStop signals one command's process group: SIGTERM from the timer
// that arm sets, SIGKILL from exec's Cancel. Nothing is sent once Wait has
// returned.
type gitPolicyStop struct {
	cmd       *exec.Cmd
	mu        sync.Mutex
	done      bool
	signalled bool
	timer     *time.Timer
}

func (s *gitPolicyStop) arm(ctx context.Context, grace time.Duration) {
	deadline, ok := ctx.Deadline()
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.timer = time.AfterFunc(time.Until(deadline.Add(-grace)), func() { _ = s.signal(false) })
}

func (s *gitPolicyStop) signal(kill bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done || s.cmd.Process == nil {
		return os.ErrProcessDone
	}
	err := gitPolicySignalGroup(s.cmd.Process, kill)
	if err == nil {
		s.signalled = true
	}
	return err
}

func (s *gitPolicyStop) disarm() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.done = true
	if s.timer != nil {
		s.timer.Stop()
	}
	return s.signalled
}

// gitPolicyBuffer keeps at most limit bytes; a write past the bound fails,
// which closes the pipe, so git stops instead of filling memory.
type gitPolicyBuffer struct {
	buf   bytes.Buffer
	limit int
	over  bool
}

func (b *gitPolicyBuffer) Write(p []byte) (int, error) {
	if b.buf.Len()+len(p) > b.limit {
		b.over = true
		return 0, ErrGitOutputTooLarge
	}
	return b.buf.Write(p)
}
