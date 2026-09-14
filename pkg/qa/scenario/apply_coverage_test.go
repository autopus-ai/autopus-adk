package scenario

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeJourneyPack writes a Journey Pack whose GUI policy declares origins in
// the given order, so tests can pin which origin a compile inherits.
func writeJourneyPack(t *testing.T, dir, id string, origins ...string) {
	t.Helper()
	packDir := filepath.Join(dir, ".autopus", "qa", "journeys")
	require.NoError(t, os.MkdirAll(packDir, 0o755))
	body := "id: " + id + "\n" +
		"title: " + id + "\n" +
		"source_refs:\n  source_spec: SPEC-QAMESH-003\n  acceptance_refs: [AC-001]\n"
	if len(origins) == 0 {
		// A CLI pack carries no GUI policy, so its inherited origin is empty.
		body += "surface: cli\n" +
			"lanes: [fast]\n" +
			"adapter:\n  id: go-test\n" +
			"command:\n  argv: [\"go\", \"test\", \"./...\"]\n  cwd: .\n  timeout: 60s\n" +
			"checks:\n  - id: " + id + "-check\n    type: unit_test\n"
	} else {
		body += "surface: frontend\n" +
			"lanes: [gui-explore]\n" +
			"adapter:\n  id: gui-explore\n" +
			"command:\n  argv: [\"npm\", \"exec\", \"playwright\", \"test\"]\n  cwd: .\n  timeout: 60s\n" +
			"checks:\n  - id: " + id + "-check\n    type: gui_exploration\n" +
			"gui:\n  allowed_origins:\n"
		for _, origin := range origins {
			body += "    - " + origin + "\n"
		}
		body += "  forbidden_actions: [mutation, payment, email_send]\n" +
			"  selector_strategy: role-first\n" +
			"  network_policy:\n    mode: summary-only\n" +
			"  artifact_retention:\n    publish_raw: false\n"
	}
	require.NoError(t, os.WriteFile(filepath.Join(packDir, id+".yaml"), []byte(body), 0o644))
}

func writeCaptureFixture(t *testing.T, dir string) {
	t.Helper()
	path := filepath.Join(dir, FixtureRel)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("module.exports = {};\n"), 0o644))
}

func scenarioYAML(id, journey string, extra string) string {
	return `schema_version: qamesh.scenario.v1
id: ` + id + `
title: ` + id + `
journey: ` + journey + `
` + extra + `screens:
  - id: home
    path: /
    steps:
      - expect_text: hello
      - expect_title: Home
  - id: pricing
    path: /pricing
    steps:
      - expect_url: /pricing
`
}

// A compile with no scenarios is a no-op that would look like success, so it
// must name the directory the author was expected to author in.
func TestCompileProjectRejectsEmptyScenarioSet(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeCaptureFixture(t, dir)
	_, err := CompileProject(dir, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), DirRel)
}

// A spec importing a fixture that was never scaffolded fails at collection time
// with a module error, so the missing fixture is reported before compiling.
func TestCompileProjectRequiresCaptureFixture(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeScenario(t, dir, "a.yaml", scenarioYAML("alpha", "gui-explore", ""))
	writeJourneyPack(t, dir, "gui-explore", "http://127.0.0.1:4173")
	_, err := CompileProject(dir, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "auto qa init")
	assert.Contains(t, err.Error(), filepath.ToSlash(FixtureRel))
}

// Naming a journey that is not a pack in this project would compile a spec the
// guard never authorized, so the unknown id is rejected with the name given.
func TestCompileProjectRejectsUnknownJourney(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeCaptureFixture(t, dir)
	writeScenario(t, dir, "a.yaml", scenarioYAML("alpha", "nope", ""))
	writeJourneyPack(t, dir, "gui-explore", "http://127.0.0.1:4173")
	_, err := CompileProject(dir, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"nope"`)
	assert.Contains(t, err.Error(), "Journey Pack")
}

// A pack with no GUI policy yields an empty origin: the compile must fail on the
// missing origin rather than silently emit a spec with a relative base URL.
func TestCompileProjectRejectsJourneyWithoutOrigin(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeCaptureFixture(t, dir)
	writeScenario(t, dir, "a.yaml", scenarioYAML("alpha", "cli-only", ""))
	writeJourneyPack(t, dir, "cli-only")
	_, err := CompileProject(dir, true)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "not a Journey Pack")
	assert.Contains(t, strings.ToLower(err.Error()), "origin")
}

// An unparseable pack must stop the compile instead of degrading to "unknown
// journey", which would misattribute the failure to the scenario author.
func TestCompileProjectPropagatesJourneyPackFailure(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeCaptureFixture(t, dir)
	writeScenario(t, dir, "a.yaml", scenarioYAML("alpha", "gui-explore", ""))
	packDir := filepath.Join(dir, ".autopus", "qa", "journeys")
	require.NoError(t, os.MkdirAll(packDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(packDir, "broken.yaml"), []byte("id: [unterminated\n"), 0o644))
	_, err := CompileProject(dir, true)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "not a Journey Pack")
}

// A dry run must report exactly what a real run would write without touching
// the filesystem, otherwise --dry-run is not a preview.
func TestCompileProjectDryRunWritesNothing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeCaptureFixture(t, dir)
	writeScenario(t, dir, "a.yaml", scenarioYAML("alpha", "gui-explore", ""))
	writeJourneyPack(t, dir, "gui-explore", "http://127.0.0.1:4173")

	result, err := CompileProject(dir, true)
	require.NoError(t, err)
	assert.True(t, result.DryRun)
	require.Len(t, result.Compiled, 1)
	assert.Positive(t, result.Compiled[0].Bytes)
	_, statErr := os.Stat(SpecDir(dir))
	assert.True(t, os.IsNotExist(statErr), "dry run must not create the spec directory")
}

// The compiled report is what the operator reviews, so screen/step counts, the
// source and spec refs, and the screen matrix must match the authored set.
func TestCompileProjectWritesSpecsAndReportsCounts(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeCaptureFixture(t, dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "playwright.config.ts"),
		[]byte("export default { testDir: './tests/e2e' };\n"), 0o644))
	writeScenario(t, dir, "b.yaml", scenarioYAML("beta", "gui-explore", ""))
	writeScenario(t, dir, "a.yaml", scenarioYAML("alpha", "gui-explore", ""))
	writeJourneyPack(t, dir, "gui-explore", "http://127.0.0.1:4173")
	writeJourneyPack(t, dir, "cli-only")

	result, err := CompileProject(dir, false)
	require.NoError(t, err)
	assert.Equal(t, "playwright.config.ts", result.ConfigRef)
	assert.Equal(t, "tests/e2e", result.TestDir)
	assert.Equal(t, "tests/e2e/"+GeneratedDirName, result.SpecDir)
	require.Len(t, result.Compiled, 2)
	assert.Equal(t, []string{"alpha", "beta"},
		[]string{result.Compiled[0].ScenarioID, result.Compiled[1].ScenarioID})
	assert.Equal(t, 2, result.Compiled[0].Screens)
	assert.Equal(t, 3, result.Compiled[0].Steps)
	assert.Equal(t, filepath.ToSlash(filepath.Join(DirRel, "a.yaml")), result.Compiled[0].SourcePath)
	assert.Equal(t, "tests/e2e/"+GeneratedDirName+"/alpha.spec.ts", result.Compiled[0].SpecPath)

	body, err := os.ReadFile(SpecPath(dir, "alpha"))
	require.NoError(t, err)
	assert.Len(t, body, result.Compiled[0].Bytes)
	assert.Contains(t, string(body), "http://127.0.0.1:4173")

	// Only journeys the scenarios actually name project into the matrix; a pack
	// with no scenarios must not appear as covered.
	require.Contains(t, result.ScreenMatrix, "gui-explore")
	assert.NotContains(t, result.ScreenMatrix, "cli-only")
}

// The scenario's own origin is authoritative when stated; the pack origin is
// only a fallback, so a declared origin must survive the compile.
func TestCompileProjectPrefersScenarioOrigin(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeCaptureFixture(t, dir)
	writeScenario(t, dir, "a.yaml", scenarioYAML("alpha", "gui-explore", "origin: http://127.0.0.1:9999\n"))
	writeJourneyPack(t, dir, "gui-explore", "http://127.0.0.1:4173")

	_, err := CompileProject(dir, false)
	require.NoError(t, err)
	body, err := os.ReadFile(SpecPath(dir, "alpha"))
	require.NoError(t, err)
	assert.Contains(t, string(body), "http://127.0.0.1:9999")
	assert.NotContains(t, string(body), "4173")
}

// The first allowed origin wins and its trailing slash is trimmed, so compiled
// paths concatenate without doubling the separator.
func TestCompileProjectInheritsFirstAllowedOriginTrimmed(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeCaptureFixture(t, dir)
	writeScenario(t, dir, "a.yaml", scenarioYAML("alpha", "gui-explore", ""))
	writeJourneyPack(t, dir, "gui-explore", "http://127.0.0.1:4173/", "http://127.0.0.1:5555")

	_, err := CompileProject(dir, false)
	require.NoError(t, err)
	body, err := os.ReadFile(SpecPath(dir, "alpha"))
	require.NoError(t, err)
	assert.Contains(t, string(body), `"http://127.0.0.1:4173"`)
	assert.NotContains(t, string(body), "5555")
}
