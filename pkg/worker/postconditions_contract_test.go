package worker

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// S1: a contract forbidding commit and push completes with no git change and
// does not depend on the remote being reachable.
func TestCommitContract_ReadOnlyForbiddenActionsAreSatisfied(t *testing.T) {
	t.Parallel()

	f := newGitFixture(t)
	f.run(f.repo, "remote", "set-url", "origin", t.TempDir()) // not a repository
	plan := f.plan(contractOf(`{"forbidden_actions":["commit","push"]}`), "")
	base := f.baseline(plan)
	g, log := f.traced()

	artifact, err := g.verify(f.repo, plan, base)
	require.NoError(t, err)
	report := decodeReport(t, artifact.Data)
	assert.Equal(t, sourceContract, report.Source)
	assert.Equal(t, statusSatisfied, report.Outcome)
	for _, name := range []string{actionCommit, actionPush} {
		check := findCheck(t, report, name)
		assert.Equal(t, statusSatisfied, check.Status, name)
		assert.Equal(t, string(levelForbidden), check.Requirement, name)
	}
	assert.Equal(t, statusNotRequired, findCheck(t, report, actionBranch).Status)
	assert.False(t, log.ran("ls-remote"), "a read-only result must not depend on the remote")
}

// S6: commit checks follow HEAD.
func TestCommitContract_StatusTable(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		level  string
		change func(f *gitFixture)
		want   string
		detail string
	}{
		{"required, no change", "required", func(*gitFixture) {}, statusUnsatisfied, "HEAD did not advance after execution"},
		{"required, new commit", "required", func(f *gitFixture) { f.commit("B") }, statusSatisfied, "HEAD advanced to "},
		{"required, reset to orphan", "required", resetToOrphan, statusUnsatisfied, "does not descend from the baseline"},
		{"forbidden, no change", "forbidden", func(*gitFixture) {}, statusSatisfied, "HEAD unchanged"},
		{"forbidden, new commit", "forbidden", func(f *gitFixture) { f.commit("B") }, statusUnsatisfied, "HEAD advanced"},
		{"forbidden, reset to orphan", "forbidden", resetToOrphan, statusUnknown, "does not descend from the baseline"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newGitFixture(t)
			plan := f.plan(contractOf(`{"`+tc.level+`_actions":["commit"]}`), "")
			base := f.baseline(plan)
			tc.change(f)
			report, _ := f.verify(plan, base)
			commit := findCheck(t, report, actionCommit)
			assert.Equal(t, tc.want, commit.Status)
			assert.Contains(t, commit.Detail, tc.detail)
		})
	}
}

func TestCommitContract_NoBaselineCommit(t *testing.T) {
	t.Parallel()

	f := newGitFixture(t)
	unborn := t.TempDir()
	f.run(unborn, "init", "-q")
	required := f.plan(contractOf(`{"required_actions":["commit"]}`), "")
	artifact, err := f.git.verify(unborn, required, f.git.captureBaseline(unborn, required))
	require.ErrorIs(t, err, errPostconditionUnverified)
	assert.Contains(t, findCheck(t, decodeReport(t, artifact.Data), actionCommit).Detail, "no baseline commit")

	plain := t.TempDir()
	forbidden := f.plan(contractOf(`{"forbidden_actions":["commit","push"]}`), "")
	artifact, err = f.git.verify(plain, forbidden, f.git.captureBaseline(plain, forbidden))
	require.NoError(t, err)
	decoded := decodeReport(t, artifact.Data)
	assert.Contains(t, findCheck(t, decoded, actionCommit).Detail, "no commit")
	assert.Equal(t, statusSatisfied, findCheck(t, decoded, actionPush).Status)
}

// S7: branch checks.
func TestBranchContract_StatusTable(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		contract string
		change   func(f *gitFixture)
		want     string
	}{
		{"required, exists", `{"required_actions":["branch"],"ref":"feature/x"}`, branchAt("feature/x"), statusSatisfied},
		{"required, missing", `{"required_actions":["branch"],"ref":"feature/x"}`, func(*gitFixture) {}, statusUnsatisfied},
		{"required, wrong tip", `{"required_actions":["branch"],"ref":"feature/x","expected_commit":"BASE"}`,
			func(f *gitFixture) { branchAt("feature/x")(f); f.commit("B") }, statusUnsatisfied},
		{"required, no ref", `{"required_actions":["branch"]}`, branchAt("feature/x"), statusUnknown},
		{"forbidden, created", `{"forbidden_actions":["branch"],"ref":"feature/new"}`, branchAt("feature/new"), statusUnsatisfied},
		{"forbidden, existed", `{"forbidden_actions":["branch"],"ref":"main"}`, func(*gitFixture) {}, statusSatisfied},
		{"forbidden, absent", `{"forbidden_actions":["branch"],"ref":"feature/new"}`, func(*gitFixture) {}, statusSatisfied},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newGitFixture(t)
			plan := f.plan(contractOf(strings.ReplaceAll(tc.contract, "BASE", f.base)), "")
			base := f.baseline(plan)
			tc.change(f)
			report, _ := f.verify(plan, base)
			assert.Equal(t, tc.want, findCheck(t, report, actionBranch).Status)
		})
	}
}

func resetToOrphan(f *gitFixture) {
	f.run(f.repo, "reset", "-q", "--hard", f.orphan("U"))
}

func branchAt(name string) func(f *gitFixture) {
	return func(f *gitFixture) { f.run(f.repo, "checkout", "-q", "-b", name) }
}

// S7: a branch that exists now but whose baseline lookup failed is unknown.
func TestBranchContract_ForbiddenWithUnknownBaseline(t *testing.T) {
	t.Parallel()

	f := newGitFixture(t)
	plan := f.plan(contractOf(`{"forbidden_actions":["branch"],"ref":"feature/new"}`), "")
	base := f.baseline(plan)
	require.Equal(t, presenceAbsent, base.RefState)
	base.RefState = presenceUnknown // the dispatch-time lookup failed
	f.run(f.repo, "checkout", "-q", "-b", "feature/new")

	report, err := f.verify(plan, base)
	require.ErrorIs(t, err, errPostconditionUnverified)
	assert.Equal(t, statusUnknown, findCheck(t, report, actionBranch).Status)
}
