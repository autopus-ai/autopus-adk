package triage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeRunFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	return path
}

const loginManifest = `{
  "schema_version": "qamesh.evidence.v1",
  "qa_result_id": "qa-login-1",
  "surface": "browser",
  "lane": "fast",
  "scenario_ref": "browser:login",
  "runner": {"name": "playwright"},
  "status": "failed",
  "artifacts": [
    {"kind": "console", "path": "console.json", "publishable": true, "redaction": "text_redacted_and_scanned"},
    {"kind": "stdout", "path": "stdout.log", "publishable": true, "redaction": "text_redacted_and_scanned"}
  ],
  "source_refs": {"journey_id": "login-journey", "adapter": "playwright-cli"}
}`

func TestInputFromManifest_ReadsRunnerOutputAndLocatesTheReplayStep(t *testing.T) {
	t.Parallel()
	project := newGeneratedProject(t)
	runDir := filepath.Join(project, ".autopus", "qa", "runs", "run-1")
	writeRunFile(t, runDir, "stdout.log", driftOutput)
	// A refused third-party beacon in the console must not turn a drifted
	// locator into an environment verdict.
	writeRunFile(t, runDir, "console.json", `{"messages":["net::ERR_CONNECTION_REFUSED https://beacon.example"]}`)
	manifestPath := writeRunFile(t, runDir, "manifest.json", loginManifest)

	in, err := InputFromManifest(project, manifestPath)

	require.NoError(t, err)
	assert.Equal(t, "login-journey", in.JourneyID)
	assert.Equal(t, "playwright-cli", in.Adapter)
	assert.Equal(t, "failed", in.Status)
	assert.Equal(t, project, in.ProjectDir)
	assert.Nil(t, in.RerunPassed)
	assert.Contains(t, in.FailureText, "locator.click: Test timeout")
	assert.NotContains(t, in.FailureText, "beacon.example")
	assert.Equal(t, ClassTestDrift, Classify(in).Class)

	replay := ReplayFor(in)

	require.NotNil(t, replay)
	assert.Equal(t, "login", replay.ScenarioID)
	assert.Equal(t, generatedSpec, replay.SpecPath)
	assert.Equal(t, 6, replay.Line)
	assert.Equal(t, "login", replay.Screen)
	assert.Equal(t, 1, replay.Index)
	assert.Equal(t, "action", replay.Kind)
	assert.Equal(t, "AC-LOGIN-001", replay.Ac)
	assert.Equal(t, project, replay.ProjectDir)
	assert.Contains(t, replay.Excerpt, "Error: locator.click: Test timeout of 30000ms exceeded.")
	assert.Contains(t, replay.Excerpt, ">  6 |")
	assert.NotContains(t, replay.Excerpt, "Running 1 test", "the excerpt starts at the failure header")
}

func TestInputFromManifest_FallsBackToScenarioRefAndRunnerName(t *testing.T) {
	t.Parallel()
	runDir := t.TempDir()
	manifestPath := writeRunFile(t, runDir, "manifest.json", `{"schema_version":"qamesh.evidence.v1","scenario_ref":"browser:login","runner":{"name":"playwright"},"status":"blocked","artifacts":[]}`)

	in, err := InputFromManifest(runDir, manifestPath)

	require.NoError(t, err)
	assert.Equal(t, "browser:login", in.JourneyID)
	assert.Equal(t, "playwright", in.Adapter)
	assert.Empty(t, in.FailureText)
	assert.Equal(t, Verdict{JourneyID: "browser:login", Class: ClassEnvironment, Signal: "status:blocked"}, Classify(in))
}

func TestInputFromManifest_MissingManifestIsAnError(t *testing.T) {
	t.Parallel()

	_, err := InputFromManifest(t.TempDir(), filepath.Join(t.TempDir(), "absent.json"))

	assert.Error(t, err)
}

func TestReplayFor_UnlocatedFailureHasNoReplay(t *testing.T) {
	t.Parallel()
	project := newGeneratedProject(t)

	assert.Nil(t, ReplayFor(Input{ProjectDir: project, FailureText: "worker process exited unexpectedly"}))
	assert.Nil(t, ReplayFor(Input{ProjectDir: project, FailureText: generatedFailure("Error: boom", 3)}), "an unmapped line is not a step")
}
