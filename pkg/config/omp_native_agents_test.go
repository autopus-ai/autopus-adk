package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOMPNativeRoleBindingKeepsResearchAndReviewSeparate(t *testing.T) {
	for role, expected := range map[string]string{
		"explorer": "scout", "reviewer": "reviewer", "security-auditor": "security-reviewer",
		"planner": "task", "executor": "task", "validator": "task", "annotator": "task",
	} {
		actual, err := OMPNativeAgentForRole(role)
		require.NoError(t, err)
		assert.Equal(t, expected, actual)
	}
}

func TestEveryWorkflowRoleUsesAnAvailableNativeAgent(t *testing.T) {
	for _, role := range CanonicalAgentNames() {
		agent, err := OMPNativeAgentForRole(role)
		require.NoError(t, err)
		assert.Contains(t, OMPNativeAgentNames(), agent)
		assert.NotEqual(t, "sonic", agent, "a workflow role may require reasoning")
	}
	_, err := OMPNativeAgentForRole("invented-worker")
	require.Error(t, err)
}
