package worker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// S8 (SPEC-WORKERPOST-001): fallback inference keeps only unnegated requests
// and never requires more than the pre-contract keyword match did.
func TestFallbackLevels_NegationAwareTable(t *testing.T) {
	t.Parallel()

	cases := []struct {
		prompt               string
		commit, push, branch requirementLevel
	}{
		{"Review only; do not commit or push", levelNone, levelNone, levelNone},
		{"Commit your changes but do not push", levelRequired, levelNone, levelNone},
		{"Don't forget to commit and push", levelRequired, levelRequired, levelNone},
		{"Never git push. Commit the fix", levelRequired, levelNone, levelNone},
		{"Commit the fix. Do not commit generated files", levelAmbiguous, levelNone, levelNone},
		{"Improve branch coverage", levelNone, levelNone, levelNone},
		{"create branch feature/login", levelNone, levelNone, levelRequired},
		{"Do not create a branch named release-2", levelNone, levelNone, levelNone},
		{"Pushing to main is not allowed", levelNone, levelNone, levelNone},
		{"commit 하지 마세요", levelNone, levelNone, levelNone},
		{"push하지마", levelNone, levelNone, levelNone},
		{"commit_message cleanup and pushed state", levelNone, levelNone, levelNone},
		{"Make no commits and avoid pushing", levelNone, levelNone, levelNone},
		{"There is no need to push", levelNone, levelNone, levelNone},
		{"Don’t push yet", levelNone, levelNone, levelNone},
	}
	for _, tc := range cases {
		plan := detectTaskPostconditions(tc.prompt)
		assert.Equal(t, tc.commit, plan.Commit, "commit: %q", tc.prompt)
		assert.Equal(t, tc.push, plan.Push, "push: %q", tc.prompt)
		assert.Equal(t, tc.branch, plan.Branch, "branch: %q", tc.prompt)
	}
}

// The fallback can only lose requirements: wherever it requires an action,
// the pre-contract keyword expressions also matched.
func TestFallbackLevels_NeverRequireMoreThanTheKeywordMatch(t *testing.T) {
	t.Parallel()

	prompts := []string{
		"commit the change, create a branch release/2.0 and git push to origin",
		"Commits must be signed", "the pushed branch is stale", "Committing is fine; pushing is not",
		"pre-commit hooks run on every commit", "force-push is forbidden", "커밋하고 push 하세요",
	}
	for _, prompt := range prompts {
		plan := detectTaskPostconditions(prompt)
		if plan.Commit == levelRequired || plan.Commit == levelAmbiguous {
			assert.Regexp(t, `(?i)\bcommit\b`, prompt, prompt)
		}
		if plan.Push == levelRequired || plan.Push == levelAmbiguous {
			assert.Regexp(t, `(?i)\bpush\b`, prompt, prompt)
		}
		if plan.Branch == levelRequired || plan.Branch == levelAmbiguous {
			assert.Regexp(t, `(?i)\bbranch\b`, prompt, prompt)
		}
	}
	assert.Equal(t, levelNone, detectTaskPostconditions("force-push is forbidden").Push)
	assert.Equal(t, levelRequired, detectTaskPostconditions("커밋하고 push 하세요").Push)
}

// An ambiguous fallback check with no evidence is unknown, not a demand to
// act; with evidence it is satisfied.
func TestFallbackVerify_AmbiguousIsUnknownWithoutEvidence(t *testing.T) {
	t.Parallel()

	f := newGitFixture(t)
	plan := f.plan(nil, "Commit the fix. Do not commit generated files")
	base := f.baseline(plan)

	report, err := f.verify(plan, base)
	require.ErrorIs(t, err, errPostconditionUnverified)
	commit := findCheck(t, report, actionCommit)
	assert.Equal(t, statusUnknown, commit.Status)
	assert.Equal(t, string(levelAmbiguous), commit.Requirement)
	assert.Contains(t, commit.Detail, "HEAD did not advance after execution")

	f.commit("B")
	report, err = f.verify(plan, base)
	require.NoError(t, err)
	assert.Equal(t, statusSatisfied, findCheck(t, report, actionCommit).Status)
}

func TestFallbackVerify_PushFromWorkerBranchWithoutNameIsUnknown(t *testing.T) {
	t.Parallel()

	f := newGitFixture(t)
	f.run(f.repo, "checkout", "-q", "-b", "worker-task-1")
	plan := f.plan(nil, "push your changes")
	base := f.baseline(plan)
	f.commit("B")

	report, err := f.verify(plan, base)
	require.ErrorIs(t, err, errPostconditionUnverified)
	push := findCheck(t, report, actionPush)
	assert.Equal(t, statusUnknown, push.Status)
	assert.Contains(t, push.Detail, "cannot determine the pushed branch")
}

func TestFallbackVerify_NegatedOnlyPromptReportsWithoutFailing(t *testing.T) {
	t.Parallel()

	f := newGitFixture(t)
	plan := f.plan(nil, "Do not push. Review the diff")
	report, err := f.verify(plan, f.baseline(plan))
	require.NoError(t, err)
	assert.Equal(t, statusNotRequired, report.Outcome)
	assert.Equal(t, sourceLegacy, report.Source)
}
