package omp

// Code review LOW: OMP rules render dispatch text in the generation's agent
// source, like skills and workflows, so a rule that names an agent stays
// byte-identical to v0.50.125 in native mode.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderOMPRule_FollowsTheAgentSource(t *testing.T) {
	t.Parallel()
	rule := []byte("# Dispatch\n\nAgent(subagent_type=\"explorer\", prompt=\"look\")\n")

	autopus, err := renderOMPRule("dispatch.md", rule, configForOMP())
	require.NoError(t, err)
	native, err := renderOMPRule("dispatch.md", rule, nativeConfigForOMP())
	require.NoError(t, err)

	assert.Contains(t, autopus, `"agent": "explorer"`)
	assert.Contains(t, native, `"agent": "scout"`)
	assert.NotContains(t, native, `"agent": "explorer"`)
}

func TestPrepareRuleMappings_RendersTheEmbeddedRulesInBothSources(t *testing.T) {
	t.Parallel()
	a := NewWithRoot(t.TempDir())
	autopus, err := a.prepareRuleMappings(configForOMP())
	require.NoError(t, err)
	native, err := a.prepareRuleMappings(nativeConfigForOMP())
	require.NoError(t, err)

	require.Equal(t, len(autopus), len(native))
	for i := range autopus {
		assert.Equal(t, autopus[i].TargetPath, native[i].TargetPath)
	}
}
