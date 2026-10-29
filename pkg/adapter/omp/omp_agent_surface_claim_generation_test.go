package omp

// A project v0.50.127 claimed holds the retired five-name task.disabledAgents
// value, recorded as ADK's emission. When the default grows (omp 18.12.3
// bundles seance), that value is still ADK's, not the user's: the next
// update re-claims the current list, a value the user chose stays theirs,
// and a release restores whichever generation the preimage held.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
)

// seedOMPRetiredClaim rewrites a freshly claimed project into what v0.50.127
// left behind: .omp/config.yml holds the retired value, and the ledger and
// manifest record that file as ADK's last emission.
func seedOMPRetiredClaim(t *testing.T, project *claimProject, retired []string) string {
	t.Helper()
	current, ok := project.read(configFile)
	require.True(t, ok)
	emitted, err := setOMPDisabledAgents([]byte(current), retired, true)
	require.NoError(t, err)
	require.NotEqual(t, current, string(emitted))
	project.write(configFile, string(emitted))

	ownership, ok := project.ledger()
	require.True(t, ok)
	salt, err := decodeOMPModelProjectSalt(ownership.Salt)
	require.NoError(t, err)
	commitment := ompModelProjectCommitment(salt, emitted, false)
	ownership.InitialEmittedCommitment, ownership.LastEmittedCommitment = commitment, commitment
	fingerprint := ompDisabledAgentsFingerprint(retired, true)
	for _, keys := range [][]ompModelProjectOwnedKey{ownership.InitialManagedKeys, ownership.ManagedKeys} {
		for index := range keys {
			if keys[index].Path == ompDisabledAgentsKey {
				keys[index].Fingerprint = fingerprint
			}
		}
	}
	_, ledger, err := canonicalOMPModelProjectOwnership(ownership)
	require.NoError(t, err)
	project.write(OMPModelProjectOwnershipRelativePath, string(ledger))

	manifest, err := adapter.LoadManifest(project.root, adapterName)
	require.NoError(t, err)
	for path, data := range map[string][]byte{configFile: emitted, OMPModelProjectOwnershipRelativePath: ledger} {
		entry, recorded := manifest.Files[path]
		require.True(t, recorded, path)
		entry.Checksum = adapter.Checksum(string(data))
		manifest.Files[path] = entry
	}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	require.NoError(t, err)
	project.write(".autopus/"+adapterName+"-manifest.json", string(encoded))
	return string(emitted)
}

func TestOMPClaimSet_ReclaimsARetiredDefaultGeneration(t *testing.T) {
	retired := config.OMPAutopusRetiredDisabledAgents()[0]
	user := "theme: dark\n"

	t.Run("an unchanged emission is re-claimed", func(t *testing.T) {
		project := newClaimProject(t)
		project.write(configFile, user)
		project.mustRun("generate", configForOMP())
		seeded := seedOMPRetiredClaim(t, project, retired)
		before, _ := project.ledger()

		project.mustRun("update", configForOMP())

		data, _ := project.read(configFile)
		assert.Equal(t, user+claimSurfaceOnly, data)
		assert.NotEqual(t, seeded, data)
		ownership, _ := project.ledger()
		assert.False(t, ownership.AgentSurfaceUserOwned)
		assert.Equal(t, ompAgentSurfaceOwner, claimOwners(ownership)[ompDisabledAgentsKey])
		assert.Equal(t, before.Salt, ownership.Salt, "the ledger keeps its origin")
		project.release()
		released, _ := project.read(configFile)
		assert.Equal(t, user, released, "the release restores the preimage, not a generation")
	})

	t.Run("an edit beside the retired value is not a takeover", func(t *testing.T) {
		project := newClaimProject(t)
		project.write(configFile, user)
		project.mustRun("generate", configForOMP())
		seeded := seedOMPRetiredClaim(t, project, retired)
		project.write(configFile, strings.Replace(seeded, "theme: dark", "theme: light", 1))

		project.mustRun("update", configForOMP())

		data, _ := project.read(configFile)
		assert.Equal(t, "theme: light\n"+claimSurfaceOnly, data)
		ownership, _ := project.ledger()
		assert.False(t, ownership.AgentSurfaceUserOwned)
		assert.Equal(t, ompAgentSurfaceOwner, claimOwners(ownership)[ompDisabledAgentsKey])
		project.release()
		released, _ := project.read(configFile)
		assert.Equal(t, "theme: light\n", released)
	})

	t.Run("a value the user chose stays theirs", func(t *testing.T) {
		project := newClaimProject(t)
		project.write(configFile, user)
		project.mustRun("generate", configForOMP())
		seeded := seedOMPRetiredClaim(t, project, retired)
		edited, err := setOMPDisabledAgents([]byte(seeded), []string{"scout"}, true)
		require.NoError(t, err)
		project.write(configFile, string(edited))

		project.mustRun("update", configForOMP())

		data, _ := project.read(configFile)
		assert.Equal(t, string(edited), data)
		ownership, _ := project.ledger()
		assert.True(t, ownership.AgentSurfaceUserOwned)
		assert.NotContains(t, claimOwners(ownership), ompDisabledAgentsKey)
	})

	t.Run("a retired list written under the current claim is the user's", func(t *testing.T) {
		// The ledger says this release emitted the current list, so the old
		// five names can only have come from the user: an edit of the key
		// alone, kept like any other.
		project := newClaimProject(t)
		project.write(configFile, user)
		project.mustRun("generate", configForOMP())
		data, _ := project.read(configFile)
		edited, err := setOMPDisabledAgents([]byte(data), retired, true)
		require.NoError(t, err)
		require.NotEqual(t, data, string(edited))
		project.write(configFile, string(edited))

		project.mustRun("update", configForOMP())

		current, _ := project.read(configFile)
		assert.Equal(t, string(edited), current, "the user's bytes stay")
		ownership, _ := project.ledger()
		assert.True(t, ownership.AgentSurfaceUserOwned)
		assert.NotContains(t, claimOwners(ownership), ompDisabledAgentsKey)
	})

	t.Run("a retired value the user had is restored as it was", func(t *testing.T) {
		// S8's twin for a retired generation: the file already held the
		// v0.50.127 list when ADK first claimed it, so the claim takes it as
		// ADK's value, writes the current list, and the release puts the
		// retired list back byte for byte.
		original, err := setOMPDisabledAgents([]byte(user), retired, true)
		require.NoError(t, err)
		project := newClaimProject(t)
		project.write(configFile, string(original))

		project.mustRun("generate", configForOMP())

		data, _ := project.read(configFile)
		assert.Equal(t, user+claimSurfaceOnly, data)
		ownership, _ := project.ledger()
		assert.False(t, ownership.AgentSurfaceUserOwned)
		assert.Equal(t, ompAgentSurfaceOwner, claimOwners(ownership)[ompDisabledAgentsKey])
		project.release()
		released, _ := project.read(configFile)
		assert.Equal(t, string(original), released)
	})
}
