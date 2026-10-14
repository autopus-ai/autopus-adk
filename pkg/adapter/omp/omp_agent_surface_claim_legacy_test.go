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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
)

const ompLegacyBridgeBlock = markerBeginYml + "\nskills:\n  customDirectories:\n    - .agents/skills\n" + markerEndYml + "\n"

// seedOMPLegacyBridge leaves project as a native install that a
// pre-v0.50.118 release upgraded: user followed by the bridge block, recorded
// under the marker policy.
func seedOMPLegacyBridge(t *testing.T, project *claimProject, user string) {
	t.Helper()
	project.mustRun("generate", nativeClaimConfig(configForOMP()))
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

	project.mustRun("update", nativeClaimConfig(configForOMP()))
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

	project.mustRun("update", nativeClaimConfig(configForOMP()))
	_, exists := project.read(configFile)
	assert.False(t, exists, "a file only ADK ever wrote leaves with the release")
}
