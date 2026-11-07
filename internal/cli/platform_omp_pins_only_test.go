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

// ompPinsOnlyTestPolicy pins reviewer and planner to fable, executor to
// astra, and frontend-specialist to luna, which carries vision_design. planner and executor have no
// agent file; their pins route the work that still dispatches by role: OMP's
// task follows planner, and the pipeline phases read both.
func ompPinsOnlyTestPolicy() config.RoleModelPolicyConf {
	fable := []config.RoleModelCandidateConf{{Selector: "anthropic/claude-fable-5-1", Thinking: "max", Family: "anthropic"}}
	astra := []config.RoleModelCandidateConf{{Selector: "openai-codex/gpt-6-astra", Thinking: "max", Family: "openai"}}
	luna := []config.RoleModelCandidateConf{{Selector: "openai-codex/gpt-6-luna", Thinking: "max", Family: "openai"}}
	return config.RoleModelPolicyConf{Version: config.RoleModelPolicyVersionV1, Agents: map[string]config.RoleAgentOverrideConf{
		"reviewer": {Candidates: fable}, "planner": {Candidates: fable},
		"frontend-specialist": {Candidates: luna}, "executor": {Candidates: astra},
	}}
}

func TestOMPPinsOnly_DoctorShowsThePinnedAgents(t *testing.T) {
	for name, catalog := range map[string][]byte{"metadata-light": ompCLIProfileNativeCatalogJSON(), "strict": ompCLIBalancedCatalogJSON()} {
		t.Run(name, func(t *testing.T) { assertOMPPinsOnlyDoctor(t, catalog) })
	}
}

func assertOMPPinsOnlyDoctor(t *testing.T, catalog []byte) {
	root := t.TempDir()
	cfg := config.DefaultFullConfig("omp-pins-only")
	cfg.Platforms = []string{"omp"}
	cfg.RoleModelPolicy = ompPinsOnlyTestPolicy()
	require.NoError(t, config.Save(root, cfg))
	generation := &ompModelDoctorE2ERunner{catalog: catalog}
	_, err := omp.NewWithRoot(root).WithModelIntegrationRunner(generation).Generate(context.Background(), cfg)
	require.NoError(t, err)

	projection := buildOMPPlatformProjection(context.Background(), root, generation, configTimeForOMPAgentCatalog())

	assert.True(t, projection.Models.Enabled)
	assert.True(t, projection.Models.ReceiptVerified, "the receipt matches the catalog it was attested by")
	assert.Equal(t, config.PinsOnlyRoleModelProfileName, projection.Models.Profile)
	rows := map[string]ompEffectiveModelProjection{}
	for _, row := range projection.Models.Models {
		rows[row.Agent] = row
	}
	assert.Equal(t, "anthropic/claude-fable-5-1:max", rows["reviewer"].EffectiveSelector)
	assert.Equal(t, "openai-codex/gpt-6-luna:max", rows["frontend-specialist"].EffectiveSelector)
	assert.Equal(t, "anthropic/claude-fable-5-1:max", rows["task"].EffectiveSelector, "task follows planner's pin")
	assert.Equal(t, "inherited", rows["ux-validator"].Status)
	assert.Empty(t, rows["ux-validator"].EffectiveSelector)
	assert.NotContains(t, rows, "planner", "a legacy role has no agent row")
}

func TestOMPPinsOnly_TableShowsPinsAndTheSessionModel(t *testing.T) {
	cfg := config.DefaultFullConfig("omp-pins-only-table")
	cfg.Platforms = []string{"omp"}
	cfg.RoleModelPolicy = ompPinsOnlyTestPolicy()

	cell := composeTableOMPRoutes(cfg)

	assert.Equal(t, composeTableOMPCell{native: "reviewer", selector: "anthropic/claude-fable-5-1", thinking: "max"}, cell("reviewer"))
	assert.Equal(t, composeTableOMPCell{}, cell("ux-validator"), "the session model, not a route error")
}

func TestOMPPinsOnly_PipelinePhasesWithoutAPinInheritTheSessionModel(t *testing.T) {
	projectDir := t.TempDir()
	cfg := config.DefaultFullConfig("omp-pins-only-phases")
	cfg.Platforms = []string{"omp"}
	cfg.RoleModelPolicy = ompPinsOnlyTestPolicy()
	require.NoError(t, config.Save(projectDir, cfg))
	writeOMPPhaseReceipt(t, projectDir, map[string]string{
		"planner": "provider/plan-model", "executor": "provider/implement-model", "task": "provider/plan-model",
	})

	models, err := loadPipelineOMPPhaseModels(projectDir, writePipelineOMPVersionFixture(t, "omp/17.1.8"))

	require.NoError(t, err)
	assert.Equal(t, map[pipeline.PhaseID]string{
		pipeline.PhasePlan: "provider/plan-model", pipeline.PhaseImplement: "provider/implement-model",
	}, models)
}
