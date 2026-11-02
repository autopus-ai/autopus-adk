package worker

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// S9 (SPEC-WORKERPOST-001): every invalid contract names its problem.
func TestPlanContract_RejectsInvalidContracts(t *testing.T) {
	t.Parallel()

	cases := []struct {
		contract string
		want     string
	}{
		{`{"version":2}`, `unsupported version 2`},
		{`{"required_actions":["deploy"]}`, `unsupported action "deploy"`},
		{`{"required_actions":["push"],"forbidden_actions":["push"]}`, `"push" is both required and forbidden`},
		{`{"ref":"refs/tags/v1"}`, `is not under refs/heads/`},
		{`{"ref":"feature/../x"}`, `invalid branch name`},
		{`{"ref":"/feature"}`, `invalid branch name`},
		{`{"ref":"feature.lock"}`, `invalid branch name`},
		{`{"remote":"-oProxyCommand=x"}`, `invalid remote`},
		{`{"expected_commit":"abc123"}`, `expected_commit must be a full`},
		{`{"push_match":"prefix"}`, `unsupported push_match "prefix"`},
	}
	for _, tc := range cases {
		_, err := planPayloadPostconditions(contractOf(tc.contract), "commit and push")
		require.Error(t, err, tc.contract)
		assert.True(t, strings.HasPrefix(err.Error(), "invalid completion_contract: "), err.Error())
		assert.Contains(t, err.Error(), tc.want, tc.contract)
	}
}

func TestPlanContract_ClipsAndQuotesSenderValues(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("x", 200) + "\nforged line"
	_, err := planPayloadPostconditions(&completionContractMessage{RequiredActions: []string{long}}, "")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "\n")
	assert.NotContains(t, err.Error(), "forged line")
	assert.Contains(t, err.Error(), `"`+strings.Repeat("x", contractValueClip)+`"`)
}

func TestPlanContract_NormalizesAValidContract(t *testing.T) {
	t.Parallel()

	plan, err := planPayloadPostconditions(
		contractOf(`{"version":1,"required_actions":["Commit"," push "],"ref":"feature/x"}`), "")
	require.NoError(t, err)
	assert.Equal(t, sourceContract, plan.Source)
	assert.Equal(t, levelRequired, plan.Commit)
	assert.Equal(t, levelRequired, plan.Push)
	assert.Equal(t, levelNone, plan.Branch)
	assert.Equal(t, "refs/heads/feature/x", plan.Ref)
	assert.Equal(t, defaultPostconditionRemote, plan.Remote)
	assert.Equal(t, matchExact, plan.Match)

	sha := strings.Repeat("AB", 20)
	plan, err = planPayloadPostconditions(&completionContractMessage{
		Ref: "refs/heads/release/1.0", ExpectedCommit: sha, PushMatch: "Contains", Remote: "upstream",
	}, "")
	require.NoError(t, err)
	assert.Equal(t, "refs/heads/release/1.0", plan.Ref)
	assert.Equal(t, strings.ToLower(sha), plan.ExpectedCommit)
	assert.Equal(t, matchContains, plan.Match)
	assert.Equal(t, "upstream", plan.Remote)
}

// REQ-WPOST-INPUT-03: a present contract switches prompt inference off.
func TestPlanContract_PromptIsNotReadWhenAContractExists(t *testing.T) {
	t.Parallel()

	plan, err := planPayloadPostconditions(contractOf(`{"required_actions":[]}`), "commit and git push origin feature/x")
	require.NoError(t, err)
	assert.Equal(t, levelNone, plan.Commit)
	assert.Equal(t, levelNone, plan.Push)
	assert.Equal(t, levelNone, plan.Branch)
	assert.Empty(t, plan.Branches)
}

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
