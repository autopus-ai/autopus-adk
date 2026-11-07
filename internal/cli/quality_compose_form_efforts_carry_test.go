package cli

// SPEC-MODELUX-003 T10 (REQ-MUX3-20; acceptance S20) through the real
// `auto quality --apply`: an edit or refresh that does not touch an agent's
// effort leaves its stored effort, its preview cell, and its generated value.

import (
	"maps"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// writeComposeEffortsFixture saves D with mutate applied to its quality.
func writeComposeEffortsFixture(t *testing.T, mutate func(*config.QualityConf)) string {
	t.Helper()
	cfg := composeFixtureConfig("claude-code", "codex")
	mutate(&cfg.Quality)
	dir := t.TempDir()
	require.NoError(t, config.Save(dir, cfg))
	return dir
}

func withComposeEfforts(q *config.QualityConf, name string, efforts map[string]string) {
	preset := q.Presets[name]
	preset.Efforts = efforts
	q.Presets[name] = preset
}

// runComposeCarryForms runs `auto quality --apply` with the answers.
func runComposeCarryForms(t *testing.T, dir string, lines ...string) (string, map[string]map[string]string) {
	t.Helper()
	newComposeUpdateRun(t, true, "")
	useAccessibleComposeForms(t)
	out, err := runQualityForms(t, dir, nil, formInput(lines...), "--apply")
	require.NoError(t, err, out)
	return out, plainComposeHeaders(composeRenderedTable(t, out, composeFormPreviewMarker))
}

// S20, first file: a Claude Code tab edit over a Codex binding with a
// reviewer effort keeps that Codex effort and its generated value.
func TestComposeEffortsCarry_ClaudeEditKeepsTheCodexEfforts(t *testing.T) {
	dir := writeComposeEffortsFixture(t, func(q *config.QualityConf) {
		agents := maps.Clone(q.Presets["balanced"].Agents)
		q.Presets["codex-custom"] = config.QualityPreset{Description: "Codex custom agent tiers", Agents: agents,
			Efforts: composeEfforts("high", "reviewer")}
		q.Providers = map[string]string{config.QualityProviderCodex: "codex-custom"}
	})

	// 구성 편집, Claude Code, 역할 그룹, 구현 on fable, every later answer kept.
	runComposeCarryForms(t, dir, "2\n", "2\n", "\n", "2\n", "\n", "\n", "\n", "2\n", "\n")

	assert.Equal(t, composeEfforts("high", "reviewer"), composeBindLoad(t, dir).Quality.Presets["codex-custom"].Efforts)
	assert.Equal(t, config.CodexProfile{Model: "gpt-6-astra", Effort: "high"}, readComposeCodexProfile(t, dir, "reviewer"))
	assert.Equal(t, "fable", readComposeTableClaudeProfile(t, dir, "frontend-specialist").model)
}

// S20, second file: 모든 도구 같게 over a custom binding with
// frontend-specialist at xhigh rewrites custom with the efforts it had;
// frontend-specialist's cells stay.
func TestComposeEffortsCarry_AllToolsEditKeepsTheBindingsEfforts(t *testing.T) {
	dir := writeComposeEffortsFixture(t, composeEffortsCustomP)

	// 구성 편집, 모든 도구 같게, 역할 그룹, 구현 kept, 검증·리뷰 on fable.
	out, preview := runComposeCarryForms(t, dir, "2\n", "1\n", "\n", "\n", "2\n", "2\n", "\n")

	assert.Equal(t, "sonnet · xhigh", preview["frontend-specialist"]["Claude Code"], "no change mark")
	assert.NotContains(t, out, ".efforts 옮겨 씀")
	saved := composeBindLoad(t, dir).Quality
	assert.Equal(t, map[string]string{"frontend-specialist": "xhigh"}, saved.Presets["custom"].Efforts)
	assert.Equal(t, "fable", saved.Presets["custom"].Agents["reviewer"])
	assert.Equal(t, composeTableClaudeCell{model: "sonnet", effort: "xhigh"}, readComposeTableClaudeProfile(t, dir, "frontend-specialist"))
	assertGeneratedEqualsTable(t, dir, preview)
}

// S20, third file: the latest-defaults refresh of a historical balanced keeps
// its efforts, and reviewer keeps its effort.
func TestComposeEffortsCarry_RefreshKeepsTheEfforts(t *testing.T) {
	dir := writeComposeEffortsFixture(t, func(q *config.QualityConf) {
		q.Presets["balanced"] = config.QualityPreset{Agents: maps.Clone(composeTableHB1), Efforts: map[string]string{"reviewer": "high"}}
	})

	runComposeCarryForms(t, dir, "2\n", "2\n", "\n")

	balanced := composeBindLoad(t, dir).Quality.Presets["balanced"]
	assert.Equal(t, composeBindCurrent("balanced").Agents, balanced.Agents)
	assert.Equal(t, map[string]string{"reviewer": "high"}, balanced.Efforts)
	assert.Equal(t, composeTableClaudeCell{model: "claude-opus-5-5", effort: "high"}, readComposeTableClaudeProfile(t, dir, "reviewer"))
}

// S20, fourth file: a new custom made from balanced with reviewer at high
// carries the effort, says so, and leaves reviewer's cells and files.
func TestComposeEffortsCarry_NewCustomCarriesTheBindingsEfforts(t *testing.T) {
	dir := writeComposeEffortsFixture(t, func(q *config.QualityConf) {
		withComposeEfforts(q, "balanced", map[string]string{"reviewer": "high"})
	})

	// 구성 편집, 모든 도구 같게, 역할 그룹, 구현 on haiku, 검증·리뷰 kept.
	out, preview := runComposeCarryForms(t, dir, "2\n", "1\n", "\n", "5\n", "\n", "2\n", "\n")

	assert.Contains(t, out, "  quality.presets.custom 새로 씀 (4개 에이전트)\n  quality.presets.custom.efforts 옮겨 씀 (1개 에이전트)\n")
	// reviewer's explicit effort leaves the standard placement, yet it keeps
	// the top-rung model before and after the edit.
	assert.Equal(t, "opus-5-5 · high", preview["reviewer"]["Claude Code"])
	assert.Equal(t, "6-astra · high", preview["reviewer"]["Codex"])
	assert.Equal(t, map[string]string{"reviewer": "high"}, composeBindLoad(t, dir).Quality.Presets["custom"].Efforts)
	assert.Equal(t, composeTableClaudeCell{model: "claude-opus-5-5", effort: "high"}, readComposeTableClaudeProfile(t, dir, "reviewer"))
	assert.Equal(t, config.CodexProfile{Model: "gpt-6-astra", Effort: "high"}, readComposeCodexProfile(t, dir, "reviewer"))
}

// S20, fifth file: an edit back to the balanced default tiers over a custom
// binding with efforts stays on custom; balanced's bytes stay. The group
// select cannot spell balanced's mixed 검증·리뷰 tiers, so the agents are set
// one by one.
func TestComposeEffortsCarry_BuiltinResultStaysOnCustom(t *testing.T) {
	dir := writeComposeEffortsFixture(t, func(q *config.QualityConf) {
		composeEffortsCustomP(q)
		for _, agent := range composeRoleGroups[1].Agents {
			q.Presets["custom"].Agents[agent] = "haiku"
		}
	})
	before := rawQualityOf(t, readBlockFixture(t, dir)).Presets["balanced"]

	// 에이전트별: reviewer and qa-explorer on opus, ux-validator on sonnet, 완료.
	runComposeCarryForms(t, dir, "2\n", "1\n", "2\n", "3\n", "3\n", "4\n", "3\n", "5\n", "4\n", "1\n", "2\n", "\n")

	saved := composeBindLoad(t, dir).Quality
	assert.Equal(t, "custom", saved.Default)
	assert.Equal(t, composeBindCurrent("balanced").Agents, saved.Presets["custom"].Agents)
	assert.Equal(t, map[string]string{"frontend-specialist": "xhigh"}, saved.Presets["custom"].Efforts)
	assert.Equal(t, before, rawQualityOf(t, readBlockFixture(t, dir)).Presets["balanced"])
	assert.Equal(t, "xhigh", readComposeTableClaudeProfile(t, dir, "frontend-specialist").effort)
}

// S20, third file's copy: per-agent edits that reach the latest balanced
// tiers refresh the historical balanced in place and keep its efforts.
func TestComposeEffortsCarry_EditToTheLatestTiersRefreshesInPlace(t *testing.T) {
	dir := writeComposeEffortsFixture(t, func(q *config.QualityConf) {
		q.Presets["balanced"] = config.QualityPreset{Agents: maps.Clone(composeTableHB1), Efforts: map[string]string{"reviewer": "high"}}
	})

	// Every generated agent is set to its latest balanced tier: frontend-
	// specialist and ux-validator on sonnet, reviewer and qa-explorer on opus.
	runComposeCarryForms(t, dir, "3\n", "1\n", "2\n",
		"2\n", "4\n", "3\n", "3\n", "4\n", "3\n", "5\n", "4\n",
		"1\n", "2\n", "\n")

	saved := composeBindLoad(t, dir).Quality
	assert.NotContains(t, saved.Presets, "custom")
	assert.Equal(t, "balanced", saved.Default)
	assert.Equal(t, composeBindCurrent("balanced").Agents, saved.Presets["balanced"].Agents)
	assert.Equal(t, map[string]string{"reviewer": "high"}, saved.Presets["balanced"].Efforts)
}

// The per-tool line editor edits tiers only: its preview and its write keep
// the efforts the tool's preset stores, a bound preset's or the global one's.
func TestQualityToolWizard_KeepsTheToolsStoredEfforts(t *testing.T) {
	for name, mutate := range map[string]func(*config.QualityConf){
		"bound": func(q *config.QualityConf) {
			q.Presets["claude-custom"] = config.QualityPreset{Agents: maps.Clone(q.Presets["balanced"].Agents),
				Efforts: map[string]string{"reviewer": "low"}}
			q.Providers = map[string]string{config.QualityProviderClaude: "claude-custom"}
		},
		"global": func(q *config.QualityConf) { withComposeEfforts(q, "balanced", map[string]string{"reviewer": "low"}) },
	} {
		t.Run(name, func(t *testing.T) {
			dir := writeComposeEffortsFixture(t, mutate)

			out, err := runQualityToolWizard(t, dir, "claude-code\nfrontend-specialist=fable\n\n\ny\n")

			require.NoError(t, err, out)
			preset := composeBindLoad(t, dir).Quality.Presets["claude-custom"]
			assert.Equal(t, "fable", preset.Agents["frontend-specialist"])
			assert.Equal(t, map[string]string{"reviewer": "low"}, preset.Efforts)
		})
	}
}
