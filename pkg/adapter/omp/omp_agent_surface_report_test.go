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

// Code review LOW: a project config the managed parser refuses is not the
// user's choice of task.disabledAgents. The report names it on its own, and
// generation still leaves the file alone.
func TestInspectOMPAgentSurface_ReportsAnUnclaimableConfigApart(t *testing.T) {
	for name, content := range map[string]string{
		"alias":          "base: &base [browser]\ntask:\n  disabledAgents: *base\n",
		"flow parent":    "task: {hidden: true}\n",
		"multi-document": "model: keep-me\n---\nother: value\n",
	} {
		t.Run(name, func(t *testing.T) {
			project := newClaimProject(t)
			project.write(configFile, content)

			project.mustRun("generate", configForOMP())
			report, err := InspectOMPAgentSurface(project.root, configForOMP())

			require.NoError(t, err)
			assert.True(t, report.ProjectConfigUnclaimable)
			assert.False(t, report.DisabledAgentsUserOwned, "the user chose no value")
			assert.False(t, report.DisabledAgentsClaimed)
			after, _ := project.read(configFile)
			assert.Equal(t, content, after)
		})
	}
}
