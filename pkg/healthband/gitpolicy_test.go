//go:build unix

package healthband

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SPEC-SIGMABAND-002 Git Execution Policy items 1–2 and S6: every band git
// command gets the full -c flag list in front of its arguments.
func TestGitPolicyRunner_EveryCommandCarriesThePolicyFlags(t *testing.T) {
	t.Parallel()
	f := newGPFixture(t)
	args := filepath.Join(f.root, "args.txt")
	_, err := f.gpFake("FAKE_ARGS="+args).Run(gpContext(t), "status", "--porcelain", "-z", "--untracked-files=all", "--ignored")
	require.NoError(t, err)
	data, err := os.ReadFile(args)
	require.NoError(t, err)
	want := []string{
		"core.hooksPath=/dev/null", "core.attributesFile=/dev/null", "core.fsmonitor=false",
		"core.untrackedCache=false", "core.symlinks=false", "core.ignoreStat=false", "core.sparseCheckout=false",
		"core.useReplaceRefs=false", "commit.gpgSign=false", "user.name=autopus-band", "user.email=band@autopus.invalid",
		"author.name=autopus-band", "author.email=band@autopus.invalid", "committer.name=autopus-band",
		"committer.email=band@autopus.invalid", "gc.auto=0", "maintenance.auto=false", "filter.lfs.process=",
		"filter.lfs.clean=", "filter.lfs.smudge=", "filter.lfs.required=false",
	}
	var argv []string
	for _, value := range want {
		argv = append(argv, "-c", value)
	}
	argv = append(argv, "status", "--porcelain", "-z", "--untracked-files=all", "--ignored")
	assert.Equal(t, strings.Join(argv, "\n")+"\n", string(data))
	assert.Equal(t, argv[:len(argv)-5], GitPolicyFlags())
}

// S6: inherited GIT_* variables, the user's GIT_DIR, GIT_INDEX_FILE, and
// GIT_CONFIG_PARAMETERS included, reach no band git; the environment holds
// the seven policy variables, plus GIT_INDEX_FILE on the temp-index
// commands only, and keeps every other variable.
func TestGitPolicyRunner_EnvironmentHoldsOnlyThePolicyVariables(t *testing.T) {
	t.Parallel()
	f := newGPFixture(t)
	ctx := gpContext(t)
	envFile := filepath.Join(f.root, "env.txt")
	r := f.gpFake("FAKE_ENV="+envFile, "GIT_DIR=/user/.git", "GIT_INDEX_FILE=/user/.git/index",
		"GIT_CONFIG_PARAMETERS='core.fsmonitor'='x'", "git_work_tree=/user", "GIT_NO_LAZY_FETCH=0", "KEEP_ME=1")
	policy := []string{
		"GIT_ATTR_NOSYSTEM=1", "GIT_EDITOR=:", "GIT_LFS_SKIP_SMUDGE=1", "GIT_NO_LAZY_FETCH=1",
		"GIT_NO_REPLACE_OBJECTS=1", "GIT_PAGER=cat", "GIT_TERMINAL_PROMPT=0",
	}
	gitVars := func() []string {
		data, err := os.ReadFile(envFile)
		require.NoError(t, err)
		var vars []string
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			if strings.HasPrefix(strings.ToUpper(line), "GIT_") {
				vars = append(vars, line)
			}
		}
		slices.Sort(vars)
		assert.Contains(t, string(data), "\nKEEP_ME=1\n")
		return vars
	}
	_, err := r.Run(ctx, "write-tree")
	require.NoError(t, err)
	assert.Equal(t, policy, gitVars())

	index := filepath.Join(f.root, "band-tmp", "index")
	withIndex := []string{
		"GIT_ATTR_NOSYSTEM=1", "GIT_EDITOR=:", "GIT_INDEX_FILE=" + index, "GIT_LFS_SKIP_SMUDGE=1",
		"GIT_NO_LAZY_FETCH=1", "GIT_NO_REPLACE_OBJECTS=1", "GIT_PAGER=cat", "GIT_TERMINAL_PROMPT=0",
	}
	for _, args := range [][]string{{"read-tree", gpOID}, {"apply", "--cached"}, {"write-tree"}} {
		_, err = r.WithIndexFile(index).Run(ctx, args...)
		require.NoError(t, err)
		assert.Equal(t, withIndex, gitVars(), "%v", args)
	}
	assert.Equal(t, []string{"A=1", "GIT_TERMINAL_PROMPT=0", "GIT_EDITOR=:", "GIT_PAGER=cat", "GIT_ATTR_NOSYSTEM=1",
		"GIT_LFS_SKIP_SMUDGE=1", "GIT_NO_LAZY_FETCH=1", "GIT_NO_REPLACE_OBJECTS=1"},
		GitPolicyEnv([]string{"A=1", "GIT_X=2", "Git_Y=3"}, ""))
}

// Git Execution Policy item 6: a command outside the allowlist never starts.
func TestGitPolicyRunner_RefusedCommandNeverStarts(t *testing.T) {
	t.Parallel()
	f := newGPFixture(t)
	args := filepath.Join(f.root, "args.txt")
	r := f.gpFake("FAKE_ARGS=" + args)
	for _, refused := range [][]string{{"push", "origin", "main"}, {"fetch"}, {}, {"status"}} {
		_, err := r.Run(gpContext(t), refused...)
		assert.ErrorIs(t, err, ErrGitCommandNotAllowed, "%v", refused)
	}
	_, err := r.WithIndexFile("relative/index").Run(gpContext(t), "read-tree", gpOID)
	assert.ErrorIs(t, err, ErrGitCommandNotAllowed, "a temp index must be absolute")
	assert.NoFileExists(t, args)
}

// REQ-07 and the step-8 temp index: stdin carries the diff, Run bounds stdout,
// RunTo streams it, and a failed git reports its exit code.
func TestGitPolicyRunner_InputOutputAndExitCodes(t *testing.T) {
	t.Parallel()
	f := newGPFixture(t)
	ctx := gpContext(t)
	diff := []byte("diff --git a/x.go b/x.go\n")
	out, err := f.gpFake("FAKE_STDIN=1").RunInput(ctx, diff, "apply", "--numstat", "--summary", "-z", "--check")
	require.NoError(t, err)
	assert.Equal(t, diff, out)

	text := filepath.Join(f.root, "out.txt")
	require.NoError(t, os.WriteFile(text, bytes.Repeat([]byte("x"), 1024), 0o600))
	bounded := f.gpFake("FAKE_OUT=" + text)
	bounded.MaxOutput = 1024
	out, err = bounded.Run(ctx, "ls-files", "-v", "-z")
	require.NoError(t, err)
	assert.Len(t, out, 1024, "output at the bound is kept")
	bounded.MaxOutput = 1023
	_, err = bounded.Run(ctx, "ls-files", "-v", "-z")
	assert.ErrorIs(t, err, ErrGitOutputTooLarge)
	var streamed bytes.Buffer
	require.NoError(t, bounded.RunTo(ctx, &streamed, "diff", "--no-ext-diff", "--no-textconv", "--binary"))
	assert.Equal(t, 1024, streamed.Len(), "RunTo leaves the bound to its writer")

	_, err = f.gpFake("FAKE_EXIT=3").Run(ctx, "version")
	assert.Equal(t, 3, GitExitCode(err))
	assert.NotErrorIs(t, err, ErrGitStopped)
	assert.EqualError(t, err, "git version: exit status 3")
	_, err = GitPolicyRunner{Binary: filepath.Join(f.root, "missing-git")}.Run(ctx, "version")
	assert.Equal(t, -1, GitExitCode(err))
	assert.Equal(t, -1, GitExitCode(errors.New("other")))
	assert.Equal(t, -1, GitExitCode(nil))
}

// REQ-12: a command that outlives its step group gets SIGTERM on its process
// group StopGrace before the deadline and SIGKILL at the deadline, and no
// child of git survives it. Each deadline clock starts only once the fake
// git is ready (gpReadyContext), so the SIGTERM never lands before its trap
// exists, however loaded the machine is.
func TestGitPolicyRunner_StopsAtTheStepGroupDeadline(t *testing.T) {
	t.Parallel()
	f := newGPFixture(t)
	signal, late := filepath.Join(f.root, "signal.txt"), filepath.Join(f.root, "late.txt")
	const grace, budget = 1200 * time.Millisecond, 2400 * time.Millisecond

	termReady := filepath.Join(f.root, "term.ready")
	terminated := f.gpFake("FAKE_MODE=term-exit", "FAKE_SIGNAL="+signal, "FAKE_READY="+termReady)
	terminated.StopGrace = grace
	ctx := newGPReadyContext(termReady, budget)
	_, err := terminated.Run(ctx, "reset", "--hard", "--no-recurse-submodules", "--quiet")
	ended := time.Now()
	deadline, _ := ctx.Deadline()
	assert.ErrorIs(t, err, ErrGitStopped)
	assert.Equal(t, 143, GitExitCode(err), "git ended on the SIGTERM it got")
	assert.True(t, ended.Before(deadline.Add(-200*time.Millisecond)), "SIGTERM came before the deadline")
	data, readErr := os.ReadFile(signal)
	require.NoError(t, readErr)
	assert.Equal(t, "term\n", string(data))

	// Control: killing only the direct child leaves its child alive, which
	// writes its file 4 s after it started.
	ignoreReady, controlReady := filepath.Join(f.root, "ignore.ready"), filepath.Join(f.root, "control.ready")
	ignoring := f.gpFake("FAKE_MODE=ignore-term", "FAKE_LATE="+late, "FAKE_READY="+ignoreReady)
	control := exec.Command(ignoring.Binary)
	control.Env = append(ignoring.Environ(), "FAKE_LATE="+late+".control", "FAKE_READY="+controlReady)
	control.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, control.Start())
	defer func() { _ = syscall.Kill(-control.Process.Pid, syscall.SIGKILL) }()
	require.Eventually(t, func() bool { return gpFileExists(controlReady) }, gpReadyWait, 5*time.Millisecond)
	require.NoError(t, control.Process.Kill())
	_ = control.Wait()

	ignoring.StopGrace = grace
	ctx = newGPReadyContext(ignoreReady, budget)
	_, err = ignoring.Run(ctx, "worktree", "add", "--no-checkout", "--detach", filepath.Join(f.root, "wt"), gpOID)
	ended = time.Now()
	deadline, _ = ctx.Deadline()
	assert.ErrorIs(t, err, ErrGitStopped)
	assert.ErrorContains(t, err, "stopped at its deadline")
	assert.False(t, ended.Before(deadline.Add(-50*time.Millisecond)), "SIGKILL waited for the deadline")
	assert.True(t, ended.Before(deadline.Add(2*time.Second)))
	assert.Eventually(t, func() bool { return gpFileExists(late + ".control") }, 10*time.Second, 20*time.Millisecond,
		"the control's child outlived a kill of its parent")
	// The killed child would have written its file 4 s after its fake got
	// ready, which is no later than the clock start (deadline - budget).
	time.Sleep(time.Until(deadline.Add(-budget + 4*time.Second + 500*time.Millisecond)))
	assert.NoFileExists(t, late, "the SIGKILL of the process group reached git's child")

	done, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = ignoring.Run(done, "version")
	assert.ErrorIs(t, err, context.Canceled)
	assert.ErrorIs(t, err, ErrGitStopped)
}

// gpReadyWait bounds how long a test waits for a fake to get ready.
const gpReadyWait = 30 * time.Second

// gpReadyContext is a context whose deadline clock starts only once the
// ready file exists, which a stop mode of the fake creates after it has
// installed its TERM trap. The runner reads the deadline when it arms its
// SIGTERM timer right after the start, so the first Deadline call waits for
// the file and then starts the clock; Done closes at that deadline.
type gpReadyContext struct {
	context.Context
	ready    string
	budget   time.Duration
	done     chan struct{}
	once     sync.Once
	mu       sync.Mutex
	deadline time.Time
	err      error
}

func newGPReadyContext(ready string, budget time.Duration) *gpReadyContext {
	return &gpReadyContext{Context: context.Background(), ready: ready, budget: budget, done: make(chan struct{})}
}

func (c *gpReadyContext) Deadline() (time.Time, bool) {
	c.once.Do(func() {
		for limit := time.Now().Add(gpReadyWait); !gpFileExists(c.ready) && time.Now().Before(limit); {
			time.Sleep(time.Millisecond)
		}
		c.mu.Lock()
		c.deadline = time.Now().Add(c.budget)
		c.mu.Unlock()
		time.AfterFunc(c.budget, func() {
			c.mu.Lock()
			c.err = context.DeadlineExceeded
			c.mu.Unlock()
			close(c.done)
		})
	})
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.deadline, true
}

func (c *gpReadyContext) Done() <-chan struct{} { return c.done }

func (c *gpReadyContext) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}
