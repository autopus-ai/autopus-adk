package worker

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func planRaw(t *testing.T, raw, prompt string) (*taskPostconditions, error) {
	t.Helper()
	return planPayloadPostconditions(json.RawMessage(raw), prompt)
}

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
		{`{"ref":"feature.lock/x"}`, `invalid branch name`},
		{`{"remote":"-oProxyCommand=x"}`, `invalid remote`},
		{`{"expected_commit":"abc123"}`, `expected_commit must be a full`},
		{`{"push_match":"prefix"}`, `unsupported push_match "prefix"`},
		{`null`, `completion_contract must be an object, got null`},
		{`["push"]`, `completion_contract must be an object`},
		{`"push"`, `completion_contract must be an object`},
		{`{"version":"1"}`, `version must be an integer`},
		{`{"version":1.5}`, `version must be an integer`},
		{`{"required_actions":"push"}`, `required_actions must be an array of strings`},
		{`{"forbidden_actions":["commit",3]}`, `forbidden_actions[1] must be a string`},
		{`{"ref":null}`, `ref must be a string`},
		{`{"requried_actions":["push"]}`, `unknown key "requried_actions"`},
	}
	for _, tc := range cases {
		_, err := planRaw(t, tc.contract, "commit and push")
		require.Error(t, err, tc.contract)
		assert.True(t, strings.HasPrefix(err.Error(), "invalid completion_contract: "), err.Error())
		assert.Contains(t, err.Error(), tc.want, tc.contract)
	}
}

func TestPlanContract_ReportsEveryProblemTogether(t *testing.T) {
	t.Parallel()

	_, err := planRaw(t, `{"version":"1","required_actions":"push","ref":"a..b","bogus":1,"push_match":"prefix"}`, "")
	require.Error(t, err)
	for _, want := range []string{
		`version must be an integer`, `required_actions must be an array of strings`, `unknown key "bogus"`,
		`invalid branch name "a..b"`, `unsupported push_match "prefix"`,
	} {
		assert.Contains(t, err.Error(), want)
	}
}

// REQ-WPOST-REPORT-01 for contract errors: credentials are removed before a
// sender value is quoted.
func TestPlanContract_RedactsCredentialsInQuotedValues(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		`{"remote":"https://user:secret@host/repo.git"}`,
		`{"ref":"refs/tags/https://user:secret@host"}`,
		`{"required_actions":["user:secret@host:repo.git"]}`,
	} {
		_, err := planRaw(t, raw, "")
		require.Error(t, err, raw)
		assert.NotContains(t, err.Error(), "secret", raw)
		assert.NotContains(t, err.Error(), "user:", raw)
		assert.Contains(t, err.Error(), "<redacted>@", raw)
	}
}

func TestPlanContract_ClipsAndQuotesSenderValues(t *testing.T) {
	t.Parallel()

	long, err := json.Marshal(map[string][]string{"required_actions": {strings.Repeat("x", 200) + "\nforged line"}})
	require.NoError(t, err)
	_, err = planRaw(t, string(long), "")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "\n")
	assert.NotContains(t, err.Error(), "forged line")
	assert.Contains(t, err.Error(), `"`+strings.Repeat("x", contractValueClip)+`"`)
}

func TestPlanContract_NormalizesAValidContract(t *testing.T) {
	t.Parallel()

	plan, err := planRaw(t, `{"version":1,"required_actions":["Commit"," push "],"ref":"feature/x"}`, "")
	require.NoError(t, err)
	assert.Equal(t, sourceContract, plan.Source)
	assert.Equal(t, levelRequired, plan.Commit)
	assert.Equal(t, levelRequired, plan.Push)
	assert.Equal(t, levelNone, plan.Branch)
	assert.Equal(t, "refs/heads/feature/x", plan.Ref)
	assert.Equal(t, defaultPostconditionRemote, plan.Remote)
	assert.Equal(t, matchExact, plan.Match)

	sha := strings.Repeat("AB", 20)
	plan, err = planRaw(t, `{"ref":"refs/heads/기능+fix","expected_commit":"`+sha+`","push_match":"Contains","remote":"upstream"}`, "")
	require.NoError(t, err)
	assert.Equal(t, "refs/heads/기능+fix", plan.Ref)
	assert.Equal(t, strings.ToLower(sha), plan.ExpectedCommit)
	assert.Equal(t, matchContains, plan.Match)
	assert.Equal(t, "upstream", plan.Remote)
}

// REQ-WPOST-INPUT-03: a present contract switches prompt inference off, and
// only an absent key selects the fallback.
func TestPlanContract_OnlyAnAbsentKeyFallsBack(t *testing.T) {
	t.Parallel()

	plan, err := planRaw(t, `{"required_actions":[]}`, "commit and git push origin feature/x")
	require.NoError(t, err)
	assert.Equal(t, levelNone, plan.Commit)
	assert.Equal(t, levelNone, plan.Push)
	assert.Empty(t, plan.Branches)

	plan, err = planPayloadPostconditions(nil, "commit and git push origin feature/x")
	require.NoError(t, err)
	assert.Equal(t, sourceLegacy, plan.Source)
	assert.Equal(t, levelRequired, plan.Commit)

	var msg taskPayloadMessage
	require.NoError(t, json.Unmarshal([]byte(`{"prompt":"p","completion_contract":null}`), &msg))
	_, err = planPayloadPostconditions(msg.CompletionContract, msg.Prompt)
	require.Error(t, err, "an explicit null must not fall back to prompt inference")
	assert.Contains(t, err.Error(), "got null")
}

// The branch rules match `git check-ref-format --branch` both ways.
func TestValidBranchName_MatchesGitCheckRefFormat(t *testing.T) {
	t.Parallel()

	names := []string{
		"feature/x", "feature+fix", "기능/한글", "feature.lock/x", "x/y.lock", "a..b", ".hidden", "a/.b", "trailing.",
		"trailing/", "/lead", "a//b", "-dash", "HEAD", "@", "a@{b", "a b", "a~b", "a^b", "a:b", "a?b", "a*b", "a[b",
		`a\b`, "tab\tx", "release/1.0", "v1@x", "@x", "x.lock", "a.lock.b", "a./b", "del\x7fx", "a,b", "x/@", "é",
	}
	env := hermeticGitEnv(t)
	dir := t.TempDir()
	for _, name := range names {
		cmd := exec.Command("git", "check-ref-format", "--branch", name)
		cmd.Dir, cmd.Env = dir, env
		gitValid := cmd.Run() == nil
		assert.Equal(t, gitValid, validBranchName(name), "%q: git says valid=%v", name, gitValid)
	}
}
