package scenario

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func writeScenario(t *testing.T, dir, name, body string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, DirRel), 0o755))
	path := filepath.Join(dir, DirRel, name)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	return path
}

// An ignored key is how a misspelled assertion becomes an always-green test, so
// the loader must reject unknown fields rather than drop them.
func TestLoadFileRejectsUnknownKeys(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeScenario(t, dir, "a.yaml", `schema_version: qamesh.scenario.v1
id: a
title: t
journey: j
screens:
  - id: s
    path: /
    steps:
      - expect_titel: typo
`)
	_, err := LoadFile(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expect_titel")
}

// Mutating actions are not fields on Step, so the schema itself refuses them.
func TestLoadFileRejectsMutationVocabulary(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, action := range []string{"click", "fill", "press", "tap", "drag_to"} {
		path := writeScenario(t, dir, action+".yaml", `schema_version: qamesh.scenario.v1
id: a
title: t
journey: j
screens:
  - id: s
    path: /
    steps:
      - `+action+`: "#pay"
`)
		_, err := LoadFile(path)
		require.Error(t, err, action)
		assert.Contains(t, err.Error(), action)
	}
}

func TestLoadDirIsDeterministicAndRejectsDuplicateIDs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeScenario(t, dir, "b.yaml", fmtScenario("beta"))
	writeScenario(t, dir, "a.yaml", fmtScenario("alpha"))
	loaded, err := LoadDir(dir)
	require.NoError(t, err)
	require.Len(t, loaded, 2)
	assert.Equal(t, []string{"alpha", "beta"}, []string{loaded[0].ID, loaded[1].ID})

	writeScenario(t, dir, "c.yaml", fmtScenario("alpha"))
	_, err = LoadDir(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already declared")
}

func fmtScenario(id string) string {
	return `schema_version: qamesh.scenario.v1
id: ` + id + `
title: t
journey: browser-gui-explore
screens:
  - id: s
    path: /
    steps:
      - expect_text: hello
`
}

func TestValidateRejectsMalformedScenarios(t *testing.T) {
	t.Parallel()
	base := validScenario()
	cases := []struct {
		name   string
		mutate func(*Scenario)
		want   string
	}{
		// v2 is a real schema since SPEC-QALOOP-001; an unknown version must
		// still be refused rather than read as the nearest known one.
		{"version", func(s *Scenario) { s.SchemaVersion = "qamesh.scenario.v3" }, "schema_version"},
		{"id shape", func(s *Scenario) { s.ID = "Not Kebab" }, "kebab-case"},
		{"title", func(s *Scenario) { s.Title = "  " }, "title is required"},
		{"journey", func(s *Scenario) { s.Journey = "" }, "journey is required"},
		{"no screens", func(s *Scenario) { s.Screens = nil }, "at least one screen"},
		{"screen id", func(s *Scenario) { s.Screens[0].ID = "BAD" }, "screen id"},
		{"no steps", func(s *Scenario) { s.Screens[0].Steps = nil }, "at least one step"},
		{"rel path", func(s *Scenario) { s.Screens[0].Path = "landing" }, "origin-relative"},
		{"protocol relative", func(s *Scenario) { s.Screens[0].Path = "//evil.test" }, "origin-relative"},
		{"traversal", func(s *Scenario) { s.Screens[0].Path = "/../etc" }, "unsupported character"},
		{"quote in path", func(s *Scenario) { s.Screens[0].Path = `/a"b` }, "unsupported character"},
		{"empty step", func(s *Scenario) { s.Screens[0].Steps = []Step{{}} }, "no assertion"},
		{"two assertions", func(s *Scenario) {
			s.Screens[0].Steps = []Step{{ExpectText: "a", ExpectTitle: "b"}}
		}, "one step per assertion"},
		{"unknown role", func(s *Scenario) {
			s.Screens[0].Steps = []Step{{ExpectRole: &RoleTarget{Role: "buton"}}}
		}, "not an ARIA role"},
		{"count role", func(s *Scenario) {
			s.Screens[0].Steps = []Step{{ExpectCount: &CountTarget{Role: "nope", Count: 1}}}
		}, "not an ARIA role"},
		{"negative count", func(s *Scenario) {
			s.Screens[0].Steps = []Step{{ExpectCount: &CountTarget{Role: "listitem", Count: -1}}}
		}, "negative"},
		{"step url", func(s *Scenario) { s.Screens[0].Steps = []Step{{ExpectURL: "https://x.test/"}} }, "origin-relative"},
		{"origin path", func(s *Scenario) { s.Origin = "http://x.test/app" }, "no path, query"},
		{"origin creds", func(s *Scenario) { s.Origin = "http://u:p@x.test" }, "no path, query"},
		{"origin scheme", func(s *Scenario) { s.Origin = "file:///etc/passwd" }, "absolute http"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := base
			s.Screens = append([]Screen(nil), base.Screens...)
			s.Screens[0].Steps = append([]Step(nil), base.Screens[0].Steps...)
			tc.mutate(&s)
			err := Validate(s)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestValidateRejectsDuplicateScreenIDs(t *testing.T) {
	t.Parallel()
	s := validScenario()
	s.Screens = append(s.Screens, s.Screens[0])
	err := Validate(s)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate screen id")
}

func TestValidateBoundsStepCount(t *testing.T) {
	t.Parallel()
	s := validScenario()
	s.Screens[0].Steps = make([]Step, maxSteps+1)
	for i := range s.Screens[0].Steps {
		s.Screens[0].Steps[i] = Step{ExpectText: "x"}
	}
	err := Validate(s)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "limit is")
}

func TestTestDirReadsPlaywrightConfig(t *testing.T) {
	t.Parallel()
	for _, name := range playwrightConfigNames {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, name),
			[]byte("export default { testDir: './tests/browser' };\n"), 0o644))
		got, ref := TestDir(dir)
		assert.Equal(t, filepath.FromSlash("tests/browser"), got)
		assert.Equal(t, name, ref)
	}
}

// A computed or escaping testDir cannot be resolved without executing project
// code, so the documented default is used instead of a guess that would write
// specs somewhere the runner never looks.
func TestTestDirFallsBackOnUnresolvableValues(t *testing.T) {
	t.Parallel()
	for _, body := range []string{
		"export default { testDir: resolve(__dirname, 'e2e') };",
		"export default { testDir: '/abs/e2e' };",
		"export default { testDir: '../outside' };",
		"export default { testDir: '' };",
		"export default { reporter: [] };",
	} {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "playwright.config.ts"), []byte(body), 0o644))
		got, ref := TestDir(dir)
		assert.Equal(t, DefaultTestDir, got, body)
		assert.Equal(t, "playwright.config.ts", ref)
	}
	dir := t.TempDir()
	got, ref := TestDir(dir)
	assert.Equal(t, DefaultTestDir, got)
	assert.Empty(t, ref)
}

func TestFixtureImportIsRelativeAndSlashed(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	spec := SpecPath(dir, "x")
	got, err := FixtureImport(dir, spec)
	require.NoError(t, err)
	assert.Equal(t, "../../.autopus/qa/capture/autopus-capture.fixture.cjs", got)
}

// The starter must parse and validate, or `scenario init` would hand the project
// a file that `scenario compile` immediately rejects.
func TestStarterCompilesAfterInit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path, created, err := WriteStarter(dir, "browser-gui-explore", "http://127.0.0.1:4173")
	require.NoError(t, err)
	require.True(t, created)
	loaded, err := LoadFile(path)
	require.NoError(t, err)
	assert.Equal(t, StarterID, loaded.ID)
	assert.Equal(t, "browser-gui-explore", loaded.Journey)
	var probe map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(StarterBody("", "")), &probe))

	_, again, err := WriteStarter(dir, "browser-gui-explore", "")
	require.NoError(t, err)
	assert.False(t, again, "starter must never overwrite authored assertions")
}

func TestCompileProjectRequiresFixture(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeScenario(t, dir, "a.yaml", fmtScenario("alpha"))
	_, err := CompileProject(dir, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "auto qa init")
}
