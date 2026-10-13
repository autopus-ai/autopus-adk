package adapter_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
)

func assertOMPLatestCLIContract(t *testing.T, fixture latestCLIFixture) {
	t.Helper()
	manifest := fixture.manifests["omp"]
	counts := map[string]int{}
	for _, path := range manifestPaths(manifest) {
		for _, root := range []string{"skills", "commands", "rules", "agents"} {
			if strings.HasPrefix(path, ".omp/"+root+"/") {
				counts[root]++
			}
		}
		assert.False(t, strings.HasPrefix(path, ".agents/skills/"), path)
		assert.False(t, strings.HasPrefix(path, ".agents/commands/"), path)
	}
	for _, root := range []string{"skills", "commands", "rules"} {
		assert.Greater(t, counts[root], 0, ".omp/%s must be manifest-owned", root)
	}
	assert.Equal(t, 16, counts["agents"], "the default source owns one definition per canonical agent")
	assert.True(t, manifestHas(manifest, ".omp/config.yml"), "the default source claims task.disabledAgents")
	assert.FileExists(t, filepath.Join(fixture.root, ".omp", "config.yml"))

	assertOMPReadinessProviderFree(t, fixture.root)
}

// agent_source: native keeps the v0.50.125 OMP surface: bundled agents are
// never shadowed and a plain install carries no base config.
func TestLatestCLIContract_OMPNativeSourceKeepsTheBundledRegistry(t *testing.T) {
	root := t.TempDir()
	cfg := config.DefaultFullConfig("latest-cli-native")
	cfg.Platforms = []string{"omp"}
	cfg.RoleModelPolicy.AgentSource = config.OMPAgentSourceNative
	require.NoError(t, config.Save(root, cfg))
	_, err := omp.NewWithRoot(root).Generate(context.Background(), cfg)
	require.NoError(t, err)
	manifest, err := adapter.LoadManifest(root, "omp")
	require.NoError(t, err)
	for _, path := range manifestPaths(manifest) {
		assert.False(t, strings.HasPrefix(path, ".omp/agents/"), "bundled agents must not be shadowed")
	}
	assert.False(t, manifestHas(manifest, ".omp/config.yml"))
	assert.NoFileExists(t, filepath.Join(root, ".omp", "config.yml"))
}

func assertLatestCLIManifestOwnership(t *testing.T, fixture latestCLIFixture) {
	t.Helper()
	owners := map[string][]string{}
	for _, platform := range latestCLIPlatforms {
		for _, path := range manifestPaths(fixture.manifests[platform]) {
			owners[path] = append(owners[path], platform)
		}
	}
	allowedShared := map[string][]string{
		"AGENTS.md":                        {"codex", "opencode"},
		".agents/plugins/marketplace.json": {"codex", "opencode"},
	}
	for path, pathOwners := range owners {
		if len(pathOwners) == 1 {
			continue
		}
		allowed, ok := allowedShared[path]
		require.True(t, ok, "unexpected shared manifest ownership: %s by %v", path, pathOwners)
		assert.ElementsMatch(t, allowed, pathOwners, path)
	}
	assert.Equal(t, []string{"opencode"}, owners["AGENTS.md"])
}
