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

	// The Role Group Contract table, restricted to the groups that hold a
	// generated agent.
	want := []struct {
		ko, en string
		agents []string
	}{
		{"구현", "Implementation", []string{"frontend-specialist"}},
		{"검증·리뷰", "Verification·Review", []string{"reviewer", "qa-explorer", "ux-validator"}},
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
			want:    []string{"sonnet", "혼합 (opus 2 · sonnet 1)"},
		},
		{
			name:    "R ultra HU1 with source fallbacks",
			quality: repo,
			want:    []string{"sonnet", "혼합 (opus 2 · sonnet 1)"},
		},
		{
			name:    "B2 historical balanced keeps the standard placement",
			quality: b2,
			want:    []string{"sonnet", "혼합 (opus 2 · sonnet 1)"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			composition := newQualityComposition(tt.quality, sources)
			assert.Equal(t, tt.want, composeGroupsDisplays(composition, "ko"))
		})
	}

	// B2 writes reviewer: sonnet, yet the runtime places it on the top rung.
	assert.Equal(t, "sonnet", b2.Presets["balanced"].Agents["reviewer"])
	assert.Equal(t, "opus", newQualityComposition(b2, sources).Agents()["reviewer"])
	// R omits qa-explorer; its embedded source declares opus.
	assert.NotContains(t, repo.Presets["ultra"].Agents, "qa-explorer")
	assert.Equal(t, "opus", newQualityComposition(repo, sources).Agents()["qa-explorer"])
	// No legacy role enters the composition, so none is saved from it.
	assert.NotContains(t, newQualityComposition(repo, sources).Agents(), "debugger")
}

func TestNewQualityComposition_ProviderOverrideDoesNotMoveTheGlobalStart(t *testing.T) {
	t.Parallel()
	quality := config.DefaultFullConfig("p").Quality
	quality.Presets["codex-custom"] = config.QualityPreset{Agents: map[string]string{"reviewer": "haiku"}}
	quality.Providers = map[string]string{config.QualityProviderCodex: "codex-custom"}

	composition := newQualityComposition(quality, composeGroupsSources(t))

	assert.Equal(t, "opus", composition.Agents()["reviewer"])
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

	// g, haiku for Implementation, sonnet for Verification·Review.
	groups.SetGroupTier(composition, 0, "haiku")
	groups.SetGroupTier(composition, 1, "sonnet")
	assert.Equal(t, []string{"haiku", "sonnet"}, displays("ko"))
	assert.Len(t, composition, len(config.CanonicalAgentNames()), "a group edit adds no agent")

	composition["qa-explorer"] = "fable"
	assert.Equal(t, "혼합 (fable 1 · sonnet 2)", displays("ko")[1])
	assert.Equal(t, "mixed (fable 1 · sonnet 2)", displays("en")[1])
	assert.Equal(t, "haiku", displays("ko")[0])
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
	agents["reviewer"] = "haiku"

	assert.Equal(t, "opus", composition.Agents()["reviewer"])
	assert.Len(t, agents, len(config.CanonicalAgentNames()))
}
