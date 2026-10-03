package generate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/qa/acceptance"
	"github.com/insajin/autopus-adk/pkg/qa/agentexec"
)

// AC-QALOOP-006: valid documents become candidates, the document citing an
// unknown criterion is reported and never written, and coverage names every
// criterion.
func TestRun_AgentOutput_WritesOnlyDocumentsThatHoldUpAgainstCriteria(t *testing.T) {
	t.Parallel()
	dir := projectX(t, acceptanceX)
	writeFile(t, filepath.Join(dir, ".autopus", "qa", "scenarios", "legacy-home.yaml"), "not: a scenario\n")
	agent := &fakeRunner{stdout: fenced(validScenarioYAML, unknownAcScenarioYAML, testScenariosYAML)}

	report, err := Run(context.Background(), Options{ProjectDir: dir, SpecID: specX, Target: agentexec.TargetClaude, Runner: agent})
	require.NoError(t, err)

	assert.Equal(t, 1, agent.calls)
	assert.Equal(t, agentexec.ModeGenerate, agent.got.Mode)
	assert.Equal(t, agentexec.TargetClaude, agent.got.Target)
	assert.Equal(t, dir, agent.got.WorkDir)
	assert.Contains(t, agent.got.Prompt, "- AC-X-003: Idle sessions expire")
	assert.Contains(t, agent.got.Prompt, "legacy-home", "existing ids reach the prompt so the agent avoids them")

	assert.Equal(t, []string{
		".autopus/qa/scenarios/candidates/sign-in-dashboard.yaml",
		".autopus/qa/test-scenarios/candidates/SPEC-X.yaml",
	}, report.Written)
	assert.Equal(t, validScenarioYAML, readFile(t, filepath.Join(dir, ".autopus", "qa", "scenarios", "candidates", "sign-in-dashboard.yaml")))
	assert.Equal(t, testScenariosYAML, readFile(t, filepath.Join(dir, ".autopus", "qa", "test-scenarios", "candidates", "SPEC-X.yaml")))
	assert.NoFileExists(t, filepath.Join(dir, ".autopus", "qa", "scenarios", "candidates", "ghost-criterion.yaml"))

	require.Len(t, report.Rejected, 1)
	assert.Equal(t, Rejection{Index: 2, Kind: KindScenario, ID: "ghost-criterion", Code: CodeAcUnknown,
		Message: `ac "AC-X-999" is not a criterion of SPEC-X`}, report.Rejected[0])
	assert.Equal(t, 3, report.Documents)
	assert.Equal(t, 4, report.Criteria)
	assert.Equal(t, []acceptance.Problem{{ID: "AC-X-004", Code: acceptance.CodeMissingThen, Line: 21}}, report.Problems)
	assert.Equal(t, []CriterionCoverage{
		{ID: "AC-X-001", Status: CoveredByUserScenario, Refs: []string{"scenario:sign-in-dashboard", "case:sign-in-happy"}},
		{ID: "AC-X-002", Status: CoveredByCase, Refs: []string{"case:wrong-password"}},
		{ID: "AC-X-003", Status: Uncovered, Refs: []string{}},
		{ID: "AC-X-004", Status: CoveredByCase, Refs: []string{"case:vague-look-around"}},
	}, report.Coverage)
}

func TestRun_RefusesBeforeCallingTheAgent(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		accept   string
		spec     string
		wantCode string
	}{
		{"no criteria", "# SPEC-X\n\nNothing to check yet.\n", specX, CodeNoCriteria},
		{"no acceptance file", "", "SPEC-NONE", CodeAcceptanceMissing},
		{"no spec id", "", " ", CodeSpecMissing},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := projectX(t, tc.accept)
			agent := &fakeRunner{stdout: fenced(validScenarioYAML)}
			_, err := Run(context.Background(), Options{ProjectDir: dir, SpecID: tc.spec, Target: agentexec.TargetClaude, Runner: agent})
			require.Error(t, err)
			assert.Equal(t, tc.wantCode, ErrorCode(err))
			assert.Zero(t, agent.calls)
		})
	}
}

func TestRun_MissingAgentCLI_SurfacesSetupGapCode(t *testing.T) {
	t.Parallel()
	dir := projectX(t, acceptanceX)
	agent := &fakeRunner{err: &agentexec.SetupGapError{Code: agentexec.CodeCLIMissing, Binary: "claude"}}
	report, err := Run(context.Background(), Options{ProjectDir: dir, SpecID: specX, Target: agentexec.TargetClaude, Runner: agent})
	require.Error(t, err)
	assert.Equal(t, agentexec.CodeCLIMissing, ErrorCode(err))
	assert.Empty(t, report.Written)
	assert.NoDirExists(t, filepath.Join(dir, ".autopus", "qa", "scenarios", "candidates"))
}

func TestRun_AgentFindsGUIJourneys_PromptNamesThem(t *testing.T) {
	t.Parallel()
	dir := projectX(t, acceptanceX)
	writeFile(t, filepath.Join(dir, ".autopus", "qa", "journeys", "browser-gui-explore.yaml"), `id: browser-gui-explore
title: GUI exploration
surface: frontend
lanes: [gui-explore]
adapter:
  id: gui-explore
command:
  argv: ["npm", "exec", "playwright", "test"]
  cwd: .
  timeout: 120s
checks:
  - id: browser-gui-explore
    type: gui_exploration
    expected:
      exit_code: 0
gui:
  allowed_origins: ["http://127.0.0.1:4173"]
  forbidden_actions: [mutation]
  selector_strategy: role-first
  network_policy:
    mode: summary-only
source_refs:
  source_spec: SPEC-QAMESH-003
  acceptance_refs: [AC-1]
  owned_paths: ["tests/**"]
`)
	agent := &fakeRunner{stdout: "I could not draft anything."}
	report, err := Run(context.Background(), Options{ProjectDir: dir, SpecID: specX, Target: agentexec.TargetCodex, Runner: agent})
	require.NoError(t, err)
	assert.Contains(t, agent.got.Prompt, "- browser-gui-explore http://127.0.0.1:4173")
	assert.Zero(t, report.Documents)
	for _, row := range report.Coverage {
		assert.Equal(t, Uncovered, row.Status, row.ID)
	}
}

func TestWriteCandidates_DifferingCandidate_IsNeverOverwritten(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	stale := filepath.Join(dir, ".autopus", "qa", "test-scenarios", "candidates", "SPEC-X.yaml")
	writeFile(t, stale, "schema_version: qamesh.test-scenarios.v1\n# reviewed by hand\n")
	out := Outcome{
		Scenarios:     []Accepted{{ID: "sign-in-dashboard", Body: validScenarioYAML}},
		TestScenarios: []Accepted{{ID: specX, Body: testScenariosYAML}},
	}

	written, err := WriteCandidates(dir, out)
	require.Error(t, err)
	assert.Equal(t, CodeCandidateConflict, ErrorCode(err))
	assert.Contains(t, err.Error(), ".autopus/qa/test-scenarios/candidates/SPEC-X.yaml")
	assert.Equal(t, []string{".autopus/qa/scenarios/candidates/sign-in-dashboard.yaml"}, written)
	assert.Equal(t, "schema_version: qamesh.test-scenarios.v1\n# reviewed by hand\n", readFile(t, stale))

	// The same content again is a no-op, not a conflict.
	again, err := WriteCandidates(dir, Outcome{Scenarios: out.Scenarios})
	require.NoError(t, err)
	assert.Equal(t, written, again)
}

func TestWriteCandidates_UnsafeID_IsRefused(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, err := WriteCandidates(dir, Outcome{TestScenarios: []Accepted{{ID: "../escape", Body: "x\n"}}})
	require.Error(t, err)
	assert.Equal(t, CodeCandidateInvalid, ErrorCode(err))
	_, statErr := os.Stat(filepath.Join(dir, ".autopus", "qa", "escape.yaml"))
	assert.True(t, os.IsNotExist(statErr))
}

func TestErrorCode_UnknownError_IsGenerationFailure(t *testing.T) {
	t.Parallel()
	assert.Equal(t, CodeFailed, ErrorCode(errors.New("boom")))
}
