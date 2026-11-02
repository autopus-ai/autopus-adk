package worker

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// S2 (SPEC-WORKERPOST-001): a stale remote branch fails the push and the
// exact SHA passes.
func TestPushContract_StaleRemoteFailsAndExactSHAPasses(t *testing.T) {
	t.Parallel()

	f := newGitFixture(t)
	f.run(f.repo, "checkout", "-q", "-b", "feature/stale")
	stale := f.commit("A")
	f.run(f.repo, "push", "-q", "origin", "feature/stale")
	plan := f.plan(contractOf(`{"required_actions":["commit","push"],"ref":"feature/stale"}`), "")
	base := f.baseline(plan)
	fresh := f.commit("B")

	report, err := f.verify(plan, base)
	require.ErrorIs(t, err, errPostconditionFailed)
	assert.True(t, strings.HasPrefix(err.Error(), "postcondition failed: "), err.Error())
	push := findCheck(t, report, actionPush)
	assert.Equal(t, statusUnsatisfied, push.Status)
	assert.Equal(t, "refs/heads/feature/stale", push.Ref)
	assert.Equal(t, fresh, push.Expected)
	assert.Equal(t, stale, push.Observed)

	f.run(f.repo, "push", "-q", "origin", "feature/stale")
	report, err = f.verify(plan, base)
	require.NoError(t, err)
	assert.Equal(t, statusSatisfied, findCheck(t, report, actionPush).Status)
	assert.Equal(t, fresh, findCheck(t, report, actionPush).Observed)
	assert.Equal(t, statusSatisfied, findCheck(t, report, actionCommit).Status)

	// An explicit older expectation: exact rejects the newer tip, contains
	// accepts it because the older commit is its ancestor.
	exact := f.plan(contractOf(`{"required_actions":["push"],"ref":"feature/stale","expected_commit":"`+stale+`"}`), "")
	report, _ = f.verify(exact, base)
	assert.Equal(t, statusUnsatisfied, findCheck(t, report, actionPush).Status)
	contains := f.plan(contractOf(`{"required_actions":["push"],"ref":"feature/stale","expected_commit":"`+stale+`","push_match":"contains"}`), "")
	report, _ = f.verify(contains, base)
	assert.Equal(t, statusSatisfied, findCheck(t, report, actionPush).Status)
}

// S3: another remote, another ref, a suffix ref, and a repointed remote never
// satisfy a push.
func TestPushContract_SuffixRefAndRepointedRemoteFail(t *testing.T) {
	t.Parallel()

	contract := `{"required_actions":["push"],"ref":"feature/x"}`
	cases := []struct {
		name   string
		change func(f *gitFixture, head string)
		detail string
	}{
		{"other remote", func(f *gitFixture, head string) {
			f.run(f.repo, "remote", "add", "upstream", f.bareRemote())
			f.run(f.repo, "push", "-q", "upstream", "feature/x")
		}, `remote branch "feature/x" not found on origin`},
		{"other ref", func(f *gitFixture, head string) {
			f.run(f.repo, "push", "-q", "origin", "feature/x:refs/heads/feature/y")
		}, `not found on origin`},
		{"suffix ref", func(f *gitFixture, head string) {
			f.run(f.repo, "push", "-q", "origin", "feature/x:refs/heads/other/feature/x")
		}, `not found on origin`},
		{"repointed url", func(f *gitFixture, head string) {
			second := f.bareRemote()
			f.run(f.repo, "push", "-q", second, "feature/x")
			f.run(f.repo, "remote", "set-url", "origin", second)
		}, `not found on origin`},
		{"insteadOf rewrite", func(f *gitFixture, head string) {
			second := f.bareRemote()
			f.run(f.repo, "push", "-q", second, "feature/x")
			f.run(f.repo, "config", "url."+second+".insteadOf", f.bare)
		}, `not found on origin`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newGitFixture(t)
			f.run(f.repo, "checkout", "-q", "-b", "feature/x")
			plan := f.plan(contractOf(contract), "")
			base := f.baseline(plan)
			head := f.commit("B")
			tc.change(f, head)

			report, err := f.verify(plan, base)
			require.ErrorIs(t, err, errPostconditionFailed)
			push := findCheck(t, report, actionPush)
			assert.Equal(t, statusUnsatisfied, push.Status)
			assert.Contains(t, push.Detail, tc.detail)
		})
	}
}

// S4: pushes that cannot be decided are unknown, never satisfied.
func TestPushContract_UnverifiablePushesAreUnknown(t *testing.T) {
	t.Parallel()

	f := newGitFixture(t)
	f.run(f.repo, "checkout", "-q", "-b", "feature/x")
	head := f.commit("B")
	f.run(f.repo, "push", "-q", "origin", "feature/x")

	noRef := f.plan(contractOf(`{"required_actions":["push"]}`), "")
	report, err := f.verify(noRef, f.baseline(noRef))
	require.ErrorIs(t, err, errPostconditionUnverified)
	assert.Equal(t, "the contract names no ref", findCheck(t, report, actionPush).Detail)

	noRemote := f.plan(contractOf(`{"required_actions":["push"],"ref":"feature/x","remote":"upstream"}`), "")
	report, _ = f.verify(noRemote, f.baseline(noRemote))
	assert.Equal(t, statusUnknown, findCheck(t, report, actionPush).Status)

	f.run(f.repo, "checkout", "-q", "--detach", head)
	detached := f.plan(contractOf(`{"required_actions":["push"],"ref":"feature/x"}`), "")
	report, _ = f.verify(detached, f.baseline(detached))
	assert.Equal(t, statusUnknown, findCheck(t, report, actionPush).Status)
	pinned := f.plan(contractOf(`{"required_actions":["push"],"ref":"feature/x","expected_commit":"`+head+`"}`), "")
	report, err = f.verify(pinned, f.baseline(pinned))
	require.NoError(t, err)
	assert.Equal(t, statusSatisfied, findCheck(t, report, actionPush).Status)
}

func TestPushContract_LookupFailuresAreUnknown(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		url     func(t *testing.T) string
		timeout time.Duration
		detail  string
	}{
		{"not a repository", func(t *testing.T) string { return t.TempDir() }, 0, "does not appear to be a git repository"},
		{"authentication", func(t *testing.T) string {
			return strings.Replace(unauthorizedGitServer(t), "http://", "http://user:pw@", 1) + "/r.git"
		}, 0, "Authentication failed"},
		{"no credentials", func(t *testing.T) string { return unauthorizedGitServer(t) + "/r.git" }, 0, "terminal prompts disabled"},
		{"timeout", func(t *testing.T) string { return stalledGitServer(t) + "/r.git" }, 2 * time.Second, "timed out"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newGitFixture(t)
			f.run(f.repo, "checkout", "-q", "-b", "feature/x")
			f.commit("B")
			f.run(f.repo, "push", "-q", "origin", "feature/x")
			f.run(f.repo, "remote", "set-url", "origin", tc.url(t))
			f.git.timeout = tc.timeout
			plan := f.plan(contractOf(`{"required_actions":["push"],"ref":"feature/x"}`), "")

			report, err := f.verify(plan, f.baseline(plan))
			require.ErrorIs(t, err, errPostconditionUnverified)
			assert.True(t, strings.HasPrefix(err.Error(), "postcondition unverified: "), err.Error())
			push := findCheck(t, report, actionPush)
			assert.Equal(t, statusUnknown, push.Status)
			assert.True(t, strings.HasPrefix(push.Detail, "remote lookup failed"), push.Detail)
			assert.Contains(t, push.Detail, tc.detail)
		})
	}
}

// unauthorizedGitServer answers every request with 401.
func unauthorizedGitServer(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("WWW-Authenticate", `Basic realm="test"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// stalledGitServer never answers until the test ends.
func stalledGitServer(t *testing.T) string {
	t.Helper()
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })
	return srv.URL
}
