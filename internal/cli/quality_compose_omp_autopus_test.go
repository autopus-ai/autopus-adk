package cli

// SPEC-MODELUX-003 T11 (REQ-MUX3-13; acceptance S10, S11): in autopus mode
// the OMP tab plans the profile once without pins before the method select,
// offers role groups and per-agent pins over the 16 canonical agents, and
// writes through the SPEC-MODELUX-002 OMP paths.

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

const composeOMPMetadataNotice = "The installed catalog reports no family or capability metadata; " +
	"a pin is attested against the built-in declarations.\n"

// composeOMPCatalogML is ML: the four models of
// ompCLIProfileNativeCatalogJSON in the shape an installed omp prints, with
// no family or capability metadata. ompCLIProfileNativeCatalogJSON itself
// lacks the display fields the native tab's catalog rule reads.
const composeOMPCatalogML = `{"models":[
	{"provider":"anthropic","id":"claude-fable-5-1","selector":"anthropic/claude-fable-5-1","name":"Claude Fable 5.1","contextWindow":200000,"reasoning":true,"thinking":["high","max"],"input":["text"]},
	{"provider":"anthropic","id":"claude-sonnet-5-5","selector":"anthropic/claude-sonnet-5-5","name":"Claude Sonnet 5.5","contextWindow":200000,"reasoning":true,"thinking":["high","max"],"input":["text"]},
	{"provider":"openai-codex","id":"gpt-6-astra","selector":"openai-codex/gpt-6-astra","name":"GPT-6 Astra","contextWindow":272000,"reasoning":true,"thinking":["high","max"],"input":["text"]},
	{"provider":"openai-codex","id":"gpt-6-luna","selector":"openai-codex/gpt-6-luna","name":"GPT-6 Luna","contextWindow":272000,"reasoning":true,"thinking":["high","max"],"input":["text"]}
]}`

// composeOMPDesignGroup is the title of the 설계·판단 model select.
const composeOMPDesignGroup = "설계·판단 — planner, architect, spec-writer, security-auditor"

// writeComposeOP writes OP: O3 on the default autopus source with
// role_model_policy {v1, balanced, openai}.
func writeComposeOP(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cfg := composeFixtureConfig("claude-code", "codex", "omp")
	cfg.RoleModelPolicy = config.RoleModelPolicyConf{
		Version: config.RoleModelPolicyVersionV1, Profile: "balanced", Family: "openai",
	}
	require.NoError(t, config.Save(dir, cfg))
	return dir
}

// composeOMPPrePlan is the pin-less balanced plan of dir: the routes the
// tab's pre-plan reads, keyed by agent.
func composeOMPPrePlan(t *testing.T, dir string, catalog []byte) map[string]ompProfileAgentPreviewPayload {
	t.Helper()
	opts, err := newOMPProfileApplyOptions("balanced", "openai", nil, false, false)
	require.NoError(t, err)
	preview, err := planOMPProfile(context.Background(), dir, opts, &ompCLIFakeRunner{catalog: catalog})
	require.NoError(t, err)
	require.Empty(t, preview.Blockers)
	rows := make(map[string]ompProfileAgentPreviewPayload, len(preview.Agents))
	for _, row := range preview.Agents {
		rows[row.Agent] = row
	}
	return rows
}

// composeOMPCatalogWithoutProvider is NO: the balanced catalog without
// provider's models.
func composeOMPCatalogWithoutProvider(t *testing.T, provider string) []byte {
	t.Helper()
	var raw struct {
		Models []map[string]any `json:"models"`
	}
	require.NoError(t, json.Unmarshal(ompCLIBalancedCatalogJSON(), &raw))
	raw.Models = slices.DeleteFunc(raw.Models, func(model map[string]any) bool { return model["provider"] == provider })
	encoded, err := json.Marshal(raw)
	require.NoError(t, err)
	return encoded
}

func composeOMPFileSum(t *testing.T, dir string) [32]byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(dir, "autopus.yaml"))
	require.NoError(t, err)
	return sha256.Sum256(body)
}

func composeOMPOptionList(options ...string) string {
	var list strings.Builder
	for i, option := range options {
		fmt.Fprintf(&list, "%d. %s\n", i+1, option)
	}
	return list.String()
}

// S10: a role group's model and thinking pin every member, the model select
// follows the catalog rule, and the write activates once.
func TestComposeOMPAutopus_S10_RoleGroupPinsEveryMember(t *testing.T) {
	for name, row := range map[string]struct {
		catalog []byte
		notice  bool
		models  []string
	}{
		"ML": {[]byte(composeOMPCatalogML), true, []string{
			"anthropic/claude-fable-5-1", "anthropic/claude-sonnet-5-5", "openai-codex/gpt-6-astra", "openai-codex/gpt-6-luna",
		}},
		"MR": {ompCLIBalancedCatalogJSON(), false, []string{"anthropic/claude-fable-5-1", "openai-codex/gpt-6-astra"}},
	} {
		t.Run(name, func(t *testing.T) {
			composeSessionCatalog(t, "CAT-A")
			dir := writeComposeOP(t)
			deps, activations := composeOMPDeps(row.catalog)
			astra := fmt.Sprintf("%d\n", slices.Index(row.models, "openai-codex/gpt-6-astra")+2)
			// 구성 편집, OMP, 에이전트별 지정, GPT, 역할 그룹, 설계·판단 model and
			// thinking max, Enter for the other three groups' model and
			// thinking, 적용, 유지.
			lines := []string{"2\n", "4\n", "4\n", "1\n", "1\n", astra, "3\n", "\n", "\n", "\n", "\n", "\n", "\n", "2\n", "\n"}

			out, err := runComposeOMPForms(t, dir, deps, lines...)

			require.NoError(t, err, out)
			models := composeOMPOptionList(append([]string{"프로필 기본값"}, row.models...)...)
			assert.Contains(t, out, composeOMPDesignGroup+" \n"+models)
			if row.notice {
				assert.Contains(t, out, composeOMPMetadataNotice+composeOMPDesignGroup)
			} else {
				assert.NotContains(t, out, composeOMPMetadataNotice)
			}
			assert.Contains(t, out, "설계·판단 thinking \n1. 프로필 기본값 (")
			policy := rawRoleModelPolicyOf(t, readBlockFixture(t, dir))
			agents := make([]string, 0, len(policy.Agents))
			for agent, override := range policy.Agents {
				agents = append(agents, agent)
				require.Len(t, override.Candidates, 1, agent)
				candidate := override.Candidates[0]
				assert.Equal(t, []string{"openai-codex/gpt-6-astra", "max", "openai"},
					[]string{candidate.Selector, candidate.Thinking, candidate.Family}, agent)
			}
			sort.Strings(agents)
			assert.Equal(t, []string{"architect", "planner", "security-auditor", "spec-writer"}, agents)
			assert.Equal(t, 1, activations.calls)
		})
	}
}

// S11: the thinking select offers the pre-plan's thinking as the profile
// default, a thinking-only change pins the pre-plan's selector, and a
// pre-plan blocker ends the tab before the method select.
func TestComposeOMPAutopus_S11_PrePlanAndThinkingOnly(t *testing.T) {
	t.Run("OP", func(t *testing.T) {
		composeSessionCatalog(t, "CAT-A")
		dir := writeComposeOP(t)
		pre := composeOMPPrePlan(t, dir, ompCLIBalancedCatalogJSON())["planner"]
		levels := []string{"high", "max"}
		level := levels[slices.IndexFunc(levels, func(l string) bool { return l != pre.EffectiveThinking })]
		deps, activations := composeOMPDeps(ompCLIBalancedCatalogJSON())
		// 구성 편집, OMP, 에이전트별 지정, GPT, 에이전트별, planner, 프로필
		// 기본값, L, 완료, 적용, 유지.
		pick := fmt.Sprintf("%d\n", slices.Index(levels, level)+2)
		lines := []string{"2\n", "4\n", "4\n", "1\n", "2\n", "2\n", "\n", pick, "\n", "2\n", "\n"}

		out, err := runComposeOMPForms(t, dir, deps, lines...)

		require.NoError(t, err, out)
		assert.Less(t, strings.Index(out, composeFormTextKO.Method), strings.Index(out, "planner thinking"))
		assert.Contains(t, out, "planner thinking \n"+
			composeOMPOptionList("프로필 기본값 ("+pre.EffectiveThinking+")", "high", "max"))
		assert.Contains(t, out, "planner — "+pre.EffectiveSelector+":"+level+"\n")
		policy := rawRoleModelPolicyOf(t, readBlockFixture(t, dir))
		require.Len(t, policy.Agents, 1)
		require.Len(t, policy.Agents["planner"].Candidates, 1)
		candidate := policy.Agents["planner"].Candidates[0]
		assert.Equal(t, []string{pre.EffectiveSelector, level}, []string{candidate.Selector, candidate.Thinking})
		assert.Equal(t, 1, activations.calls)
	})
	t.Run("NO copy", func(t *testing.T) {
		composeSessionCatalog(t, "CAT-A")
		dir := writeComposeOP(t)
		before := composeOMPFileSum(t, dir)
		deps, activations := composeOMPDeps(composeOMPCatalogWithoutProvider(t, "openai-codex"))

		out, err := runComposeOMPForms(t, dir, deps, "2\n", "4\n", "4\n", "1\n", "\n")

		require.NoError(t, err, out)
		assert.Contains(t, out, "  경고: omp_profile_candidate_unavailable")
		assert.NotContains(t, out, composeFormTextKO.Method)
		assert.Zero(t, activations.calls)
		assert.Equal(t, before, composeOMPFileSum(t, dir))
	})
}

// The edit's level and pin rules (Effort Option Contract, OMP row).
func TestOMPAutopusEdit_LevelsAndPins(t *testing.T) {
	edit := &ompAutopusEdit{
		routes: map[string]ompAutopusRoute{
			"planner":   {selector: "p/a", thinking: "high"},
			"architect": {selector: "p/b", thinking: "max"},
		},
		choices: []ompCustomModelChoice{
			{Provider: "p", Model: "a", Thinking: []string{"low", "high", "max"}},
			{Provider: "p", Model: "b", Thinking: []string{"high", "max"}},
			{Provider: "p", Model: "c", Thinking: []string{"low", "medium"}},
		},
		pins: map[string]string{},
	}
	group := []string{"planner", "architect"}

	levels, keep := edit.levels("", group)
	assert.Equal(t, []string{"high", "max"}, levels, "the profile default intersects the pre-plan models")
	assert.True(t, keep)
	levels, keep = edit.levels("p/c", group)
	assert.Equal(t, []string{"low", "medium"}, levels)
	assert.False(t, keep, "neither pre-plan thinking is a level of p/c")
	assert.Equal(t, "high, max", edit.defaultThinking(group))

	edit.set(group, "", "max")
	assert.Equal(t, map[string]string{"planner": "p/a:max"}, edit.pins, "architect already runs max")
	edit.set(group, "p/b", "")
	assert.Equal(t, map[string]string{"planner": "p/b:high", "architect": "p/b:max"}, edit.pins)
	edit.set([]string{"planner"}, "", "")
	assert.Equal(t, []string{"architect=p/b:max"}, edit.ordered())
}
