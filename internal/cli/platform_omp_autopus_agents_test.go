package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/pipeline"
)

func writeGeneratedAutopusOMPWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	cfg := config.DefaultFullConfig("omp-autopus-catalog")
	cfg.Platforms = []string{"omp"}
	require.NoError(t, config.Save(root, cfg))
	_, err := omp.NewWithRoot(root).Generate(context.Background(), cfg)
	require.NoError(t, err)
	return root
}

// S15: a generated autopus project projects the 16 canonical agents and task;
// a generated definition under its own name is never a shadow.
func TestOMPAgentCatalog_ProjectsTheAutopusAgents(t *testing.T) {
	root := writeGeneratedAutopusOMPWorkspace(t)
	projection := buildOMPPlatformProjection(context.Background(), root, &ompCLIFakeRunner{}, configTimeForOMPAgentCatalog())

	assert.Equal(t, 17, projection.Models.ExpectedAgents)
	assert.Zero(t, projection.Models.ShadowedAgents)
	assert.Equal(t, "ready", projection.Models.AgentCatalogStatus)
	assert.Equal(t, "autopus_agent_registry", projection.Models.AgentCatalogReason)
	agents := make([]string, 0, len(projection.Models.Models))
	for _, row := range projection.Models.Models {
		agents = append(agents, row.Agent)
		assert.False(t, row.Shadowed, row.Agent)
	}
	assert.Equal(t, config.OMPAutopusAgentNames(), agents)
	assert.Equal(t, "ready", projection.Status)
	assert.Empty(t, projection.Blockers)
}

// S15: a generated definition that went missing is reported, not ignored.
func TestOMPAgentCatalog_ReportsAMissingAutopusAgent(t *testing.T) {
	root := writeGeneratedAutopusOMPWorkspace(t)
	require.NoError(t, os.Remove(filepath.Join(root, ".omp", "agents", "validator.md")))
	projection := buildOMPPlatformProjection(context.Background(), root, &ompCLIFakeRunner{}, configTimeForOMPAgentCatalog())

	assert.Equal(t, "degraded", projection.Models.AgentCatalogStatus)
	assert.Equal(t, "autopus_agent_missing", projection.Models.AgentCatalogReason)
	assert.Equal(t, "degraded", projection.Status)
	assert.Contains(t, projection.Blockers, "agents:autopus_agent_missing")
}

// S8 (UC) and S24: a user-owned task.disabledAgents keeps three bundled agents
// enabled next to the Autopus ones, which the doctor reports as degraded.
func TestOMPAgentCatalog_ReportsAUserOwnedDisabledAgents(t *testing.T) {
	root := t.TempDir()
	cfg := config.DefaultFullConfig("omp-autopus-user-owned")
	cfg.Platforms = []string{"omp"}
	require.NoError(t, config.Save(root, cfg))
	path := filepath.Join(root, ".omp", "config.yml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte("task:\n  disabledAgents:\n    - browser\n"), 0o600))
	_, err := omp.NewWithRoot(root).Update(context.Background(), cfg)
	require.NoError(t, err)

	projection := buildOMPPlatformProjection(context.Background(), root, &ompCLIFakeRunner{}, configTimeForOMPAgentCatalog())
	assert.Equal(t, "degraded", projection.Models.AgentCatalogStatus)
	assert.Equal(t, "degraded", projection.Status)
	assert.Contains(t, projection.Blockers, "agents:disabled_agents_user_owned")
}

// S15: in autopus mode every pipeline phase runs on the agent of its own
// role, so the receipt is read by that name and an omitted one fails closed.
func TestPipelineOMPPhaseModels_ReadTheAutopusAgentOfEachPhase(t *testing.T) {
	projectDir := t.TempDir()
	cfg := config.DefaultFullConfig("omp-autopus-phases")
	cfg.Platforms = []string{"omp"}
	require.NoError(t, config.Save(projectDir, cfg))
	selectors := map[string]string{
		"planner": "provider/plan-model", "tester": "provider/test-model",
		"executor": "provider/implement-model", "validator": "provider/validate-model",
		"reviewer": "provider/review-model", "task": "provider/task-model",
	}
	writeOMPNativePhaseReceipt(t, projectDir, selectors)

	models, err := loadPipelineOMPPhaseModels(projectDir, writePipelineOMPVersionFixture(t, "omp/17.1.8"))
	require.NoError(t, err)
	assert.Equal(t, map[pipeline.PhaseID]string{
		pipeline.PhasePlan:         "provider/plan-model",
		pipeline.PhaseTestScaffold: "provider/test-model",
		pipeline.PhaseImplement:    "provider/implement-model",
		pipeline.PhaseValidate:     "provider/validate-model",
		pipeline.PhaseReview:       "provider/review-model",
	}, models)

	delete(selectors, "executor")
	writeOMPNativePhaseReceipt(t, projectDir, selectors)
	_, err = loadPipelineOMPPhaseModels(projectDir, writePipelineOMPVersionFixture(t, "omp/17.1.8"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "executor")
}

// The compose table's OMP cell is the agent's own route in autopus mode.
func TestComposeTable_OMPCellIsTheAutopusAgentsOwnRoute(t *testing.T) {
	cfg := config.DefaultFullConfig("omp-autopus-table")
	cfg.Platforms = []string{"omp"}
	cfg.RoleModelPolicy = config.RoleModelPolicyConf{Version: config.RoleModelPolicyVersionV1, Profile: "balanced", Family: "openai"}
	cell := composeTableOMPRoutes(cfg)
	_, profile, ok := cfg.RoleModelPolicy.SelectedRoleModelProfileForQuality(cfg.Quality)
	require.True(t, ok)
	for _, agent := range config.CanonicalAgentNames() {
		_, route, err := profile.OMPAutopusAgentRoute(agent)
		require.NoError(t, err)
		want := composeTableOMPCell{native: agent, selector: route.Candidates[0].Selector, thinking: route.Candidates[0].Thinking}
		assert.Equal(t, want, cell(agent), agent)
	}
}

// S9 (OP3): autopus mode disables sonic, so a sonic pin reaches no agent; the
// plan says so in one warning and routes no sonic key.
func TestOMPProfilePlan_WarnsThatASonicPinReachesNoAgent(t *testing.T) {
	root, runner := writeOMPBalancedProject(t)
	dir := root
	text := executeOMPSubcommand(t, newPlatformOMPProfileApplyCmd(&dir, ompBalancedDeps(runner, nil)),
		"balanced", "--plan", "--agent", "sonic=anthropic/claude-sonnet-5-5:high")
	assert.Equal(t, 1, strings.Count(text, "Warning: role_model_policy.agents.sonic"), text)
	assert.NotContains(t, text, "agent=sonic ")

	payload := planOMPProfileJSON(t, root, runner, "balanced")
	assert.Empty(t, payload.Warnings, "no sonic pin, no warning")
	agents := make([]string, 0, len(payload.Agents))
	for _, row := range payload.Agents {
		agents = append(agents, row.Agent)
	}
	assert.Equal(t, config.OMPAutopusAgentNames(), agents)
}
