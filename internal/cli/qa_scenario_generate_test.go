package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const qaGenAcceptance = `# SPEC-X — Acceptance

### S1: AC-X-001 — Sign in reaches the dashboard

GIVEN a registered user on the sign-in page
WHEN they submit valid credentials
THEN the dashboard greets them with "Welcome back".

### S2: AC-X-002 — Wrong password is refused

GIVEN a registered user
WHEN they submit a wrong password
THEN the form shows "Invalid credentials".

### S3: AC-X-003 — Idle sessions expire

GIVEN a signed-in user
WHEN the session has been idle for 30 minutes
THEN they are asked to sign in again.
`

const qaGenScenario = `schema_version: qamesh.scenario.v2
id: sign-in-dashboard
title: Sign in reaches the dashboard
journey: browser-gui-explore
intent_source: acceptance
spec: SPEC-X
acceptance_refs: [AC-X-001]
screens:
  - id: sign-in
    path: /login
    steps:
      - fill: {label: Password, value_env: E2E_PASSWORD}
      - click: {role: button, name: Sign in}
      - expect_text: Welcome back
        ac: AC-X-001
`

const qaGenTestScenarios = `schema_version: qamesh.test-scenarios.v1
spec: SPEC-X
cases:
  - {id: sign-in-happy, ac: AC-X-001, kind: happy, title: t, given: g, when: w, then: th, automation: {type: gui, scenario: sign-in-dashboard}}
  - {id: wrong-password, ac: AC-X-002, kind: negative, title: t, given: g, when: w, then: th, automation: {type: command, check: {argv: [go, test, ./auth/...]}}}
`

// qaGenProject writes SPEC-X and a fake agent CLI that drains the prompt into
// prompt.txt and prints one valid scenario, one citing an unknown criterion, and
// one valid test-scenarios document. AUTOPUS_QA_AGENT_ARGV points at it.
func qaGenProject(t *testing.T) (dir, promptPath string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake agent CLI is a POSIX shell script")
	}
	dir = t.TempDir()
	scratch := t.TempDir()
	writeQAGenFile(t, filepath.Join(dir, ".autopus", "specs", "SPEC-X", "acceptance.md"), qaGenAcceptance)
	ghost := strings.NewReplacer("id: sign-in-dashboard", "id: ghost-criterion", "AC-X-001", "AC-X-999").Replace(qaGenScenario)
	var stdout strings.Builder
	for _, doc := range []string{qaGenScenario, ghost, qaGenTestScenarios} {
		stdout.WriteString("Draft:\n```yaml\n" + doc + "```\n")
	}
	outputPath := filepath.Join(scratch, "agent-output.txt")
	promptPath = filepath.Join(scratch, "prompt.txt")
	writeQAGenFile(t, outputPath, stdout.String())
	script := filepath.Join(scratch, "fake-agent.sh")
	writeQAGenFile(t, script, "#!/bin/sh\ncat > '"+promptPath+"'\ncat '"+outputPath+"'\n")
	argv, err := json.Marshal([]string{"/bin/sh", script})
	require.NoError(t, err)
	t.Setenv("AUTOPUS_QA_AGENT_ARGV", string(argv))
	return dir, promptPath
}

func writeQAGenFile(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func execQAScenarioSubcmd(t *testing.T, cmd *cobra.Command, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func decodeQAEnvelope(t *testing.T, out string) map[string]any {
	t.Helper()
	var envelope map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &envelope), out)
	return envelope
}

// AC-QALOOP-006 through the CLI and a real subprocess.
func TestQAScenarioGenerate_FakeAgent_WritesValidCandidatesAndReportsCoverage(t *testing.T) {
	dir, promptPath := qaGenProject(t)

	out, err := execQAScenarioSubcmd(t, newQAScenarioGenerateCmd(), "--spec", "SPEC-X", "--agent", "claude", "--project-dir", dir)
	require.NoError(t, err, out)

	assert.Contains(t, out, "spec: SPEC-X  agent: claude  criteria: 3  documents: 3\n")
	assert.Contains(t, out, "candidate: .autopus/qa/scenarios/candidates/sign-in-dashboard.yaml\n")
	assert.Contains(t, out, "candidate: .autopus/qa/test-scenarios/candidates/SPEC-X.yaml\n")
	assert.Contains(t, out, `rejected: document 2 (scenario ghost-criterion): qa_generate_ac_unknown: ac "AC-X-999" is not a criterion of SPEC-X`)
	assert.Contains(t, out, "  AC-X-001  covered_by_user_scenario  scenario:sign-in-dashboard, case:sign-in-happy\n")
	assert.Contains(t, out, "  AC-X-002  covered_by_case           case:wrong-password\n")
	assert.Contains(t, out, "  AC-X-003  uncovered\n")
	assert.Contains(t, out, "next: review the candidates, then auto qa scenario promote --all --project-dir "+dir)

	written, err := os.ReadFile(filepath.Join(dir, ".autopus", "qa", "scenarios", "candidates", "sign-in-dashboard.yaml"))
	require.NoError(t, err)
	assert.Equal(t, qaGenScenario, string(written))
	assert.FileExists(t, filepath.Join(dir, ".autopus", "qa", "test-scenarios", "candidates", "SPEC-X.yaml"))
	assert.NoFileExists(t, filepath.Join(dir, ".autopus", "qa", "scenarios", "candidates", "ghost-criterion.yaml"))
	assert.NoFileExists(t, filepath.Join(dir, ".autopus", "qa", "scenarios", "sign-in-dashboard.yaml"), "candidates are not active")

	prompt, err := os.ReadFile(promptPath)
	require.NoError(t, err)
	assert.Contains(t, string(prompt), "- AC-X-003: Idle sessions expire\n")
}

func TestQAScenarioGenerate_JSON_ReturnsCoverageAndWarnings(t *testing.T) {
	dir, _ := qaGenProject(t)

	out, err := execQAScenarioSubcmd(t, newQAScenarioGenerateCmd(), "--spec", "SPEC-X", "--agent", "codex", "--project-dir", dir, "--format", "json")
	require.NoError(t, err, out)
	envelope := decodeQAEnvelope(t, out)
	assert.Equal(t, "warn", envelope["status"])
	data := envelope["data"].(map[string]any)
	assert.Equal(t, []any{"sign-in-dashboard"}, data["scenarios"])
	assert.Equal(t, []any{"SPEC-X"}, data["test_scenarios"])
	rejected := data["rejected"].([]any)
	require.Len(t, rejected, 1)
	assert.Equal(t, "qa_generate_ac_unknown", rejected[0].(map[string]any)["code"])
	coverage := data["coverage"].([]any)
	require.Len(t, coverage, 3)
	assert.Equal(t, "uncovered", coverage[2].(map[string]any)["status"])
	warnings := envelope["warnings"].([]any)
	assert.Equal(t, "qa_generate_documents_rejected", warnings[0].(map[string]any)["code"])
}

func TestQAScenarioGenerate_JSON_ErrorsCarryStableCodes(t *testing.T) {
	dir, _ := qaGenProject(t)
	cases := []struct {
		name     string
		argv     string
		args     []string
		wantCode string
	}{
		{"agent CLI missing", `["autopus-no-such-agent-cli"]`, []string{"--spec", "SPEC-X", "--agent", "claude"}, "qa_agent_cli_missing"},
		{"unknown agent", "", []string{"--spec", "SPEC-X", "--agent", "nope"}, "qa_agent_target_unknown"},
		{"no spec flag", "", []string{"--agent", "claude"}, "qa_generate_spec_missing"},
		{"spec without acceptance", "", []string{"--spec", "SPEC-NONE", "--agent", "claude"}, "qa_generate_acceptance_missing"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.argv != "" {
				t.Setenv("AUTOPUS_QA_AGENT_ARGV", tc.argv)
			}
			args := append(append([]string{}, tc.args...), "--project-dir", dir, "--format", "json")
			out, err := execQAScenarioSubcmd(t, newQAScenarioGenerateCmd(), args...)
			require.Error(t, err)
			envelope := decodeQAEnvelope(t, out)
			assert.Equal(t, "error", envelope["status"])
			assert.Equal(t, tc.wantCode, envelope["error"].(map[string]any)["code"])
		})
	}
	assert.NoDirExists(t, filepath.Join(dir, ".autopus", "qa", "scenarios", "candidates"))
}
