package worker

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newPartialCloneFixture makes the work repository a blob-less partial clone
// of a bare server, so the server is a promisor remote that plain git fetches
// missing objects from on demand. seed is a second clone that can push.
func newPartialCloneFixture(t *testing.T) (*gitFixture, string) {
	t.Helper()
	f := &gitFixture{t: t, env: hermeticGitEnv(t), bare: t.TempDir(), repo: t.TempDir()}
	f.git = postconditionGit{env: f.env}
	seed := t.TempDir()
	f.run(f.bare, "init", "-q", "--bare")
	f.run(f.bare, "config", "uploadpack.allowFilter", "true")
	f.run(seed, "init", "-q", "-b", "main")
	f.run(seed, "commit", "-q", "--allow-empty", "-m", "A0")
	f.run(seed, "checkout", "-q", "-b", "feature/x")
	f.run(seed, "commit", "-q", "--allow-empty", "-m", "B")
	f.run(seed, "push", "-q", f.bare, "main", "feature/x")
	f.run(f.repo, "clone", "-q", "--filter=blob:none", "--no-checkout", "file://"+f.bare, ".")
	f.base = f.run(seed, "rev-parse", "main")
	return f, seed
}

// S10 and REQ-WPOST-READONLY-01: a remote tip missing from a partial clone
// stays missing; the verifier neither fetches it nor writes anything.
func TestTrust_LazyFetchIsBlocked(t *testing.T) {
	t.Parallel()

	f, seed := newPartialCloneFixture(t)
	expected := f.run(seed, "rev-parse", "feature/x")
	f.run(seed, "commit", "-q", "--allow-empty", "-m", "C")
	f.run(seed, "push", "-q", f.bare, "feature/x")
	tip := f.run(seed, "rev-parse", "feature/x")
	plan := f.plan(contractOf(`{"required_actions":["push"],"ref":"feature/x","expected_commit":"`+expected+`","push_match":"contains"}`), "")
	base := f.baseline(plan)
	before := dirDigest(t, filepath.Join(f.repo, ".git"))

	report, err := f.verify(plan, base)
	require.ErrorIs(t, err, errPostconditionUnverified)
	push := findCheck(t, report, actionPush)
	assert.Equal(t, statusUnknown, push.Status)
	assert.Contains(t, push.Detail, "not available locally")
	assert.Equal(t, before, dirDigest(t, filepath.Join(f.repo, ".git")), "verification wrote into the repository")

	// Plain git would have fetched the tip from the promisor remote.
	f.run(f.repo, "cat-file", "-e", tip)
}

// S5: a commit-graph the task rewrote cannot hide a gained commit from the
// forbidden-push check; plain git is fooled by the same file.
func TestTrust_CommitGraphCannotHideAGainedCommit(t *testing.T) {
	t.Parallel()

	f := newGitFixture(t)
	plan := f.plan(contractOf(`{"forbidden_actions":["push"]}`), "")
	base := f.baseline(plan)
	gained := f.orphan("U")
	f.run(f.repo, "reset", "-q", "--hard", gained)
	f.run(f.repo, "push", "-q", "origin", "HEAD:refs/heads/sneaky")
	f.tamperCommitGraph(f.base, gained) // the baseline now claims U as its parent
	require.Empty(t, f.run(f.repo, "rev-list", gained, "^"+f.base), "plain git must be fooled by the tampered graph")

	report, err := f.verify(plan, base)
	require.ErrorIs(t, err, errPostconditionFailed)
	push := findCheck(t, report, actionPush)
	assert.Equal(t, statusUnsatisfied, push.Status)
	assert.Equal(t, "refs/heads/sneaky", push.Ref)
}

// S7: a graft file that makes the gained commit an ancestor of the baseline
// leaves the forbidden push unknown instead of satisfied.
func TestTrust_GraftMakesForbiddenPushUnknown(t *testing.T) {
	t.Parallel()

	f := newGitFixture(t)
	plan := f.plan(contractOf(`{"forbidden_actions":["push"]}`), "")
	base := f.baseline(plan)
	gained := f.orphan("U")
	f.run(f.repo, "reset", "-q", "--hard", gained)
	f.run(f.repo, "push", "-q", "origin", "HEAD:refs/heads/sneaky")
	require.NoError(t, os.WriteFile(f.gitPath("info/grafts"), []byte(f.base+" "+gained+"\n"), 0o644))

	report, err := f.verify(plan, base)
	require.ErrorIs(t, err, errPostconditionUnverified)
	push := findCheck(t, report, actionPush)
	assert.Equal(t, statusUnknown, push.Status)
	assert.Contains(t, push.Detail, "graft")
}

// S5: behind a shallow boundary a negative ancestry answer is unknown.
func TestTrust_ShallowNegativeAncestryIsUnknown(t *testing.T) {
	t.Parallel()

	f := newGitFixture(t)
	f.run(f.repo, "checkout", "-q", "-b", "feature/x")
	expected := f.commit("E")
	tip := f.commit("T")
	f.run(f.repo, "push", "-q", "origin", "feature/x")
	plan := f.plan(contractOf(`{"required_actions":["push"],"ref":"feature/x","expected_commit":"`+expected+`","push_match":"contains"}`), "")
	base := f.baseline(plan)
	require.NoError(t, os.WriteFile(f.gitPath("shallow"), []byte(tip+"\n"), 0o644))

	report, err := f.verify(plan, base)
	require.ErrorIs(t, err, errPostconditionUnverified)
	push := findCheck(t, report, actionPush)
	assert.Equal(t, statusUnknown, push.Status)
	assert.Contains(t, push.Detail, "shallow")
}

// S3: a relative remote URL is pinned against the work tree, as git resolves
// it, and not against the neutral directory of the lookup.
func TestTrust_RelativeRemoteIsPinnedToTheWorkTree(t *testing.T) {
	t.Parallel()

	f := newGitFixture(t)
	relative, err := filepath.Rel(f.repo, f.bare)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(relative, ".."), relative)
	f.run(f.repo, "remote", "set-url", "origin", relative)
	f.run(f.repo, "checkout", "-q", "-b", "feature/x")
	plan := f.plan(contractOf(`{"required_actions":["commit","push"],"ref":"feature/x"}`), "")
	base := f.baseline(plan)
	require.True(t, filepath.IsAbs(base.RemoteURL), base.RemoteURL)
	f.commit("B")
	f.run(f.repo, "push", "-q", "origin", "feature/x")

	report, err := f.verify(plan, base)
	require.NoError(t, err)
	assert.Equal(t, statusSatisfied, findCheck(t, report, actionPush).Status)
}

func TestIsRelativeLocalPath(t *testing.T) {
	t.Parallel()

	for url, relative := range map[string]bool{
		"../remote.git": true, "remote.git": true, "sub/remote.git": true, "./r:x": true,
		"/abs/remote.git": false, "https://h/r.git": false, "file:///r.git": false,
		"git@github.com:org/r.git": false, "host:path/r.git": false,
	} {
		assert.Equal(t, relative, isRelativeLocalPath(url), url)
	}
}

// R05 of the review: a lookup that failed is never read as absence, so a
// damaged repository cannot turn a forbidden action into satisfied.
func TestLookup_FailedLookupsAreUnknownNotAbsent(t *testing.T) {
	t.Parallel()

	f := newGitFixture(t)
	forbidden := f.plan(contractOf(`{"forbidden_actions":["commit","push"]}`), "")
	repair := f.breakConfig()
	base := f.baseline(forbidden)
	repair()
	assert.Equal(t, presenceUnknown, base.Head)
	assert.Equal(t, presenceUnknown, base.Remote)
	report, err := f.verify(forbidden, base)
	require.ErrorIs(t, err, errPostconditionUnverified)
	assert.Equal(t, statusUnknown, findCheck(t, report, actionCommit).Status)
	assert.Equal(t, statusUnknown, findCheck(t, report, actionPush).Status)

	legacy := f.plan(nil, "commit the change")
	repair = f.breakConfig()
	legacyBase := f.baseline(legacy)
	repair()
	report, err = f.verify(legacy, legacyBase)
	require.ErrorIs(t, err, errPostconditionUnverified, "fallback mode must report instead of staying silent")
	assert.Equal(t, statusUnknown, findCheck(t, report, actionCommit).Status)

	branch := f.plan(contractOf(`{"required_actions":["branch"],"forbidden_actions":["commit","push"],"ref":"main"}`), "")
	branchBase := f.baseline(branch)
	repair = f.breakConfig()
	report, err = f.verify(branch, branchBase)
	repair()
	require.ErrorIs(t, err, errPostconditionUnverified)
	for _, name := range []string{actionCommit, actionPush, actionBranch} {
		assert.Equal(t, statusUnknown, findCheck(t, report, name).Status, name)
	}
}

func TestLookup_HeadClassification(t *testing.T) {
	t.Parallel()

	f := newGitFixture(t)
	plain, unborn := t.TempDir(), t.TempDir()
	f.run(unborn, "init", "-q")
	assert.Equal(t, presenceAbsent, f.git.head(plain).state)
	assert.Equal(t, presenceAbsent, f.git.head(unborn).state)
	assert.Equal(t, presencePresent, f.git.head(f.repo).state)
	state, _, _ := f.git.remoteURL(unborn, "origin")
	assert.Equal(t, presenceAbsent, state)

	// A ref under another namespace never stands in for a missing branch.
	f.run(f.repo, "update-ref", "refs/tags/refs/heads/ghost", "HEAD")
	_, ghost, _ := f.git.resolveRef(f.repo, "refs/heads/ghost")
	assert.Equal(t, presenceAbsent, ghost)
	require.Error(t, exec.Command("git", "-C", plain, "rev-parse").Run(), "plain must not be inside a repository")
}
