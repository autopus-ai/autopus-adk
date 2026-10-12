package cli

// SPEC-MODELUX-002 T6, probe A3 (REQ-MUX2-11; acceptance S8): an OMP pin
// confirmed inside the update's composition step survives the same run's
// later saves and reaches its OMP generation, because the step syncs the
// loaded config from the file it wrote.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
)

// composeUpdateS8 answers S8 in `auto update` over O3: 구성 편집, OMP,
// 에이전트별 지정, GPT, task, openai-codex/gpt-6-astra, max, 완료, 적용,
// 유지. The update menu has no default preset choice.
var composeUpdateS8 = []string{"2\n", "4\n", "4\n", "1\n", "5\n", "3\n", "2\n", "\n", "2\n", "\n"}

// useComposeUpdateOMPDeps replaces the OMP dependencies of the update's
// composition step for one test.
func useComposeUpdateOMPDeps(t *testing.T, deps ompPlatformDependencies) {
	t.Helper()
	original := updateComposeOMPDeps
	t.Cleanup(func() { updateComposeOMPDeps = original })
	updateComposeOMPDeps = deps
}

// usePlatformOMPModelRunner makes the OMP platform generation read the
// catalog and its written config through runner instead of an omp binary.
func usePlatformOMPModelRunner(t *testing.T, runner omp.OMPModelCatalogRunner) {
	t.Helper()
	original := platformOMPModelRunner
	t.Cleanup(func() { platformOMPModelRunner = original })
	platformOMPModelRunner = runner
}

// S8: the step's cfg holds exactly the role_model_policy the file holds.
func TestUpdateComposeStep_OMPPinSyncsTheLoadedConfig(t *testing.T) {
	newComposeDetectRun(t, "codex", "omp")
	useAccessibleComposeForms(t)
	deps, activations := composeOMPDeps(ompCLIBalancedCatalogJSON())
	useComposeUpdateOMPDeps(t, deps)
	dir := writeComposeFixture(t, "claude-code", "codex", "omp")
	cfg, err := config.Load(dir)
	require.NoError(t, err)
	cmd, out := newComposeTestCommand()
	cmd.SetIn(formInput(composeUpdateS8...))

	runUpdateComposeStep(cmd, dir, cfg, composeGate{})

	reloaded, err := config.LoadPreview(dir)
	require.NoError(t, err)
	assert.Equal(t, reloaded.RoleModelPolicy, cfg.RoleModelPolicy, out.String())
	assert.Equal(t, "task", strings.Join(sortedOMPProfileOverriddenAgents(ompProfileOverriddenAgents(&cfg.RoleModelPolicy)), ","))
	assert.Equal(t, 1, activations.calls)
}

// Probe A3, update half: with a platform detected in the same run, the
// detection save after the step keeps the pin, and the update's own OMP
// generation renders it into the routing overlay it reads back. The pin,
// task on anthropic/claude-fable-5-1 at high, differs from the balanced
// openai route (openai-codex/gpt-6-astra at max), so only a generation that
// received the synced config writes it. The generation reads the
// metadata-light catalog an installed omp prints: a built-in profile is
// operator-attested, and its receipt refuses a strict catalog's evidence.
func TestUpdateCompose_OMPPinSurvivesTheDetectionSaveAndGeneration(t *testing.T) {
	newComposeDetectRun(t, "codex", "omp", "opencode")
	useAccessibleComposeForms(t)
	deps, activations := composeOMPDeps(ompCLIBalancedCatalogJSON())
	useComposeUpdateOMPDeps(t, deps)
	usePlatformOMPModelRunner(t, &ompModelDoctorE2ERunner{catalog: ompCLIProfileNativeCatalogJSON()})
	dir := writeComposeFixture(t, "claude-code", "codex", "omp")

	out, err := runComposeUpdate(t, formInput("2\n", "4\n", "4\n", "1\n", "5\n", "2\n", "1\n", "\n", "2\n", "\n"), "--dir", dir)

	require.NoError(t, err, out)
	assert.Contains(t, out, "  + 새 플랫폼 감지: opencode\n")
	assert.Equal(t, 1, activations.calls)
	assert.Contains(t, composeBindLoad(t, dir).Platforms, "opencode")
	policy := rawRoleModelPolicyOf(t, readBlockFixture(t, dir))
	require.Len(t, policy.Agents["task"].Candidates, 1)
	assert.Equal(t, "anthropic/claude-fable-5-1", policy.Agents["task"].Candidates[0].Selector)
	overlay, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(omp.DefaultOMPModelOverlayPath)))
	require.NoError(t, err)
	route, err := ompModelDoctorYAMLPath(overlay, "task.agentModelOverrides.task")
	require.NoError(t, err)
	assert.Equal(t, "anthropic/claude-fable-5-1:high", route)
}
