package omp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
)

func claimSnapshot(t *testing.T, project *claimProject) map[string]string {
	t.Helper()
	snapshot := map[string]string{}
	for _, relative := range []string{configFile, OMPModelProjectOwnershipRelativePath,
		filepath.ToSlash(filepath.Join(".autopus", "omp-manifest.json")), DefaultOMPModelOverlayPath} {
		data, _ := project.read(relative)
		snapshot[relative] = data
	}
	for agent, digest := range ompTreeDigests(t, project.root, ".omp/agents") {
		snapshot[agent] = digest
	}
	return snapshot
}

// S22: the claim set moves between profile modes over one ledger, re-bases on
// a user edit outside it, rolls a failed transaction back, and native mode or
// clean restores the preimage.
func TestOMPClaimSet_TransitionsReBaseAndRestore(t *testing.T) {
	project := newClaimProject(t)
	project.mustRun("generate", configForOMP())
	cleanCopy := copyClaimProject(t, project.root)

	project.mustRun("update", integrationHarnessConfig(config.RoleModelConfigModeProjectManaged))
	data, _ := project.read(configFile)
	assert.Contains(t, data, "agentModelOverrides:")
	assert.Equal(t, config.OMPAutopusDisabledAgents(), toStrings(claimDisabledAgents(t, data)))
	ownership, _ := project.ledger()
	owners := claimOwners(ownership)
	assert.Equal(t, ompAgentSurfaceOwner, owners[ompDisabledAgentsKey])
	assert.Equal(t, "", owners[config.OMPNativeAgentModelOverridesKey], "the profile owns its keys")

	project.mustRun("update", integrationHarnessConfig(config.RoleModelConfigModeOverlay))
	data, _ = project.read(configFile)
	assert.Equal(t, claimSurfaceOnly, data)

	project.write(configFile, "# team note\n"+data)
	project.mustRun("update", integrationHarnessConfig(config.RoleModelConfigModeOverlay))
	data, _ = project.read(configFile)
	assert.Equal(t, "# team note\n"+claimSurfaceOnly, data)
	preimage, missing := project.preimage()
	assert.False(t, missing)
	assert.Equal(t, "# team note\n", string(preimage))
	before := claimSnapshot(t, project)

	ompRootedWriteFailureHook = func(root, _ string, index, total int) error {
		if root == project.root && index == total-1 {
			return errors.New("injected write failure")
		}
		return nil
	}
	err := project.run("update", integrationHarnessConfig(config.RoleModelConfigModeProjectManaged))
	ompRootedWriteFailureHook = nil
	require.ErrorContains(t, err, "injected write failure")
	require.NoError(t, config.Save(project.root, integrationHarnessConfig(config.RoleModelConfigModeOverlay)))
	assert.Equal(t, before, claimSnapshot(t, project), "a failed transaction restores every byte")
	assert.GreaterOrEqual(t, rolledBackOMPWrites(t, project.root), 1, "the transaction really opened")

	project.mustRun("update", nativeClaimConfig(configForOMP()))
	data, ok := project.read(configFile)
	require.True(t, ok)
	assert.Equal(t, "# team note\n", data)
	_, ledgerLeft := project.ledger()
	assert.False(t, ledgerLeft)

	require.NoError(t, NewWithRoot(cleanCopy).Clean(context.Background()))
	assert.NoFileExists(t, filepath.Join(cleanCopy, configFile))
	assert.NoFileExists(t, filepath.Join(cleanCopy, OMPModelProjectOwnershipRelativePath))
}

// copyClaimProject copies a project keeping each file's permission bits, which
// the owner-only ledger and manifest readers check.
func copyClaimProject(t *testing.T, root string) string {
	t.Helper()
	target := t.TempDir()
	require.NoError(t, filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		if entry.IsDir() {
			return os.MkdirAll(filepath.Join(target, rel), info.Mode().Perm())
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		return os.WriteFile(filepath.Join(target, rel), data, info.Mode().Perm())
	}))
	return target
}

func toStrings(values []any) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		text, _ := value.(string)
		result = append(result, text)
	}
	return result
}

func rolledBackOMPWrites(t *testing.T, root string) int {
	t.Helper()
	journals, err := filepath.Glob(filepath.Join(root, ".autopus", "txns", "*", "journal.json"))
	require.NoError(t, err)
	writes := 0
	for _, path := range journals {
		data, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		var journal adapter.TransactionJournal
		require.NoError(t, json.Unmarshal(data, &journal))
		if journal.Status != adapter.TransactionStatusRolledBack {
			continue
		}
		for _, entry := range journal.Entries {
			if entry.AfterChecksum != "" {
				writes++
			}
		}
	}
	return writes
}

// S24: with a profile claim in the same ledger, editing task.disabledAgents
// alone hands the key to the user; the profile keys stay; leaving the profile
// keeps the user's value; restoring the default set claims it again.
func TestOMPClaimSet_HandsAnEditedDisabledAgentsToTheUserUnderAProfile(t *testing.T) {
	managed := func() *config.HarnessConfig {
		return integrationHarnessConfig(config.RoleModelConfigModeProjectManaged)
	}
	overlay := func() *config.HarnessConfig { return integrationHarnessConfig(config.RoleModelConfigModeOverlay) }
	project := newClaimProject(t)
	project.mustRun("generate", managed())
	data, _ := project.read(configFile)
	edited := strings.Replace(data, "    - scout\n    - security-reviewer\n    - sonic\n", "    - scout\n", 1)
	require.NotEqual(t, data, edited)
	project.write(configFile, edited)

	project.mustRun("update", managed())
	current, _ := project.read(configFile)
	assert.Equal(t, edited, current, "the user's bytes stay")
	ownership, _ := project.ledger()
	assert.True(t, ownership.AgentSurfaceUserOwned)
	assert.NotContains(t, claimOwners(ownership), ompDisabledAgentsKey)
	assert.Contains(t, claimOwners(ownership), config.OMPNativeAgentModelOverridesKey)

	project.mustRun("update", overlay())
	current, ok := project.read(configFile)
	require.True(t, ok)
	assert.Equal(t, []string{"scout"}, toStrings(claimDisabledAgents(t, current)))
	assert.NotContains(t, current, "agentModelOverrides")
	ownership, _ = project.ledger()
	assert.True(t, ownership.markerOnly())

	project.write(configFile, claimSurfaceOnly)
	project.mustRun("update", overlay())
	ownership, _ = project.ledger()
	assert.False(t, ownership.AgentSurfaceUserOwned)
	assert.Equal(t, ompAgentSurfaceOwner, claimOwners(ownership)[ompDisabledAgentsKey])
}

// S24 (first copy): deleting the key is an edit of that key alone too, and
// the mark outlives the release until clean removes it.
func TestOMPClaimSet_DeletedDisabledAgentsStaysDeletedThroughRelease(t *testing.T) {
	managed := func() *config.HarnessConfig {
		return integrationHarnessConfig(config.RoleModelConfigModeProjectManaged)
	}
	overlay := func() *config.HarnessConfig { return integrationHarnessConfig(config.RoleModelConfigModeOverlay) }
	project := newClaimProject(t)
	project.mustRun("generate", managed())
	data, _ := project.read(configFile)
	deleted := strings.Replace(data, "  disabledAgents:\n    - scout\n    - security-reviewer\n    - sonic\n", "", 1)
	require.NotEqual(t, data, deleted)
	project.write(configFile, deleted)

	project.mustRun("update", managed())
	current, _ := project.read(configFile)
	assert.Equal(t, deleted, current)

	project.mustRun("update", overlay())
	assert.NoFileExists(t, filepath.Join(project.root, configFile), "ADK created the file")
	ownership, ok := project.ledger()
	require.True(t, ok)
	assert.True(t, ownership.markerOnly())
	ledger, _ := project.read(OMPModelProjectOwnershipRelativePath)
	project.mustRun("update", overlay())
	project.mustRun("update", overlay())
	assert.NoFileExists(t, filepath.Join(project.root, configFile))
	ledgerAgain, _ := project.read(OMPModelProjectOwnershipRelativePath)
	assert.Equal(t, ledger, ledgerAgain)

	require.NoError(t, project.adapter().Clean(context.Background()))
	assert.NoFileExists(t, filepath.Join(project.root, OMPModelProjectOwnershipRelativePath))
}

// S24 (second copy): with a profile claim, an edit outside the claim set is a
// conflict even when task.disabledAgents changed with it; nothing is written.
func TestOMPClaimSet_RefusesAnOutsideEditUnderAProfile(t *testing.T) {
	managed := func() *config.HarnessConfig {
		return integrationHarnessConfig(config.RoleModelConfigModeProjectManaged)
	}
	project := newClaimProject(t)
	project.mustRun("generate", managed())
	data, _ := project.read(configFile)
	edited := "# team note\n" + strings.Replace(data, "    - sonic\n", "", 1)
	project.write(configFile, edited)
	ledger, _ := project.read(OMPModelProjectOwnershipRelativePath)

	err := project.run("update", managed())
	require.ErrorContains(t, err, "managed_key_conflict")
	current, _ := project.read(configFile)
	assert.Equal(t, edited, current)
	ledgerAgain, _ := project.read(OMPModelProjectOwnershipRelativePath)
	assert.Equal(t, ledger, ledgerAgain)
}
