package cli

// SPEC-MODELUX-003 S12 and S13 after v0.50.129 removed agent_source: native.
// A project last updated under native holds what one generated before
// v0.50.127 holds: no recorded agent definition and no claim. The next update
// migrates it, the removed key fails the update with the way back, and
// deleting the key updates as before. autopus.yaml is never written.

import (
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
)

// ompMigrationStripAgentSurface removes the agent definitions, the claimed
// project config, and the ledger from the OMP files of dir, together with
// their manifest records: the layout native generation left behind.
func ompMigrationStripAgentSurface(t *testing.T, dir string) {
	t.Helper()
	manifestPath := filepath.Join(dir, ".autopus", "omp-manifest.json")
	body, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	var manifest map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &manifest))
	var files map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(manifest["files"], &files))
	for path := range files {
		if strings.HasPrefix(path, ".omp/agents/") || path == ".omp/config.yml" ||
			path == omp.OMPModelProjectOwnershipRelativePath {
			delete(files, path)
			require.NoError(t, os.Remove(filepath.Join(dir, filepath.FromSlash(path))))
		}
	}
	manifest["files"], err = json.Marshal(files)
	require.NoError(t, err)
	body, err = json.MarshalIndent(manifest, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(manifestPath, body, 0o600))
}

func TestOMPAgentMigration_S12_S13_UpdateMigratesANativeEraProject(t *testing.T) {
	newComposeUpdateRun(t, false, "")
	usePlatformOMPModelRunner(t, &ompCLIFakeRunner{fail: true})
	dir := t.TempDir()
	cfg := config.DefaultFullConfig("nm")
	cfg.Platforms = []string{"omp"}
	require.NoError(t, config.Save(dir, cfg))
	ompMigrationUpdate(t, dir)
	ompMigrationStripAgentSurface(t, dir)
	yamlPath := filepath.Join(dir, "autopus.yaml")
	nmYAML, err := os.ReadFile(yamlPath)
	require.NoError(t, err)
	require.NotContains(t, string(nmYAML), "role_model_policy")
	require.Empty(t, ompMigrationAgentFiles(t, dir), "NM is the v0.50.125 OMP layout")

	first := ompMigrationUpdate(t, dir)

	for _, agent := range config.CanonicalAgentNames() {
		assert.Contains(t, first, "    + .omp/agents/"+agent+".md\n")
	}
	assert.Contains(t, first, "    task.disabledAgents: scout, security-reviewer, sonic, designer, librarian, seance\n")
	assert.Contains(t, first, "\n    OMP는 이제 Autopus 에이전트 17개를 씁니다.\n")
	assert.NotContains(t, first, "agent_source: native", "the migration offers no way back")
	assert.Less(t, strings.Index(first, "+ .omp/agents/planner.md"), strings.Index(first, "✓ omp updated"),
		"the lines preview the write")
	assert.Len(t, ompMigrationAgentFiles(t, dir), 17)
	ompConfig, err := os.ReadFile(filepath.Join(dir, ".omp", "config.yml"))
	require.NoError(t, err)
	assert.Contains(t, string(ompConfig), "disabledAgents")
	assert.FileExists(t, filepath.Join(dir, filepath.FromSlash(omp.OMPModelProjectOwnershipRelativePath)))
	afterYAML, err := os.ReadFile(yamlPath)
	require.NoError(t, err)
	assert.Equal(t, sha256.Sum256(nmYAML), sha256.Sum256(afterYAML), "the migration does not write autopus.yaml")
	migrated := ompMigrationFileSums(t, dir)

	second := ompMigrationUpdate(t, dir)

	assert.NotContains(t, second, "+ .omp/agents/")
	assert.NotContains(t, second, "OMP는 이제")
	assert.Equal(t, migrated, ompMigrationFileSums(t, dir), "a second update changes no OMP file")

	native := append(append([]byte(nil), nmYAML...), "role_model_policy:\n  agent_source: native\n"...)
	require.NoError(t, os.WriteFile(yamlPath, native, 0o644))
	out, err := runComposeUpdate(t, strings.NewReader(""), "--dir", dir, "--yes")

	require.Error(t, err)
	assert.Contains(t, err.Error()+out, "role_model_policy.agent_source_removed")
	assert.Contains(t, err.Error()+out, "delete role_model_policy.agent_source from autopus.yaml")
	assert.Equal(t, migrated, ompMigrationFileSums(t, dir), "the refused update writes no OMP file")

	require.NoError(t, os.WriteFile(yamlPath, nmYAML, 0o644))
	ompMigrationUpdate(t, dir)

	assert.Equal(t, migrated, ompMigrationFileSums(t, dir), "deleting the key updates as before")
}
