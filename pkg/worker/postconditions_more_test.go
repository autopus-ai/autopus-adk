package worker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifyExecutionPostconditions_CommitAdvancedPasses(t *testing.T) {
	t.Parallel()

	f := newGitFixture(t)
	plan := f.plan(nil, "please commit the changes")
	base := f.baseline(plan)
	require.Equal(t, presencePresent, base.Head)
	head := f.commit("advance head")

	report, err := f.verify(plan, base)
	require.NoError(t, err)
	commit := findCheck(t, report, actionCommit)
	assert.Equal(t, statusSatisfied, commit.Status)
	assert.Equal(t, head, commit.Observed)
}

func TestVerifyExecutionPostconditions_CommitNotAdvancedFails(t *testing.T) {
	t.Parallel()

	f := newGitFixture(t)
	plan := f.plan(nil, "remember to commit your work")
	base := f.baseline(plan)

	// No new commit -> HEAD did not advance.
	report, err := f.verify(plan, base)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "postcondition failed: commit: HEAD did not advance after execution")
	assert.Equal(t, statusUnsatisfied, findCheck(t, report, actionCommit).Status)
}

func TestVerifyExecutionPostconditions_BranchExistsPasses(t *testing.T) {
	t.Parallel()

	f := newGitFixture(t)
	plan := f.plan(nil, "create branch feature/login")
	base := f.baseline(plan)
	f.run(f.repo, "checkout", "-q", "-b", "feature/login")

	report, err := f.verify(plan, base)
	require.NoError(t, err)
	branch := findCheck(t, report, actionBranch)
	assert.Equal(t, statusSatisfied, branch.Status)
	assert.Equal(t, "refs/heads/feature/login", branch.Ref)
}

func TestVerifyExecutionPostconditions_BranchMissingFails(t *testing.T) {
	t.Parallel()

	f := newGitFixture(t)
	// Reference a branch that was never created.
	plan := f.plan(nil, "create branch feature/never-made")
	base := f.baseline(plan)

	report, err := f.verify(plan, base)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `local branch "feature/never-made" not found`)
	assert.Equal(t, statusUnsatisfied, findCheck(t, report, actionBranch).Status)
}

func TestVerifyExecutionPostconditions_NoBaselineSkips(t *testing.T) {
	t.Parallel()

	// A plain directory yields an empty baseline; fallback mode stays silent.
	f := newGitFixture(t)
	dir := t.TempDir()
	plan := f.plan(nil, "commit and push")
	base := f.git.captureBaseline(dir, plan)
	assert.Equal(t, presenceAbsent, base.Head)

	artifact, err := f.git.verify(dir, plan, base)
	require.NoError(t, err)
	assert.Empty(t, artifact.Name)
}

func TestVerifyExecutionPostconditions_NoMentionSkips(t *testing.T) {
	t.Parallel()

	f := newGitFixture(t)
	plan := f.plan(nil, "summarize the README")
	artifact, err := f.git.verify(f.repo, plan, f.baseline(plan))
	require.NoError(t, err)
	assert.Empty(t, artifact.Name)
}

func TestFallbackPushRef_CurrentBranchAmongNamesWins(t *testing.T) {
	t.Parallel()

	plan := taskPostconditions{Branches: []string{"hotfix/x", "release/v1"}}
	assert.Equal(t, "refs/heads/release/v1", fallbackPushRef(plan, "release/v1"))
}

func TestFallbackPushRef_SingleNameWhenCurrentIsWorkerBranch(t *testing.T) {
	t.Parallel()

	// Worker-managed branches are never the target of a fallback push.
	assert.Equal(t, "refs/heads/feature/x", fallbackPushRef(taskPostconditions{Branches: []string{"feature/x"}}, "worker-abc123"))
	assert.Equal(t, "", fallbackPushRef(taskPostconditions{}, "worker-abc123"))
}

func TestFallbackPushRef_FallsBackToCurrentBranch(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "refs/heads/feature/topic", fallbackPushRef(taskPostconditions{}, "feature/topic"))
}

func TestFallbackPushRef_SeveralNamesWithoutCurrentIsUnclear(t *testing.T) {
	t.Parallel()

	plan := taskPostconditions{Branches: []string{"hotfix/x", "release/v1"}}
	assert.Equal(t, "", fallbackPushRef(plan, "main"))
	assert.Equal(t, "", fallbackPushRef(plan, ""))
}

func TestSanitizeBranchName_RejectsTraversalAndFlags(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "", sanitizeBranchName(""))
	assert.Equal(t, "", sanitizeBranchName("  feature/..hack  "))
	assert.Equal(t, "", sanitizeBranchName("-rf"))
	assert.Equal(t, "feature/x", sanitizeBranchName(`"feature/x".`))
}

func TestDetectTaskPostconditions_DetectsAllIntents(t *testing.T) {
	t.Parallel()

	reqs := detectTaskPostconditions("commit the change, create a branch release/2.0 and git push to origin")
	assert.Equal(t, levelRequired, reqs.Commit)
	assert.Equal(t, levelRequired, reqs.Push)
	assert.Equal(t, levelRequired, reqs.Branch)
	assert.Contains(t, reqs.Branches, "release/2.0")
	assert.True(t, reqs.Mentioned)
}
