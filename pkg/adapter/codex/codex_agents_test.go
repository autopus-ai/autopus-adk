package codex

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateAgents(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	files, err := a.generateAgents(cfg)
	require.NoError(t, err)
	assert.Len(t, files, 16, "should generate 16 TOML agent files")

	for _, f := range files {
		fullPath := filepath.Join(dir, f.TargetPath)
		assert.FileExists(t, fullPath)
		assert.Contains(t, f.TargetPath, ".codex/agents/")
		assert.Contains(t, string(f.Content), "test-project")
	}
}

func TestGenerateAgents_TOMLContent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	files, err := a.generateAgents(cfg)
	require.NoError(t, err)

	for _, f := range files {
		content := string(f.Content)
		assert.Contains(t, content, "name =", "TOML %s should have name field", f.TargetPath)
		assert.Contains(t, content, "description =", "TOML %s should have description field", f.TargetPath)
		assert.Contains(t, content, `model = "gpt-`, "TOML %s should use a managed GPT role model", f.TargetPath)
		assert.Contains(t, content, "model_reasoning_effort =", "TOML %s should set effort explicitly", f.TargetPath)
		assert.Contains(t, content, "developer_instructions =", "TOML %s should have instructions", f.TargetPath)
		assert.Contains(t, content, "developer_instructions = '''", "TOML %s should use literal multiline strings", f.TargetPath)
		assert.NotContains(t, content, "[developer_instructions]", "TOML %s should use flat instructions field", f.TargetPath)
		assert.Contains(t, content, "Supervisor Return Contract", "TOML %s should include common worker result contract", f.TargetPath)
		assert.Contains(t, content, "`owned_paths`", "TOML %s should require owned_paths", f.TargetPath)
		assert.Contains(t, content, "`changed_files`", "TOML %s should require changed_files", f.TargetPath)
		assert.Contains(t, content, "`verification`", "TOML %s should require verification", f.TargetPath)
		assert.Contains(t, content, "`blockers`", "TOML %s should require blockers", f.TargetPath)
		assert.Contains(t, content, "`next_required_step`", "TOML %s should require next_required_step", f.TargetPath)
	}
}

// Balanced renders the native placement rather than the agent template's
// declared effort: the seven reasoning-core roles land on Astra/max and the
// nine execution roles on Luna/max. This adapter has no probed catalog, so it
// covers the config-only render path; the catalog-verified path is in
// codex_native_balanced_test.go.
func TestGenerateAgents_BalancedQualityRendersNativePlacement(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	files, err := a.generateAgents(cfg)
	require.NoError(t, err)
	require.Len(t, files, len(nativeBalancedCodexTOML))

	for _, f := range files {
		name := filepath.Base(f.TargetPath)
		want, ok := nativeBalancedCodexTOML[name]
		require.True(t, ok, "unexpected managed agent %q", name)
		assertCodexRenderedProfile(t, string(f.Content), want)
	}
}

func TestGenerateAgents_UltraQualityUsesFableAndOpusProfiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")
	cfg.Quality.Default = "ultra"
	fableAgents := map[string]bool{
		"architect.toml":        true,
		"debugger.toml":         true,
		"deep-worker.toml":      true,
		"planner.toml":          true,
		"reviewer.toml":         true,
		"security-auditor.toml": true,
		"spec-writer.toml":      true,
	}

	files, err := a.generateAgents(cfg)
	require.NoError(t, err)

	seenFable := make(map[string]bool, len(fableAgents))
	for _, f := range files {
		content := string(f.Content)
		model := config.CodexSolModel
		effort := config.CodexEffortXHigh
		name := filepath.Base(f.TargetPath)
		if fableAgents[name] {
			model = config.CodexAstraModel
			effort = config.CodexEffortMax
			seenFable[name] = true
		}
		assert.Contains(t, content, `model = "`+model+`"`, f.TargetPath)
		assert.Contains(t, content, `model_reasoning_effort = "`+effort+`"`, f.TargetPath)
		assert.NotContains(t, content, `model_reasoning_effort = "ultra"`, "managed workers must not auto-delegate", f.TargetPath)
	}
	assert.Equal(t, fableAgents, seenFable)
}

func TestPrepareAgentFiles_NoDiskWrite(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	files, err := a.prepareAgentFiles(cfg)
	require.NoError(t, err)
	assert.Len(t, files, 16)

	agentsDir := filepath.Join(dir, ".codex", "agents")
	_, err = os.Stat(agentsDir)
	assert.True(t, os.IsNotExist(err), "prepareAgentFiles should not create files on disk")
}
