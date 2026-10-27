package omp

// Code review LOW: OMP rules render dispatch text like skills and workflows,
// so a rule that names an agent dispatches the Autopus agent of its role.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderOMPRule_DispatchesTheAutopusAgent(t *testing.T) {
	t.Parallel()
	rule := []byte("# Dispatch\n\nAgent(subagent_type=\"explorer\", prompt=\"look\")\n")

	rendered, err := renderOMPRule("dispatch.md", rule)
	require.NoError(t, err)

	assert.Contains(t, rendered, `"agent": "explorer"`)
	assert.NotContains(t, rendered, `"agent": "scout"`)
}

func TestPrepareRuleMappings_RendersEveryEmbeddedRule(t *testing.T) {
	t.Parallel()
	rules, err := NewWithRoot(t.TempDir()).prepareRuleMappings()
	require.NoError(t, err)

	require.NotEmpty(t, rules)
	for _, rule := range rules {
		assert.Contains(t, rule.TargetPath, ompRuleFilePrefix, rule.TargetPath)
		assert.NotEmpty(t, rule.Content, rule.TargetPath)
	}
}
