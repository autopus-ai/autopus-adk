package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Both modes persist an orchestra provider at Astra/max. Ultra keeps the tier
// ladder for agents (fable-tier planner Astra/max, opus-tier executor
// Sol/xhigh); balanced routes both through the native placement (planner
// Astra/max, executor Luna/max).
func TestInitCmd_QualityUltraInheritsCodexSupervisorAndSetsManagedAgents(t *testing.T) {
	assertInitCodexQualityProfile(t, "ultra", "max", "gpt-6-sol", "xhigh", "max")
}

func TestInitCmd_QualityBalancedInheritsCodexSupervisorAndSetsManagedAgents(t *testing.T) {
	assertInitCodexQualityProfile(t, "balanced", "max", "gpt-6-luna", "max", "max")
}

func assertInitCodexQualityProfile(t *testing.T, quality, orchestraEffort, executorModel, executorEffort, plannerEffort string) {
	t.Helper()
	installCodex56CatalogFixture(t)
	dir := t.TempDir()

	cmd := newTestRootCmd()
	cmd.SetArgs([]string{"init", "--dir", dir, "--project", "test-proj", "--platforms", "codex", "--yes", "--quality", quality})
	require.NoError(t, cmd.Execute())

	data, err := os.ReadFile(filepath.Join(dir, ".codex", "config.toml"))
	require.NoError(t, err)
	rootSection := strings.SplitN(string(data), "[agents]", 2)[0]
	assert.NotContains(t, rootSection, "\nmodel =")
	assert.NotContains(t, rootSection, "model_reasoning_effort")

	executor, err := os.ReadFile(filepath.Join(dir, ".codex", "agents", "executor.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(executor), `model = "`+executorModel+`"`)
	assert.Contains(t, string(executor), `model_reasoning_effort = "`+executorEffort+`"`)
	planner, err := os.ReadFile(filepath.Join(dir, ".codex", "agents", "planner.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(planner), `model = "gpt-6-astra"`)
	assert.Contains(t, string(planner), `model_reasoning_effort = "`+plannerEffort+`"`)

	harnessData, err := os.ReadFile(filepath.Join(dir, "autopus.yaml"))
	require.NoError(t, err)
	harness := string(harnessData)
	assert.Contains(t, harness, "supervisor_model_policy: inherit")
	assert.Contains(t, harness, "model_policy: quality")
	assert.Contains(t, harness, "gpt-6-astra")
	assert.GreaterOrEqual(t, strings.Count(harness, `model_reasoning_effort="`+orchestraEffort+`"`), 2)
}

func installCodex56CatalogFixture(t *testing.T) {
	t.Helper()
	binDir := t.TempDir()
	script := `#!/bin/sh
printf '%s' '{"models":[{"slug":"gpt-6-astra","supported_reasoning_levels":[{"effort":"xhigh"},{"effort":"max"},{"effort":"ultra"}]},{"slug":"gpt-6-sol","supported_reasoning_levels":[{"effort":"xhigh"},{"effort":"max"},{"effort":"ultra"}]},{"slug":"gpt-5.6-terra","supported_reasoning_levels":[{"effort":"medium"},{"effort":"high"}]},{"slug":"gpt-6-luna","supported_reasoning_levels":[{"effort":"low"},{"effort":"medium"},{"effort":"max"}]},{"slug":"gpt-5.5","supported_reasoning_levels":[{"effort":"xhigh"}]}]}'
`
	path := filepath.Join(binDir, "codex")
	require.NoError(t, os.WriteFile(path, []byte(script), 0755))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
