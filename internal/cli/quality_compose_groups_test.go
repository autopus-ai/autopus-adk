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

func TestComposeGroupOf_CanonicalAgent_ReturnsItsGroup(t *testing.T) {
	t.Parallel()

	group, ok := composeGroupOf("debugger")
	require.True(t, ok)
	assert.Equal(t, "구현", group.Label("ko"))

	group, ok = composeGroupOf("Security_Auditor")
	require.True(t, ok, "workflow spelling folds onto the agent name")
	assert.Equal(t, "Design·Judgment", group.Label("en"))

	_, ok = composeGroupOf("gpt-writer")
	assert.False(t, ok)
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
	assert.Equal(t, "fable", newQualityComposition(b2, sources).Tier("debugger"))
	// R omits deep-worker; its embedded source declares opus.
	assert.Equal(t, "opus", newQualityComposition(repo, sources).Tier("deep-worker"))
}

func TestNewQualityComposition_ProviderOverrideDoesNotMoveTheGlobalStart(t *testing.T) {
	t.Parallel()
	quality := config.DefaultFullConfig("p").Quality
	quality.Presets["codex-custom"] = config.QualityPreset{Agents: map[string]string{"planner": "haiku"}}
	quality.Providers = map[string]string{config.QualityProviderCodex: "codex-custom"}

	composition := newQualityComposition(quality, composeGroupsSources(t))

	assert.Equal(t, "fable", composition.Tier("planner"))
}

func TestQualityComposition_GroupSelectionAndAdvancedEdits(t *testing.T) {
	t.Parallel()
	composition := newQualityComposition(config.DefaultFullConfig("p").Quality, composeGroupsSources(t))

	// S3: g, Enter, 3, Enter, 4.
	for i, answer := range []string{"", "3", "", "4"} {
		tier, keep, err := parseComposeGroupAnswer(answer)
		require.NoError(t, err)
		if keep {
			continue
		}
		require.NoError(t, composition.SetGroupTier(composeRoleGroups[i], tier))
	}
	assert.Equal(t, []string{"fable", "sonnet", "혼합 (fable 1 · sonnet 3)", "haiku"},
		composeGroupsDisplays(composition, "ko"))

	agent, tier, err := composition.ApplyAssignment("debugger=opus")
	require.NoError(t, err)
	assert.Equal(t, [2]string{"debugger", "opus"}, [2]string{agent, tier})
	assert.Equal(t, "혼합 (opus 1 · sonnet 5)", composition.GroupDisplay(composeRoleGroups[1], "ko"))

	_, _, err = composition.ApplyAssignment("tester=gpt")
	require.EqualError(t, err, `unknown tier "gpt" (choose one of: fable, opus, sonnet, haiku)`)
	assert.Equal(t, "opus", composition.Tier("debugger"), "a rejected line keeps earlier edits")
	assert.Equal(t, "sonnet", composition.Tier("tester"))

	_, _, err = composition.ApplyAssignment("explorer=fable")
	require.NoError(t, err)
	assert.Equal(t, "혼합 (fable 1 · haiku 1)", composition.GroupDisplay(composeRoleGroups[3], "ko"))
	assert.Equal(t, "mixed (fable 1 · haiku 1)", composition.GroupDisplay(composeRoleGroups[3], "en"))
}

func TestQualityComposition_RejectedAssignment_LeavesStateUnchanged(t *testing.T) {
	t.Parallel()
	composition := newQualityComposition(config.DefaultFullConfig("p").Quality, composeGroupsSources(t))
	before := composition.Clone()

	for _, line := range []string{"bogus=fable", "planner", "planner=", "=opus"} {
		_, _, err := composition.ApplyAssignment(line)
		assert.Error(t, err, line)
	}

	assert.True(t, composition.Equal(before))
}

func TestQualityComposition_SetGroupTier_RejectsUnknownTier(t *testing.T) {
	t.Parallel()
	composition := newQualityComposition(config.DefaultFullConfig("p").Quality, composeGroupsSources(t))
	before := composition.Clone()

	err := composition.SetGroupTier(composeRoleGroups[0], "gpt")

	require.Error(t, err)
	assert.True(t, composition.Equal(before))
}

func TestQualityComposition_CloneAndAgents_AreDetached(t *testing.T) {
	t.Parallel()
	composition := newQualityComposition(config.DefaultFullConfig("p").Quality, composeGroupsSources(t))
	clone := composition.Clone()
	require.True(t, composition.Equal(clone))

	require.NoError(t, clone.SetGroupTier(composeRoleGroups[3], "haiku"))
	agents := composition.Agents()
	agents["planner"] = "haiku"

	assert.False(t, composition.Equal(clone))
	assert.Equal(t, "sonnet", composition.Tier("explorer"))
	assert.Equal(t, "fable", composition.Tier("planner"))
	assert.Len(t, agents, len(config.CanonicalAgentNames()))
}

func TestParseComposeGroupAnswer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		answer   string
		wantTier string
		wantKeep bool
		wantErr  bool
	}{
		{answer: "1", wantTier: "fable"},
		{answer: "2", wantTier: "opus"},
		{answer: " 3 \n", wantTier: "sonnet"},
		{answer: "4", wantTier: "haiku"},
		{answer: "Opus", wantTier: "opus"},
		{answer: "haiku\r\n", wantTier: "haiku"},
		{answer: "", wantKeep: true},
		{answer: "  \n", wantKeep: true},
		{answer: "0", wantErr: true},
		{answer: "5", wantErr: true},
		{answer: "gpt", wantErr: true},
		{answer: "1)", wantErr: true},
	}
	for _, tt := range tests {
		tier, keep, err := parseComposeGroupAnswer(tt.answer)
		if tt.wantErr {
			assert.Error(t, err, "answer %q", tt.answer)
			assert.False(t, keep, "answer %q", tt.answer)
			continue
		}
		require.NoError(t, err, "answer %q", tt.answer)
		assert.Equal(t, tt.wantTier, tier, "answer %q", tt.answer)
		assert.Equal(t, tt.wantKeep, keep, "answer %q", tt.answer)
	}
}
