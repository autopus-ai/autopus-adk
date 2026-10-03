package scenario

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// goldenV1Fixtures are v1 scenarios whose compiled output was captured before
// SPEC-QALOOP-001 added schema v2. They go through LoadFile, so decoding,
// trimming, validation, and rendering are all pinned, not just the renderer.
var goldenV1Fixtures = []struct {
	name string
	body string
	opts Options
}{
	{
		name: "every-step-kind",
		body: `schema_version: qamesh.scenario.v1
id: first-visit
title: A visitor lands
journey: browser-gui-explore
origin: http://127.0.0.1:4173
screens:
  - id: landing
    path: /
    steps:
      - expect_title: Shop
      - expect_role:
          role: heading
          name: Catalog
          exact: true
      - expect_count:
          role: listitem
          count: 3
      - expect_text: "Total: 42.00"
      - expect_url: /
`,
		opts: Options{FixtureImport: "../../fixture.cjs"},
	},
	{
		name: "hostile-text",
		body: `schema_version: qamesh.scenario.v1
id: hostile
title: "break */ out\nsecond line"
journey: browser-gui-explore
origin: http://127.0.0.1:4173
screens:
  - id: landing
    path: /
    steps:
      - expect_text: "a\"b\\c` + "`" + `d${e}\n</script>"
      - expect_role:
          role: heading
          name: '") ; process.exit(1); //'
`,
		opts: Options{FixtureImport: "../../fixture.cjs"},
	},
	{
		name: "inherited-origin-two-screens",
		body: `schema_version: qamesh.scenario.v1
id: alpha
title: alpha
journey: gui-explore
acceptance_refs: [AC-001, AC-002]
screens:
  - id: home
    path: /
    steps:
      - expect_text: hello
      - expect_title: Home
  - id: pricing
    path: /pricing
    steps:
      - expect_url: /pricing
      - expect_count:
          role: row
          name: Plan
          count: 0
`,
		opts: Options{Origin: "https://staging.example/", FixtureImport: "../../.autopus/qa/capture/autopus-capture.fixture.cjs"},
	},
	{
		name: "starter",
		body: StarterBody("browser-gui-explore", "http://127.0.0.1:4173"),
		opts: Options{Origin: "http://127.0.0.1:4173", FixtureImport: "../../.autopus/qa/capture/autopus-capture.fixture.cjs"},
	},
}

func goldenV1Path(name string) string {
	return filepath.Join("testdata", "golden", "v1", name+".spec.ts.golden")
}

// compileGoldenV1Fixture loads a fixture from disk exactly as a project would
// author it, then compiles it.
func compileGoldenV1Fixture(t *testing.T, name, body string, opts Options) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), name+".yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	loaded, err := LoadFile(path)
	require.NoError(t, err)
	out, err := Compile(loaded, opts)
	require.NoError(t, err)
	return out
}

// AC-QALOOP-002: schema v2 must not move a single byte of v1 output. The
// goldens were written by the pre-v2 compiler and are never regenerated, so a
// diff here means v1 behaviour changed, not that the golden is stale.
func TestCompileV1OutputIsByteIdenticalToPreV2Goldens(t *testing.T) {
	t.Parallel()
	for _, fixture := range goldenV1Fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			want, err := os.ReadFile(goldenV1Path(fixture.name))
			require.NoError(t, err)
			got := compileGoldenV1Fixture(t, fixture.name, fixture.body, fixture.opts)
			require.Equal(t, string(want), string(got))
		})
	}
}
