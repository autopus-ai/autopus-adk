package cli

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/pipeline"
)

// Every retired ADK role name must be gone from the operator-facing preview.
// A row naming one would tell the operator to configure an agent OMP does not
// register.
func TestOMPProfilePlanNamesOnlyBundledAgents(t *testing.T) {
	root, runner := writeOMPBalancedProject(t)

	payload := planOMPProfileJSON(t, root, runner, "balanced")

	bundled := make(map[string]bool, len(config.OMPNativeAgentNames()))
	for _, native := range config.OMPNativeAgentNames() {
		bundled[native] = true
	}
	require.Len(t, payload.Agents, len(bundled))
	for _, row := range payload.Agents {
		assert.True(t, bundled[row.Agent], "preview names a non-bundled agent: %s", row.Agent)
		assert.NotContains(t, row.EffectiveSelector, "@",
			"the effective selector must be a concrete model, not an alias: %s", row.Agent)
	}
	for _, retired := range []string{
		"executor", "planner", "tester", "validator", "explorer", "spec-writer",
		"architect", "debugger", "deep-worker", "devops", "annotator",
		"security-auditor", "ux-validator", "frontend-specialist", "perf-engineer",
	} {
		for _, row := range payload.Agents {
			assert.NotEqual(t, retired, row.Agent)
		}
	}
}

// Reasoning work must never be routed onto the mechanical agent, and read-only
// exploration must never claim the reasoning model. Both would be silent
// quality changes introduced by the collapse itself.
func TestOMPProfilePlanNeverDowngradesReasoningOntoMechanicalAgents(t *testing.T) {
	root, runner := writeOMPBalancedProject(t)

	payload := planOMPProfileJSON(t, root, runner, "balanced")

	task := agentPreviewRow(t, payload, "task")
	assert.Equal(t, "planner", task.PolicyKey,
		"the general agent must inherit the planning role's model, not an implementation rung")
	assert.Equal(t, config.OMPAgentRoleName("planner"), task.Role)
	for _, mechanical := range []string{"scout", "sonic"} {
		row := agentPreviewRow(t, payload, mechanical)
		assert.NotEqual(t, task.EffectiveSelector, row.EffectiveSelector,
			"%s must not receive the reasoning model", mechanical)
	}
	assert.Equal(t, config.OMPAgentRoleName("explorer"), agentPreviewRow(t, payload, "scout").Role)
	assert.Equal(t, config.OMPAgentRoleName("validator"), agentPreviewRow(t, payload, "sonic").Role)
	assert.Equal(t,
		config.OMPAgentRoleName("security-auditor"),
		agentPreviewRow(t, payload, "security-reviewer").Role,
	)
}

// An operator may address a bundled agent directly. The pin then governs that
// agent's row, and the stored key is the native name the operator typed. The
// pinned model still has to satisfy the capability the row carries, so `task`
// takes a deep-reasoning model rather than any available selector.
func TestOMPProfilePlanAcceptsNativeAgentOverride(t *testing.T) {
	root, runner := writeOMPBalancedProject(t)

	payload := planOMPProfileJSON(
		t, root, runner, "balanced", "--agent", "task=anthropic/claude-fable-5-1:max",
	)

	row := agentPreviewRow(t, payload, "task")
	assert.Equal(t, ompProfileSourceAgent, row.Source)
	assert.Equal(t, "task", row.PolicyKey)
	assert.Equal(t, "anthropic/claude-fable-5-1", row.EffectiveSelector)
	assert.Equal(t, "max", row.EffectiveThinking)
	assert.Equal(t, []string{"task"}, payload.Persisted.Agents)
}

// A full per-role policy names 16 roles with different models, so two keys
// landing on one bundled agent is the normal case. The representative role is
// the declared tie-break, and the row reports it, so the operator can see which
// entry governs instead of guessing which model was dropped.
func TestOMPProfilePlanResolvesCollapsingRoutesThroughTheRepresentative(t *testing.T) {
	root, runner := writeOMPBalancedProject(t)

	payload := planOMPProfileJSON(
		t, root, runner, "balanced",
		"--agent", "executor=anthropic/claude-sonnet-5:max",
		"--agent", "planner=anthropic/claude-fable-5-1:max",
	)

	row := agentPreviewRow(t, payload, "task")
	assert.Empty(t, payload.Blockers)
	assert.Equal(t, "planner", row.PolicyKey)
	assert.Equal(t, "anthropic/claude-fable-5-1", row.EffectiveSelector)
}

// Without the representative row nothing ranks the disagreeing entries, so the
// plan refuses and names them rather than discarding one at random.
func TestOMPProfilePlanRefusesConflictingRoutesForOneBundledAgent(t *testing.T) {
	root, runner := writeOMPBalancedProject(t)
	before, err := os.ReadFile(filepath.Join(root, autopusConfigName))
	require.NoError(t, err)
	dir := root

	text, err := executeOMPSubcommandExpectingError(
		t, newPlatformOMPProfileApplyCmd(&dir, ompBalancedDeps(runner, nil)),
		"balanced", "--plan",
		"--agent", "executor=anthropic/claude-sonnet-5:max",
		"--agent", "tester=anthropic/claude-fable-5-1:max",
	)

	assert.Contains(t, text, "omp_native_agent_conflict")
	assert.Contains(t, text, "keep exactly one entry")
	assert.Contains(t, text, "executor")
	assert.Contains(t, text, "tester")
	require.Error(t, err)

	after, readErr := os.ReadFile(filepath.Join(root, autopusConfigName))
	require.NoError(t, readErr)
	assert.Equal(t, before, after, "a refused plan must not write anything")
}

// The same selector written twice for two collapsing roles is redundant, not
// contradictory: there is only one model to apply, so it applies.
func TestOMPProfilePlanAcceptsAgreeingRoutesForOneBundledAgent(t *testing.T) {
	root, runner := writeOMPBalancedProject(t)

	payload := planOMPProfileJSON(
		t, root, runner, "balanced",
		"--agent", "executor=anthropic/claude-sonnet-5:max",
		"--agent", "planner=anthropic/claude-sonnet-5:max",
	)

	row := agentPreviewRow(t, payload, "task")
	assert.Empty(t, payload.Blockers)
	assert.Equal(t, "anthropic/claude-sonnet-5", row.EffectiveSelector)
	assert.Equal(t, ompProfileSourceAgent, row.Source)
}

func TestOMPProfileAgentOverrideRejectsNamesNoRegistryHas(t *testing.T) {
	for _, name := range []string{"autopus_executor", "sonic-fast", "Task"} {
		_, err := parseOMPProfileAgentAssignment(name + "=anthropic/claude-sonnet-5:max")
		require.Error(t, err, name)
		assert.Contains(t, err.Error(), "agent_override_unknown_agent", name)
	}
}

// Four pipeline phases legitimately run on the same bundled agent. The loader
// must accept that instead of reporting a duplicate route, and must still
// refuse a receipt that omits an agent a phase needs.
func TestPipelineOMPPhaseModelsCollapseOntoBundledAgents(t *testing.T) {
	projectDir := t.TempDir()
	writeOMPNativePhaseReceipt(t, projectDir, map[string]string{
		"scout": "provider/scout-model", "reviewer": "provider/review-model",
		"security-reviewer": "provider/security-model", "task": "provider/task-model",
		"sonic": "provider/sonic-model",
	})

	models, err := loadPipelineOMPPhaseModels(projectDir, writePipelineOMPVersionFixture(t, "omp/17.1.8"))

	require.NoError(t, err)
	assert.Equal(t, map[pipeline.PhaseID]string{
		pipeline.PhasePlan:         "provider/task-model",
		pipeline.PhaseTestScaffold: "provider/task-model",
		pipeline.PhaseImplement:    "provider/task-model",
		pipeline.PhaseValidate:     "provider/task-model",
		pipeline.PhaseReview:       "provider/review-model",
	}, models)
}

func TestPipelineOMPPhaseModelsRejectReceiptMissingARequiredAgent(t *testing.T) {
	projectDir := t.TempDir()
	writeOMPNativePhaseReceipt(t, projectDir, map[string]string{
		"scout": "provider/scout-model", "task": "provider/task-model",
		"sonic": "provider/sonic-model",
	})

	models, err := loadPipelineOMPPhaseModels(projectDir, writePipelineOMPVersionFixture(t, "omp/17.1.8"))

	require.ErrorContains(t, err, "no route for native agent reviewer")
	assert.Nil(t, models)
}

// A receipt still keyed by the retired per-role names resolves to nothing: the
// loader must fail loudly rather than invent a selector for a missing agent.
func TestPipelineOMPPhaseModelsRejectRetiredRoleKeyedReceipt(t *testing.T) {
	projectDir := t.TempDir()
	writeOMPNativePhaseReceipt(t, projectDir, map[string]string{
		"planner": "provider/plan-model", "tester": "provider/test-model",
		"executor": "provider/implement-model", "validator": "provider/validate-model",
		"reviewer": "provider/review-model",
	})

	models, err := loadPipelineOMPPhaseModels(projectDir, writePipelineOMPVersionFixture(t, "omp/17.1.8"))

	require.ErrorContains(t, err, "no route for native agent task")
	assert.Nil(t, models)
}

func writeOMPNativePhaseReceipt(t *testing.T, root string, selectors map[string]string) {
	t.Helper()
	hash := "sha256:" + strings.Repeat("b", 64)
	// The receipt is bound to the overlay it activated, so the fixture has to
	// write that file too or the binding check fails before any route lookup.
	overlay := []byte("task:\n  agentModelOverrides: {}\n")
	overlayPath := filepath.Join(root, filepath.FromSlash(omp.DefaultOMPModelOverlayPath))
	require.NoError(t, os.MkdirAll(filepath.Dir(overlayPath), 0o700))
	require.NoError(t, os.WriteFile(overlayPath, overlay, 0o600))
	receipt := omp.OMPModelResolutionReceipt{
		OMPVersion: "omp/17.1.8", CatalogFingerprint: hash, Profile: "balanced", ConfigSource: "overlay",
		Activation: omp.OMPModelActivationReceipt{
			Argv: []string{"omp"}, ConfigHash: omp.OMPModelSHA256(overlay), ReadbackHash: hash,
		},
		GeneratedAt: time.Now().UTC(),
	}
	for _, agent := range orderedOMPPhaseReceiptAgents(selectors) {
		provider, model, _ := strings.Cut(selectors[agent], "/")
		receipt.Roles = append(receipt.Roles, omp.OMPModelRoleReceipt{
			Agent: agent, Profile: "balanced", ConfigSource: "overlay",
			RequestedRole: "autopus_planner", EffectiveRole: "autopus_planner",
			Capability: config.CapabilityDeepReasoning,
			Provider:   provider, Model: model, Selector: selectors[agent], Thinking: "high",
			FamilyDiversity: omp.OMPModelFamilyDiversityReceipt{Status: "not_applicable"},
			SafetySource:    "user_effective",
		})
	}
	_, err := omp.WriteOMPModelResolutionReceipt(omp.OMPModelReceiptWriteInput{
		WorkspaceRoot: root, Receipt: receipt,
	})
	require.NoError(t, err)
}

// orderedOMPPhaseReceiptAgents keeps a receipt deterministic: bundled agents
// first in registry order, then any other key the fixture declares.
func orderedOMPPhaseReceiptAgents(selectors map[string]string) []string {
	ordered := make([]string, 0, len(selectors))
	for _, native := range config.OMPNativeAgentNames() {
		if _, ok := selectors[native]; ok {
			ordered = append(ordered, native)
		}
	}
	rest := make([]string, 0, len(selectors))
	for agent := range selectors {
		if _, err := config.OMPNativeAgentRepresentative(agent); err != nil {
			rest = append(rest, agent)
		}
	}
	sort.Strings(rest)
	return append(ordered, rest...)
}
