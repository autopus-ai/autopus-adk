package cli

// SPEC-MODELUX-003 REQ-MUX3-22 (Pins-only Policy Contract): consumers of a
// pins-only policy show the pinned agents' routes and leave every other agent
// on the OMP session model: the doctor projection, the composition table,
// and the pipeline phase models.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/pipeline"
)

// ompPinsOnlyTestPolicy pins planner to fable and executor to astra.
func ompPinsOnlyTestPolicy() config.RoleModelPolicyConf {
	return config.RoleModelPolicyConf{Version: config.RoleModelPolicyVersionV1, Agents: map[string]config.RoleAgentOverrideConf{
		"planner":  {Candidates: []config.RoleModelCandidateConf{{Selector: "anthropic/claude-fable-5-1", Thinking: "max", Family: "anthropic"}}},
		"executor": {Candidates: []config.RoleModelCandidateConf{{Selector: "openai-codex/gpt-6-astra", Thinking: "max", Family: "openai"}}},
	}}
}

func TestOMPPinsOnly_DoctorShowsThePinnedAgents(t *testing.T) {
	root := t.TempDir()
	cfg := config.DefaultFullConfig("omp-pins-only")
	cfg.Platforms = []string{"omp"}
	cfg.RoleModelPolicy = ompPinsOnlyTestPolicy()
	require.NoError(t, config.Save(root, cfg))
	generation := &ompModelDoctorE2ERunner{catalog: ompCLIProfileNativeCatalogJSON()}
	_, err := omp.NewWithRoot(root).WithModelIntegrationRunner(generation).Generate(context.Background(), cfg)
	require.NoError(t, err)

	projection := buildOMPPlatformProjection(context.Background(), root, generation, configTimeForOMPAgentCatalog())

	assert.True(t, projection.Models.Enabled)
	assert.Equal(t, config.PinsOnlyRoleModelProfileName, projection.Models.Profile)
	rows := map[string]ompEffectiveModelProjection{}
	for _, row := range projection.Models.Models {
		rows[row.Agent] = row
	}
	assert.Equal(t, "anthropic/claude-fable-5-1:max", rows["planner"].EffectiveSelector)
	assert.Equal(t, "openai-codex/gpt-6-astra:max", rows["executor"].EffectiveSelector)
	assert.Equal(t, "anthropic/claude-fable-5-1:max", rows["task"].EffectiveSelector)
	assert.Equal(t, "inherited", rows["explorer"].Status)
	assert.Empty(t, rows["explorer"].EffectiveSelector)
}

func TestOMPPinsOnly_TableShowsPinsAndTheSessionModel(t *testing.T) {
	cfg := config.DefaultFullConfig("omp-pins-only-table")
	cfg.Platforms = []string{"omp"}
	cfg.RoleModelPolicy = ompPinsOnlyTestPolicy()

	cell := composeTableOMPRoutes(cfg)

	assert.Equal(t, composeTableOMPCell{native: "planner", selector: "anthropic/claude-fable-5-1", thinking: "max"}, cell("planner"))
	assert.Equal(t, composeTableOMPCell{}, cell("explorer"), "the session model, not a route error")
}

func TestOMPPinsOnly_PipelinePhasesWithoutAPinInheritTheSessionModel(t *testing.T) {
	projectDir := t.TempDir()
	cfg := config.DefaultFullConfig("omp-pins-only-phases")
	cfg.Platforms = []string{"omp"}
	cfg.RoleModelPolicy = ompPinsOnlyTestPolicy()
	require.NoError(t, config.Save(projectDir, cfg))
	writeOMPNativePhaseReceipt(t, projectDir, map[string]string{
		"planner": "provider/plan-model", "executor": "provider/implement-model", "task": "provider/plan-model",
	})

	models, err := loadPipelineOMPPhaseModels(projectDir, writePipelineOMPVersionFixture(t, "omp/17.1.8"))

	require.NoError(t, err)
	assert.Equal(t, map[pipeline.PhaseID]string{
		pipeline.PhasePlan: "provider/plan-model", pipeline.PhaseImplement: "provider/implement-model",
	}, models)
}
