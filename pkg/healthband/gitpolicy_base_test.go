//go:build unix

package healthband

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gpFakeGit is one fake git for runner tests, so macOS assesses a single new
// executable: FAKE_ARGS and FAKE_ENV record the argv and environment,
// FAKE_OUT is printed, FAKE_STDIN echoes stdin, FAKE_MODE selects a stop
// behavior, and FAKE_EXIT is the exit status. A stop mode creates
// FAKE_READY once its TERM trap is installed (and, in ignore-term, its child
// has started), so a test can start its deadline clock only then.
func (f *gpFixture) gpFakeGit() string {
	if path := filepath.Join(f.bin, "fakegit"); gpFileExists(path) {
		return path
	}
	return f.script("fakegit", `[ -n "$FAKE_ARGS" ] && printf '%s\n' "$@" > "$FAKE_ARGS"
[ -n "$FAKE_ENV" ] && env > "$FAKE_ENV"
[ -n "$FAKE_OUT" ] && cat "$FAKE_OUT"
[ -n "$FAKE_STDIN" ] && cat
case "$FAKE_MODE" in
ignore-term) trap '' TERM; (sleep 4; touch "$FAKE_LATE") & : > "$FAKE_READY"; sleep 30 & wait ;;
term-exit) trap 'echo term > "$FAKE_SIGNAL"; exit 143' TERM; : > "$FAKE_READY"; sleep 30 & wait ;;
esac
exit "${FAKE_EXIT:-0}"
`)
}

// gpFake is a runner on the fake git whose environment adds vars.
func (f *gpFixture) gpFake(vars ...string) GitPolicyRunner {
	env := append(f.home("fake-home", "", ""), vars...)
	return GitPolicyRunner{Binary: f.gpFakeGit(), Dir: f.root, Environ: func() []string { return env }}
}

// SPEC-SIGMABAND-002 S6: git older than 2.44 or an unparsable git version is
// refused at step 1 with git_version_unsupported:<major>.<minor>.
func TestGitPolicyRunner_CheckVersionRefusesGitBefore244(t *testing.T) {
	t.Parallel()
	f := newGPFixture(t)
	ctx := gpContext(t)
	out := filepath.Join(f.root, "version.txt")
	r := f.gpFake("FAKE_OUT=" + out)
	for version, want := range map[string]string{
		"git version 2.43.0\n":                 "git_version_unsupported:2.43",
		"git version 2.44.0\n":                 "",
		"git version 2.50.1 (Apple Git-155)\n": "",
		"git version 3.0.0\n":                  "",
		"git version 1.99.9\n":                 "git_version_unsupported:1.99",
		"git version 2.43.0.windows.1\n":       "git_version_unsupported:2.43",
		"version 2.44\n":                       "git_version_unsupported:unknown",
		"git version 99999999999.1\n":          "git_version_unsupported:unknown",
	} {
		require.NoError(t, os.WriteFile(out, []byte(version), 0o600))
		code, err := r.CheckVersion(ctx)
		require.NoError(t, err)
		assert.Equal(t, want, code, version)
	}
	code, err := f.gpFake("FAKE_EXIT=1").CheckVersion(ctx)
	require.NoError(t, err)
	assert.Equal(t, "git_version_unsupported:unknown", code, "a failing git version")

	code, err = GitPolicyRunner{}.CheckVersion(ctx)
	require.NoError(t, err)
	assert.Empty(t, code, "the git these tests run on is 2.44 or later")
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = r.CheckVersion(cancelled)
	assert.ErrorIs(t, err, context.Canceled)
}

// SPEC-SIGMABAND-002 Git Execution Policy item 4 and S3, S6: the base is the
// fetched remote-tracking commit of the default branch, else the local
// branch; origin/HEAD names the branch under --no-fetch; a name that fails
// git check-ref-format --branch, or no candidate, is base_unavailable.
func TestGitPolicyRunner_ResolveBaseTakesTheLastFetchedDefaultBranch(t *testing.T) {
	t.Parallel()
	f := newGPFixture(t)
	ctx := gpContext(t)
	repo, first := f.repo("repo", map[string]string{"a.txt": "1\n"})
	f.writeFiles(repo, map[string]string{"a.txt": "2\n"})
	f.git(f.setupEnv, repo, "commit", "-q", "-am", "second")
	second := strings.TrimSpace(f.git(f.setupEnv, repo, "rev-parse", "HEAD"))
	f.git(f.setupEnv, repo, "update-ref", "refs/remotes/origin/main", first)
	f.git(f.setupEnv, repo, "update-ref", "refs/remotes/origin/release/2.0", second)
	r := f.runner(f.home("band-home", "", ""), repo)
	resolve := func(branch string) (string, string) {
		t.Helper()
		sha, code, err := r.ResolveBase(ctx, branch)
		require.NoError(t, err)
		return sha, code
	}

	assert.Equal(t, [2]string{first, ""}, gpPair(resolve("main")), "the remote-tracking ref wins")
	assert.Equal(t, [2]string{second, ""}, gpPair(resolve("release/2.0")))
	f.git(f.setupEnv, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	assert.Equal(t, [2]string{first, ""}, gpPair(resolve("")), "--no-fetch reads origin/HEAD")
	f.git(f.setupEnv, repo, "update-ref", "-d", "refs/remotes/origin/main")
	assert.Equal(t, [2]string{second, ""}, gpPair(resolve("main")), "otherwise refs/heads/<default>")
	for _, bad := range []string{"nope", "a..b", "-x", "@{-1}", "@", "has space", "x.lock", "HEAD", "a\nb", "main^{tree}", "x/"} {
		assert.Equal(t, [2]string{"", GitBaseUnavailable}, gpPair(resolve(bad)), "%q", bad)
	}
	f.git(f.setupEnv, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/heads/main")
	assert.Equal(t, [2]string{"", GitBaseUnavailable}, gpPair(resolve("")), "origin/HEAD outside origin/")
	f.git(f.setupEnv, repo, "symbolic-ref", "--delete", "refs/remotes/origin/HEAD")
	assert.Equal(t, [2]string{"", GitBaseUnavailable}, gpPair(resolve("")), "no origin/HEAD")

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	for _, branch := range []string{"", "main"} {
		_, _, err := r.ResolveBase(cancelled, branch)
		assert.ErrorIs(t, err, context.Canceled)
	}
}

func gpPair(a, b string) [2]string { return [2]string{a, b} }

func gpFileExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
