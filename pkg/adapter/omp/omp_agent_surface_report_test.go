package omp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// The operator surfaces read the agent surface without writing: which
// definitions the manifest records, which same-name files are the user's,
// which are missing, and whose task.disabledAgents it is.
func TestInspectOMPAgentSurface_ReportsFilesAndDisabledAgentsOwnership(t *testing.T) {
	project := newClaimProject(t)
	project.write(".omp/agents/reviewer.md", "---\nname: reviewer\ndescription: mine\n---\nmine\n")
	project.write(configFile, "task:\n  disabledAgents:\n    - browser\n")

	before, err := InspectOMPAgentSurface(project.root, configForOMP())
	require.NoError(t, err)
	assert.Equal(t, config.OMPAgentSourceAutopus, before.Source)
	assert.Empty(t, before.RecordedAgents)
	assert.Equal(t, []string{"reviewer"}, before.UserAgentFiles)
	assert.Len(t, before.MissingAgents, 15)
	assert.True(t, before.DisabledAgentsUserOwned, "a value the user chose is theirs before any claim")

	project.mustRun("generate", configForOMP())
	after, err := InspectOMPAgentSurface(project.root, configForOMP())
	require.NoError(t, err)
	assert.Len(t, after.RecordedAgents, 15)
	assert.Equal(t, []string{"reviewer"}, after.UserAgentFiles)
	assert.Empty(t, after.MissingAgents)
	assert.True(t, after.DisabledAgentsUserOwned)
	assert.False(t, after.DisabledAgentsClaimed)

	require.NoError(t, os.Remove(filepath.Join(project.root, ".omp", "agents", "validator.md")))
	missing, err := InspectOMPAgentSurface(project.root, configForOMP())
	require.NoError(t, err)
	assert.Equal(t, []string{"validator"}, missing.MissingAgents)
}

func TestInspectOMPAgentSurface_ReportsAnADKClaim(t *testing.T) {
	project := newClaimProject(t)
	project.mustRun("generate", configForOMP())
	report, err := InspectOMPAgentSurface(project.root, configForOMP())
	require.NoError(t, err)
	assert.True(t, report.DisabledAgentsClaimed)
	assert.False(t, report.DisabledAgentsUserOwned)
	assert.Len(t, report.RecordedAgents, 16)
}
