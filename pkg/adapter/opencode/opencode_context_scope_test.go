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
	a := NewWithRoot(root)
	cfg := config.DefaultFullConfig("scoped-rules")
	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	old := map[string]any{"instructions": []string{"custom-policy.md", ".opencode/rules/autopus/spec-quality.md"}}
	data, err := json.Marshal(old)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, configFile), data, 0o600))
	_, err = a.Update(context.Background(), cfg)
	require.NoError(t, err)
	doc := readConfigJSON(t, filepath.Join(root, configFile))
	instructions := jsonStringSlice(doc["instructions"])
	assert.Contains(t, instructions, "custom-policy.md")
	assert.Contains(t, instructions, ".opencode/rules/autopus/branding.md")
	for _, name := range []string{"context7-docs", "doc-storage", "spec-quality", "techstack-freshness"} {
		path := ".opencode/rules/autopus/" + name + ".md"
		assert.NotContains(t, instructions, path, "workflow-only rules are not startup instructions")
		require.FileExists(t, filepath.Join(root, path), "explicit workflow references remain readable")
	}
}
