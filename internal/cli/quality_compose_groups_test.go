package cli

// SPEC-MODELUX-001 REQ-MUX-01..04, acceptance S1-S3: the four UI-only role
// groups, the composition state that starts from the tiers generation
// projects, the mixed group display, group selection, and advanced edits.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

func composeGroupsSources(t *testing.T) map[string]composeAgentSource {
	t.Helper()
	sources, err := loadComposeAgentSources()
	require.NoError(t, err)
	return sources
}

func composeGroupsDisplays(c qualityComposition, lang string) []string {
	displays := make([]string, 0, len(composeRoleGroups))
	for _, group := range composeRoleGroups {
		displays = append(displays, c.GroupDisplay(group, lang))
	}
	return displays
}

func TestComposeRoleGroups_ContractOrder_CoversEveryCanonicalAgentOnce(t *testing.T) {
	t.Parallel()

	// The Role Group Contract table, verbatim.
	want := []struct {
		ko, en string
		agents []string
	}{
		{"설계·판단", "Design·Judgment", []string{"planner", "architect", "spec-writer", "security-auditor"}},
		{"구현", "Implementation", []string{
			"executor", "debugger", "deep-worker", "devops", "frontend-specialist", "perf-engineer",
		}},
		{"검증·리뷰", "Verification·Review", []string{"reviewer", "validator", "tester", "ux-validator"}},
		{"탐색·보조", "Exploration·Support", []string{"explorer", "annotator"}},
	}
	require.Len(t, composeRoleGroups, len(want))
	for i, group := range composeRoleGroups {
		assert.Equal(t, want[i].ko, group.Label("ko"))
		assert.Equal(t, want[i].en, group.Label("en"))
		assert.Equal(t, want[i].agents, group.Agents)
	}

	seen := map[string]int{}
	for _, group := range composeRoleGroups {
		for _, agent := range group.Agents {
			seen[agent]++
		}
	}
	canonical := config.CanonicalAgentNames()
	assert.Len(t, seen, len(canonical), "groups must cover exactly the canonical agents")
	for _, agent := range canonical {
		assert.Equal(t, 1, seen[agent], "agent %s must belong to exactly one group", agent)
	}
}

func TestNewQualityComposition_StartsFromEffectiveTiers(t *testing.T) {
	t.Parallel()
	sources := composeGroupsSources(t)
	repo := rawQualityOf(t, blockRepoQuality)
	b2 := config.DefaultFullConfig("p").Quality
	b2.Presets["balanced"] = repo.Presets["balanced"] // balanced HB2

	tests := []struct {
		name    string
		quality config.QualityConf
		want    []string
	}{
		{
			name:    "D current balanced",
			quality: config.DefaultFullConfig("p").Quality,
			want:    []string{"fable", "혼합 (fable 2 · sonnet 4)", "혼합 (fable 1 · sonnet 3)", "sonnet"},
		},
		{
			name:    "R ultra HU1 with source fallbacks",
			quality: repo,
			want:    []string{"opus", "혼합 (opus 4 · sonnet 2)", "혼합 (opus 3 · sonnet 1)", "혼합 (opus 1 · sonnet 1)"},
		},
		{
			name:    "B2 historical balanced keeps the standard placement",
			quality: b2,
			want:    []string{"fable", "혼합 (fable 2 · sonnet 4)", "혼합 (fable 1 · sonnet 3)", "sonnet"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			composition := newQualityComposition(tt.quality, sources)
			assert.Equal(t, tt.want, composeGroupsDisplays(composition, "ko"))
		})
	}

	// B2 writes debugger: sonnet, yet the runtime places it on the top rung.
	assert.Equal(t, "sonnet", b2.Presets["balanced"].Agents["debugger"])
	assert.Equal(t, "fable", newQualityComposition(b2, sources).Agents()["debugger"])
	// R omits deep-worker; its embedded source declares opus.
	assert.Equal(t, "opus", newQualityComposition(repo, sources).Agents()["deep-worker"])
}

func TestNewQualityComposition_ProviderOverrideDoesNotMoveTheGlobalStart(t *testing.T) {
	t.Parallel()
	quality := config.DefaultFullConfig("p").Quality
	quality.Presets["codex-custom"] = config.QualityPreset{Agents: map[string]string{"planner": "haiku"}}
	quality.Providers = map[string]string{config.QualityProviderCodex: "codex-custom"}

	composition := newQualityComposition(quality, composeGroupsSources(t))

	assert.Equal(t, "fable", composition.Agents()["planner"])
}

// The driver edits a composition map through composeSessionGroups: a group
// selection sets every member, an advanced edit sets one agent, and each
// group display is recomputed from the map it is handed (S3).
func TestComposeSessionGroups_EditsRecomputeTheGroupDisplay(t *testing.T) {
	t.Parallel()
	composition := newQualityComposition(config.DefaultFullConfig("p").Quality, composeGroupsSources(t)).Agents()
	displays := func(lang string) []string {
		var shown []string
		for _, group := range (composeSessionGroups{lang: lang}).Groups(composition) {
			shown = append(shown, group.Display)
		}
		return shown
	}
	groups := composeSessionGroups{lang: "ko"}

	// g, Enter, 3, Enter, 4.
	groups.SetGroupTier(composition, 1, "sonnet")
	groups.SetGroupTier(composition, 3, "haiku")
	assert.Equal(t, []string{"fable", "sonnet", "혼합 (fable 1 · sonnet 3)", "haiku"}, displays("ko"))

	composition["debugger"] = "opus"
	assert.Equal(t, "혼합 (opus 1 · sonnet 5)", displays("ko")[1])
	composition["explorer"] = "fable"
	assert.Equal(t, "혼합 (fable 1 · haiku 1)", displays("ko")[3])
	assert.Equal(t, "mixed (fable 1 · haiku 1)", displays("en")[3])
}

func TestQualityComposition_SetGroupTier_RejectsUnknownTier(t *testing.T) {
	t.Parallel()
	composition := newQualityComposition(config.DefaultFullConfig("p").Quality, composeGroupsSources(t))
	before := composition.Agents()

	err := composition.SetGroupTier(composeRoleGroups[0], "gpt")

	require.Error(t, err)
	assert.Equal(t, before, composition.Agents())
}

func TestQualityComposition_Agents_IsDetached(t *testing.T) {
	t.Parallel()
	composition := newQualityComposition(config.DefaultFullConfig("p").Quality, composeGroupsSources(t))

	agents := composition.Agents()
	agents["planner"] = "haiku"

	assert.Equal(t, "fable", composition.Agents()["planner"])
	assert.Len(t, agents, len(config.CanonicalAgentNames()))
}
