package scenario

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const v2LoginYAML = `schema_version: qamesh.scenario.v2
id: login
title: A member signs in
journey: browser-staging
intent_source: acceptance
spec: SPEC-AUTH-001
acceptance_refs: [AC-AUTH-001]
screens:
  - id: sign-in
    path: /login
    steps:
      - click:
          role: button
          name: Sign in
      - expect_text: Welcome back
        ac: AC-AUTH-001
`

// REQ-4: every compiled spec gets a sidecar, and the sidecar points at real
// lines of the spec written next to it.
func TestCompileProjectWritesStepMapSidecar(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeCaptureFixture(t, dir)
	writeJourneyPack(t, dir, "browser-staging", "http://127.0.0.1:4173")
	writeScenario(t, dir, "login.yaml", v2LoginYAML)
	writeScenario(t, dir, "alpha.yaml", scenarioYAML("alpha", "browser-staging", ""))

	result, err := CompileProject(dir, false)
	require.NoError(t, err)
	require.Len(t, result.Compiled, 2)
	login := result.Compiled[1]
	assert.Equal(t, "login", login.ScenarioID)
	assert.Equal(t, "e2e/"+GeneratedDirName+"/login.spec.map.json", login.StepMapPath)

	loaded, err := LoadStepMap(StepMapPath(SpecPath(dir, "login")))
	require.NoError(t, err)
	assert.Equal(t, login.SpecPath, loaded.SpecPath)
	assert.Equal(t, "SPEC-AUTH-001", loaded.Spec)
	spec, err := os.ReadFile(SpecPath(dir, "login"))
	require.NoError(t, err)
	lines := strings.Split(string(spec), "\n")
	kinds := map[string]string{}
	for index, line := range lines {
		ref, ok := loaded.Lookup(index + 1)
		if !ok {
			continue
		}
		kinds[ref.Kind] = line
		if ref.Kind == StepKindExpect {
			assert.Equal(t, "AC-AUTH-001", ref.Ac)
		}
	}
	assert.Contains(t, kinds[StepKindGoto], `page.goto(ORIGIN + "/login")`)
	assert.Contains(t, kinds[StepKindAction], ".click();")
	assert.Contains(t, kinds[StepKindExpect], "Welcome back")

	// A v1 spec is mapped too; its steps can only be gotos and expects.
	alpha, err := LoadStepMap(StepMapPath(SpecPath(dir, "alpha")))
	require.NoError(t, err)
	assert.Len(t, alpha.Lines, 5)
	for _, ref := range alpha.Lines {
		assert.NotEqual(t, StepKindAction, ref.Kind)
	}
	_, ok := alpha.Lookup(1)
	assert.False(t, ok, "header lines map to no step")
}

func TestCompileProjectDryRunWritesNoStepMap(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeCaptureFixture(t, dir)
	writeJourneyPack(t, dir, "browser-staging", "http://127.0.0.1:4173")
	writeScenario(t, dir, "login.yaml", v2LoginYAML)

	result, err := CompileProject(dir, true)
	require.NoError(t, err)
	require.Len(t, result.Compiled, 1)
	assert.NotEmpty(t, result.Compiled[0].StepMapPath)
	assert.NoFileExists(t, StepMapPath(SpecPath(dir, "login")))
}

// A loosely decoded map could send triage to the wrong step, so anything off
// shape is refused with the step-map reason code.
func TestLoadStepMapRejectsMalformedMaps(t *testing.T) {
	t.Parallel()
	valid := `{"schema_version":"qamesh.stepmap.v1","scenario_id":"a","lines":{"12":{"screen":"s","index":1,"kind":"action"}}}`
	for name, body := range map[string]string{
		"unknown field":  strings.Replace(valid, `"scenario_id"`, `"extra":1,"scenario_id"`, 1),
		"foreign schema": strings.Replace(valid, "qamesh.stepmap.v1", "qamesh.stepmap.v9", 1),
		"word key":       strings.Replace(valid, `"12"`, `"twelve"`, 1),
		"padded key":     strings.Replace(valid, `"12"`, `"012"`, 1),
		"unknown kind":   strings.Replace(valid, `"action"`, `"hover"`, 1),
	} {
		path := filepath.Join(t.TempDir(), "a.spec.map.json")
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
		_, err := LoadStepMap(path)
		assert.Equal(t, "qa_scenario_stepmap_invalid", reasonCode(t, err), name)
	}

	path := filepath.Join(t.TempDir(), "a.spec.map.json")
	require.NoError(t, os.WriteFile(path, []byte(valid), 0o644))
	loaded, err := LoadStepMap(path)
	require.NoError(t, err)
	ref, ok := loaded.Lookup(12)
	assert.True(t, ok)
	assert.Equal(t, StepRef{Screen: "s", Index: 1, Kind: StepKindAction}, ref)
	_, ok = loaded.Lookup(13)
	assert.False(t, ok)
	assert.Equal(t, "e2e/x/a.spec.map.json", StepMapPath("e2e/x/a.spec.ts"))
}

// Candidates live under the scenario directory but are never compiled: LoadDir
// stays non-recursive, and LoadCandidates applies the same strictness.
func TestLoadCandidatesIsSeparateFromActiveScenarios(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	missing, err := LoadCandidates(dir)
	require.NoError(t, err)
	assert.Empty(t, missing)

	writeScenario(t, dir, "alpha.yaml", fmtScenario("alpha"))
	candidates := filepath.Join(dir, CandidatesDirRel)
	require.NoError(t, os.MkdirAll(candidates, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(candidates, "login.yaml"), []byte(v2LoginYAML), 0o644))

	active, err := LoadDir(dir)
	require.NoError(t, err)
	require.Len(t, active, 1)
	assert.Equal(t, "alpha", active[0].ID)

	pending, err := LoadCandidates(dir)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Equal(t, "login", pending[0].ID)
	assert.Equal(t, "login.yaml", pending[0].Path)
	assert.Equal(t, filepath.Join(dir, CandidatesDirRel), CandidatesDir(dir))

	require.NoError(t, os.WriteFile(filepath.Join(candidates, "broken.yaml"),
		[]byte(strings.Replace(v2LoginYAML, "        ac: AC-AUTH-001\n", "", 1)), 0o644))
	_, err = LoadCandidates(dir)
	assert.Equal(t, "qa_scenario_step_ac_missing", reasonCode(t, err))
}
