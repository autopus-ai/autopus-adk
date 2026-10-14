package cli

// SPEC-MODELUX-003 T16 (REQ-MUX3-13, 22, 23): the attested model list, the
// pins-only plan and apply, and the conversion of a profile-based policy.

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// composeOMPCatalogML2 is ML2: ML's models, anthropic/claude-opus-5-5, and
// example/foo-1, which no built-in profile declares, in the shape an
// installed omp prints.
const composeOMPCatalogML2 = `{"models":[
	{"provider":"anthropic","id":"claude-fable-5-1","selector":"anthropic/claude-fable-5-1","name":"Claude Fable 5.1","contextWindow":200000,"reasoning":true,"thinking":["high","max"],"input":["text"]},
	{"provider":"anthropic","id":"claude-opus-5-5","selector":"anthropic/claude-opus-5-5","name":"Claude Opus 5.5","contextWindow":200000,"reasoning":true,"thinking":["high","max"],"input":["text"]},
	{"provider":"anthropic","id":"claude-sonnet-5-5","selector":"anthropic/claude-sonnet-5-5","name":"Claude Sonnet 5.5","contextWindow":200000,"reasoning":true,"thinking":["high","max"],"input":["text"]},
	{"provider":"example","id":"foo-1","selector":"example/foo-1","name":"Foo 1","contextWindow":100000,"reasoning":true,"thinking":["high","max"],"input":["text"]},
	{"provider":"openai-codex","id":"gpt-6-astra","selector":"openai-codex/gpt-6-astra","name":"GPT-6 Astra","contextWindow":272000,"reasoning":true,"thinking":["high","max"],"input":["text"]},
	{"provider":"openai-codex","id":"gpt-6-luna","selector":"openai-codex/gpt-6-luna","name":"GPT-6 Luna","contextWindow":272000,"reasoning":true,"thinking":["high","max"],"input":["text"]}
]}`

var (
	pinFable = config.RoleModelCandidateConf{Selector: "anthropic/claude-fable-5-1", Thinking: "high", Family: "anthropic"}
	pinAstra = config.RoleModelCandidateConf{Selector: "openai-codex/gpt-6-astra", Thinking: "max", Family: "openai"}
)

func pinsOf(pins map[string]config.RoleModelCandidateConf) map[string]config.RoleAgentOverrideConf {
	agents := make(map[string]config.RoleAgentOverrideConf, len(pins))
	for agent, candidate := range pins {
		agents[agent] = config.RoleAgentOverrideConf{Candidates: []config.RoleModelCandidateConf{candidate}}
	}
	return agents
}

func writeOMPPinsProject(t *testing.T, policy config.RoleModelPolicyConf) string {
	t.Helper()
	root := t.TempDir()
	cfg := config.DefaultFullConfig("omp-pins")
	cfg.Platforms = []string{"omp"}
	cfg.RoleModelPolicy = policy
	require.NoError(t, config.Save(root, cfg))
	return root
}

func TestOMPBuiltinDeclaredFamilies_NamesEveryBuiltinModel(t *testing.T) {
	declared := ompBuiltinDeclaredFamilies()

	for selector, family := range map[string]string{
		"anthropic/claude-opus-5-5": "anthropic", "anthropic/claude-fable-5-1": "anthropic",
		"anthropic/claude-sonnet-5-5": "anthropic", "openai-codex/gpt-6-astra": "openai",
		"openai-codex/gpt-6-luna": "openai", "openai-codex/gpt-6.1-sol": "openai",
	} {
		assert.Equal(t, family, declared[selector], selector)
	}
	assert.NotContains(t, declared, "example/foo-1")
}

func TestInstalledOMPAttestedModelChoices_ListsOnlyAttestableModels(t *testing.T) {
	declared := ompBuiltinDeclaredFamilies()
	var out bytes.Buffer

	light, err := installedOMPAttestedModelChoices(context.Background(), &out,
		&ompCLIFakeRunner{catalog: []byte(composeOMPCatalogML2)}, declared, "NOTE")

	require.NoError(t, err)
	assert.Equal(t, "NOTE\n", out.String())
	var selectors []string
	for _, choice := range light {
		selectors = append(selectors, choice.selector()+"="+choice.Family)
	}
	assert.Equal(t, []string{
		"anthropic/claude-fable-5-1=anthropic", "anthropic/claude-opus-5-5=anthropic", "anthropic/claude-sonnet-5-5=anthropic",
		"openai-codex/gpt-6-astra=openai", "openai-codex/gpt-6-luna=openai",
	}, selectors)

	out.Reset()
	strict, err := installedOMPAttestedModelChoices(context.Background(), &out,
		&ompCLIFakeRunner{catalog: ompCLIBalancedCatalogJSON()}, declared, "NOTE")

	require.NoError(t, err)
	assert.Empty(t, out.String())
	require.Len(t, strict, 4)
	assert.Equal(t, "anthropic", strict[0].Family)
	assert.NotEmpty(t, strict[0].Capabilities)
}

func TestPlanOMPPinsOnly_PlansThePinnedAgentsAndTask(t *testing.T) {
	root := writeOMPPinsProject(t, config.RoleModelPolicyConf{})
	opts, err := newOMPPinsOnlyApplyOptions(pinsOf(map[string]config.RoleModelCandidateConf{"planner": pinFable, "executor": pinAstra}))
	require.NoError(t, err)

	preview, err := planOMPProfile(context.Background(), root, opts, &ompCLIFakeRunner{catalog: []byte(composeOMPCatalogML2)})

	require.NoError(t, err)
	assert.Empty(t, preview.Blockers)
	assert.Equal(t, config.PinsOnlyRoleModelProfileName, preview.Name)
	assert.Empty(t, preview.Persisted.Profile)
	rows := map[string]string{}
	for _, row := range preview.Agents {
		rows[row.Agent] = row.EffectiveSelector + ":" + row.EffectiveThinking
	}
	assert.Equal(t, map[string]string{
		"planner": "anthropic/claude-fable-5-1:high", "executor": "openai-codex/gpt-6-astra:max", "task": "anthropic/claude-fable-5-1:high",
	}, rows)
}

func TestPlanOMPPinsOnly_BlocksAFamilyNothingAttests(t *testing.T) {
	root := writeOMPPinsProject(t, config.RoleModelPolicyConf{})
	wrong := pinAstra
	wrong.Family = "anthropic"
	opts, err := newOMPPinsOnlyApplyOptions(pinsOf(map[string]config.RoleModelCandidateConf{"executor": wrong}))
	require.NoError(t, err)

	preview, err := planOMPProfile(context.Background(), root, opts, &ompCLIFakeRunner{catalog: []byte(composeOMPCatalogML2)})

	require.NoError(t, err)
	require.NotEmpty(t, preview.Blockers)
	assert.Contains(t, preview.Blockers[len(preview.Blockers)-1], "agent_override_model_undeclared:openai-codex/gpt-6-astra")
}

func TestApplyOMPPinsOnly_WritesPinsWithoutAProfileAndClearsThemAgain(t *testing.T) {
	root := writeOMPPinsProject(t, config.RoleModelPolicyConf{Version: config.RoleModelPolicyVersionV1, Profile: "balanced", Family: "openai"})
	runner := &ompCLIFakeRunner{catalog: []byte(composeOMPCatalogML2)}
	activations := 0
	activate := func(context.Context, string, *config.HarnessConfig) error { activations++; return nil }
	opts, err := newOMPPinsOnlyApplyOptions(pinsOf(map[string]config.RoleModelCandidateConf{"planner": pinFable}))
	require.NoError(t, err)

	_, err = applyOMPProfile(context.Background(), root, opts, runner, activate)

	require.NoError(t, err)
	policy := rawRoleModelPolicyOf(t, readBlockFixture(t, root))
	assert.Empty(t, policy.Profile)
	assert.Empty(t, policy.Family)
	assert.Equal(t, config.RoleModelPolicyVersionV1, policy.Version)
	assert.Equal(t, pinsOf(map[string]config.RoleModelCandidateConf{"planner": pinFable}), policy.Agents)

	cleared, err := newOMPPinsOnlyApplyOptions(nil)
	require.NoError(t, err)
	_, err = applyOMPProfile(context.Background(), root, cleared, runner, activate)

	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(root, "autopus.yaml"))
	require.NoError(t, err)
	assert.NotContains(t, string(data), "role_model_policy")
	assert.Equal(t, 2, activations)
}

func TestPlanInitOMPPinsOnly_LeavesInitsConfigAlone(t *testing.T) {
	cfg := config.DefaultFullConfig("omp-init-pins")
	cfg.Platforms = []string{"omp"}
	opts, err := newOMPPinsOnlyApplyOptions(pinsOf(map[string]config.RoleModelCandidateConf{"executor": pinAstra}))
	require.NoError(t, err)

	_, candidate, err := planInitOMP(context.Background(), cfg, opts, &ompCLIFakeRunner{catalog: []byte(composeOMPCatalogML2)})

	require.NoError(t, err)
	assert.Equal(t, pinsOf(map[string]config.RoleModelCandidateConf{"executor": pinAstra}), candidate.Agents)
	assert.Empty(t, candidate.Profile)
	assert.Equal(t, config.RoleModelPolicyConf{}, cfg.RoleModelPolicy)
}

// REQ-MUX3-23: unchanged agents keep their whole route, session-model agents
// lose theirs, and task keeps a route only where it differs from planner's.
func TestOMPPinsOnlyTarget_ConvertsAProfileByTheTaskRule(t *testing.T) {
	ladder := []config.RoleModelCandidateConf{pinAstra, {Selector: "openai-codex/gpt-6.1-sol", Thinking: "xhigh", Family: "openai"}}
	start := ompAgentRoutes{"planner": ladder, "explorer": {pinAstra}, "executor": {pinAstra}, "task": ladder}
	final := ompAgentRoutes{"planner": ladder, "explorer": nil, "executor": {pinAstra}}

	same := ompPinsOnlyTarget(start, final, true, false, nil)
	assert.Equal(t, map[string]config.RoleAgentOverrideConf{
		"planner": {Candidates: ladder}, "executor": {Candidates: []config.RoleModelCandidateConf{pinAstra}},
	}, same, "task equals planner, so it follows planner")

	final["planner"] = []config.RoleModelCandidateConf{pinFable}
	moved := ompPinsOnlyTarget(start, final, true, false, nil)
	assert.Equal(t, ladder, moved["task"].Candidates, "task keeps the route it had")

	follow := []config.RoleModelCandidateConf{}
	assert.NotContains(t, ompPinsOnlyTarget(start, final, true, false, &follow), "task", "planner를 따름")
	own := ompPinsOnlyTarget(ompAgentRoutes{"task": {pinAstra}}, ompAgentRoutes{}, false, true, nil)
	assert.Equal(t, []config.RoleModelCandidateConf{pinAstra}, own["task"].Candidates, "a pins-only task pin stays")
	assert.Empty(t, ompPinsOnlyTarget(ompAgentRoutes{"task": {pinAstra}}, ompAgentRoutes{}, false, false, nil))
}

// Security review P1: a pin already in autopus.yaml vouches for nothing. On a
// catalog without family metadata a hand-written unknown model is a blocker
// naming the agent and model, never an attested route.
func TestPlanOMPPinsOnly_ExistingPinsDoNotAttestThemselves(t *testing.T) {
	foo := config.RoleModelCandidateConf{Selector: "example/foo-1", Thinking: "high", Family: "example"}
	root := writeOMPPinsProject(t, config.RoleModelPolicyConf{
		Version: config.RoleModelPolicyVersionV1, Agents: pinsOf(map[string]config.RoleModelCandidateConf{"executor": foo}),
	})
	opts, err := newOMPPinsOnlyApplyOptions(pinsOf(map[string]config.RoleModelCandidateConf{"executor": foo, "planner": pinFable}))
	require.NoError(t, err)

	preview, err := planOMPProfile(context.Background(), root, opts, &ompCLIFakeRunner{catalog: []byte(composeOMPCatalogML2)})

	require.NoError(t, err)
	assert.Contains(t, preview.Blockers, "agent=executor reason=agent_override_model_undeclared:example/foo-1")
	assert.NotContains(t, strings.Join(preview.Blockers, " "), "planner")
}
