package antigravity

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepareAntigravityHooksJSON_UsesOfficialSchema(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	hooks := []adapter.HookConfig{{
		Event:   "PreToolUse",
		Matcher: "run_command",
		Type:    "command",
		Command: "auto check --arch --quiet --warn-only",
		Timeout: 30,
	}, {
		Event:   "PreInvocation",
		Type:    "command",
		Command: "auto ready",
		Timeout: 10,
	}, {
		Event:   "Stop",
		Type:    "command",
		Command: "hook-gemini-stop.sh",
		Timeout: 300,
	}}

	files, err := a.prepareAntigravityHooksJSON(hooks)
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, ".agents/hooks.json", files[0].TargetPath)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(files[0].Content, &parsed))
	autopus := parsed["autopus"].(map[string]any)
	entries := autopus["PreToolUse"].([]any)
	first := entries[0].(map[string]any)
	assert.Equal(t, "run_command", first["matcher"])
	assert.Contains(t, first, "hooks")

	for _, event := range []string{"PreInvocation", "Stop"} {
		entries := autopus[event].([]any)
		handler := entries[0].(map[string]any)
		assert.Equal(t, "command", handler["type"])
		assert.NotContains(t, handler, "matcher")
		assert.NotContains(t, handler, "hooks")
	}
}

func TestGenerate_WritesProviderSpecificCompletionHookEvents(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, err := NewWithRoot(dir).Generate(
		context.Background(), config.DefaultFullConfig("test-project"),
	)
	require.NoError(t, err)

	settings := readJSONDocument(t, filepath.Join(dir, ".gemini", "settings.json"))
	legacyHooks := requireJSONObject(t, settings["hooks"])
	assert.Contains(t, legacyHooks, "AfterAgent")
	assert.NotContains(t, legacyHooks, "Stop")

	agents := readJSONDocument(t, filepath.Join(dir, ".agents", "hooks.json"))
	agyHooks := requireJSONObject(t, agents["autopus"])
	assert.Contains(t, agyHooks, "Stop")
	assert.NotContains(t, agyHooks, "AfterAgent")
	stop := agyHooks["Stop"].([]any)[0].(map[string]any)
	assert.NotContains(t, stop, "hooks", "Stop must be a flat handler list")
}

func TestGenerate_AntigravityStopCommandRunsOutsideGit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, err := NewWithRoot(dir).Generate(
		context.Background(), config.DefaultFullConfig("test-project"),
	)
	require.NoError(t, err)

	agents := readJSONDocument(t, filepath.Join(dir, ".agents", "hooks.json"))
	hooks := requireJSONObject(t, agents["autopus"])
	handler := hooks["Stop"].([]any)[0].(map[string]any)
	command := handler["command"].(string)
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = filepath.Join(dir, ".agents")
	cmd.Env = []string{"PATH=/nonexistent", "AUTOPUS_SESSION_ID="}
	stdout, err := cmd.Output()
	require.NoError(t, err)
	assert.Equal(t, `{"decision":"stop"}`, string(bytes.TrimSpace(stdout)))
}

func TestMirrorAntigravityPluginMappings(t *testing.T) {
	t.Parallel()

	files := mirrorAntigravityPluginMappings([]adapter.FileMapping{{
		TargetPath: ".gemini/commands/auto/plan.toml",
		Content:    []byte("Load .gemini/skills/autopus/auto-plan/SKILL.md"),
	}, {
		TargetPath: ".gemini/commands/auto.toml",
		Content:    []byte("Load .gemini/skills/auto/SKILL.md"),
	}})

	require.Len(t, files, 2)
	assert.Equal(t, ".agents/plugins/autopus/commands/auto/plan.toml", files[0].TargetPath)
	assert.Contains(t, string(files[0].Content), ".agents/plugins/autopus/skills/auto-plan/SKILL.md")

	assert.Equal(t, ".agents/plugins/autopus/commands/auto.toml", files[1].TargetPath)
	assert.Contains(t, string(files[1].Content), ".agents/plugins/autopus/skills/auto/SKILL.md")
}

func TestGenerate_CreatesAntigravityHooksJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, ".agents", "hooks.json"))
	require.NoError(t, err)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))
	assert.Contains(t, parsed, "autopus")
}

func TestGenerate_CreatesAntigravityPluginSurface(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	// Verify plugin-level surface
	assert.FileExists(t, filepath.Join(dir, ".agents", "plugins", "autopus", "plugin.json"))
	assert.FileExists(t, filepath.Join(dir, ".agents", "plugins", "autopus", "skills", "auto-plan", "SKILL.md"))
	assert.FileExists(t, filepath.Join(dir, ".agents", "plugins", "autopus", "rules", "lore-commit.md"))
	assert.FileExists(t, filepath.Join(dir, ".agents", "plugins", "autopus", "agents", "executor.md"))
	assert.FileExists(t, filepath.Join(dir, ".agents", "plugins", "autopus", "commands", "auto.toml"))
	assert.FileExists(t, filepath.Join(dir, ".agents", "plugins", "autopus", "commands", "auto", "plan.toml"))

	commandData, err := os.ReadFile(filepath.Join(dir, ".agents", "plugins", "autopus", "commands", "auto", "plan.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(commandData), ".agents/plugins/autopus/skills/auto-plan/SKILL.md")

	autoCommandData, err := os.ReadFile(filepath.Join(dir, ".agents", "plugins", "autopus", "commands", "auto.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(autoCommandData), ".agents/plugins/autopus/skills/auto/SKILL.md")

	// Skill directories are what `agy` converts into slash commands, so the
	// route inventory must be reachable from the plugin alone.
	assert.FileExists(t, filepath.Join(dir, ".agents", "plugins", "autopus", "skills", "auto", "SKILL.md"))
}

func TestRemoveAntigravityHooksJSON_PreservesUserHooks(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	hooksDir := filepath.Join(dir, ".agents")
	require.NoError(t, os.MkdirAll(hooksDir, 0755))
	data, _ := json.Marshal(map[string]any{
		"autopus": map[string]any{"enabled": true},
		"user":    map[string]any{"enabled": true},
	})
	require.NoError(t, os.WriteFile(filepath.Join(hooksDir, "hooks.json"), data, 0644))

	require.NoError(t, a.removeAntigravityHooksJSON())

	updated, err := os.ReadFile(filepath.Join(hooksDir, "hooks.json"))
	require.NoError(t, err)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(updated, &parsed))
	assert.NotContains(t, parsed, "autopus")
	assert.Contains(t, parsed, "user")
}
