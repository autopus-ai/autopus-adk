package cli

// SPEC-MODELUX-003 T11 (REQ-MUX3-23; acceptance S25): editing a profile-based
// policy in the autopus OMP tab converts it to pins. Unchanged agents keep
// their routes, task keeps its own route only where it differs from
// planner's, and a pre-plan blocker ends the tab before anything is asked.

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// composeOMPS25Edit answers 역할 그룹 with Enter for 설계·판단, 구현, and
// 검증·리뷰, the session model for 탐색·보조, and 적용.
var composeOMPS25Edit = []string{"2\n", "4\n", "1\n", "\n", "\n", "\n", "2\n", "2\n"}

// writeComposeOPT is OP with OPT's planner and task pins.
func writeComposeOPT(t *testing.T) string {
	t.Helper()
	return writeComposePinsFixture(t, config.RoleModelPolicyConf{
		Version: config.RoleModelPolicyVersionV1, Profile: "balanced", Family: "openai",
		Agents: pinsOf(map[string]config.RoleModelCandidateConf{"planner": pinFable, "task": pinAstra}),
	})
}

func writeComposeOPPolicy(t *testing.T) string {
	t.Helper()
	return writeComposePinsFixture(t, config.RoleModelPolicyConf{Version: config.RoleModelPolicyVersionV1, Profile: "balanced", Family: "openai"})
}

// composeOMPRecorded activates dir's policy as it stands and returns the
// overrides and the pin-less plan's route candidates per agent.
func composeOMPRecorded(t *testing.T, dir string) (map[string]string, map[string][]config.RoleModelCandidateConf) {
	t.Helper()
	cfg, err := config.LoadPreview(dir)
	require.NoError(t, err)
	overrides := composeOMPActivated(t, dir, cfg)
	opts, err := newOMPProfileApplyOptions(cfg.RoleModelPolicy.Profile, "", nil, false, false)
	require.NoError(t, err)
	preview, err := planOMPProfile(context.Background(), dir, opts, &ompCLIFakeRunner{catalog: []byte(composeOMPCatalogML)})
	require.NoError(t, err)
	routes := map[string][]config.RoleModelCandidateConf{}
	for _, row := range preview.Agents {
		for _, candidate := range row.Candidates {
			routes[row.Agent] = append(routes[row.Agent], config.RoleModelCandidateConf(candidate))
		}
	}
	return overrides, routes
}

func TestComposeOMPAutopus_S25_ConvertsTheProfileOnApply(t *testing.T) {
	t.Run("OP and its cancelled copy", func(t *testing.T) {
		composeSessionCatalog(t, "CAT-A")
		dir := writeComposeOPPolicy(t)
		recorded, routes := composeOMPRecorded(t, dir)
		deps, activations := composeOMPDeps([]byte(composeOMPCatalogML))

		out, err := runComposeOMPForms(t, dir, deps, composeFormLines(composeOMPS25Edit, "\n")...)

		require.NoError(t, err, out)
		var labels []string
		for _, agent := range composeOMPDesignAgents {
			label := routes[agent][0].Selector + " · " + routes[agent][0].Thinking
			if !slices.Contains(labels, label) {
				labels = append(labels, label)
			}
		}
		assert.Contains(t, out, composeOMPDesignGroup+" \n1. 그대로 — "+strings.Join(labels, ", ")+"\n")
		convert := strings.Index(out, "OMP 프로필 balanced의 route를 에이전트별 지정으로 옮깁니다. 바꾸지 않은 에이전트의 route는 그대로입니다.")
		require.GreaterOrEqual(t, convert, 0)
		assert.Less(t, convert, strings.Index(out, "OMP pins-only — model preview"))
		policy := rawRoleModelPolicyOf(t, readBlockFixture(t, dir))
		assert.Empty(t, policy.Profile)
		assert.Empty(t, policy.Family)
		assert.NotContains(t, policy.Agents, "task", "task's route equals planner's")
		assert.Len(t, policy.Agents, 14)
		for _, agent := range []string{"explorer", "annotator"} {
			assert.NotContains(t, policy.Agents, agent)
		}
		for agent, override := range policy.Agents {
			assert.Equal(t, routes[agent], override.Candidates, agent)
		}
		overrides := composeOMPActivated(t, dir, activations.cfgs[0])
		assert.Len(t, overrides, 15)
		for agent, value := range overrides {
			assert.Equal(t, recorded[agent], value, agent)
		}

		cancelled := writeComposeOPPolicy(t)
		before := composeOMPFileSum(t, cancelled)
		edit := append(append([]string(nil), composeOMPS25Edit[:len(composeOMPS25Edit)-1]...), "1\n", "\n")
		_, err = runComposeOMPForms(t, cancelled, deps, edit...)
		require.NoError(t, err)
		assert.Equal(t, before, composeOMPFileSum(t, cancelled))
	})
	t.Run("OPT keeps its task pin", func(t *testing.T) {
		composeSessionCatalog(t, "CAT-A")
		dir := writeComposeOPT(t)
		recorded, _ := composeOMPRecorded(t, dir)
		require.NotEqual(t, recorded["planner"], recorded["task"])
		deps, activations := composeOMPDeps([]byte(composeOMPCatalogML))

		out, err := runComposeOMPForms(t, dir, deps, composeFormLines(composeOMPS25Edit, "\n")...)

		require.NoError(t, err, out)
		policy := rawRoleModelPolicyOf(t, readBlockFixture(t, dir))
		assert.Equal(t, []config.RoleModelCandidateConf{pinAstra}, policy.Agents["task"].Candidates)
		overrides := composeOMPActivated(t, dir, activations.cfgs[0])
		assert.Equal(t, recorded["task"], overrides["task"])
		assert.Equal(t, recorded["planner"], overrides["planner"])
	})
	t.Run("a new planner pin, then task follows planner", func(t *testing.T) {
		composeSessionCatalog(t, "CAT-A")
		dir := writeComposeOPPolicy(t)
		recorded, routes := composeOMPRecorded(t, dir)
		deps, activations := composeOMPDeps([]byte(composeOMPCatalogML))
		// 설계·판단 fable → high, 구현·검증·리뷰 Enter, 탐색·보조 세션 모델, 적용, 유지.
		lines := []string{"2\n", "4\n", "1\n", "3\n", "1\n", "\n", "\n", "2\n", "2\n", "\n"}

		out, err := runComposeOMPForms(t, dir, deps, lines...)

		require.NoError(t, err, out)
		policy := rawRoleModelPolicyOf(t, readBlockFixture(t, dir))
		assert.Equal(t, routes["task"], policy.Agents["task"].Candidates, "task keeps the route it had")
		overrides := composeOMPActivated(t, dir, activations.cfgs[0])
		assert.Equal(t, recorded["task"], overrides["task"])
		assert.Equal(t, "anthropic/claude-fable-5-1:high", overrides["planner"])

		// 구성 편집, OMP, 에이전트별, task, planner를 따름, 완료, 적용, 유지.
		taskLine := len(composeRoleGroupAgents()) + 2
		out, err = runComposeOMPForms(t, dir, deps, "2\n", "4\n", "2\n", itoaLine(taskLine), "2\n", "\n", "2\n", "\n")

		require.NoError(t, err, out)
		first := routes["task"][0]
		assert.Contains(t, out, "task — "+first.Selector+" · "+first.Thinking+"\n")
		policy = rawRoleModelPolicyOf(t, readBlockFixture(t, dir))
		assert.NotContains(t, policy.Agents, "task")
		overrides = composeOMPActivated(t, dir, activations.cfgs[1])
		assert.Equal(t, "anthropic/claude-fable-5-1:high", overrides["task"])
	})
	t.Run("a pre-plan blocker", func(t *testing.T) {
		composeSessionCatalog(t, "CAT-A")
		dir := writeComposeOPPolicy(t)
		before := composeOMPFileSum(t, dir)
		deps, activations := composeOMPDeps(composeOMPCatalogWithoutProvider(t, "openai-codex"))

		out, err := runComposeOMPForms(t, dir, deps, "2\n", "4\n", "\n")

		require.NoError(t, err, out)
		assert.Contains(t, out, "  경고: omp_profile_candidate_unavailable")
		assert.NotContains(t, out, composeFormTextKO.Method)
		assert.Zero(t, activations.calls)
		assert.Equal(t, before, composeOMPFileSum(t, dir))
	})
}

func composeRoleGroupAgents() []string {
	var agents []string
	for _, group := range composeRoleGroups {
		agents = append(agents, group.Agents...)
	}
	return agents
}

func itoaLine(n int) string {
	encoded, _ := json.Marshal(n)
	return string(encoded) + "\n"
}

// composeOMPCatalogWithoutProvider is NO: the balanced catalog without
// provider's models.
func composeOMPCatalogWithoutProvider(t *testing.T, provider string) []byte {
	t.Helper()
	var raw struct {
		Models []map[string]any `json:"models"`
	}
	require.NoError(t, json.Unmarshal(ompCLIBalancedCatalogJSON(), &raw))
	kept := raw.Models[:0]
	for _, model := range raw.Models {
		if model["provider"] != provider {
			kept = append(kept, model)
		}
	}
	raw.Models = kept
	encoded, err := json.Marshal(raw)
	require.NoError(t, err)
	return encoded
}

// Review A1: the pin-less pre-plan reports a built-in route that falls back
// to the runtime default as a blocker, so a conversion never drops a route
// silently: the tab stops before the method select and writes nothing.
func TestComposeOMPAutopus_DegradedProfileRoutesStopTheConversion(t *testing.T) {
	composeSessionCatalog(t, "CAT-A")
	dir := writeComposePinsFixture(t, config.RoleModelPolicyConf{Version: config.RoleModelPolicyVersionV1, Profile: "ultra", Family: "anthropic"})
	before := composeOMPFileSum(t, dir)
	deps, activations := composeOMPDeps(composeOMPCatalogWithoutProvider(t, "anthropic"))

	out, err := runComposeOMPForms(t, dir, deps, "2\n", "4\n", "\n")

	require.NoError(t, err, out)
	assert.Contains(t, out, "omp_profile_candidate_unavailable")
	assert.Contains(t, out, "reason=explicit_degraded")
	assert.NotContains(t, out, composeFormTextKO.Method)
	assert.Zero(t, activations.calls)
	assert.Equal(t, before, composeOMPFileSum(t, dir))
}

// Review N2: a profile whose routes name a model no built-in profile
// declares cannot be converted on a metadata-light catalog (Pins-only
// attestation). The tab says so at its start, naming the agents and model,
// and points at the paths that keep editing the profile, instead of failing
// only at the final plan.
func TestComposeOMPAutopus_NamesUnattestableProfileModelsAtStart(t *testing.T) {
	composeSessionCatalog(t, "CAT-A")
	fable := config.RoleModelCandidateConf{Selector: "anthropic/claude-fable-5-1", Thinking: "high", Family: "anthropic"}
	foo := config.RoleModelCandidateConf{Selector: "example/foo-1", Thinking: "high", Family: "example"}
	capabilities := map[string]config.RoleCapabilityRouteConf{}
	for _, capability := range config.OMPProviderNeutralCapabilities() {
		candidate := fable
		if capability == config.CapabilityVisionDesign {
			candidate = foo
		}
		capabilities[capability] = config.RoleCapabilityRouteConf{Required: true, Candidates: []config.RoleModelCandidateConf{candidate}}
	}
	dir := writeComposePinsFixture(t, config.RoleModelPolicyConf{Version: config.RoleModelPolicyVersionV1, Profile: "team",
		Profiles: map[string]config.RoleModelProfileConf{"team": {
			ConfigMode: config.RoleModelConfigModeOverlay, CatalogTrust: config.RoleModelCatalogTrustOperatorAttested, Capabilities: capabilities,
		}}})
	deps, _ := composeOMPDeps([]byte(composeOMPCatalogML2))

	out, err := runComposeOMPForms(t, dir, deps, "2\n", "4\n", "1\n", "\n", "\n", "\n", "\n", "\n")

	require.NoError(t, err, out)
	line := fmt.Sprintf(composeFormTextKO.OMPProfileUnattested, "frontend-specialist=example/foo-1, ux-validator=example/foo-1")
	assert.Contains(t, out, line+"\n")
	assert.Less(t, strings.Index(out, line), strings.Index(out, composeFormTextKO.Method))
	assert.Contains(t, line, "auto platform omp profile apply")
}
