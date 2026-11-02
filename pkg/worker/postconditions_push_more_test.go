package worker

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// S5 (SPEC-WORKERPOST-001): an unrelated commit fails, and contains needs
// trusted ancestry from local objects.
func TestPushContract_ContainsNeedsTrustedAncestry(t *testing.T) {
	t.Parallel()

	setup := func(t *testing.T) (*gitFixture, string, executionBaseline) {
		f := newGitFixture(t)
		f.run(f.repo, "checkout", "-q", "-b", "feature/x")
		expected := f.commit("B")
		plan := f.plan(contractOf(`{"required_actions":["push"],"ref":"feature/x"}`), "")
		return f, expected, f.baseline(plan)
	}
	judge := func(f *gitFixture, expected, match string, base executionBaseline) postconditionCheck {
		plan := f.plan(contractOf(`{"required_actions":["push"],"ref":"feature/x","expected_commit":"`+
			expected+`","push_match":"`+match+`"}`), "")
		report, _ := f.verify(plan, base)
		return findCheck(t, report, actionPush)
	}

	t.Run("unrelated tip", func(t *testing.T) {
		t.Parallel()
		f, expected, base := setup(t)
		unrelated := f.orphan("U")
		f.run(f.repo, "push", "-q", "origin", unrelated+":refs/heads/feature/x")
		for _, match := range []string{matchExact, matchContains} {
			push := judge(f, expected, match, base)
			assert.Equal(t, statusUnsatisfied, push.Status, match)
			assert.Equal(t, unrelated, push.Observed, match)
		}

		// A replace ref cannot fake the ancestry: the runner ignores it.
		f.run(f.repo, "replace", "--graft", unrelated, expected)
		assert.Equal(t, statusUnsatisfied, judge(f, expected, matchContains, base).Status)

		// A graft file is honoured even without replace objects, so any
		// answer becomes unknown.
		f.run(f.repo, "replace", "-d", unrelated)
		grafts := f.run(f.repo, "rev-parse", "--path-format=absolute", "--git-path", "info/grafts")
		require.NoError(t, os.WriteFile(grafts, []byte(unrelated+" "+expected+"\n"), 0o644))
		push := judge(f, expected, matchContains, base)
		assert.Equal(t, statusUnknown, push.Status)
		assert.Contains(t, push.Detail, "graft")
	})

	t.Run("tip absent locally", func(t *testing.T) {
		t.Parallel()
		f, expected, base := setup(t)
		f.run(f.repo, "push", "-q", "origin", "feature/x")
		clone := t.TempDir()
		f.run(clone, "clone", "-q", "--branch", "feature/x", f.bare, ".")
		f.run(clone, "commit", "-q", "--allow-empty", "-m", "C")
		f.run(clone, "push", "-q", "origin", "feature/x")

		push := judge(f, expected, matchContains, base)
		assert.Equal(t, statusUnknown, push.Status)
		assert.Contains(t, push.Detail, "not available locally")
	})

	t.Run("descendant present locally", func(t *testing.T) {
		t.Parallel()
		f, expected, base := setup(t)
		descendant := f.commit("D")
		f.run(f.repo, "push", "-q", "origin", "feature/x")

		push := judge(f, expected, matchContains, base)
		assert.Equal(t, statusSatisfied, push.Status)
		assert.Equal(t, descendant, push.Observed)
		assert.Equal(t, statusUnsatisfied, judge(f, expected, matchExact, base).Status)
	})
}

// S7: the forbidden push blames only remote heads that moved to a commit HEAD
// gained during execution.
func TestPushContract_ForbiddenPush(t *testing.T) {
	t.Parallel()

	forbidden := `{"forbidden_actions":["push"]}`
	cases := []struct {
		name   string
		before func(f *gitFixture)
		change func(f *gitFixture)
		want   string
		detail string
	}{
		{"new commit not pushed", nil, func(f *gitFixture) { f.commit("B") }, statusSatisfied, "no branch on origin moved"},
		{"new commit pushed elsewhere", nil, func(f *gitFixture) {
			f.commit("B")
			f.run(f.repo, "push", "-q", "origin", "HEAD:refs/heads/sneaky")
		}, statusUnsatisfied, "refs/heads/sneaky"},
		{"pulled commit the remote already had", func(f *gitFixture) {
			f.run(f.repo, "checkout", "-q", "-b", "side")
			f.commit("P")
			f.run(f.repo, "push", "-q", "origin", "side")
			f.run(f.repo, "checkout", "-q", "main")
		}, func(f *gitFixture) { f.run(f.repo, "merge", "-q", "--ff-only", "side") }, statusSatisfied, "no branch on origin moved"},
		{"remote unreachable", func(f *gitFixture) {
			f.run(f.repo, "remote", "set-url", "origin", f.t.TempDir())
		}, func(f *gitFixture) { f.commit("B") }, statusUnknown, "remote lookup failed"},
		{"no remote before or after", func(f *gitFixture) {
			f.run(f.repo, "remote", "remove", "origin")
		}, func(f *gitFixture) { f.commit("B") }, statusSatisfied, "is not configured"},
		{"remote added during execution", func(f *gitFixture) {
			f.run(f.repo, "remote", "remove", "origin")
		}, func(f *gitFixture) {
			f.commit("B")
			f.run(f.repo, "remote", "add", "origin", f.bare)
		}, statusUnknown, "gained a URL"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newGitFixture(t)
			if tc.before != nil {
				tc.before(f)
			}
			plan := f.plan(contractOf(forbidden), "")
			base := f.baseline(plan)
			tc.change(f)

			report, _ := f.verify(plan, base)
			push := findCheck(t, report, actionPush)
			assert.Equal(t, tc.want, push.Status)
			assert.Contains(t, push.Detail, tc.detail)
		})
	}
}

func TestPushContract_ForbiddenPushWithoutSnapshotIsUnknown(t *testing.T) {
	t.Parallel()

	f := newGitFixture(t)
	plan := f.plan(contractOf(`{"forbidden_actions":["push"]}`), "")
	base := f.baseline(plan)
	require.NotNil(t, base.RemoteHeads)
	f.commit("B")
	f.run(f.repo, "push", "-q", "origin", "HEAD:refs/heads/sneaky")

	base.RemoteHeads = nil // the dispatch-time lookup failed
	report, err := f.verify(plan, base)
	require.ErrorIs(t, err, errPostconditionUnverified)
	push := findCheck(t, report, actionPush)
	assert.Equal(t, statusUnknown, push.Status)
	assert.Contains(t, push.Detail, "no baseline snapshot")
}
