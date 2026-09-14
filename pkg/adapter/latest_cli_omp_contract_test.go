package adapter_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func assertOMPLatestCLIContract(t *testing.T, fixture latestCLIFixture) {
	t.Helper()
	manifest := fixture.manifests["omp"]
	counts := map[string]int{}
	for _, path := range manifestPaths(manifest) {
		for _, root := range []string{"skills", "commands", "rules"} {
			if strings.HasPrefix(path, ".omp/"+root+"/") {
				counts[root]++
			}
		}
		assert.False(t, strings.HasPrefix(path, ".agents/skills/"), path)
		assert.False(t, strings.HasPrefix(path, ".agents/commands/"), path)
		assert.False(t, strings.HasPrefix(path, ".omp/agents/"), "bundled agents must not be shadowed")
	}
	for _, root := range []string{"skills", "commands", "rules"} {
		assert.Greater(t, counts[root], 0, ".omp/%s must be manifest-owned", root)
	}
	assert.False(t, manifestHas(manifest, ".omp/config.yml"))
	assert.NoFileExists(t, filepath.Join(fixture.root, ".omp", "config.yml"))

	assertOMPReadinessProviderFree(t, fixture.root)
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
