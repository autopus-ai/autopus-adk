package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	qascenario "github.com/insajin/autopus-adk/pkg/qa/scenario"
)

// scenarioProject builds the minimum a compile needs: a GUI Journey Pack to
// inherit the origin from, and the capture fixture the generated spec imports.
func scenarioProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".autopus", "qa", "journeys"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".autopus", "qa", "capture"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".autopus", "qa", "capture", "autopus-capture.fixture.cjs"),
		[]byte("module.exports = {};\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "playwright.config.ts"),
		[]byte("export default { testDir: './tests' };\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".autopus", "qa", "journeys", "browser-gui-explore.yaml"),
		[]byte(`id: browser-gui-explore
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
`), 0o644))
	return dir
}

func runQACmd(t *testing.T, args ...string) (map[string]any, error) {
	t.Helper()
	cmd := newQACmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	if out.Len() == 0 {
		return nil, err
	}
	return decodeJSONMap(t, out.Bytes()), err
}

// The scenario namespace is the authoring entrypoint; losing its registration
// would leave scenarios uncompilable with no error anywhere.
func TestQAScenarioCmd_IsRegisteredUnderQA(t *testing.T) {
	t.Parallel()
	var found []string
	for _, sub := range newQACmd().Commands() {
		found = append(found, sub.Name())
	}
	assert.Contains(t, found, "scenario")
	assert.Contains(t, found, "loop")
	assert.Contains(t, found, "go")
	var leaves []string
	for _, sub := range newQAScenarioCmd().Commands() {
		leaves = append(leaves, sub.Name())
	}
	assert.ElementsMatch(t, []string{"init", "compile", "generate", "promote"}, leaves)
}

func TestQAScenarioInit_InheritsJourneyAndNeverOverwrites(t *testing.T) {
	t.Parallel()
	dir := scenarioProject(t)

	payload, err := runQACmd(t, "scenario", "init", "--project-dir", dir, "--format", "json")
	require.NoError(t, err)
	data := payload["data"].(map[string]any)
	assert.Equal(t, true, data["created"])
	assert.Equal(t, "browser-gui-explore", data["journey"])
	starter := filepath.Join(dir, qascenario.DirRel, qascenario.StarterID+".yaml")
	require.FileExists(t, starter)

	require.NoError(t, os.WriteFile(starter, []byte("authored: true\n"), 0o644))
	payload, err = runQACmd(t, "scenario", "init", "--project-dir", dir, "--format", "json")
	require.NoError(t, err)
	assert.Equal(t, false, payload["data"].(map[string]any)["created"])
	body, err := os.ReadFile(starter)
	require.NoError(t, err)
	assert.Equal(t, "authored: true\n", string(body), "authored assertions must survive a re-init")
}

const cliScenario = `schema_version: qamesh.scenario.v1
id: guest
title: Guest lands
journey: browser-gui-explore
screens:
  - id: landing
    path: /
    steps:
      - expect_role:
          role: heading
          name: Catalog
`

func TestQAScenarioCompile_WritesIntoDetectedTestDir(t *testing.T) {
	t.Parallel()
	dir := scenarioProject(t)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, qascenario.DirRel), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, qascenario.DirRel, "guest.yaml"), []byte(cliScenario), 0o644))

	payload, err := runQACmd(t, "scenario", "compile", "--project-dir", dir, "--format", "json")
	require.NoError(t, err)
	data := payload["data"].(map[string]any)
	assert.Equal(t, "tests", data["test_dir"])
	assert.Equal(t, "tests/autopus-generated", data["spec_dir"])

	spec := filepath.Join(dir, "tests", "autopus-generated", "guest.spec.ts")
	require.FileExists(t, spec)
	body, err := os.ReadFile(spec)
	require.NoError(t, err)
	// Origin comes from the pack, so a compiled spec can only navigate where the
	// guard already allows.
	assert.Contains(t, string(body), `const ORIGIN = "http://127.0.0.1:4173";`)
	assert.Contains(t, string(body), qascenario.ExploreTag)

	matrix := data["screen_matrix"].(map[string]any)["browser-gui-explore"].([]any)
	assert.Len(t, matrix, 1)
}

func TestQAScenarioCompile_DryRunWritesNothing(t *testing.T) {
	t.Parallel()
	dir := scenarioProject(t)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, qascenario.DirRel), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, qascenario.DirRel, "guest.yaml"), []byte(cliScenario), 0o644))

	payload, err := runQACmd(t, "scenario", "compile", "--project-dir", dir, "--dry-run", "--format", "json")
	require.NoError(t, err)
	assert.Equal(t, true, payload["data"].(map[string]any)["dry_run"])
	assert.NoDirExists(t, filepath.Join(dir, "tests", "autopus-generated"))
}

func TestQAScenarioCompile_FailsClosedWithErrorCode(t *testing.T) {
	t.Parallel()
	dir := scenarioProject(t)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, qascenario.DirRel), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, qascenario.DirRel, "bad.yaml"),
		[]byte("schema_version: qamesh.scenario.v1\nid: bad\ntitle: t\njourney: browser-gui-explore\nscreens:\n  - id: s\n    path: /\n    steps:\n      - click: '#pay'\n"), 0o644))

	payload, err := runQACmd(t, "scenario", "compile", "--project-dir", dir, "--format", "json")
	require.Error(t, err)
	assert.Equal(t, "error", payload["status"])
	assert.Equal(t, "qa_scenario_compile_failed", payload["error"].(map[string]any)["code"])
}

// A compile with no scenarios must say so rather than report success over an
// empty set, which would read as "coverage compiled" with zero coverage.
func TestQAScenarioCompile_RejectsEmptyScenarioSet(t *testing.T) {
	t.Parallel()
	dir := scenarioProject(t)
	_, err := runQACmd(t, "scenario", "compile", "--project-dir", dir, "--format", "json")
	require.Error(t, err)
}
