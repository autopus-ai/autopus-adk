package worker

import (
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifyExecutionPostconditions_PushBranchMissingFails(t *testing.T) {
	t.Parallel()

	f := newGitFixture(t)
	prompt := "create branch autopus-canary-missing, commit changes, and git push origin autopus-canary-missing"
	plan := f.plan(nil, prompt)
	base := f.baseline(plan)
	f.run(f.repo, "checkout", "-q", "-b", "autopus-canary-missing")
	f.commit("change")

	report, err := f.verify(plan, base)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "remote branch")
	assert.Equal(t, statusUnsatisfied, findCheck(t, report, actionPush).Status)
}

func TestVerifyExecutionPostconditions_PushBranchExistsPasses(t *testing.T) {
	t.Parallel()

	f := newGitFixture(t)
	prompt := "create branch autopus-canary-pass, commit changes, and git push origin autopus-canary-pass"
	plan := f.plan(nil, prompt)
	base := f.baseline(plan)
	f.run(f.repo, "checkout", "-q", "-b", "autopus-canary-pass")
	head := f.commit("change")
	f.run(f.repo, "push", "-q", "-u", "origin", "autopus-canary-pass")

	report, err := f.verify(plan, base)
	require.NoError(t, err)
	push := findCheck(t, report, actionPush)
	assert.Equal(t, statusSatisfied, push.Status)
	assert.Equal(t, head, push.Observed)
	assert.Equal(t, statusSatisfied, report.Outcome)
}

// Reproduction 1 of SPEC-WORKERPOST-001: on 109c7ea4 this read-only prompt
// failed with "postcondition failed: commit: HEAD did not advance after
// execution".
func TestPostconditionRegression_ReadOnlyPromptRequiresNothing(t *testing.T) {
	t.Parallel()

	f := newGitFixture(t)
	plan := f.plan(nil, "Review only; do not commit or push")
	base := f.baseline(plan)
	repoRefs, remoteRefs := f.refs(f.repo), f.refs(f.bare)

	report, err := f.verify(plan, base)
	require.NoError(t, err)
	assert.Equal(t, sourceLegacy, report.Source)
	assert.Equal(t, statusNotRequired, report.Outcome)
	for _, name := range []string{actionCommit, actionPush, actionBranch} {
		assert.Equal(t, statusNotRequired, findCheck(t, report, name).Status, name)
	}
	assert.Equal(t, repoRefs, f.refs(f.repo))
	assert.Equal(t, remoteRefs, f.refs(f.bare))
}

// Reproduction 2 of SPEC-WORKERPOST-001: on 109c7ea4 the push check passed
// because the branch existed on origin, although only an older commit was
// there and the new local commit was never pushed.
func TestPostconditionRegression_StaleRemoteBranchFailsPush(t *testing.T) {
	t.Parallel()

	f := newGitFixture(t)
	f.run(f.repo, "checkout", "-q", "-b", "feature/stale")
	stale := f.commit("A")
	f.run(f.repo, "push", "-q", "origin", "feature/stale")
	plan := f.plan(nil, "commit and git push origin feature/stale")
	base := f.baseline(plan)
	fresh := f.commit("B")

	report, err := f.verify(plan, base)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errPostconditionFailed))
	push := findCheck(t, report, actionPush)
	assert.Equal(t, statusUnsatisfied, push.Status)
	assert.Equal(t, "refs/heads/feature/stale", push.Ref)
	assert.Equal(t, fresh, push.Expected)
	assert.Equal(t, stale, push.Observed)

	f.run(f.repo, "push", "-q", "origin", "feature/stale")
	report, err = f.verify(plan, base)
	require.NoError(t, err)
	assert.Equal(t, statusSatisfied, findCheck(t, report, actionPush).Status)
	assert.Equal(t, statusSatisfied, findCheck(t, report, actionCommit).Status)
}

func TestDetectTaskPostconditions_IgnoresFillerWords(t *testing.T) {
	t.Parallel()

	reqs := detectTaskPostconditions("create and switch to a new branch named autopus-canary-20260414-072233 and push the branch to origin")
	assert.Equal(t, []string{"autopus-canary-20260414-072233"}, reqs.Branches)
}

func initGitRepoWithOrigin(t *testing.T) string {
	t.Helper()

	repo := t.TempDir()
	bare := t.TempDir()

	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "test@test.com")
	runGit(t, repo, "config", "user.name", "Test")
	runGit(t, repo, "commit", "--allow-empty", "-m", "init")
	runGit(t, bare, "init", "--bare")
	runGit(t, repo, "remote", "add", "origin", bare)
	defaultBranch := strings.TrimSpace(gitOutputForTest(t, repo, "branch", "--show-current"))
	runGit(t, repo, "push", "-u", "origin", defaultBranch)

	return repo
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, string(out))
}

func gitOutputForTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, string(out))
	return string(out)
}
