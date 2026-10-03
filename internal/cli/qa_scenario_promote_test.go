package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const qaPromoteRecording = `schema_version: qamesh.scenario.v2
id: recorded-checkout
title: Recorded checkout
journey: browser-gui-explore
intent_source: recording
recording_ref: .autopus/qa/recordings/checkout.jsonl
screens:
  - id: cart
    path: /cart
    steps:
      - click: {role: button, name: Checkout}
        by: agent
      - expect_text: Order placed
        by: agent
        confirm: required
`

func qaPromoteHome(text string) string {
	return "schema_version: qamesh.scenario.v1\nid: home\ntitle: Home\njourney: browser-gui-explore\n" +
		"screens:\n  - id: home\n    path: /\n    steps:\n      - expect_text: " + text + "\n"
}

// qaPromoteProject holds the three AC-QALOOP-007 candidates: a valid acceptance
// scenario, a recording with an unconfirmed agent assertion, and a candidate
// whose file clashes with a different active one.
func qaPromoteProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	candidates := filepath.Join(dir, ".autopus", "qa", "scenarios", "candidates")
	writeQAGenFile(t, filepath.Join(dir, ".autopus", "specs", "SPEC-X", "acceptance.md"), qaGenAcceptance)
	writeQAGenFile(t, filepath.Join(candidates, "sign-in-dashboard.yaml"), qaGenScenario)
	writeQAGenFile(t, filepath.Join(candidates, "recorded-checkout.yaml"), qaPromoteRecording)
	writeQAGenFile(t, filepath.Join(candidates, "home.yaml"), qaPromoteHome("Hi"))
	writeQAGenFile(t, filepath.Join(dir, ".autopus", "qa", "scenarios", "home.yaml"), qaPromoteHome("Hello"))
	return dir
}

func qaPromoteIDs(t *testing.T, list any) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, raw := range list.([]any) {
		entry := raw.(map[string]any)
		code, _ := entry["code"].(string)
		out[entry["id"].(string)] = code
	}
	return out
}

// AC-QALOOP-007 through the CLI.
func TestQAScenarioPromote_All_GatesThenAcceptsAgentAssertions(t *testing.T) {
	t.Parallel()
	dir := qaPromoteProject(t)
	active := filepath.Join(dir, ".autopus", "qa", "scenarios")

	out, err := execQAScenarioSubcmd(t, newQAScenarioPromoteCmd(), "--all", "--project-dir", dir, "--format", "json")
	require.NoError(t, err, out)
	envelope := decodeQAEnvelope(t, out)
	assert.Equal(t, "warn", envelope["status"])
	data := envelope["data"].(map[string]any)
	assert.Equal(t, map[string]string{"sign-in-dashboard": ""}, qaPromoteIDs(t, data["promoted"]))
	assert.Equal(t, map[string]string{
		"recorded-checkout": "qa_promote_unconfirmed_agent_assertion",
		"home":              "qa_promote_conflict",
	}, qaPromoteIDs(t, data["skipped"]))
	assert.FileExists(t, filepath.Join(active, "sign-in-dashboard.yaml"))

	out, err = execQAScenarioSubcmd(t, newQAScenarioPromoteCmd(), "--all", "--accept-agent-assertions", "--project-dir", dir)
	require.NoError(t, err, out)
	assert.Contains(t, out, "promoted scenario recorded-checkout: .autopus/qa/scenarios/candidates/recorded-checkout.yaml -> "+
		".autopus/qa/scenarios/recorded-checkout.yaml (accepted 1 agent assertion(s))\n")
	assert.Contains(t, out, "skipped home: qa_promote_conflict: ")
	assert.Contains(t, out, "next: auto qa scenario compile --project-dir "+dir)
	recorded, err := os.ReadFile(filepath.Join(active, "recorded-checkout.yaml"))
	require.NoError(t, err)
	assert.NotContains(t, string(recorded), "confirm")

	home, err := os.ReadFile(filepath.Join(active, "home.yaml"))
	require.NoError(t, err)
	assert.Equal(t, qaPromoteHome("Hello"), string(home), "the clash is never overwritten")
}

func TestQAScenarioPromote_NamedCandidateLeftBehind_FailsTheCommand(t *testing.T) {
	t.Parallel()
	dir := qaPromoteProject(t)

	out, err := execQAScenarioSubcmd(t, newQAScenarioPromoteCmd(), "recorded-checkout", "--project-dir", dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "qa_promote_incomplete")
	assert.Contains(t, out, "skipped recorded-checkout: qa_promote_unconfirmed_agent_assertion: ")
	assert.Contains(t, out, "hint: review the agent assertions, then re-run with --accept-agent-assertions\n")

	out, err = execQAScenarioSubcmd(t, newQAScenarioPromoteCmd(), "--project-dir", dir, "--format", "json")
	require.Error(t, err)
	assert.Equal(t, "qa_promote_selection_invalid", decodeQAEnvelope(t, out)["error"].(map[string]any)["code"])

	out, err = execQAScenarioSubcmd(t, newQAScenarioPromoteCmd(), "sign-in-dashboard", "--dry-run", "--project-dir", dir)
	require.NoError(t, err, out)
	assert.Contains(t, out, "would promote scenario sign-in-dashboard: ")
	assert.NoFileExists(t, filepath.Join(dir, ".autopus", "qa", "scenarios", "sign-in-dashboard.yaml"))
}
