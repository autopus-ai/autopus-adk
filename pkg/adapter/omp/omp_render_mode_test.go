package omp

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

func generatedOMPTree(t *testing.T, cfg *config.HarnessConfig) map[string]string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, config.Save(root, cfg))
	_, err := NewWithRoot(root).Generate(context.Background(), cfg)
	require.NoError(t, err)
	tree := map[string]string{}
	require.NoError(t, filepath.WalkDir(filepath.Join(root, ".omp"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, readErr := os.ReadFile(path)
		rel, _ := filepath.Rel(root, path)
		tree[filepath.ToSlash(rel)] = string(data)
		return readErr
	}))
	return tree
}

// S14 (oracle): no generated OMP file selects an agent autopus mode disables,
// and the pipeline skill no longer says there is no Autopus agent.
func TestOMPGenerate_AutopusSurfaceNeverSelectsADisabledAgent(t *testing.T) {
	tree := generatedOMPTree(t, configForOMP())
	require.NotEmpty(t, tree)
	for path, body := range tree {
		for _, disabled := range config.OMPAutopusDisabledAgents() {
			assert.NotContains(t, body, `"agent": "`+disabled+`"`, path)
		}
		assert.NotContains(t, body, "There is no Autopus-specific agent", path)
	}
	pipeline := tree[".omp/skills/agent-pipeline/SKILL.md"]
	assert.Contains(t, pipeline, "16 Autopus agents")
	assert.Contains(t, pipeline, "`task.disabledAgents`")
}

// S14 (native): agent_source: native keeps the bundled-registry guidance.
func TestOMPGenerate_NativePipelineKeepsTheBundledRegistrySentence(t *testing.T) {
	cfg := configForOMP()
	cfg.RoleModelPolicy.AgentSource = config.OMPAgentSourceNative
	pipeline := generatedOMPTree(t, cfg)[".omp/skills/agent-pipeline/SKILL.md"]
	assert.Contains(t, pipeline, "OMP registers five agents")
	assert.Contains(t, pipeline, "There is no Autopus-specific agent to select")
	assert.False(t, strings.Contains(pipeline, "16 Autopus agents"))
}
