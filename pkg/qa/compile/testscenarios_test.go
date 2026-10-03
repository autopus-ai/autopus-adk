package compile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/insajin/autopus-adk/pkg/qa/testscenario"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// acQALOOP005Doc is the AC-QALOOP-005 fixture: one allowlisted command case,
// one curl case, one gui case, and one manual case.
const acQALOOP005Doc = `schema_version: qamesh.test-scenarios.v1
spec: SPEC-X
cases:
  - id: unit-suite
    ac: AC-X-001
    kind: happy
    title: unit suite passes
    given: the module builds
    when: the suite runs
    then: it exits 0
    automation:
      type: command
      check:
        argv: ["go", "test", "./..."]
  - id: health-probe
    ac: AC-X-002
    kind: negative
    title: health endpoint answers
    automation:
      type: command
      check:
        argv: ["curl", "-fsS", "http://127.0.0.1:3000/health"]
  - id: login-flow
    ac: AC-X-003
    kind: happy
    title: user logs in
    automation:
      type: gui
      scenario: login
  - id: visual-review
    ac: AC-X-004
    kind: edge
    title: layout at 320px
    automation:
      type: manual
      reason: needs a human eye
`

func writeTestScenarioFile(t *testing.T, projectDir, rel, name, body string) {
	t.Helper()
	dir := filepath.Join(projectDir, rel)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
}

// commandCase renders a one-case document so each test states only the check.
func commandCase(spec, id, check string) string {
	return "schema_version: qamesh.test-scenarios.v1\nspec: " + spec + "\ncases:\n  - id: " + id +
		"\n    ac: AC-X-009\n    kind: edge\n    title: t\n    automation:\n      type: command\n      check:\n" + check
}

func TestFromProjectCompilesTestScenarios(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTestScenarioFile(t, dir, testscenario.DirRel, "SPEC-X.yaml", acQALOOP005Doc)

	candidates := FromProject(dir)

	// gui and manual cases are not commands, so only two candidates exist.
	require.Len(t, candidates, 2)
	goCase := candidates[0]
	assert.Equal(t, "test-scenarios", goCase.Source)
	assert.False(t, goCase.ManualOrDeferred)
	assert.Empty(t, goCase.ErrorCode)
	assert.Equal(t, "go-test", goCase.Adapter)
	assert.Equal(t, []string{"go", "test", "./..."}, goCase.Command)
	assert.Equal(t, []string{"AC-X-001"}, goCase.AcceptanceRefs)
	assert.Equal(t, ".", goCase.CWD)
	assert.Equal(t, "60s", goCase.Timeout)
	assert.Equal(t, "step-1", goCase.StepID)
	assert.Equal(t, "compiled-test-scenarios-spec-x-unit-suite-go-test", goCase.JourneyID)

	curlCase := candidates[1]
	assert.Equal(t, "test-scenarios", curlCase.Source)
	assert.True(t, curlCase.ManualOrDeferred)
	assert.Equal(t, "qa_test_scenario_command_not_allowlisted", curlCase.ErrorCode)
	assert.Equal(t, []string{"AC-X-002"}, curlCase.AcceptanceRefs)
	assert.Equal(t, "compiled-test-scenarios-spec-x-health-probe-custom-command", curlCase.JourneyID)
}

func TestFromProjectCarriesTestScenarioCheckSettings(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "web"), 0o755))
	npm := commandCase("SPEC-X", "web-suite", "        argv: [\"npm\", \"test\"]\n        cwd: web\n        timeout: 5m\n        env_allowlist: [\"CI\"]\n")
	pytest := commandCase("SPEC-Y", "py-suite", "        adapter: pytest\n        argv: [\"python\", \"-m\", \"pytest\"]\n")
	writeTestScenarioFile(t, dir, testscenario.DirRel, "SPEC-X.yaml", npm)
	writeTestScenarioFile(t, dir, testscenario.DirRel, "SPEC-Y.yaml", pytest)

	candidates := FromProject(dir)

	require.Len(t, candidates, 2)
	assert.False(t, candidates[0].ManualOrDeferred, candidates[0].ErrorCode)
	assert.Equal(t, "custom-command", candidates[0].Adapter)
	assert.Equal(t, "web", candidates[0].CWD)
	assert.Equal(t, "5m", candidates[0].Timeout)
	assert.Equal(t, []string{"CI"}, candidates[0].EnvAllowlist)
	// inferAdapter would say custom-command; pytest proves the check's adapter won.
	assert.False(t, candidates[1].ManualOrDeferred, candidates[1].ErrorCode)
	assert.Equal(t, "pytest", candidates[1].Adapter)
	assert.Equal(t, "compiled-test-scenarios-spec-y-py-suite-pytest", candidates[1].JourneyID)
}

func TestFromProjectReportsInvalidTestScenarioFileAlone(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	broken := strings.Replace(acQALOOP005Doc, "    kind: edge\n", "    kind: edge\n    owner: qa\n", 1)
	writeTestScenarioFile(t, dir, testscenario.DirRel, "SPEC-A.yaml", broken)
	writeTestScenarioFile(t, dir, testscenario.DirRel, "SPEC-B.yaml", commandCase("SPEC-B", "unit-suite", "        argv: [\"go\", \"test\", \"./...\"]\n"))

	candidates := FromProject(dir)

	require.Len(t, candidates, 2)
	assert.Equal(t, Candidate{
		JourneyID: "compiled-test-scenarios-spec-a", Source: "test-scenarios",
		ManualOrDeferred: true, ErrorCode: "qa_test_scenario_invalid",
	}, candidates[0])
	assert.False(t, candidates[1].ManualOrDeferred)
	assert.Equal(t, "compiled-test-scenarios-spec-b-unit-suite-go-test", candidates[1].JourneyID)
}

func TestFromProjectKeepsUnsafeTestScenarioTraceable(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTestScenarioFile(t, dir, testscenario.DirRel, "SPEC-X.yaml",
		commandCase("SPEC-X", "escape", "        argv: [\"go\", \"test\", \"./...\"]\n        cwd: ../outside\n"))

	candidates := FromProject(dir)

	require.Len(t, candidates, 1)
	assert.True(t, candidates[0].ManualOrDeferred)
	assert.Equal(t, "qa_compiler_cwd_outside_project", candidates[0].ErrorCode)
	assert.Equal(t, "test-scenarios", candidates[0].Source)
	assert.Equal(t, "compiled-test-scenarios-spec-x-escape-go-test", candidates[0].JourneyID)
	assert.Equal(t, []string{"AC-X-009"}, candidates[0].AcceptanceRefs)
}

func TestFromProjectIgnoresUnpromotedTestScenarioCandidates(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTestScenarioFile(t, dir, testscenario.CandidatesDirRel, "SPEC-X.yaml", acQALOOP005Doc)

	assert.Empty(t, FromProject(dir))
}

func TestFromProjectDefersDuplicateTestScenarioCase(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	same := commandCase("SPEC-X", "unit-suite", "        argv: [\"go\", \"test\", \"./...\"]\n")
	writeTestScenarioFile(t, dir, testscenario.DirRel, "SPEC-X.yaml", same)
	writeTestScenarioFile(t, dir, testscenario.DirRel, "SPEC-X-copy.yaml", same)

	candidates := FromProject(dir)

	// Two packs with one journey id would write the same manifest directory, so
	// the later file loses instead of silently overwriting the first result.
	require.Len(t, candidates, 2)
	assert.False(t, candidates[0].ManualOrDeferred)
	assert.True(t, candidates[1].ManualOrDeferred)
	assert.Equal(t, "qa_test_scenario_case_id_duplicate", candidates[1].ErrorCode)
	assert.Equal(t, candidates[0].JourneyID, candidates[1].JourneyID)
}
