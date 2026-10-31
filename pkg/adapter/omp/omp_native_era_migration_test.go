package omp

// v0.50.131 removed agent_source: native. A project last updated under it
// holds what a project generated before v0.50.127 holds: no recorded agent
// definition and no claim, native generation having pruned the one and
// released the other. Once the key is deleted, the next update must migrate
// it the way the first v0.50.127 update did.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

func TestOMPUpdate_ClaimsTheAgentSurfaceOfANativeEraInstall(t *testing.T) {
	user := "theme: dark\n"
	project := newClaimProject(t)
	seedOMPNativeEraInstall(t, project)
	project.write(configFile, user)
	seeded, err := InspectOMPAgentSurface(project.root)
	require.NoError(t, err)
	require.Empty(t, seeded.RecordedAgents)
	require.False(t, seeded.DisabledAgentsClaimed)
	require.False(t, seeded.DisabledAgentsUserOwned)

	project.mustRun("update", configForOMP())

	data, _ := project.read(configFile)
	assert.Equal(t, user+claimSurfaceOnly, data, "the claim lands on the user's bytes")
	ownership, ok := project.ledger()
	require.True(t, ok)
	assert.Equal(t, map[string]string{ompDisabledAgentsKey: ompAgentSurfaceOwner}, claimOwners(ownership))
	assert.False(t, ownership.OriginalMissing)
	migrated, err := InspectOMPAgentSurface(project.root)
	require.NoError(t, err)
	assert.Equal(t, config.CanonicalAgentNames(), migrated.RecordedAgents)
	assert.Empty(t, migrated.MissingAgents)
	assert.True(t, migrated.DisabledAgentsClaimed)

	before := claimSnapshot(t, project)
	project.mustRun("update", configForOMP())
	assert.Equal(t, before, claimSnapshot(t, project), "a migrated project stays as it is")

	project.release()
	data, _ = project.read(configFile)
	assert.Equal(t, user, data, "a release gives the user's bytes back")
}

// A value the user chose while native mode left the key alone is the user's:
// the migration keeps it and only marks it.
func TestOMPUpdate_LeavesAUserValueOfANativeEraInstall(t *testing.T) {
	user := "task:\n  disabledAgents:\n    - browser\n"
	project := newClaimProject(t)
	seedOMPNativeEraInstall(t, project)
	project.write(configFile, user)

	project.mustRun("update", configForOMP())

	data, _ := project.read(configFile)
	assert.Equal(t, user, data)
	ownership, ok := project.ledger()
	require.True(t, ok)
	assert.True(t, ownership.markerOnly(), "%+v", ownership)
	report, err := InspectOMPAgentSurface(project.root)
	require.NoError(t, err)
	assert.True(t, report.DisabledAgentsUserOwned)
	assert.Equal(t, config.CanonicalAgentNames(), report.RecordedAgents)
}
