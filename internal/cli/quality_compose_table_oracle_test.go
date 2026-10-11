package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter/codex"
	"github.com/insajin/autopus-adk/pkg/config"
)

// readComposeTableClaudeProfile reads the model and effort frontmatter lines of a
// generated Claude agent; a missing effort line reads as "".
func readComposeTableClaudeProfile(t *testing.T, dir, agent string) composeTableClaudeCell {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".claude", "agents", "autopus", agent+".md"))
	require.NoError(t, err)
	lines := strings.Split(string(data), "\n")
	require.Equal(t, "---", lines[0], agent)
	var cell composeTableClaudeCell
	for _, line := range lines[1:] {
		if line == "---" {
			break
		}
		if value, ok := strings.CutPrefix(line, "model:"); ok {
			cell.model = strings.TrimSpace(value)
		}
		if value, ok := strings.CutPrefix(line, "effort:"); ok {
			cell.effort = strings.TrimSpace(value)
		}
	}
	return cell
}

// generateComposeTableOracle runs the real Claude Code and Codex adapters into a
// fresh root with the run snapshot, exactly as the composition run's own
// generation builds them, and returns the root and the Codex error.
func generateComposeTableOracle(t *testing.T, cfg *config.HarnessConfig, snapshot driftGenerationSnapshot) (string, error) {
	t.Helper()
	dir := t.TempDir()
	_, err := driftAdapterFor("claude-code", dir, snapshot).Generate(context.Background(), cfg)
	require.NoError(t, err)
	_, codexErr := driftAdapterFor("codex", dir, snapshot).Generate(context.Background(), cfg)
	return dir, codexErr
}

// Probe A3 (S5, S19): every Claude Code and Codex raw cell equals what the
// real adapters generate from the same config and the same catalog snapshot.
func TestComposeTable_OracleCellsEqualRealGeneration(t *testing.T) {
	cases := []struct{ fixture, catalog string }{
		{"D", "CAT-A"}, {"R", "CAT-A"}, {"P", "CAT-A"}, {"O1", "CAT-A"}, {"O2", "CAT-A"},
		{"X", "CAT-A"}, {"K", "CAT-A"}, {"H", "CAT-A"}, {"B2", "CAT-A"}, {"C", "CAT-A"},
		{"R", "CAT-B"}, {"R", "CAT-C"}, {"R", "CAT-D"}, {"D", "CAT-B"}, {"D", "CAT-C"},
	}
	sources, err := loadComposeAgentSources()
	require.NoError(t, err)
	for _, tc := range cases {
		t.Run(tc.fixture+"/"+tc.catalog, func(t *testing.T) {
			cfg := composeTableFixture(t, tc.fixture)
			snapshot := composeTableSnapshot(t, tc.catalog)
			table := buildComposeTable(cfg, snapshot, sources)

			dir, codexErr := generateComposeTableOracle(t, cfg, snapshot)

			require.NoError(t, codexErr)
			require.False(t, table.codexRefused())
			require.Len(t, table.rows, len(config.CanonicalAgentNames()))
			for _, row := range table.rows {
				assert.Equal(t, readComposeTableClaudeProfile(t, dir, row.agent), row.claude, "Claude %s", row.agent)
				assert.Equal(t, readComposeCodexProfile(t, dir, row.agent), row.codex.resolution.Effective,
					"Codex %s", row.agent)
			}
		})
	}
}

// S19: CAT-D's refused cells are exactly a failing Codex generation, refused
// for the standard native placement.
func TestComposeTable_OracleRefusalEqualsCodexGenerationError(t *testing.T) {
	sources, err := loadComposeAgentSources()
	require.NoError(t, err)
	cfg := composeTableFixture(t, "D")
	snapshot := composeTableSnapshot(t, "CAT-D")
	table := buildComposeTable(cfg, snapshot, sources)

	dir, codexErr := generateComposeTableOracle(t, cfg, snapshot)

	require.True(t, table.codexRefused())
	require.Error(t, codexErr)
	assert.True(t, errors.Is(codexErr, codex.ErrCodexNativePlacementUnavailable), codexErr.Error())
	for _, row := range table.rows {
		assert.Equal(t, readComposeTableClaudeProfile(t, dir, row.agent), row.claude, "Claude %s", row.agent)
	}
}

// S14: the cells the after-table marks are exactly the generated files that
// differ between the start config and the candidate under one snapshot.
func TestComposeTable_OracleStarsEqualGeneratedDifferences(t *testing.T) {
	sources, err := loadComposeAgentSources()
	require.NoError(t, err)
	snapshot := composeTableSnapshot(t, "CAT-A")
	startCfg, candidateCfg := composeTableFixture(t, "D"), composeTableFixture(t, "K")
	start := buildComposeTable(startCfg, snapshot, sources)
	candidate := buildComposeTable(candidateCfg, snapshot, sources)
	startDir, err := generateComposeTableOracle(t, startCfg, snapshot)
	require.NoError(t, err)
	candidateDir, err := generateComposeTableOracle(t, candidateCfg, snapshot)
	require.NoError(t, err)

	layout := candidate.layout("ko", &start)
	for i, row := range candidate.rows {
		claudeDiffers := readComposeTableClaudeProfile(t, startDir, row.agent) != readComposeTableClaudeProfile(t, candidateDir, row.agent)
		codexDiffers := readComposeCodexProfile(t, startDir, row.agent) != readComposeCodexProfile(t, candidateDir, row.agent)
		assert.Equal(t, claudeDiffers, strings.HasSuffix(layout.rows[i][3], "*"), "Claude %s", row.agent)
		assert.Equal(t, codexDiffers, strings.HasSuffix(layout.rows[i][4], "*"), "Codex %s", row.agent)
	}
}
