package opencode

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

func TestAdapter_Generate_PropagatesWorkspacePolicyContext(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir, WithCLIVersion("1.18.7"))

	_, err := a.Generate(context.Background(), config.DefaultFullConfig("demo"))
	require.NoError(t, err)

	autoSkill, err := os.ReadFile(filepath.Join(dir, ".agents", "skills", "auto", "SKILL.md"))
	require.NoError(t, err)
	assert.Contains(t, string(autoSkill), ".autopus/project/workspace.md")

	autoSetupSkill, err := os.ReadFile(filepath.Join(dir, ".agents", "skills", "auto-setup", "SKILL.md"))
	require.NoError(t, err)
	assert.Contains(t, string(autoSetupSkill), "workspace.md")
	assert.Contains(t, string(autoSetupSkill), "nested git repo")
	assert.Contains(t, string(autoSetupSkill), "auto qa init --project-dir <repo>")

	autoGoSkill, err := os.ReadFile(filepath.Join(dir, ".agents", "skills", "auto-go", "SKILL.md"))
	require.NoError(t, err)
	assert.Contains(t, string(autoGoSkill), ".autopus/project/workspace.md")

	autoSyncSkill, err := os.ReadFile(filepath.Join(dir, ".agents", "skills", "auto-sync", "SKILL.md"))
	require.NoError(t, err)
	assert.Contains(t, string(autoSyncSkill), ".autopus/project/workspace.md")
	assert.Contains(t, string(autoSyncSkill), "QA/Journey Pack")
}
