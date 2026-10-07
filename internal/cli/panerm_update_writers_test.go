package cli_test

// SPEC-PANERM-001 T3 (S7 b): `auto update` on C1 and C2, which hold group K
// keys and no other migration condition, must rewrite autopus.yaml without
// them through a raw-node prune that keeps everything else, and the written
// file must load again. Red at B: update saves only for another migration, so
// the group K keys stay. T13 un-skipped it; since T8 retired the group K
// fields, the in-tree loader is no longer O's, so binary O loads the file too.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/internal/cli"
	"github.com/insajin/autopus-adk/pkg/config"
)

// panermKeptLines are the C2 lines S7 requires byte for byte: a comment, an
// env placeholder, and the reserved future_extension block.
var panermKeptLines = []string{
	"        # keep-me\n",
	"        work_dir: \"${AUTOPUS_WORKDIR}\"\n",
	"future_extension:\n    note: keep\n",
}

func TestPanermS7_UpdatePrunesGroupKAndKeepsTheRest(t *testing.T) {
	binaryO := cli.BuildPanermBinaryO(t) // before the subtests pin PATH to a scratch dir
	for _, fixture := range []string{"c1.yaml", "c2.yaml"} {
		t.Run(fixture, func(t *testing.T) {
			useStaleHookEnv(t, "")
			input := readLegacyPaneConfig(t, fixture)
			pruned := groupKPaths(t, input)
			require.NotEmpty(t, pruned, "%s holds group K keys", fixture)
			dir := t.TempDir()
			path := filepath.Join(dir, "autopus.yaml")
			require.NoError(t, os.WriteFile(path, input, 0o644))

			out, err := runStaleHookUpdate(t, dir)
			require.NoError(t, err, out)

			written, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Empty(t, groupKPaths(t, written), "the written autopus.yaml still holds group K keys")
			for _, token := range []string{"pane_args", "interactive_input", "working_patterns", "monitor_pattern_timeout_ms"} {
				assert.NotContains(t, string(written), token)
			}
			assert.Equal(t, yamlShape(t, input, pruned), yamlShape(t, written, nil),
				"the written tree must equal the input tree minus exactly the pruned group K entries")
			for _, line := range panermKeptLines {
				if strings.Contains(string(input), line) {
					assert.Contains(t, string(written), line, "kept byte for byte")
				}
			}

			_, err = config.Load(dir)
			require.NoError(t, err, "the written autopus.yaml must load again")
			cli.RequirePanermOLoads(t, binaryO, path)
		})
	}
}

// C2' is C2 without its group K lines, so its tree is the expected tree of
// the C2 rewrite; this pins the computed expectation to the hand fixture.
func TestPanermS7_C2PrimeIsC2WithoutP2(t *testing.T) {
	t.Parallel()
	assert.Equal(t, panermP2, groupKPaths(t, readLegacyPaneConfig(t, "c2.yaml")))
	assert.Equal(t, yamlShape(t, readLegacyPaneConfig(t, "c2.yaml"), panermP2),
		yamlShape(t, readLegacyPaneConfig(t, "c2-prime.yaml"), nil))
}
