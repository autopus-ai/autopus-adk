package opencode

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateDefersWorkflowRulesAndPreservesUserInstructions(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	a := NewWithRoot(root, WithCLIVersion("1.18.7"))
	cfg := config.DefaultFullConfig("scoped-rules")
	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	old := map[string]any{"instructions": []string{"custom-policy.md"}}
	data, err := json.Marshal(old)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, configFile), data, 0o600))
	_, err = a.Update(context.Background(), cfg)
	require.NoError(t, err)
	doc := readConfigJSON(t, filepath.Join(root, configFile))
	instructions := jsonStringSlice(doc["instructions"])
	assert.Contains(t, instructions, "custom-policy.md")
	assert.Contains(t, instructions, ".opencode/rules/autopus/language-policy.md")
	// lore-commit is delivered by the plugin hook, not as a startup
	// instruction, and its body stays readable on disk.
	require.FileExists(t, filepath.Join(root, ".opencode/rules/autopus/lore-commit.md"))
}
