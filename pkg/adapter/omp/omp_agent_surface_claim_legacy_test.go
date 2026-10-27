package omp

// A pre-v0.50.118 install kept a bridge block in .omp/config.yml between
// # AUTOPUS:BEGIN and # AUTOPUS:END (skills.customDirectories:
// [.agents/skills]) and recorded the file under the marker policy. When
// nothing claims the file, Update strips the block and keeps the user's bytes
// (migrateOMPLegacyBridgeConfigAt). Autopus mode claims task.disabledAgents in
// the same file, so the claim starts from the file without the block: it
// neither keeps the retired bridge nor lands inside its markers, and a
// release restores the user's bytes alone.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
)

const ompLegacyBridgeBlock = markerBeginYml + "\nskills:\n  customDirectories:\n    - .agents/skills\n" + markerEndYml + "\n"

// seedOMPNativeEraInstall leaves project as an install that holds no agent
// surface: one generated before v0.50.127, or last updated under the removed
// agent_source: native, which pruned the definitions and released the claim.
// The manifest records no agent definition, and neither a ledger nor
// .omp/config.yml carries a claim.
func seedOMPNativeEraInstall(t *testing.T, project *claimProject) {
	t.Helper()
	project.mustRun("generate", configForOMP())
	stripOMPAgentSurface(t, project.root)
}

// stripOMPAgentSurface removes, from a generated install, the agent
// definitions, the claimed project config, and the ledger, together with
// their manifest records.
func stripOMPAgentSurface(t *testing.T, root string) {
	t.Helper()
	manifest, err := adapter.LoadManifest(root, adapterName)
	require.NoError(t, err)
	for path := range manifest.Files {
		if isOMPAgentDefinitionPath(path) || path == configFile || path == OMPModelProjectOwnershipRelativePath {
			delete(manifest.Files, path)
			require.NoError(t, os.Remove(filepath.Join(root, filepath.FromSlash(path))))
		}
	}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	require.NoError(t, err)
	manifestPath := filepath.Join(root, ".autopus", adapterName+"-manifest.json")
	require.NoError(t, os.WriteFile(manifestPath, encoded, 0o600))
}

// seedOMPLegacyBridge leaves project as a native-era install that a
// pre-v0.50.118 release upgraded: user followed by the bridge block, recorded
// under the marker policy.
func seedOMPLegacyBridge(t *testing.T, project *claimProject, user string) {
	t.Helper()
	seedOMPNativeEraInstall(t, project)
	legacy := user + ompLegacyBridgeBlock
	project.write(configFile, legacy)
	manifest, err := adapter.LoadManifest(project.root, adapterName)
	require.NoError(t, err)
	manifest.Files[configFile] = adapter.ManifestFile{Checksum: adapter.Checksum(legacy), Policy: adapter.OverwriteMarker}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	require.NoError(t, err)
	project.write(".autopus/"+adapterName+"-manifest.json", string(encoded))
}

func TestOMPClaimSet_StartsFromTheConfigWithoutTheLegacyBridgeBlock(t *testing.T) {
	user := "theme:\n  dark: user-theme\n\n"
	plain := newClaimProject(t)
	plain.write(configFile, user)
	plain.mustRun("generate", configForOMP())
	want, ok := plain.read(configFile)
	require.True(t, ok)

	project := newClaimProject(t)
	seedOMPLegacyBridge(t, project, user)
	project.mustRun("update", configForOMP())

	data, ok := project.read(configFile)
	require.True(t, ok)
	assert.Equal(t, want, data, "the claim lands on the user's bytes as if the bridge had never been there")
	assert.NotContains(t, data, "customDirectories")
	assert.NotContains(t, data, markerBeginYml)
	manifest, err := adapter.LoadManifest(project.root, adapterName)
	require.NoError(t, err)
	assert.Equal(t, adapter.OverwriteAlways, manifest.Files[configFile].Policy)

	project.release()
	data, ok = project.read(configFile)
	require.True(t, ok)
	assert.Equal(t, user, data, "a release restores the user's bytes, not the retired bridge")
}

func TestOMPClaimSet_ReplacesAConfigThatHeldOnlyTheLegacyBridge(t *testing.T) {
	project := newClaimProject(t)
	seedOMPLegacyBridge(t, project, "")

	project.mustRun("update", configForOMP())
	data, ok := project.read(configFile)
	require.True(t, ok)
	assert.Equal(t, claimSurfaceOnly, data)

	project.release()
	_, exists := project.read(configFile)
	assert.False(t, exists, "a file only ADK ever wrote leaves with the release")
}
