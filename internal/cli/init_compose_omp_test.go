package cli

// SPEC-MODELUX-002 T12 (REQ-MUX2-22, REQ-MUX2-23; acceptance S3, S21, S22):
// init's OMP tab plans a detached copy of init's config under the file
// path's rules, puts only the confirmed role_model_policy into that config,
// and leaves the write and the activation to init's own save and OMP
// generation. Only the wizard result, the seams, the catalog probes, and the
// OMP runners are stand-ins.

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/internal/cli/tui"
	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
)

// composeInitS21 answers S21 in init: 구성 편집, OMP, 에이전트별 지정, GPT,
// task, openai-codex/gpt-6-astra, max, 완료, 적용, 유지. Init's menu has no
// default preset choice.
var composeInitS21 = []string{"2\n", "4\n", "4\n", "1\n", "5\n", "3\n", "2\n", "\n", "2\n", "\n"}

// newComposeInitOMPRun is the environment of one form-driven init on Claude
// Code, Codex, and OMP: the update run's seams, the role-groups wizard
// answer, accessible forms, the OMP tab's catalog deps, and the OMP
// generation runner.
func newComposeInitOMPRun(t *testing.T, deps ompPlatformDependencies, generation omp.OMPModelCatalogRunner) {
	t.Helper()
	newComposeUpdateRun(t, true, "")
	stubInitWizard(t, tui.QualityRoleGroups)
	useAccessibleComposeForms(t)
	original := initComposeOMPDeps
	t.Cleanup(func() { initComposeOMPDeps = original })
	initComposeOMPDeps = deps
	usePlatformOMPModelRunner(t, generation)
}

func runComposeInitOMP(t *testing.T, lines ...string) (string, string, error) {
	t.Helper()
	return runComposeInit(t, formInput(lines...), "--platforms", "claude-code,codex,omp")
}

// S21, first project: the pin is planned on init's config, confirmed, and
// saved by init; no OMP activation runs, and init's own OMP generation
// writes it into the overlay it reads back.
func TestInitComposeOMP_PinIsSavedAndGeneratedByInit(t *testing.T) {
	deps, activations := composeOMPDeps(ompCLIBalancedCatalogJSON())
	generation := &ompModelDoctorE2ERunner{catalog: ompCLIProfileNativeCatalogJSON()}
	newComposeInitOMPRun(t, deps, generation)

	dir, out, err := runComposeInitOMP(t, composeInitS21...)

	require.NoError(t, err, out)
	assert.Contains(t, out, "어느 도구를 바꿀까요? \n1. 모든 도구 같게\n2. Claude Code\n3. Codex\n4. OMP\n")
	assert.NotContains(t, out, composeFormTextKO.InitOMPUnavailable)
	assert.Contains(t, out, "OMP balanced — model preview (no changes yet)\n")
	assert.Regexp(t, `(?m)^task\*\s+openai-codex/gpt-6-astra\s+max$`, out)
	assert.NotContains(t, out, composeFormTextKO.OMPRewrite)
	assert.Contains(t, out, composeFormTextKO.InitOMPApplied+"\n")
	policy := rawRoleModelPolicyOf(t, readBlockFixture(t, dir))
	assert.Equal(t, "balanced", policy.Profile)
	assert.Equal(t, "openai", policy.Family)
	require.Len(t, policy.Agents, 1)
	require.Len(t, policy.Agents["task"].Candidates, 1)
	candidate := policy.Agents["task"].Candidates[0]
	assert.Equal(t, []string{"openai-codex/gpt-6-astra", "max", "openai"},
		[]string{candidate.Selector, candidate.Thinking, candidate.Family})
	assert.Zero(t, activations.calls)
	overlay := filepath.Join(dir, filepath.FromSlash(omp.DefaultOMPModelOverlayPath))
	body, err := generation.Run(context.Background(), "omp", "--config", overlay, "config", "get", "task.agentModelOverrides", "--json")
	require.NoError(t, err)
	var readback struct {
		Value map[string]string `json:"value"`
	}
	require.NoError(t, json.Unmarshal(body, &readback))
	assert.Equal(t, "openai-codex/gpt-6-astra:max", readback.Value["task"])
	assert.FileExists(t, filepath.Join(dir, ".claude", "agents", "autopus", "planner.md"))
	assert.FileExists(t, filepath.Join(dir, ".codex", "agents", "planner.toml"))
}

// S21, second project: a blocker refuses without a confirmation, and init
// goes on without a role_model_policy.
func TestInitComposeOMP_BlockerRefusesAndInitContinues(t *testing.T) {
	deps, _ := composeOMPDeps(ompCLIBalancedCatalogWithout(t, "openai-codex/gpt-6-astra"))
	newComposeInitOMPRun(t, deps, &ompCLIFakeRunner{fail: true})

	dir, out, err := runComposeInitOMP(t, "2\n", "4\n", "2\n", "1\n", "\n")

	require.NoError(t, err, out)
	assert.Contains(t, out, "omp_profile_candidate_unavailable")
	assert.Contains(t, out, "agent=task")
	assert.NotContains(t, out, "적용할까요?")
	assert.NotContains(t, readBlockFixture(t, dir), "role_model_policy")
}

// S21, third input: init's check refuses a candidate holding a placeholder
// and leaves the config's role_model_policy as it was.
func TestInitComposeOMP_PlaceholderCandidateIsRefused(t *testing.T) {
	cfg := composeFixtureConfig("claude-code", "codex", "omp")
	candidate := config.RoleModelPolicyConf{
		Version: config.RoleModelPolicyVersionV1, Profile: "balanced", Family: "openai",
		Agents: map[string]config.RoleAgentOverrideConf{"task": {Candidates: []config.RoleModelCandidateConf{
			{Selector: "openai-codex/${HOME}", Thinking: "max", Family: "openai"},
		}}},
	}

	err := applyInitOMPPolicy(cfg, candidate)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "role_model_policy_placeholder_unsupported")
	assert.Equal(t, config.RoleModelPolicyConf{}, cfg.RoleModelPolicy)
}

// S21: the extracted plan answers exactly what the file path answers for
// the same content.
func TestPlanOMPProfileForConfig_EqualsTheFilePlan(t *testing.T) {
	cfg := composeFixtureConfig("claude-code", "codex", "omp")
	dir := t.TempDir()
	require.NoError(t, config.Save(dir, cfg))
	opts, err := newOMPProfileApplyOptions("balanced", "openai", []string{"task=openai-codex/gpt-6-astra:max"}, false, false)
	require.NoError(t, err)
	runner := &ompCLIFakeRunner{catalog: ompCLIBalancedCatalogJSON()}
	detached, err := cloneHarnessConfig(cfg)
	require.NoError(t, err)

	fromConfig, err := planOMPProfileForConfig(context.Background(), detached, opts, runner)
	require.NoError(t, err)
	fromFile, err := planOMPProfile(context.Background(), dir, opts, runner)
	require.NoError(t, err)

	assert.Equal(t, fromFile, fromConfig)
}

// S21, fourth project: a generation whose read-back of the routing it
// wrote disagrees fails init, whose existing rollback leaves no file behind.
func TestInitComposeOMP_FailedGenerationRollsInitBack(t *testing.T) {
	deps, _ := composeOMPDeps(ompCLIBalancedCatalogJSON())
	generation := &composeMismatchRunner{inner: &ompModelDoctorE2ERunner{catalog: ompCLIProfileNativeCatalogJSON()}}
	newComposeInitOMPRun(t, deps, generation)

	dir, out, err := runComposeInitOMP(t, composeInitS21...)

	require.Error(t, err, out)
	assert.Contains(t, err.Error(), "플랫폼 설정 적용 실패")
	assert.True(t, generation.mismatched, "the generation read the routing back")
	assert.NoFileExists(t, filepath.Join(dir, "autopus.yaml"))
	assert.NoFileExists(t, filepath.Join(dir, ".claude", "agents", "autopus", "planner.md"))
	assert.NoFileExists(t, filepath.Join(dir, ".codex", "agents", "planner.toml"))
}

// composeMismatchRunner is ompModelDoctorE2ERunner except that reading the
// agent overrides back answers another route than the one written.
type composeMismatchRunner struct {
	inner      *ompModelDoctorE2ERunner
	mismatched bool
}

func (r *composeMismatchRunner) Run(ctx context.Context, executable string, args ...string) ([]byte, error) {
	if key := ompModelDoctorConfigGetKey(args); key == config.OMPNativeAgentModelOverridesKey {
		r.mismatched = true
		return json.Marshal(map[string]any{"key": key, "value": map[string]string{"task": "openai-codex/gpt-6-luna:max"}})
	}
	return r.inner.Run(ctx, executable, args...)
}

// composeFailingAfter answers the OMP catalog until it has handed out one
// runner, then fails every read of the runners it hands out after it.
type composeFailingAfter struct{ handed int }

func (c *composeFailingAfter) newRunner() omp.OMPModelCatalogRunner {
	c.handed++
	if c.handed > 1 {
		return &ompCLIFakeRunner{fail: true}
	}
	return &ompCLIFakeRunner{catalog: ompCLIBalancedCatalogJSON()}
}

// S22: an unreadable catalog, at the plan and at the model list alike,
// prints the init hint and its cause and returns to the menu; init ends
// normally. An earlier applied choice of the same init stays.
func TestInitComposeOMP_UnreadableCatalogHintsAndInitContinues(t *testing.T) {
	t.Run("never readable", func(t *testing.T) {
		failing := &ompCLIFakeRunner{fail: true}
		deps := ompPlatformDependencies{newRunner: func() omp.OMPModelCatalogRunner { return failing }}
		generation := &ompCLIFakeRunner{fail: true}
		newComposeInitOMPRun(t, deps, generation)

		dir, out, err := runComposeInitOMP(t, "2\n", "4\n", "2\n", "1\n", "2\n", "4\n", "4\n", "1\n", "\n")

		require.NoError(t, err, out)
		assert.Equal(t, 2, strings.Count(out, composeFormTextKO.InitOMPUnavailable+"\n"), out)
		assert.Equal(t, 2, strings.Count(out, "  경고: "), out)
		assert.NotContains(t, readBlockFixture(t, dir), "role_model_policy")
		assert.FileExists(t, filepath.Join(dir, ".claude", "agents", "autopus", "planner.md"))
		assert.FileExists(t, filepath.Join(dir, ".codex", "agents", "planner.toml"))
		assert.NotEmpty(t, failing.calls, "the tab read the catalog")
		assert.Empty(t, generation.calls, "an empty profile skips the model integration")
	})
	t.Run("readable once", func(t *testing.T) {
		runners := &composeFailingAfter{}
		newComposeInitOMPRun(t, ompPlatformDependencies{newRunner: runners.newRunner},
			&ompModelDoctorE2ERunner{catalog: ompCLIProfileNativeCatalogJSON()})

		dir, out, err := runComposeInitOMP(t, "2\n", "4\n", "2\n", "1\n", "2\n", "2\n", "4\n", "4\n", "1\n", "\n")

		require.NoError(t, err, out)
		assert.Equal(t, 1, strings.Count(out, composeFormTextKO.InitOMPUnavailable+"\n"), out)
		policy := rawRoleModelPolicyOf(t, readBlockFixture(t, dir))
		assert.Equal(t, []string{"balanced", "openai"}, []string{policy.Profile, policy.Family})
	})
}
