package worker

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// S10 (SPEC-WORKERPOST-001): only the read-only forms the checks need can
// start, whatever the arguments.
func TestPostconditionGit_RefusesEveryWritingCommand(t *testing.T) {
	t.Parallel()

	refused := [][]string{
		{"commit", "-m", "x"}, {"push", "origin", "main"}, {"fetch", "origin"}, {"update-ref", "refs/heads/x", "HEAD"},
		{"config", "user.name", "x"}, {"remote", "add", "x", "/tmp/x"}, {"remote", "set-url", "origin", "/tmp/x"},
		{"merge-base", "HEAD", "HEAD"}, {"symbolic-ref", "HEAD", "refs/heads/x"}, {"cat-file", "-p", "HEAD"},
		{"ls-remote", "--upload-pack=touch /tmp/x", "origin"}, {"reset", "--hard"}, {"replace", "-d", "x"}, {},
		{"rev-list", "--output=/tmp/x", "HEAD"}, {"rev-list", "HEAD", "^--output=/tmp/x"}, {"rev-parse", "--output=/tmp/x"},
		{"merge-base", "--is-ancestor", "--output=/tmp/x", "HEAD"}, {"cat-file", "-e", "--batch"},
		{"remote", "get-url", "--push"}, {"ls-remote", "--", "origin", "--upload-pack=x"},
	}
	f := newGitFixture(t)
	g, log := f.traced()
	for _, args := range refused {
		res := g.run(f.repo, nil, args...)
		require.Error(t, res.err, "%v", args)
		assert.Equal(t, -1, res.code, "%v", args)
		if len(args) > 0 {
			assert.Contains(t, res.err.Error(), "not an allowed read-only postcondition command", "%v", args)
		}
	}
	assert.Empty(t, log.argvs, "a refused command must never start")
}

func TestPostconditionGit_RecordedCommandsAreReadOnly(t *testing.T) {
	t.Parallel()

	allowed := map[string]bool{
		"rev-parse": true, "symbolic-ref": true, "remote get-url": true, "ls-remote": true,
		"merge-base --is-ancestor": true, "cat-file -e": true, "rev-list": true, "for-each-ref": true,
	}
	scenarios := []struct {
		contract *completionContractMessage
		prompt   string
	}{
		{contractOf(`{"required_actions":["commit","push","branch"],"ref":"feature/x","push_match":"contains"}`), ""},
		{contractOf(`{"forbidden_actions":["commit","push","branch"],"ref":"feature/x"}`), ""},
		{nil, "create branch feature/x, commit, and git push origin feature/x"},
	}
	for _, sc := range scenarios {
		f := newGitFixture(t)
		g, log := f.traced()
		plan := planTaskPostconditions(sc.contract, sc.prompt)
		base := g.captureBaseline(f.repo, plan)
		f.run(f.repo, "checkout", "-q", "-b", "feature/x")
		f.commit("B")
		f.run(f.repo, "push", "-q", "origin", "HEAD:refs/heads/feature/x")
		f.commit("C")
		repoDigest, remoteDigest := dirDigest(t, filepath.Join(f.repo, ".git")), dirDigest(t, f.bare)

		_, err := g.verify(f.repo, plan, base)
		require.Error(t, err, "every scenario leaves at least one check failing")
		assert.Equal(t, repoDigest, dirDigest(t, filepath.Join(f.repo, ".git")), "verification wrote into the repository")
		assert.Equal(t, remoteDigest, dirDigest(t, f.bare), "verification wrote into the remote")
		require.NotEmpty(t, log.argvs)
		at := len(postconditionGlobalArgs)
		for _, argv := range log.argvs {
			require.Equal(t, postconditionGlobalArgs, argv[:at], "%v", argv)
			form := argv[at]
			if len(argv) > at+1 && (form == "remote" || form == "merge-base" || form == "cat-file") {
				form += " " + argv[at+1]
			}
			assert.True(t, allowed[form], "unexpected git invocation %v", argv)
		}
	}
}

func TestPostconditionGit_EnvironmentDropsRedirects(t *testing.T) {
	t.Parallel()

	g := postconditionGit{env: []string{"PATH=/bin", "GIT_DIR=/elsewhere", "git_work_tree=/x", "GIT_GRAFT_FILE=/g",
		"GIT_TERMINAL_PROMPT=1", "GIT_CEILING_DIRECTORIES=/c", "GIT_CONFIG_GLOBAL=/dev/null"}}
	env := g.environ()
	assert.Contains(t, env, "PATH=/bin")
	assert.Contains(t, env, "GIT_CONFIG_GLOBAL=/dev/null")
	assert.Contains(t, env, "GIT_TERMINAL_PROMPT=0")
	for _, entry := range env {
		for _, dropped := range []string{"GIT_DIR=", "git_work_tree=", "GIT_GRAFT_FILE=", "GIT_TERMINAL_PROMPT=1", "GIT_CEILING_DIRECTORIES="} {
			assert.False(t, strings.HasPrefix(entry, dropped), entry)
		}
	}
}

// S11: report shape and redaction.
func TestPostconditionReport_ShapeAndRedaction(t *testing.T) {
	t.Parallel()

	f := newGitFixture(t)
	f.run(f.repo, "checkout", "-q", "-b", "feature/x")
	plan := f.plan(contractOf(`{"required_actions":["push"],"ref":"feature/x"}`), "")
	base := f.baseline(plan)
	f.commit("B")

	artifact, err := f.git.verify(f.repo, plan, base)
	require.Error(t, err)
	var raw map[string]any
	require.NoError(t, json.Unmarshal([]byte(artifact.Data), &raw))
	assert.Equal(t, float64(2), raw["version"])
	assert.Equal(t, true, raw["active"])
	assert.Equal(t, sourceContract, raw["source"])
	assert.Equal(t, statusUnsatisfied, raw["outcome"])
	report := decodeReport(t, artifact.Data)
	require.Len(t, report.Checks, 3)
	for _, check := range report.Checks {
		assert.NotEmpty(t, check.Name)
		assert.NotEmpty(t, check.Requirement)
		assert.NotEmpty(t, check.Status)
		assert.NotEmpty(t, check.Detail)
	}
	assert.NotContains(t, artifact.Data, f.bare, "the report names remotes, never URLs")

	secret := newGitFixture(t)
	secret.run(secret.repo, "checkout", "-q", "-b", "feature/x")
	server := strings.Replace(unauthorizedGitServer(t), "http://", "http://user:secret-token@", 1) + "/r.git"
	secret.run(secret.repo, "remote", "set-url", "origin", server)
	secretPlan := secret.plan(contractOf(`{"required_actions":["push"],"ref":"feature/x","expected_commit":"`+secret.base+`"}`), "")
	artifact, err = secret.git.verify(secret.repo, secretPlan, secret.baseline(secretPlan))
	require.ErrorIs(t, err, errPostconditionUnverified)
	for _, text := range []string{artifact.Data, err.Error()} {
		assert.NotContains(t, text, "secret-token")
		assert.NotContains(t, text, "user:")
	}
}

func TestRedactGitMessage(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "fatal: unable to access 'https://<redacted>@h/r.git/': x",
		redactGitMessage("fatal: unable to access 'https://u:p@h/r.git/': x"))
	for raw, want := range map[string]string{
		"fatal: unable to access 'TOKEN@github.com/org/repo.git/': x": "fatal: unable to access '<redacted>@github.com/org/repo.git/': x",
		"secret-token@host:repo.git":                                  "<redacted>@host:repo.git",
		"user:pw@host:r.git":                                          "<redacted>@host:r.git",
		"git@github.com:org/repo.git":                                 "<redacted>@github.com:org/repo.git",
		"ssh://git@host/x":                                            "ssh://<redacted>@host/x",
		"remote=TOKEN@host/r.git":                                     "remote=<redacted>@host/r.git",
		"HEAD@{0} refs/heads/v1@x a@{b}":                              "HEAD@{0} refs/heads/v1@x a@{b}",
		"no credentials in this message":                              "no credentials in this message",
	} {
		assert.Equal(t, want, redactGitMessage(raw), raw)
	}
	assert.Equal(t, "first", redactGitMessage("first\nsecond"))
	assert.Len(t, redactGitMessage(strings.Repeat("é", 300)), gitMessageClip)
}

// S11: an unsatisfied and an unknown check together give the outcome
// unsatisfied, and the error lists both.
func TestPostconditionReport_MixedOutcomeListsEveryCheck(t *testing.T) {
	t.Parallel()

	f := newGitFixture(t)
	f.run(f.repo, "remote", "set-url", "origin", t.TempDir()) // not a repository
	plan := f.plan(contractOf(`{"required_actions":["commit","push"],"ref":"main"}`), "")

	report, err := f.verify(plan, f.baseline(plan))
	require.ErrorIs(t, err, errPostconditionFailed)
	assert.Equal(t, statusUnsatisfied, report.Outcome)
	assert.Equal(t, statusUnsatisfied, findCheck(t, report, actionCommit).Status)
	assert.Equal(t, statusUnknown, findCheck(t, report, actionPush).Status)
	assert.True(t, strings.HasPrefix(err.Error(), "postcondition failed: commit: HEAD did not advance after execution; "), err.Error())
	assert.Contains(t, err.Error(), "push (unverified): remote lookup failed")
}
