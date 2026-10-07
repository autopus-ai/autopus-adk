package cli

// SPEC-PANERM-001 T13 (S7 a, c, d, e, f): every autopus.yaml writer drops the
// group K keys and keeps the rest, and binary O loads each written file
// (auto update, S7 b, is panerm_update_writers_test.go). Each raw writer also
// runs on the same input without its group K lines (C2' for C2): a writer that
// keeps everything else writes the same bytes for both.

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// panermKeptText is what S7 (c), (d), and (e) keep byte for byte: a comment,
// an env placeholder, and the reserved future_extension block.
var panermKeptText = []string{
	"        # keep-me\n",
	"        work_dir: \"${AUTOPUS_WORKDIR}\"\n",
	"future_extension:\n    note: keep\n",
}

// runQualityWriter runs `auto --config <dir>/autopus.yaml <args>` on input and
// returns the written file.
func runQualityWriter(t *testing.T, input []byte, args ...string) string {
	t.Helper()
	path := filepath.Join(legacyPaneDir(t, input), "autopus.yaml")
	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"--config", path}, args...))
	require.NoError(t, root.Execute(), out.String())
	written, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(written)
}

// runOMPProfileWriter applies the balanced OMP profile with the fake runner of
// TestPlatformOMPProfileApplyPersistsAndRollsBackAtomically. The apply needs
// the omp platform, so it joins claude-code.
func runOMPProfileWriter(t *testing.T, input string) string {
	t.Helper()
	input = strings.Replace(input, "platforms:\n    - claude-code\n", "platforms:\n    - claude-code\n    - omp\n", 1)
	root := legacyPaneDir(t, []byte(input))
	runner := &ompCLIFakeRunner{catalog: ompCLIBalancedCatalogJSON()}
	activate := func(context.Context, string, *config.HarnessConfig) error { return nil }
	_, err := applyOMPProfile(context.Background(), root, ompProfileApplyOptions{name: "balanced"}, runner, activate)
	require.NoError(t, err)
	written, err := os.ReadFile(filepath.Join(root, "autopus.yaml"))
	require.NoError(t, err)
	return string(written)
}

func writeS7Output(t *testing.T, name, data string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name+".yaml")
	require.NoError(t, os.WriteFile(path, []byte(data), 0o644))
	return path
}

func TestPanermS7_EveryWriterDropsGroupKAndBinaryOLoadsTheResult(t *testing.T) {
	original := qualityPlatformUpdater
	t.Cleanup(func() { qualityPlatformUpdater = original })
	var applied []string
	qualityPlatformUpdater = func(_ context.Context, _ string, platform string, _ *config.HarnessConfig) (bool, error) {
		applied = append(applied, platform)
		return true, nil
	}
	c2, c2Prime := readLegacyPane(t, "c2.yaml"), readLegacyPane(t, "c2-prime.yaml")
	operator := "operator_extension:\n    credential_ref: ${OMP_SECRET}\n"
	require.Equal(t, string(c2)+operator, string(readLegacyPane(t, "c2o.yaml")), "C2o is C2 plus operator_extension")

	written := map[string]string{
		"c supervisor": runQualityWriter(t, c2, "quality", "supervisor", "quality"),
		"d provider":   runQualityWriter(t, c2, "quality", "provider", "claude", "ultra", "--apply"),
		"e omp":        runOMPProfileWriter(t, string(c2)+operator),
	}
	assert.Equal(t, []string{"claude-code"}, applied, "--apply reached the claude platform")
	assert.Equal(t, runQualityWriter(t, c2Prime, "quality", "supervisor", "quality"), written["c supervisor"])
	assert.Equal(t, runQualityWriter(t, c2Prime, "quality", "provider", "claude", "ultra", "--apply"), written["d provider"])
	assert.Equal(t, runOMPProfileWriter(t, string(c2Prime)+operator), written["e omp"])
	assert.Contains(t, written["c supervisor"], "    supervisor_model_policy: quality\n")
	assert.Contains(t, written["d provider"], "    providers:\n      claude: ultra\n")
	assert.Contains(t, written["e omp"], "role_model_policy:")
	assert.Contains(t, written["e omp"], operator, "(e) keeps operator_extension")
	for name, data := range written {
		requireNoRetiredKeys(t, []byte(data))
		for _, kept := range panermKeptText {
			assert.Contains(t, data, kept, "%s keeps it byte for byte", name)
		}
	}

	// (a) config.Save of C2's loaded config and (f) the loader's platform-name
	// rewrite of C7 re-marshal the schema, which has no group K field.
	saved := legacyPaneDir(t, c2)
	cfg, err := config.Load(saved)
	require.NoError(t, err)
	require.NoError(t, config.Save(saved, cfg))
	normalized := legacyPaneDir(t, readLegacyPane(t, "c7.yaml"))
	_, err = config.Load(normalized)
	require.NoError(t, err)
	for name, dir := range map[string]string{"a save": saved, "f normalization": normalized} {
		data, err := os.ReadFile(filepath.Join(dir, "autopus.yaml"))
		require.NoError(t, err)
		requireNoRetiredKeys(t, data)
		written[name] = string(data)
	}
	assert.Contains(t, written["f normalization"], "    - claude-code\n", "(f) lists claude-code")

	bin := BuildPanermBinaryO(t)
	requirePanermORejectsATypo(t, bin)
	for name, data := range written {
		RequirePanermOLoads(t, bin, writeS7Output(t, strings.ReplaceAll(name, " ", "-"), data))
	}
}
