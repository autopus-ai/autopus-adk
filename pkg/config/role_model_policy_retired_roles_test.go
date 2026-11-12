package config

import (
	"maps"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// undispatchedLegacyRoles are the legacy roles nothing routes work to.
var undispatchedLegacyRoles = []string{"architect", "debugger", "deep-worker", "devops", "perf-engineer", "spec-writer"}

// The role-model matrix and the balanced rungs carry exactly the generated
// agents and the legacy roles the binary still dispatches.
func TestRoleModelMatrix_CoversGeneratedAgentsAndDispatchedRolesOnly(t *testing.T) {
	t.Parallel()
	want := slices.Sorted(slices.Values(append(CanonicalAgentNames(), dispatchedLegacyRoles...)))
	assert.Equal(t, want, slices.Sorted(maps.Keys(capabilityBySourceAgent)))
	assert.Equal(t, want, slices.Sorted(maps.Keys(balancedRungByAgent)))
	for _, role := range dispatchedLegacyRoles {
		assert.True(t, IsLegacyAgentName(role), role)
	}
	for _, role := range undispatchedLegacyRoles {
		assert.True(t, IsLegacyAgentName(role), role)
		assert.NotContains(t, capabilityBySourceAgent, role)
	}
}

// Every role a runtime path dispatches keeps a row: the team workflow phases,
// the OMP pipeline phases, and the rows OMP's bundled agents borrow.
func TestRoleModelMatrix_KeepsEveryDispatchedRole(t *testing.T) {
	t.Parallel()
	for _, native := range []string{"task", "scout", "sonic", "security-reviewer", "reviewer"} {
		representative, err := OMPNativeAgentRepresentative(native)
		require.NoError(t, err, native)
		assert.Contains(t, capabilityBySourceAgent, representative, native)
	}
	for _, capability := range providerNeutralCapabilities {
		assert.Contains(t, capabilityBySourceAgent, canonicalAgentByCapability[capability], capability)
	}
	for _, role := range []string{"planner", "tester", "executor", "annotator", "security-auditor", "validator", "reviewer"} {
		_, standard := DefaultFullConfig("p").Quality.NativeBalancedAgentCandidate(QualityProviderClaude, role)
		assert.True(t, standard, role)
	}
}

// A policy written for an older release may still pin a retired role. The
// key keeps validating: it resolves through the bundled agent it collapsed
// onto, task, which borrows the planner row.
func TestResolveOMPPolicyAgent_RetiredRoleKeyCollapsesOntoTask(t *testing.T) {
	t.Parallel()
	for _, role := range undispatchedLegacyRoles {
		route, err := ResolveOMPPolicyAgent(role)
		require.NoError(t, err, role)
		assert.Equal(t, OMPPolicyAgentRoute{
			Key: role, Native: "task", Role: "autopus_planner", Capability: CapabilityDeepReasoning,
		}, route, role)
	}
}

// Off the matrix a retired role resolves through its legacy tier on the plain
// ladder, as any role outside the standard placement does.
func TestClaudeAgentProfile_RetiredRoleUsesItsLegacyTier(t *testing.T) {
	t.Parallel()
	q := DefaultFullConfig("p").Quality
	model, _ := q.ClaudeAgentProfile("architect", "sonnet")
	assert.Equal(t, "opus", model)
	assert.Equal(t, "opus", q.EffectiveAgentTier("architect", "sonnet"))
}
