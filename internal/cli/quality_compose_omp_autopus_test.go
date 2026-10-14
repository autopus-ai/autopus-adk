package cli

// SPEC-MODELUX-003 T11 (REQ-MUX3-13, 22; acceptance S10, S11): in autopus
// mode the OMP tab asks no profile or family, offers role groups and
// per-agent pins over the 16 canonical agents with attested installed models
// of every provider, and saves a pins-only policy through the OMP paths.

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
)

// composeOMPCatalogML is ML: the four models of
// ompCLIProfileNativeCatalogJSON in the shape an installed omp prints, with
// no family or capability metadata. ompCLIProfileNativeCatalogJSON itself
// lacks the display fields the tab's catalog rule reads.
const composeOMPCatalogML = `{"models":[
	{"provider":"anthropic","id":"claude-fable-5-1","selector":"anthropic/claude-fable-5-1","name":"Claude Fable 5.1","contextWindow":200000,"reasoning":true,"thinking":["high","max"],"input":["text"]},
	{"provider":"anthropic","id":"claude-sonnet-5-5","selector":"anthropic/claude-sonnet-5-5","name":"Claude Sonnet 5.5","contextWindow":200000,"reasoning":true,"thinking":["high","max"],"input":["text"]},
	{"provider":"openai-codex","id":"gpt-6-astra","selector":"openai-codex/gpt-6-astra","name":"GPT-6 Astra","contextWindow":272000,"reasoning":true,"thinking":["high","max"],"input":["text"]},
	{"provider":"openai-codex","id":"gpt-6-luna","selector":"openai-codex/gpt-6-luna","name":"GPT-6 Luna","contextWindow":272000,"reasoning":true,"thinking":["high","max"],"input":["text"]}
]}`

const composeOMPDesignGroup = "설계·판단 — planner, architect, spec-writer, security-auditor"

var (
	composeOMPDesignAgents = []string{"planner", "architect", "spec-writer", "security-auditor"}
	composeOMPBuildAgents  = []string{"executor", "debugger", "deep-worker", "devops", "frontend-specialist", "perf-engineer"}
)

func composeOMPOptionList(options ...string) string {
	var list strings.Builder
	for i, option := range options {
		fmt.Fprintf(&list, "%d. %s\n", i+1, option)
	}
	return list.String()
}

func composeOMPFileSum(t *testing.T, dir string) [32]byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(dir, "autopus.yaml"))
	require.NoError(t, err)
	return sha256.Sum256(body)
}

// composeOMPS10Policy is the policy S10 saves.
func composeOMPS10Policy() config.RoleModelPolicyConf {
	agents := map[string]config.RoleModelCandidateConf{}
	for _, agent := range composeOMPDesignAgents {
		agents[agent] = pinFable
	}
	for _, agent := range composeOMPBuildAgents {
		agents[agent] = pinAstra
	}
	return config.RoleModelPolicyConf{Version: config.RoleModelPolicyVersionV1, Agents: pinsOf(agents)}
}

func writeComposePinsFixture(t *testing.T, policy config.RoleModelPolicyConf) string {
	t.Helper()
	dir := t.TempDir()
	cfg := composeFixtureConfig("claude-code", "codex", "omp")
	cfg.RoleModelPolicy = policy
	require.NoError(t, config.Save(dir, cfg))
	return dir
}

// composeOMPActivated generates OMP with cfg through the IO runner and
// returns the task.agentModelOverrides it activated.
func composeOMPActivated(t *testing.T, dir string, cfg *config.HarnessConfig) map[string]string {
	t.Helper()
	runner := &ompModelDoctorE2ERunner{catalog: ompCLIProfileNativeCatalogJSON()}
	_, err := omp.NewWithRoot(dir).WithModelIntegrationRunner(runner).Generate(context.Background(), cfg)
	require.NoError(t, err)
	body, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(omp.DefaultOMPModelOverlayPath)))
	require.NoError(t, err)
	var overlay struct {
		Task struct {
			Overrides map[string]string `yaml:"agentModelOverrides"`
		} `yaml:"task"`
	}
	require.NoError(t, yaml.Unmarshal(body, &overlay))
	return overlay.Task.Overrides
}

// S10: role groups mix providers, the catalog rule lists attested models
// only, and the result is a pins-only policy whose activation routes the
// pinned agents and task alone.
func TestComposeOMPAutopus_S10_MultiProviderRoleGroups(t *testing.T) {
	composeSessionCatalog(t, "CAT-A")
	dir := writeComposeFixture(t, "claude-code", "codex", "omp")
	deps, activations := composeOMPDeps([]byte(composeOMPCatalogML2))
	// 구성 편집, OMP, 역할 그룹, 설계·판단 fable → high, 구현 astra → max,
	// 검증·리뷰 Enter, 탐색·보조 세션 모델, 적용, 유지.
	lines := []string{"2\n", "4\n", "1\n", "3\n", "1\n", "6\n", "2\n", "\n", "2\n", "2\n", "\n"}

	out, err := runComposeOMPForms(t, dir, deps, lines...)

	require.NoError(t, err, out)
	assert.NotContains(t, out, composeFormTextKO.OMP+" \n", "no profile Select")
	assert.NotContains(t, out, composeFormTextKO.OMPFamily, "no family Select")
	models := composeOMPOptionList("그대로 — OMP 세션 모델", "OMP 세션 모델 그대로",
		"anthropic/claude-fable-5-1", "anthropic/claude-opus-5-5", "anthropic/claude-sonnet-5-5",
		"openai-codex/gpt-6-astra", "openai-codex/gpt-6-luna")
	assert.Contains(t, out, composeOMPDesignGroup+" \n"+models)
	assert.NotContains(t, out, "example/foo-1")
	assert.Equal(t, 1, strings.Count(out, composeFormTextKO.OMPAttestedOnly+"\n"))
	assert.Less(t, strings.Index(out, composeFormTextKO.OMPAttestedOnly), strings.Index(out, composeOMPDesignGroup))
	assert.Contains(t, out, "설계·판단 thinking \n"+composeOMPOptionList("high", "max"))
	assert.Less(t, strings.Index(out, composeFormTextKO.OMPOldBinary), strings.Index(out, "OMP pins-only — model preview"))
	assert.Equal(t, 1, activations.calls)
	want := composeOMPS10Policy()
	assert.Equal(t, want, rawRoleModelPolicyOf(t, readBlockFixture(t, dir)))

	overrides := composeOMPActivated(t, dir, activations.cfgs[0])

	keys := append(append([]string{"task"}, composeOMPDesignAgents...), composeOMPBuildAgents...)
	assert.ElementsMatch(t, keys, composeOMPKeys(overrides))
	assert.Equal(t, "anthropic/claude-fable-5-1:high", overrides["task"], "task follows the planner pin")
	explorer, err := os.ReadFile(filepath.Join(dir, ".omp", "agents", "explorer.md"))
	require.NoError(t, err)
	assert.Contains(t, string(explorer), "\nthinkingLevel: medium\n")
	planner, err := os.ReadFile(filepath.Join(dir, ".omp", "agents", "planner.md"))
	require.NoError(t, err)
	assert.NotContains(t, string(planner), "thinkingLevel")
}

// S11: a pins-only start lists each agent's pin, the thinking default is the
// current level, a strict catalog filters by capability without the note,
// and native mode refuses the policy at load.
func TestComposeOMPAutopus_S11_StartThinkingAndCatalogRules(t *testing.T) {
	t.Run("per agent", func(t *testing.T) {
		composeSessionCatalog(t, "CAT-A")
		dir := writeComposePinsFixture(t, composeOMPS10Policy())
		before := composeOMPFileSum(t, dir)
		deps, activations := composeOMPDeps([]byte(composeOMPCatalogML2))
		// 구성 편집, OMP, 에이전트별, planner, fable, thinking Enter, 완료, 유지.
		lines := []string{"2\n", "4\n", "2\n", "2\n", "3\n", "\n", "\n", "\n"}

		out, err := runComposeOMPForms(t, dir, deps, lines...)

		require.NoError(t, err, out)
		assert.Contains(t, out, "planner — anthropic/claude-fable-5-1 · high\n")
		assert.Contains(t, out, "explorer — OMP 세션 모델\n")
		assert.Contains(t, out, "planner thinking \n"+composeOMPOptionList("high", "max"))
		assert.Contains(t, out, composeTextKO.NoChange+"\n", "Enter kept high, the current level")
		assert.Zero(t, activations.calls)
		assert.Equal(t, before, composeOMPFileSum(t, dir))
	})
	t.Run("strict catalog", func(t *testing.T) {
		composeSessionCatalog(t, "CAT-A")
		dir := writeComposePinsFixture(t, composeOMPS10Policy())
		deps, _ := composeOMPDeps(ompCLIBalancedCatalogJSON())
		lines := []string{"2\n", "4\n", "1\n", "\n", "\n", "\n", "\n", "\n"}

		out, err := runComposeOMPForms(t, dir, deps, lines...)

		require.NoError(t, err, out)
		assert.NotContains(t, out, composeFormTextKO.OMPAttestedOnly)
		assert.Contains(t, out, composeOMPDesignGroup+" \n"+composeOMPOptionList("그대로 — anthropic/claude-fable-5-1 · high",
			"OMP 세션 모델 그대로", "anthropic/claude-fable-5-1", "openai-codex/gpt-6-astra"))
	})
	t.Run("native mode", func(t *testing.T) {
		policy := composeOMPS10Policy()
		policy.AgentSource = config.OMPAgentSourceNative
		dir := writeComposePinsFixture(t, config.RoleModelPolicyConf{})
		data, err := os.ReadFile(filepath.Join(dir, "autopus.yaml"))
		require.NoError(t, err)
		encoded, err := yaml.Marshal(map[string]any{"role_model_policy": policy})
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "autopus.yaml"), append(data, encoded...), 0o644))

		_, err = config.LoadPreview(dir)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "role_model_policy.agents_require_profile")
	})
}

// S11 (4): a pinned model the catalog lacks fails the update closed and
// leaves every OMP file as it was.
func TestOMPUpdate_S11_PinsOnlyFailsClosedOnAMissingProvider(t *testing.T) {
	dir := writeComposePinsFixture(t, composeOMPS10Policy())
	cfg, err := config.LoadPreview(dir)
	require.NoError(t, err)
	composeOMPActivated(t, dir, cfg)
	overlay := filepath.Join(dir, filepath.FromSlash(omp.DefaultOMPModelOverlayPath))
	before, err := os.ReadFile(overlay)
	require.NoError(t, err)
	agentsBefore, err := os.ReadFile(filepath.Join(dir, ".omp", "agents", "executor.md"))
	require.NoError(t, err)
	without := &ompModelDoctorE2ERunner{catalog: []byte(`{"models":[
		{"provider":"anthropic","id":"claude-fable-5-1","thinking":["high","max"],"available":true}]}`)}

	_, err = omp.NewWithRoot(dir).WithModelIntegrationRunner(without).Update(context.Background(), cfg)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "required_route_unresolved")
	assert.Regexp(t, `required_route_unresolved: (executor|debugger|deep-worker|devops|frontend-specialist|perf-engineer)`, err.Error())
	after, err := os.ReadFile(overlay)
	require.NoError(t, err)
	assert.Equal(t, before, after)
	agentsAfter, err := os.ReadFile(filepath.Join(dir, ".omp", "agents", "executor.md"))
	require.NoError(t, err)
	assert.Equal(t, agentsBefore, agentsAfter)
}

func composeOMPKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}
