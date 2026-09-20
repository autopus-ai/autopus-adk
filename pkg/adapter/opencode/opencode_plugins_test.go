package opencode

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
)

func TestAdapter_InstallHooks_WritesPlugin(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir, WithCLIVersion("1.18.7"))
	hooks := []adapter.HookConfig{{Event: "PreToolUse", Matcher: "Bash", Type: "command", Command: "auto check --arch --quiet --warn-only", Timeout: 30}}

	err := a.InstallHooks(context.Background(), hooks, nil)
	require.NoError(t, err)

	data, readErr := os.ReadFile(filepath.Join(dir, ".opencode", "plugins", "autopus-hooks.js"))
	require.NoError(t, readErr)
	assert.Contains(t, string(data), "auto check --arch --quiet --warn-only")
}

func TestInjectOrchestraPlugin_MergesPluginArray(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir, WithCLIVersion("1.18.7"))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "opencode.json"), []byte(`{"plugin":["existing-plugin"]}`), 0644))

	err := a.InjectOrchestraPlugin("/path/to/script.js")
	require.NoError(t, err)

	configDoc := readConfigJSON(t, filepath.Join(dir, "opencode.json"))
	plugins := jsonStringSlice(configDoc["plugin"])
	assert.Contains(t, plugins, "existing-plugin")
	assert.Contains(t, plugins, "/path/to/script.js")
	assert.Contains(t, jsonStringSlice(configDoc["instructions"]), ".opencode/rules/autopus/branding.md")
}

func TestAdapter_Generate_WorkflowSkillsUseOpenCodeSurface(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir, WithCLIVersion("1.18.7"))
	_, err := a.Generate(context.Background(), config.DefaultFullConfig("demo"))
	require.NoError(t, err)

	bannedInSkills := []string{
		"spawn_agent",
		"Agent(",
		"mode =",
		"permissionMode",
		"bypassPermissions",
	}

	for _, spec := range workflowSpecs {
		skillPath := filepath.Join(dir, ".agents", "skills", spec.Name, "SKILL.md")
		data, readErr := os.ReadFile(skillPath)
		require.NoError(t, readErr, skillPath)
		content := string(data)
		assert.Contains(t, content, fmt.Sprintf("description: %q", spec.Description), "%s should keep workflow description", skillPath)
		assert.Contains(t, content, "## Autopus Branding", "%s should keep workflow body", skillPath)

		for _, banned := range bannedInSkills {
			assert.NotContains(t, content, banned, "%s should not contain %q", skillPath, banned)
		}

		if spec.Name == "auto" {
			assert.Contains(t, content, "얇은 라우터", skillPath)
			continue
		}

		assert.Contains(t, content, "## OpenCode Invocation", skillPath)
		assert.True(t, strings.Contains(content, "/auto "+strings.TrimPrefix(spec.Name, "auto-")) || strings.Contains(content, "/auto "+spec.Name), "%s should describe /auto invocation", skillPath)
	}

	bannedInCommands := []string{"spawn_agent", "Agent(", "mode =", "permissionMode"}
	for _, spec := range workflowSpecs {
		cmdPath := filepath.Join(dir, ".opencode", "commands", spec.Name+".md")
		data, readErr := os.ReadFile(cmdPath)
		require.NoError(t, readErr, cmdPath)
		content := string(data)
		assert.Contains(t, content, "`$ARGUMENTS`", cmdPath)
		assert.Contains(t, content, "Do not restate or expand the arguments", cmdPath)
		for _, banned := range bannedInCommands {
			assert.NotContains(t, content, banned, "%s should not contain %q", cmdPath, banned)
		}
	}
}
