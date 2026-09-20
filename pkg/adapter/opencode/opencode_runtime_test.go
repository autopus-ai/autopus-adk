package opencode

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

func TestAdapter_Generate_RegistersManagedPlugin(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir, WithCLIVersion("1.18.7"))

	_, err := a.Generate(context.Background(), config.DefaultFullConfig("demo"))
	require.NoError(t, err)

	configDoc := readConfigJSON(t, filepath.Join(dir, "opencode.json"))
	plugins := jsonPluginSlice(configDoc["plugin"])
	assert.Contains(t, plugins, ".opencode/plugins/autopus-hooks.js")

	autoCommand, readErr := os.ReadFile(filepath.Join(dir, ".opencode", "commands", "auto.md"))
	require.NoError(t, readErr)
	content := string(autoCommand)
	assert.Contains(t, content, "Immediately load skill `auto`")

	autoSkill, readErr := os.ReadFile(filepath.Join(dir, ".agents", "skills", "auto", "SKILL.md"))
	require.NoError(t, readErr)
	content = string(autoSkill)
	assert.Contains(t, content, "`status`")
	assert.Contains(t, content, "`map`")
	assert.Contains(t, content, "`why`")
	assert.Contains(t, content, "`verify`")
	assert.Contains(t, content, "`secure`")
	assert.Contains(t, content, "`test`")
	assert.Contains(t, content, "`dev`")
	assert.Contains(t, content, "`doctor`")

	// The reusable (non-workflow) surface is emitted alongside the routes.
	assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "planning", "SKILL.md"))
}

func TestAdapter_Validate_ReportsMissingPluginRegistration(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir, WithCLIVersion("1.18.7"))

	_, err := a.Generate(context.Background(), config.DefaultFullConfig("demo"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "opencode.json"), []byte("{\n  \"$schema\": \"https://opencode.ai/config.json\",\n  \"instructions\": []\n}\n"), 0644))

	errList, validateErr := a.Validate(context.Background())
	require.NoError(t, validateErr)
	assert.NotEmpty(t, errList)
	var found bool
	for _, item := range errList {
		if item.File == "opencode.json" && item.Level == "warning" {
			found = found || strings.Contains(item.Message, "OpenCode hook plugin 등록 누락")
		}
	}
	assert.True(t, found)
}
