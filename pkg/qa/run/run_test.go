package run

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/insajin/autopus-adk/pkg/qa/journey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteDryRunDoesNotCreateRunDirectory(t *testing.T) {
	t.Parallel()

	dir := fixtureGoProject(t, true)
	output := filepath.Join(dir, "runs")

	result, err := Execute(Options{ProjectDir: dir, Lane: "fast", Output: output, DryRun: true})

	require.NoError(t, err)
	assert.True(t, result.DryRun)
	assert.Empty(t, result.RunIndexPath)
	assert.NoDirExists(t, output)
}

func TestExecuteGoJourneyWritesManifestAndRunIndex(t *testing.T) {
	// Serial: see writeGoProjectFixture.
	dir := fixtureGoProject(t, true)
	output := filepath.Join(dir, "runs")

	result, err := Execute(Options{ProjectDir: dir, Lane: "fast", Output: output})

	require.NoError(t, err)
	assert.Equal(t, "passed", result.Status)
	require.Len(t, result.ManifestPaths, 1)
	// Published refs are project-root-relative; resolve them the way a consumer
	// does rather than assuming an absolute leak.
	manifestPath := filepath.Join(dir, result.ManifestPaths[0])
	runIndexPath := filepath.Join(dir, result.RunIndexPath)
	assert.False(t, filepath.IsAbs(result.RunIndexPath))
	assert.FileExists(t, manifestPath)
	assert.FileExists(t, runIndexPath)
	assert.Equal(t, "go-test", result.AdapterResults[0].Adapter)

	body, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	assert.Contains(t, string(body), `"schema_version": "qamesh.evidence.v2"`)
	assert.Contains(t, string(body), `"journey_id": "go-unit"`)

	var index Index
	indexBody, err := os.ReadFile(runIndexPath)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(indexBody, &index))
	assert.Equal(t, filepath.Base(dir), index.Workspace.WorkspaceID)
	assert.Equal(t, filepath.Base(dir), index.Workspace.RepoID)
	assert.Equal(t, ".", index.Workspace.RepoRoot)
	assert.Contains(t, index.SourceRefs, "qamesh://source/"+filepath.Base(dir)+"/specs/SPEC-QAMESH-002")
}

func TestExecuteFailedJourneyWritesFeedback(t *testing.T) {
	// Serial: see writeGoProjectFixture.
	dir := fixtureGoProject(t, false)
	output := filepath.Join(dir, "runs")

	result, err := Execute(Options{ProjectDir: dir, Lane: "fast", Output: output, FeedbackTo: "codex"})

	require.Error(t, err)
	assert.Equal(t, "failed", result.Status)
	require.Len(t, result.ManifestPaths, 1)
	require.Len(t, result.FeedbackBundlePaths, 1)
	assert.True(t, result.AdapterResults[0].RepairPromptAvailable)
}

func TestExecuteRejectsUnsupportedFeedbackTargetBeforeWriting(t *testing.T) {
	t.Parallel()

	dir := fixtureGoProject(t, false)
	output := filepath.Join(dir, "runs")

	result, err := Execute(Options{ProjectDir: dir, Lane: "fast", Output: output, FeedbackTo: "unsupported"})

	require.Error(t, err)
	assert.Empty(t, result.RunID)
	assert.NoDirExists(t, output)
}

func TestExecuteProfileRequirementBecomesSetupGap(t *testing.T) {
	t.Parallel()

	dir := fixtureGoProject(t, true)
	journeyPath := filepath.Join(dir, ".autopus", "qa", "journeys", "go-unit.yaml")
	body := []byte(`id: go-unit
title: Go unit
surface: cli
lanes: [fast]
adapter:
  id: go-test
command:
  run: go test ./...
  cwd: .
  timeout: 60s
checks:
  - id: go-test
    type: unit_test
artifacts:
  - root: .autopus/qa/runs
source_refs:
  source_spec: SPEC-QAMESH-002
  acceptance_refs: [AC-QAMESH2-005]
  owned_paths: ["."]
  do_not_modify_paths: [".codex/**"]
profile_requirements:
  capabilities: [frontend-server]
`)
	require.NoError(t, os.WriteFile(journeyPath, body, 0o644))

	result, err := Execute(Options{ProjectDir: dir, Profile: "standalone", Lane: "fast", Output: filepath.Join(dir, "runs")})

	require.NoError(t, err)
	assert.Equal(t, "warning", result.Status)
	require.Len(t, result.SetupGaps, 1)
	assert.Contains(t, result.SetupGaps[0].Reason, "frontend-server")
	assert.Empty(t, result.ManifestPaths)
}

func TestExecuteUnavailableFrontendAdapterIsSkipped(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	journeyDir := filepath.Join(dir, ".autopus", "qa", "journeys")
	require.NoError(t, os.MkdirAll(journeyDir, 0o755))
	body := []byte(`id: ui-vitest
title: UI vitest
surface: frontend
lanes: [fast]
adapter:
  id: vitest
checks:
  - id: vitest
    type: unit_test
`)
	require.NoError(t, os.WriteFile(filepath.Join(journeyDir, "ui-vitest.yaml"), body, 0o644))

	result, err := Execute(Options{ProjectDir: dir, Lane: "fast", Output: filepath.Join(dir, "runs")})

	require.NoError(t, err)
	assert.Equal(t, "warning", result.Status)
	require.Len(t, result.SetupGaps, 1)
	assert.Equal(t, "vitest", result.SetupGaps[0].Adapter)
	assert.Empty(t, result.ManifestPaths)
}

func TestBuildPlanUsesDetectedFallbackWhenNoJourneyPackExists(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.test\n"), 0o644))

	plan, err := BuildPlan(Options{ProjectDir: dir, Lane: "fast", Output: filepath.Join(dir, "runs")})

	require.NoError(t, err)
	assert.Contains(t, plan.DetectedAdapters, "go-test")
	assert.Contains(t, plan.SelectedJourneys, "detected-go-test")
	assert.Contains(t, plan.SelectedAdapters, "go-test")
	assert.NotEmpty(t, plan.ManifestOutputPreviewPaths)
}

func TestExecuteUsesCompiledScenarioCandidate(t *testing.T) {
	// Serial: see writeGoProjectFixture.
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.test\n\ngo 1.26\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "example_test.go"), []byte("package example\n\nimport \"testing\"\n\nfunc TestExample(t *testing.T) {}\n"), 0o644))
	projectDir := filepath.Join(dir, ".autopus", "project")
	require.NoError(t, os.MkdirAll(projectDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "scenarios.md"), []byte("- command: `go test ./...`\n"), 0o644))

	result, err := Execute(Options{ProjectDir: dir, Lane: "fast", Output: filepath.Join(dir, "runs")})

	require.NoError(t, err)
	assert.Contains(t, result.SelectedJourneys, "compiled-scenario-go-test")
	require.Len(t, result.ManifestPaths, 1)
}

func TestExecutePreservesFailedCheckActualInRunIndex(t *testing.T) {
	// Serial: see writeGoProjectFixture.
	dir := fixtureGoProject(t, false)
	result, err := Execute(Options{ProjectDir: dir, Lane: "fast", Output: filepath.Join(dir, "runs")})

	require.Error(t, err)
	require.Len(t, result.Checks, 1)
	assert.Equal(t, "go-test", result.Checks[0].ID)
	assert.Contains(t, result.Checks[0].Actual, "exit_code=1")
	body, readErr := os.ReadFile(filepath.Join(dir, result.RunIndexPath))
	require.NoError(t, readErr)
	assert.Contains(t, string(body), `"actual": "exit_code=1"`)
}

func TestBuildPlanRejectsGeneratedSurfaceOutput(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	_, err := BuildPlan(Options{ProjectDir: dir, Output: filepath.Join(dir, ".codex", "qa")})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "generated surface")
}

func TestExecuteUsesDetectedFallbackWhenNoJourneyPackExists(t *testing.T) {
	// Serial: see writeGoProjectFixture.
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.test\n\ngo 1.26\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "example_test.go"), []byte("package example\n\nimport \"testing\"\n\nfunc TestExample(t *testing.T) {}\n"), 0o644))

	result, err := Execute(Options{ProjectDir: dir, Lane: "fast", Output: filepath.Join(dir, "runs")})

	require.NoError(t, err)
	assert.Equal(t, "passed", result.Status)
	assert.Contains(t, result.SelectedJourneys, "detected-go-test")
	require.Len(t, result.ManifestPaths, 1)
}

func TestBuildPlanFiltersConfiguredJourneys(t *testing.T) {
	t.Parallel()

	dir := fixtureGoProject(t, true)
	plan, err := BuildPlan(Options{ProjectDir: dir, Lane: "fast", JourneyID: "missing", Output: filepath.Join(dir, "runs")})

	require.NoError(t, err)
	assert.Empty(t, plan.SelectedJourneys)
	assert.Empty(t, plan.SelectedAdapters)
}

func journeyPack(adapterID, run string) journey.Pack {
	return journey.Pack{
		ID:      "helper",
		Lanes:   []string{"fast"},
		Adapter: journey.AdapterRef{ID: adapterID},
		Command: journey.Command{Run: run, CWD: ".", Timeout: "60s"},
		Checks:  []journey.Check{{ID: "helper", Type: "unit_test"}},
	}
}

func fixtureGoProject(t *testing.T, passing bool) string {
	t.Helper()
	dir := t.TempDir()
	writeGoProjectFixture(t, dir, passing)
	return dir
}

// writeGoProjectFixture is split out so a test can place the fixture under a
// specific parent (an absolute /Users path, /tmp, a symlinked $TMPDIR) instead
// of always taking t.TempDir().
//
// Executing this fixture runs a real `go test` under a fresh, empty GOCACHE,
// so every run compiles the testing package and its std dependencies from
// scratch (~18 CPU-seconds). Tests that execute it stay serial: run in
// parallel on a small CI runner they starve each other past the command's
// 60s budget and the run reports "blocked" instead of its real status.
func writeGoProjectFixture(t *testing.T, dir string, passing bool) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.test\n\ngo 1.26\n"), 0o644))
	testBody := "package example\n\nimport \"testing\"\n\nfunc TestExample(t *testing.T) {}\n"
	if !passing {
		testBody = "package example\n\nimport \"testing\"\n\nfunc TestExample(t *testing.T) { t.Fatal(\"boom\") }\n"
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "example_test.go"), []byte(testBody), 0o644))
	journeyDir := filepath.Join(dir, ".autopus", "qa", "journeys")
	require.NoError(t, os.MkdirAll(journeyDir, 0o755))
	pack := map[string]any{
		"id":      "go-unit",
		"title":   "Go unit",
		"surface": "cli",
		"lanes":   []string{"fast"},
		"adapter": map[string]any{"id": "go-test"},
		"command": map[string]any{"run": "go test ./...", "cwd": ".", "timeout": "60s"},
		"checks":  []map[string]any{{"id": "go-test", "type": "unit_test"}},
		"artifacts": []map[string]any{
			{"root": ".autopus/qa/runs"},
		},
		"source_refs": map[string]any{
			"source_spec":         "SPEC-QAMESH-002",
			"acceptance_refs":     []string{"AC-QAMESH2-005"},
			"owned_paths":         []string{"."},
			"do_not_modify_paths": []string{".codex/**"},
		},
	}
	body, err := json.Marshal(pack)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(journeyDir, "go-unit.yaml"), body, 0o644))
}
