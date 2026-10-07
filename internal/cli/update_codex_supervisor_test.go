package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
)

func TestUpdateCmdMigratesUnchangedLegacyCodexSupervisorToInherit(t *testing.T) {
	dir := writeLegacySupervisorUpdateFixture(t, true)
	t.Setenv("PATH", t.TempDir())

	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"update", "--dir", dir, "--local", "--yes"})
	require.NoError(t, root.Execute())

	updated, err := config.LoadPreview(dir)
	require.NoError(t, err)
	assert.Equal(t, config.SupervisorModelPolicyInherit, updated.Quality.SupervisorModelPolicy)

	projectConfig, err := os.ReadFile(filepath.Join(dir, ".codex", "config.toml"))
	require.NoError(t, err)
	rootSection := strings.SplitN(string(projectConfig), "[agents]", 2)[0]
	assert.NotContains(t, rootSection, "\nmodel =")
	assert.NotContains(t, rootSection, "model_reasoning_effort")
	assert.Contains(t, out.String(), "Codex supervisor model now inherits the user default")
}

func TestUpdateCmdRepairsLegacyOrchestraBeforePersistingSupervisorInherit(t *testing.T) {
	dir := writeLegacySupervisorUpdateFixture(t, true)
	t.Setenv("PATH", t.TempDir())

	cfg, err := config.LoadPreview(dir)
	require.NoError(t, err)
	cfg.Orchestra.Enabled = true
	cfg.Orchestra.Providers["codex"] = config.ProviderEntry{
		Binary:      "codex",
		Args:        []string{"exec", "--sandbox", "workspace-write", "-m", config.CodexLegacyModel},
		ModelPolicy: config.ProviderModelPolicyPinned,
		Subprocess: config.SubprocessProvConf{
			SchemaFlag: "--output-schema",
			Timeout:    config.CodexOrchestraTimeoutSeconds,
		},
	}
	require.NoError(t, config.Save(dir, cfg))

	root := NewRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"update", "--dir", dir, "--local", "--yes"})
	require.NoError(t, root.Execute())

	updated, err := config.LoadPreview(dir)
	require.NoError(t, err)
	assert.Equal(t, config.SupervisorModelPolicyInherit, updated.Quality.SupervisorModelPolicy)
	provider := updated.Orchestra.Providers["codex"]
	assert.Equal(t, config.ProviderModelPolicyQuality, provider.ModelPolicy)
	assert.Contains(t, provider.Args, config.CodexAstraModel)
	assert.NotContains(t, provider.Args, config.CodexLegacyModel)
}

func TestUpdatePlanReportsLegacyCodexMigrationWithoutWriting(t *testing.T) {
	dir := writeLegacySupervisorUpdateFixture(t, true)
	t.Setenv("PATH", t.TempDir())
	harnessPath := filepath.Join(dir, "autopus.yaml")
	configPath := filepath.Join(dir, ".codex", "config.toml")
	manifestPath := filepath.Join(dir, ".autopus", "codex-manifest.json")
	harnessBefore := readUpdateFixtureFile(t, harnessPath)
	configBefore := readUpdateFixtureFile(t, configPath)
	manifestBefore := readUpdateFixtureFile(t, manifestPath)

	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"update", "--dir", dir, "--local", "--plan", "--yes"})
	require.NoError(t, root.Execute())

	assert.Contains(t, out.String(), "legacy Codex supervisor model would migrate to user-default inheritance")
	assert.Equal(t, harnessBefore, readUpdateFixtureFile(t, harnessPath))
	assert.Equal(t, configBefore, readUpdateFixtureFile(t, configPath))
	assert.Equal(t, manifestBefore, readUpdateFixtureFile(t, manifestPath))
}

func TestUpdateCmdPreservesDriftedLegacyCodexSupervisor(t *testing.T) {
	dir := writeLegacySupervisorUpdateFixture(t, false)
	t.Setenv("PATH", t.TempDir())

	root := NewRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"update", "--dir", dir, "--local", "--yes"})
	require.NoError(t, root.Execute())

	updated, err := config.LoadPreview(dir)
	require.NoError(t, err)
	assert.Empty(t, updated.Quality.SupervisorModelPolicy)
	projectConfig := readUpdateFixtureFile(t, filepath.Join(dir, ".codex", "config.toml"))
	assert.Contains(t, string(projectConfig), `model = "gpt-5.5"`)
	assert.Contains(t, string(projectConfig), `model_reasoning_effort = "xhigh"`)
}

func TestUpdateCmdRollsBackSupervisorPolicyWhenCodexWriteFails(t *testing.T) {
	dir := writeLegacySupervisorUpdateFixture(t, true)
	t.Setenv("PATH", t.TempDir())
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".codex", "agents", "planner.toml"), 0o755))

	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"update", "--dir", dir, "--local", "--yes"})
	require.Error(t, root.Execute())

	updated, err := config.LoadPreview(dir)
	require.NoError(t, err)
	assert.Empty(t, updated.Quality.SupervisorModelPolicy)
	projectConfig := readUpdateFixtureFile(t, filepath.Join(dir, ".codex", "config.toml"))
	assert.Contains(t, string(projectConfig), `model = "gpt-5.5"`)
	assert.NotContains(t, out.String(), "Codex supervisor model now inherits the user default")
}

func writeLegacySupervisorUpdateFixture(t *testing.T, checksumMatches bool) string {
	t.Helper()
	dir := t.TempDir()
	cfg := config.DefaultFullConfig("legacy-project")
	cfg.Platforms = []string{"codex"}
	cfg.Quality.SupervisorModelPolicy = ""
	cfg.Orchestra.Enabled = false
	require.NoError(t, config.Save(dir, cfg))

	content := `# Codex configuration (auto-generated by Autopus-ADK)
# Project: legacy-project

model = "gpt-5.5"
model_reasoning_effort = "xhigh"

model_reasoning_summary = "auto"
model_verbosity = "medium"
approval_policy = "on-request"
sandbox_mode = "workspace-write"
web_search = "cached"
project_doc_max_bytes = 262144

[agents]
max_threads = 6
max_depth = 1
job_max_runtime_seconds = 1800
`
	configPath := filepath.Join(dir, ".codex", "config.toml")
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0o755))
	require.NoError(t, os.WriteFile(configPath, []byte(content), 0o644))

	checksum := adapter.Checksum(content)
	if !checksumMatches {
		checksum = adapter.Checksum("previous generated content")
	}
	manifest := adapter.NewManifest("codex")
	manifest.Files[filepath.ToSlash(filepath.Join(".codex", "config.toml"))] = adapter.ManifestFile{
		Checksum: checksum,
		Policy:   adapter.OverwriteMerge,
	}
	require.NoError(t, manifest.Save(dir))
	return dir
}

func readUpdateFixtureFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}
