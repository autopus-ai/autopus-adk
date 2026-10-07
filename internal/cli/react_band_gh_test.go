package cli

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeBandAnswer scripts one command: its stdout and error, or a hang that
// lasts until the call's own timeout ends it.
type fakeBandAnswer struct {
	stdout string
	err    error
	hang   bool
}

// fakeBandCall is one recorded call: argv including the binary, the full
// environment, the directory, and the timeout budget left when it started.
type fakeBandCall struct {
	argv    []string
	env     []string
	dir     string
	timeout time.Duration
}

// fakeBandRunner is the command-runner seam for tests, so no test starts a
// real gh. Answers match the whole argv first, then its first three words.
type fakeBandRunner struct {
	mu      sync.Mutex
	calls   []fakeBandCall
	noGH    bool
	answers map[string]fakeBandAnswer
}

func (f *fakeBandRunner) LookPath(file string) (string, error) {
	if file == "gh" && f.noGH {
		return "", exec.ErrNotFound
	}
	return "/fake/bin/" + file, nil
}

func (f *fakeBandRunner) Run(ctx context.Context, command bandCommand) error {
	call := fakeBandCall{argv: append([]string{command.Name}, command.Args...), env: command.Env, dir: command.Dir}
	if deadline, ok := ctx.Deadline(); ok {
		call.timeout = time.Until(deadline)
	}
	f.mu.Lock()
	f.calls = append(f.calls, call)
	answer, ok := f.answers[strings.Join(call.argv, " ")]
	if !ok {
		answer = f.answers[strings.Join(call.argv[:min(3, len(call.argv))], " ")]
	}
	f.mu.Unlock()
	if answer.hang {
		<-ctx.Done()
		return ctx.Err()
	}
	if command.Stdout != nil {
		_, _ = io.WriteString(command.Stdout, answer.stdout)
	}
	return answer.err
}

// recorded returns the calls of one binary in call order.
func (f *fakeBandRunner) recorded(name string) []fakeBandCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	var calls []fakeBandCall
	for _, call := range f.calls {
		if call.argv[0] == name {
			calls = append(calls, call)
		}
	}
	return calls
}

// argvs returns the joined argv of every call of one binary in call order.
func (f *fakeBandRunner) argvs(name string) []string {
	var argvs []string
	for _, call := range f.recorded(name) {
		argvs = append(argvs, strings.Join(call.argv, " "))
	}
	return argvs
}

// scriptedBandRunner answers origin, authentication, default branch, and the
// run list of acme/app.
func scriptedBandRunner(origin, branch, runs string) *fakeBandRunner {
	return &fakeBandRunner{answers: map[string]fakeBandAnswer{
		"git remote get-url":    {stdout: origin + "\n"},
		"gh auth status":        {},
		"gh api repos/acme/app": {stdout: branch + "\n"},
		"gh run list":           {stdout: runs},
	}}
}

// testBandClient inherits an environment that names another repository and
// host, which every gh call must replace.
func testBandClient(runner bandRunner) bandGHClient {
	client := newBandGHClient(runner)
	client.environ = func() []string {
		return []string{"PATH=/usr/bin", "GH_REPO=other/repo", "GH_HOST=stale.example.com", "GH_TOKEN=synthetic"}
	}
	return client
}

const bandRunListArgv = "gh run list -R acme/app --limit 200 --json databaseId,attempt,conclusion,status,headBranch,event,workflowName,createdAt"

// Only the gh Invocation Table (plus the read-only origin lookup) may run:
// no git mutation, no gh pr, no gh api method or field flag, no status filter.
func TestReactBandGH_CheckCommand_AllowsOnlyTheInvocationTable(t *testing.T) {
	t.Parallel()
	allowed := [][]string{
		{"git", "remote", "get-url", "origin"},
		{"git", "-c", "core.fsmonitor=false", "ls-files", "-z", "--", ":(icase,glob).autopu*/metric*", ":(icase,glob).autopu*/metric*/**"},
		{"gh", "auth", "status", "--hostname", "github.com"},
		{"gh", "api", "repos/acme/app", "--hostname", "ghe.example.com", "--jq", ".default_branch"},
		{"gh", "run", "list", "-R", "acme/app", "--limit", "1000", "--json", bandRunListFields},
		{"gh", "run", "view", "4242", "-R", "acme/app", "--attempt", "2", "--log-failed"},
	}
	for _, argv := range allowed {
		assert.NoError(t, checkBandCommand(bandCommand{Name: argv[0], Args: argv[1:]}), "%v", argv)
	}
	refused := [][]string{
		{"git", "stash"},
		{"git", "push", "origin", "main"},
		{"git", "worktree", "add", "../x"},
		{"git", "commit", "-m", "x"},
		{"git", "remote", "get-url", "upstream"},
		{"git", "ls-files", "-z", "--", ".autopus/metrics"},
		{"git", "-c", "core.fsmonitor=false", "ls-files", "-z", "--", ":(icase).autopus/metrics"},
		{"git", "-c", "core.fsmonitor=/tmp/hook", "ls-files", "-z", "--", ":(icase,glob).autopu*/metric*", ":(icase,glob).autopu*/metric*/**"},
		{"gh", "pr", "create", "--draft"},
		{"gh", "api", "-X", "POST", "repos/acme/app/pulls"},
		{"gh", "api", "repos/acme/app", "--hostname", "github.com", "--jq", ".default_branch", "--method", "PATCH"},
		{"gh", "api", "repos/acme/app/git/refs", "--hostname", "github.com", "--jq", ".default_branch"},
		{"gh", "api", "repos/acme/app", "--hostname", "github.com", "-f", "name=x"},
		{"gh", "auth", "status", "--hostname", "-x"},
		{"gh", "auth", "login", "--hostname", "github.com"},
		{"gh", "run", "list", "-R", "acme/app", "--limit", "1001", "--json", bandRunListFields},
		{"gh", "run", "list", "-R", "acme/app", "--limit", "0200", "--json", bandRunListFields},
		{"gh", "run", "list", "-R", "acme/../app", "--limit", "200", "--json", bandRunListFields},
		{"gh", "run", "list", "-R", "acme/app", "--limit", "200", "--json", "databaseId", "--status", "failure"},
		{"gh", "run", "view", "4242", "-R", "acme/app", "--attempt", "0", "--log-failed"},
		{"gh", "run", "view", "42x", "-R", "acme/app", "--attempt", "1", "--log-failed"},
		{"gh", "run", "rerun", "4242", "-R", "acme/app", "--attempt", "1", "--failed"},
		{"sh", "-c", "gh run list"},
	}
	for _, argv := range refused {
		err := checkBandCommand(bandCommand{Name: argv[0], Args: argv[1:]})
		assert.ErrorIs(t, err, errBandCommandNotAllowed, "%v", argv)
	}
}

// Host and owner/repo come from the origin URL in every common spelling; a
// subdomain of github.com is github.com, as gh normalizes it.
func TestReactBandGH_ParseOrigin_ResolvesHostAndRepository(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		origin string
		want   bandGHTarget
	}{
		{"git@github.com:acme/app.git", bandGHTarget{"github.com", "acme", "app"}},
		{"https://github.com/acme/app.git", bandGHTarget{"github.com", "acme", "app"}},
		{"https://user:ghp_synthetic@GitHub.com/acme/app/", bandGHTarget{"github.com", "acme", "app"}},
		{"ssh://git@ssh.github.com:443/acme/app.git", bandGHTarget{"github.com", "acme", "app"}},
		{"git://github.com/Acme/my.app", bandGHTarget{"github.com", "Acme", "my.app"}},
		{"git@ghe.example.com:team/svc.git", bandGHTarget{"ghe.example.com", "team", "svc"}},
		{"https://gitlab.com/acme/app.git", bandGHTarget{"gitlab.com", "acme", "app"}},
	} {
		got, ok := parseBandOrigin(tc.origin)
		assert.True(t, ok, tc.origin)
		assert.Equal(t, tc.want, got, tc.origin)
	}
	for _, origin := range []string{
		"", "/srv/git/app.git", "file:///srv/git/app.git", "../app", "https://github.com/acme",
		"https://github.com/acme/app/tree/main", "git@github.com:acme/../app.git", "ftp://github.com/acme/app",
		"git@-x.example.com:acme/app.git", "https://github.com/-acme/app", "https://github.com/acme/a%20b",
	} {
		_, ok := parseBandOrigin(origin)
		assert.False(t, ok, origin)
	}
	assert.Equal(t, "acme/app", bandGHTarget{"github.com", "acme", "app"}.Slug())
}

// REQ-19: --limit accepts 1 to 1000 and rejects every other value.
func TestReactBandGH_ValidateLimit_AcceptsOneTo1000(t *testing.T) {
	t.Parallel()
	for _, limit := range []int{1, 50, bandDefaultLimit, bandMaxLimit} {
		assert.NoError(t, validateBandLimit(limit), limit)
	}
	for _, limit := range []int{0, -1, 1001} {
		assert.Error(t, validateBandLimit(limit), limit)
	}
	assert.Equal(t, 200, bandDefaultLimit)
}

// The production runner refuses a command outside the table before anything
// executes, so no caller can reach git stash or gh pr through it.
func TestReactBandGH_ExecRunner_RefusesCommandsOutsideTheTable(t *testing.T) {
	t.Parallel()
	marker := filepath.Join(t.TempDir(), "touched")
	err := execBandRunner{}.Run(t.Context(), bandCommand{Name: "touch", Args: []string{marker}})
	require.ErrorIs(t, err, errBandCommandNotAllowed)
	assert.NoFileExists(t, marker)
	err = execBandRunner{}.Run(t.Context(), bandCommand{Name: "git", Args: []string{"stash"}, Dir: t.TempDir()})
	assert.ErrorIs(t, err, errBandCommandNotAllowed)
}

// The production runner executes the read-only origin lookup with the given
// directory and, separately, with an environment that points git elsewhere.
func TestReactBandGH_ExecRunner_RunsGitWithDirAndEnv(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	repo := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"remote", "add", "origin", "git@github.com:acme/app.git"}} {
		require.NoError(t, exec.Command("git", append([]string{"-C", repo}, args...)...).Run())
	}
	path, err := execBandRunner{}.LookPath("git")
	require.NoError(t, err)
	assert.NotEmpty(t, path)

	var byDir strings.Builder
	origin := bandCommand{Name: "git", Args: []string{"remote", "get-url", "origin"}, Dir: repo, Stdout: &byDir}
	require.NoError(t, execBandRunner{}.Run(t.Context(), origin))
	assert.Equal(t, "git@github.com:acme/app.git\n", byDir.String())

	var byEnv strings.Builder
	origin.Dir, origin.Stdout = t.TempDir(), &byEnv
	origin.Env = append(os.Environ(), "GIT_DIR="+filepath.Join(repo, ".git"))
	require.NoError(t, execBandRunner{}.Run(t.Context(), origin))
	assert.Equal(t, "git@github.com:acme/app.git\n", byEnv.String())

	origin.Env, origin.Stdout = nil, nil
	var exitErr *exec.ExitError
	assert.True(t, errors.As(execBandRunner{}.Run(t.Context(), origin), &exitErr), "no origin outside a repository")
}
